package mining

import (
	"container/heap"
	"fmt"
	blockchain "github.com/p9c/pod/pkg/chain"
	"github.com/p9c/pod/pkg/chain/fork"
	chainhash "github.com/p9c/pod/pkg/chain/hash"
	txscript "github.com/p9c/pod/pkg/chain/tx/script"
	"github.com/p9c/pod/pkg/chain/wire"
	"github.com/p9c/pod/pkg/util"
	"math/rand"
	"time"
)

// BlockTemplates is a collection of block templates indexed by their version number
type BlockTemplates map[int32]*BlockTemplate

// NewBlockTemplates returns a data structure which has methods to construct
// block version specific block headers and reconstruct their transactions
func (g *BlkTmplGenerator) NewBlockTemplates(
	workerNumber uint32,
	payToAddress util.Address,
) (BlockTemplates, error) {
	// Extend the most recently known best block.
	best := g.Chain.BestSnapshot()
	height := best.Height + 1
	// Create a standard coinbase transaction paying to the provided address.
	//
	// NOTE: The coinbase value will be updated to include the fees from the
	// selected transactions later after they have actually been selected. It is
	// created here to detect any errors early before potentially doing a lot of
	// work below. The extra nonce helps ensure the transaction is not a duplicate
	// transaction (paying the same value to the same public key address would
	// otherwise be an identical transaction for block version 1).
	rand.Seed(time.Now().UnixNano())
	extraNonce := rand.Uint64()
	var err error
	numAlgos := fork.GetNumAlgos(height)
	coinbaseScripts := make(map[int32][]byte, numAlgos)
	coinbaseTxs := make(map[int32]*util.Tx, numAlgos)
	coinbaseSigOpCosts := make(map[int32]int64, numAlgos)
	blockTemplates := make(BlockTemplates, numAlgos)
	var priorityQueues *txPriorityQueue
	// Get the current source transactions and create a priority queue to hold the
	// transactions which are ready for inclusion into a block along with some
	// priority related and fee metadata. Reserve the same number of items that are
	// available for the priority queue. Also, choose the initial sort order for the
	// priority queue based on whether or not there is an area allocated for
	// high-priority transactions.
	sourceTxns := g.TxSource.MiningDescs()
	sortedByFee := g.Policy.BlockPrioritySize == 0
	for next, curr, more := fork.AlgoVerIterator(height); more(); next() {
		priorityQueues = newTxPriorityQueue(len(sourceTxns), sortedByFee)
		var coinbaseScript []byte
		var coinbaseTx *util.Tx
		if coinbaseScript, err = standardCoinbaseScript(height, extraNonce); Check(err) {
			return nil, err
		}
		coinbaseScripts[curr()] = coinbaseScript
		if coinbaseTx, err = createCoinbaseTx(
			g.ChainParams, coinbaseScript, height, payToAddress, curr(),
		); Check(err) {
			return nil, err
		}
		coinbaseTxs[curr()] = coinbaseTx
		coinbaseSigOpCosts[curr()] = int64(blockchain.CountSigOps(coinbaseTx))
		// Create a slice to hold the transactions to be included in the generated block
		// with reserved space. Also create a utxo view to house all of the input
		// transactions so multiple lookups can be avoided.
		blockTxns := make([]*util.Tx, 0, len(sourceTxns))
		blockTxns = append(blockTxns, coinbaseTx)
		blockUtxos := blockchain.NewUtxoViewpoint()
		// dependers is used to track transactions which depend on another transaction
		// in the source pool. This, in conjunction with the dependsOn map kept with
		// each dependent transaction helps quickly determine which dependent
		// transactions are now eligible for inclusion in the block once each
		// transaction has been included.
		dependers := make(map[chainhash.Hash]map[chainhash.Hash]*txPrioItem)
		// Create slices to hold the fees and number of signature operations for each of
		// the selected transactions and add an entry for the coinbase. This allows the
		// code below to simply append details about a transaction as it is selected for
		// inclusion in the final block. However, since the total fees aren't known yet,
		// use a dummy value for the coinbase fee which will be updated later.
		txFees := make([]int64, 0, len(sourceTxns))
		txSigOpCosts := make([]int64, 0, len(sourceTxns))
		txFees = append(txFees, -1) // Updated once known
		txSigOpCosts = append(txSigOpCosts, coinbaseSigOpCosts[curr()])
		Tracef("considering %d transactions for inclusion to new block", len(sourceTxns))
	mempoolLoop:
		for _, txDesc := range sourceTxns {
			// A block can't have more than one coinbase or contain non-finalized
			// transactions.
			tx := txDesc.Tx
			if blockchain.IsCoinBase(tx) {
				Tracec(
					func() string {
						return fmt.Sprintf("skipping coinbase tx %s", tx.Hash())
					},
				)
				continue
			}
			if !blockchain.IsFinalizedTransaction(
				tx, height,
				g.TimeSource.AdjustedTime(),
			) {
				Tracec(
					func() string {
						return "skipping non-finalized tx " + tx.Hash().String()
					},
				)
				continue
			}
			// Fetch all of the utxos referenced by the this transaction.
			//
			// NOTE: This intentionally does not fetch inputs from the mempool since a
			// transaction which depends on other transactions in the mempool must come
			// after those dependencies in the final generated block.
			utxos, err := g.Chain.FetchUtxoView(tx)
			if err != nil {
				Warnc(
					func() string {
						return "unable to fetch utxo view for tx " + tx.Hash().String() + ": " + err.Error()
					},
				)
				continue
			}
			// Setup dependencies for any transactions which reference other transactions in
			// the mempool so they can be properly ordered below.
			prioItem := &txPrioItem{tx: tx}
			for _, txIn := range tx.MsgTx().TxIn {
				originHash := &txIn.PreviousOutPoint.Hash
				entry := utxos.LookupEntry(txIn.PreviousOutPoint)
				if entry == nil || entry.IsSpent() {
					if !g.TxSource.HaveTransaction(originHash) {
						Tracec(
							func() string {
								return "skipping tx %s because it references unspent output %s which is not available" +
									tx.Hash().String() +
									txIn.PreviousOutPoint.String()
							},
						)
						continue mempoolLoop
					}
					// The transaction is referencing another transaction in the source pool, so
					// setup an ordering dependency.
					deps, exists := dependers[*originHash]
					if !exists {
						deps = make(map[chainhash.Hash]*txPrioItem)
						dependers[*originHash] = deps
					}
					deps[*prioItem.tx.Hash()] = prioItem
					if prioItem.dependsOn == nil {
						prioItem.dependsOn = make(
							map[chainhash.Hash]struct{},
						)
					}
					prioItem.dependsOn[*originHash] = struct{}{}
					// Skip the check below. We already know the referenced transaction is
					// available.
					continue
				}
			}
			// Calculate the final transaction priority using the input value age sum as
			// well as the adjusted transaction size. The formula is: sum (inputValue *
			// inputAge) / adjustedTxSize
			prioItem.priority = CalcPriority(
				tx.MsgTx(), utxos,
				height,
			)
			// Calculate the fee in Satoshi/kB.
			prioItem.feePerKB = txDesc.FeePerKB
			prioItem.fee = txDesc.Fee
			// Add the transaction to the priority queue to mark it ready for inclusion in
			// the block unless it has dependencies.
			if prioItem.dependsOn == nil {
				heap.Push(priorityQueues, prioItem)
			}
			// Merge the referenced outputs from the input transactions to this transaction
			// into the block utxo view. This allows the code below to avoid a second
			// lookup.
			mergeUtxoView(blockUtxos, utxos)
		}
		// The starting block size is the size of the block header plus the max possible
		// transaction count size, plus the size of the coinbase transaction.
		blockWeight := uint32((blockHeaderOverhead) + blockchain.GetTransactionWeight(coinbaseTx))
		blockSigOpCost := coinbaseSigOpCosts[curr()]
		totalFees := int64(0)
		// Choose which transactions make it into the block.
		for priorityQueues.Len() > 0 {
			// Grab the highest priority (or highest fee per kilobyte depending on the sort
			// order) transaction.
			prioItem := heap.Pop(priorityQueues).(*txPrioItem)
			tx := prioItem.tx
			// Grab any transactions which depend on this one.
			deps := dependers[*tx.Hash()]
			// Enforce maximum block size.  Also check for overflow.
			txWeight := uint32(blockchain.GetTransactionWeight(tx))
			blockPlusTxWeight := blockWeight + txWeight
			if blockPlusTxWeight < blockWeight ||
				blockPlusTxWeight >= g.Policy.BlockMaxWeight {
				Tracef("skipping tx %s because it would exceed the max block weight", tx.Hash())
				logSkippedDeps(tx, deps)
				continue
			}
			// Enforce maximum signature operation cost per block. Also check for overflow.
			sigOpCost, err := blockchain.GetSigOpCost(tx, false, blockUtxos, true)
			if err != nil {
				Tracec(
					func() string {
						return "skipping tx " + tx.Hash().String() +
							"due to error in GetSigOpCost: " + err.Error()
					},
				)
				logSkippedDeps(tx, deps)
				continue
			}
			if blockSigOpCost+int64(sigOpCost) < blockSigOpCost ||
				blockSigOpCost+int64(sigOpCost) > blockchain.MaxBlockSigOpsCost {
				Tracec(
					func() string {
						return "skipping tx " + tx.Hash().String() +
							" because it would exceed the maximum sigops per block"
					},
				)
				logSkippedDeps(tx, deps)
				continue
			}
			// Skip free transactions once the block is larger than the minimum block size.
			if sortedByFee &&
				prioItem.feePerKB < int64(g.Policy.TxMinFreeFee) &&
				blockPlusTxWeight >= g.Policy.BlockMinWeight {
				Tracec(
					func() string {
						return fmt.Sprintf(
							"skipping tx %v with feePerKB %v < TxMinFreeFee %v and block weight %v >= minBlockWeight %v",
							tx.Hash(),
							prioItem.feePerKB,
							g.Policy.TxMinFreeFee,
							blockPlusTxWeight,
							g.Policy.BlockMinWeight,
						)
					},
				)
				logSkippedDeps(tx, deps)
				continue
			}
			// Prioritize by fee per kilobyte once the block is larger than the priority
			// size or there are no more high-priority transactions.
			if !sortedByFee && (blockPlusTxWeight >= g.Policy.BlockPrioritySize ||
				prioItem.priority <= MinHighPriority.ToDUO()) {
				Tracef(
					"switching to sort by fees per kilobyte blockSize %d"+
						" >= BlockPrioritySize %d || priority %.2f <= minHighPriority %.2f",
					blockPlusTxWeight,
					g.Policy.BlockPrioritySize,
					prioItem.priority,
					MinHighPriority,
				)
				sortedByFee = true
				priorityQueues.SetLessFunc(txPQByFee)
			}
			// Put the transaction back into the priority queue and skip it so it is
			// re-prioritized by fees if it won't fit into the high-priority section or the
			// priority is too low. Otherwise this transaction will be the final one in the
			// high-priority section, so just fall though to the code below so it is added
			// now.
			if blockPlusTxWeight > g.Policy.BlockPrioritySize ||
				prioItem.priority < MinHighPriority.ToDUO() {
				heap.Push(priorityQueues, prioItem)
				continue
			}
			
			// Ensure the transaction inputs pass all of the necessary preconditions before
			// allowing it to be added to the block.
			_, err = blockchain.CheckTransactionInputs(
				tx, height,
				blockUtxos, g.ChainParams,
			)
			if err != nil {
				Tracef(
					"skipping tx %s due to error in CheckTransactionInputs: %v",
					tx.Hash(), err,
				)
				logSkippedDeps(tx, deps)
				continue
			}
			if err = blockchain.ValidateTransactionScripts(
				g.Chain, tx, blockUtxos,
				txscript.StandardVerifyFlags, g.SigCache,
				g.HashCache,
			); Check(err) {
				Tracef(
					"skipping tx %s due to error in ValidateTransactionScripts: %v",
					tx.Hash(), err,
				)
				logSkippedDeps(tx, deps)
				continue
			}
			
			// Spend the transaction inputs in the block utxo view and add an entry for it
			// to ensure any transactions which reference this one have it available as an
			// input and can ensure they aren't double spending.
			if err = spendTransaction(blockUtxos, tx, height); Check(err) {
			}
			// Add the transaction to the block, increment counters, and save the fees and
			// signature operation counts to the block template.
			blockTxns = append(blockTxns, tx)
			blockWeight += txWeight
			blockSigOpCost += int64(sigOpCost)
			totalFees += prioItem.fee
			txFees = append(txFees, prioItem.fee)
			txSigOpCosts = append(txSigOpCosts, int64(sigOpCost))
			Tracef(
				"adding tx %s (priority %.2f, feePerKB %.2f)",
				prioItem.tx.Hash(),
				prioItem.priority,
				prioItem.feePerKB,
			)
			// Add transactions which depend on this one (and also do not have any other
			// unsatisfied dependencies) to the priority queue.
			for _, item := range deps {
				// Add the transaction to the priority queue if there are no more dependencies
				// after this one.
				delete(item.dependsOn, *tx.Hash())
				if len(item.dependsOn) == 0 {
					heap.Push(priorityQueues, item)
				}
			}
		}
		// Now that the actual transactions have been selected, update the block weight
		// for the real transaction count and coinbase value with the total fees
		// accordingly.
		blockWeight -= wire.MaxVarIntPayload -
			(uint32(wire.VarIntSerializeSize(uint64(len(blockTxns)))))
		coinbaseTx.MsgTx().TxOut[0].Value += totalFees
		txFees[0] = -totalFees
		// Calculate the required difficulty for the block. The timestamp is potentially
		// adjusted to ensure it comes after the median time of the last several blocks
		// per the chain consensus rules.
		ts := medianAdjustedTime(best, g.TimeSource)
		// Trace("algo ", ts, " ", algo)
		var reqDifficulty uint32
		algo := fork.GetAlgoName(height, curr())
		if reqDifficulty, err = g.Chain.CalcNextRequiredDifficulty(
			workerNumber,
			ts,
			algo,
		); Check(err) {
			return nil, err
		}
		Tracef("reqDifficulty %d %08x %064x", curr(), reqDifficulty, fork.CompactToBig(reqDifficulty))
		// Create a new block ready to be solved.
		merkles := blockchain.BuildMerkleTreeStore(blockTxns, false)
		var msgBlock wire.MsgBlock
		msgBlock.Header = wire.BlockHeader{
			Version:    curr(),
			PrevBlock:  best.Hash,
			MerkleRoot: *merkles[len(merkles)-1],
			Timestamp:  ts,
			Bits:       reqDifficulty,
		}
		for _, tx := range blockTxns {
			if err := msgBlock.AddTransaction(tx.MsgTx()); err != nil {
				return nil, err
			}
		}
		// Finally, perform a full check on the created block against the chain
		// consensus rules to ensure it properly connects to the current best chain with
		// no issues.
		block := util.NewBlock(&msgBlock)
		block.SetHeight(height)
		err = g.Chain.CheckConnectBlockTemplate(workerNumber, block)
		if err != nil {
			Debug("checkconnectblocktemplate err:", err)
			return nil, err
		}
		bh := msgBlock.Header.BlockHash()
		Tracec(
			func() string {
				return fmt.Sprintf(
					"created new block template (algo %s, %d transactions, "+
						"%d in fees, %d signature operations cost, %d weight, "+
						"target difficulty %064x prevblockhash %064x %064x subsidy %d)",
					algo,
					len(msgBlock.Transactions),
					totalFees,
					blockSigOpCost,
					blockWeight,
					fork.CompactToBig(msgBlock.Header.Bits),
					msgBlock.Header.PrevBlock.CloneBytes(),
					bh.CloneBytes(),
					msgBlock.Transactions[0].TxOut[0].Value,
				)
			},
		)
		// Tracec(func() string { return spew.Sdump(msgBlock) })
		blockTemplate := &BlockTemplate{
			Block:           &msgBlock,
			Fees:            txFees,
			SigOpCosts:      txSigOpCosts,
			Height:          height,
			ValidPayAddress: payToAddress != nil,
		}
		blockTemplates[curr()] = blockTemplate
	}
	return blockTemplates, nil
}
