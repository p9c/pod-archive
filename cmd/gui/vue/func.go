package vue

import (
	"fmt"
	"github.com/parallelcointeam/parallelcoin/cmd/gui/vue/mod"
	"github.com/parallelcointeam/parallelcoin/cmd/node/rpc"
	"github.com/parallelcointeam/parallelcoin/pkg/chain/fork"
	chainhash "github.com/parallelcointeam/parallelcoin/pkg/chain/hash"
	database "github.com/parallelcointeam/parallelcoin/pkg/db"
	"github.com/parallelcointeam/parallelcoin/pkg/rpc/json"
	"github.com/parallelcointeam/parallelcoin/pkg/util"
)

func (d *DuoVUE) GetNetworkLastBlock() int32 {
	for _, g := range d.cx.RPCServer.Cfg.ConnMgr.ConnectedPeers() {
		l := g.ToPeer().StatsSnapshot().LastBlock
		if l > d.Data.Status.NetworkLastBlock {
			d.Data.Status.NetworkLastBlock = l
		}
	}
	return d.Data.Status.NetworkLastBlock
}

//func (n *DuoVUEnode) GetBlocks() {
//	blks := []mod.Block{}
//	getBlockChain, err := rpc.HandleGetBlockChainInfo(n.rpc, nil, nil)
//	if err != nil {
//		alert.Alert.Time = time.Now()
//		alert.Alert.Alert = err.Error()
//		alert.Alert.AlertType = "error"
//	}
//	blockChain := getBlockChain.(*json.GetBlockChainInfoResult)
//	blockCount := int(blockChain.Blocks)
//	for ibh := blockCount; ibh >= 0; {
//		block := json.GetBlockVerboseResult{}
//		hcmd := json.GetBlockHashCmd{
//			Index: int64(ibh),
//		}
//		hash, err := rpc.HandleGetBlockHash(n.rpc, &hcmd, nil)
//		if err != nil {
//			alert.Alert.Time = time.Now()
//			alert.Alert.Alert = err.Error()
//			alert.Alert.AlertType = "error"
//		}
//		if hash != nil {
//			verbose, verbosetx := true, true
//			bcmd := json.GetBlockCmd{
//				Hash:      hash.(string),
//				Verbose:   &verbose,
//				VerboseTx: &verbosetx,
//			}
//			bl, err := rpc.HandleGetBlock(n.rpc, &bcmd, nil)
//			if err != nil {
//				alert.Alert.Time = time.Now()
//				alert.Alert.Alert = err.Error()
//				alert.Alert.AlertType = "error"
//			}
//			block = bl.(json.GetBlockVerboseResult)
//			blks = append(blks, mod.Block{
//				Hash:          block.Hash,
//				Confirmations: block.Confirmations,
//				Height:        block.Height,
//				TxNum:         block.TxNum,
//				Time:          block.Time,
//			})
//			ibh--
//		}
//	}
//
//	n.Blocks = blks
//}

//func (n *DuoVUEnode) GetBlocks(per, page int) {
//	blks := []json.GetBlockVerboseResult{}
//	getBlockChain, err := rpc.HandleGetBlockChainInfo(n.rpc, nil, nil)
//	if err != nil {
//		alert.Alert.Time = time.Now()
//		alert.Alert.Alert = err.Error()
//		alert.Alert.AlertType = "error"
//	}
//	blockChain := getBlockChain.(*json.GetBlockChainInfoResult)
//	blockCount := int(blockChain.Blocks)
//	startBlock := blockCount - per*page
//	minusBlockStart := int(startBlock + per)
//	for ibh := minusBlockStart; ibh >= startBlock; {
//		block := json.GetBlockVerboseResult{}
//		hcmd := json.GetBlockHashCmd{
//			Index: int64(ibh),
//		}
//		hash, err := rpc.HandleGetBlockHash(n.rpc, &hcmd, nil)
//		if err != nil {
//			alert.Alert.Time = time.Now()
//			alert.Alert.Alert = err.Error()
//			alert.Alert.AlertType = "error"
//		}
//		if hash != nil {
//			verbose, verbosetx := true, true
//			bcmd := json.GetBlockCmd{
//				Hash:      hash.(string),
//				Verbose:   &verbose,
//				VerboseTx: &verbosetx,
//			}
//			bl, err := rpc.HandleGetBlock(n.rpc, &bcmd, nil)
//			if err != nil {
//				alert.Alert.Time = time.Now()
//				alert.Alert.Alert = err.Error()
//				alert.Alert.AlertType = "error"
//			}
//			block = bl.(json.GetBlockVerboseResult)
//			blks = append(blks, block)
//			ibh--
//		}
//	}
//	n.Blocks.Blocks = blks
//	n.Blocks.CurrentPage = page
//	n.Blocks.PageCount = blockCount / per
//
//}

func (dv DuoVUE) GetBlockExcerpt(height int) (b mod.DuoVUEblock) {
	b = *new(mod.DuoVUEblock)
	hashHeight, err := dv.cx.RPCServer.Cfg.Chain.BlockHashByHeight(int32(height))
	if err != nil {
	}
	// Load the raw block bytes from the database.
	hash, err := chainhash.NewHashFromStr(hashHeight.String())
	if err != nil {
	}
	var blkBytes []byte
	err = dv.cx.RPCServer.Cfg.DB.View(func(dbTx database.Tx) error {
		var err error
		blkBytes, err = dbTx.FetchBlock(hash)
		return err
	})
	if err != nil {
	}
	// The verbose flag is set, so generate the JSON object and return it.
	// Deserialize the block.
	blk, err := util.NewBlockFromBytes(blkBytes)
	if err != nil {
	}
	// Get the block height from chain.
	blockHeight, err := dv.cx.RPCServer.Cfg.Chain.BlockHeightByHash(hash)
	if err != nil {
	}
	blk.SetHeight(blockHeight)
	params := dv.cx.RPCServer.Cfg.ChainParams
	blockHeader := &blk.MsgBlock().Header
	algoname := fork.GetAlgoName(blockHeader.Version, blockHeight)
	a := fork.GetAlgoVer(algoname, blockHeight)
	algoid := fork.GetAlgoID(algoname, blockHeight)
	//var value float64
	b.PowAlgoID = algoid
	b.Time = blockHeader.Timestamp.Unix()

	b.Height = int64(blockHeight)
	b.TxNum = len(blk.Transactions())
	b.Difficulty = rpc.GetDifficultyRatio(blockHeader.Bits, params, a)
	//txns := blk.Transactions()
	//
	//for _, tx := range txns {
	//	// Try to fetch the transaction from the memory pool and if that fails, try
	//	// the block database.
	//	var mtx *wire.MsgTx
	//
	//	// Look up the location of the transaction.
	//	blockRegion, err := b.rpc.Cfg.TxIndex.TxBlockRegion(tx.Hash())
	//	if err != nil {
	//	}
	//	if blockRegion == nil {
	//	}
	//	// Load the raw transaction bytes from the database.
	//	var txBytes []byte
	//	err = b.rpc.Cfg.DB.View(func(dbTx database.Tx) error {
	//		var err error
	//		txBytes, err = dbTx.FetchBlockRegion(blockRegion)
	//		return err
	//	})
	//	if err != nil {
	//	}
	//	// Deserialize the transaction
	//	var msgTx wire.MsgTx
	//	err = msgTx.Deserialize(bytes.NewReader(txBytes))
	//	if err != nil {
	//	}
	//	mtx = &msgTx
	//
	//	for _, vout := range rpc.CreateVoutList(mtx, b.rpc.Cfg.ChainParams, nil) {
	//
	//		value = value + vout.Value
	//	}
	//
	fmt.Println("Uzebekistanka malalalallalalaazsa")
	fmt.Println("Uzebekistanka malalalallalalaazsa")
	fmt.Println("Uzebekistanka malalalallalalaazsa")
	//fmt.Println("Uzebekistanka malalalallalalaazsa", b)
	fmt.Println("Uzebekistanka malalalallalalaazsa")
	//b.Amount = value
	//}
	return
}

func (dv *DuoVUE) GetBlocksExcerpts(startBlock, blockHeight int) mod.DuoVUEblocks {
	for i := startBlock; i <= blockHeight; i++ {

		dv.Data.Blocks = append(dv.Data.Blocks, dv.GetBlockExcerpt(i))
	}
	return dv.Data.Blocks
}





// func (v *DuoVUEnode) Addnode(a *json.AddNodeCmd) {
// 	r, err := v.cx.RPCServer.HandleAddNode(v.cx.RPCServer, a, nil)
// 	return
// }
// func (v *DuoVUEnode) Createrawtransaction(a *json.CreateRawTransactionCmd) {
// 	r, err := v.cx.RPCServer.HandleCreateRawTransaction(v.cx.RPCServer, a, nil)
// 	r = ""
// 	return
// }
// func (v *DuoVUEnode) Decoderawtransaction(a *json.DecodeRawTransactionCmd) {
// 	r, err := v.cx.RPCServer.HandleDecodeRawTransaction(v.cx.RPCServer, a, nil)
// 	r = json.TxRawDecodeResult{}
// 	return
// }
// func (v *DuoVUEnode) Decodescript(a *json.DecodeScriptCmd) {
// 	r, err := v.cx.RPCServer.HandleDecodeScript(v.cx.RPCServer, a, nil)
// 	return
// }
// func (v *DuoVUEnode) Estimatefee(a *json.EstimateFeeCmd) {
// 	r, err := v.cx.RPCServer.HandleEstimateFee(v.cx.RPCServer, a, nil)
// 	r = 0.0
// 	return
// }
// func (v *DuoVUEnode) Generate(a *json.GenerateCmd) {
// 	r, err := v.cx.RPCServer.HandleGenerate(v.cx.RPCServer, a, nil)
// 	r = []string{}
// 	return
// }
// func (v *DuoVUEnode) Getaddednodeinfo(a *json.GetAddedNodeInfoCmd) {
// 	r, err := v.cx.RPCServer.HandleGetAddedNodeInfo(v.cx.RPCServer, a, nil)
// 	r = []string{}
// 	return
// }
// func (v *DuoVUEnode) Getbestblock() {
// 	r, err := v.cx.RPCServer.HandleGetBestBlock(v.cx.RPCServer, a, nil)
// 	r = json.GetBestBlockResult{}
// 	return
// }
// func (v *DuoVUEnode) Getbestblockhash() {
// 	r, err := v.cx.RPCServer.HandleGetBestBlockHash(v.cx.RPCServer, a, nil)
// 	r = ""
// 	return
// }
// func (v *DuoVUEnode) Getblock(a *json.GetBlockCmd) {
// 	r, err := v.cx.RPCServer.HandleGetBlock(v.cx.RPCServer, a, nil)
// 	r = json.GetBlockVerboseResult{}
// 	return
// }
//func (dv *DuoVUE) GetBlockChainInfo() {
//	getBlockChainInfo, err := rpc.HandleGetBlockChainInfo(dv.cx.RPCServer, nil, nil)
//	if err != nil {
//		dv.PushDuoVUEalert("Error",err.Error(), "error")
//	}
//	var ok bool
//	dv.Core.Node.BlockChainInfo, ok = getBlockChainInfo.(*json.GetBlockChainInfoResult)
//	if !ok {
//		dv.Core.Node.BlockChainInfo = &json.GetBlockChainInfoResult{}
//	}
//
//}

func (dv *DuoVUE) GetBlockCount() int64 {
	getBlockCount, err := rpc.HandleGetBlockCount(dv.cx.RPCServer, nil, nil)
	if err != nil {
		dv.PushDuoVUEalert("Error", err.Error(), "error")
	}
	dv.Data.Status.BlockCount = getBlockCount.(int64)
	return dv.Data.Status.BlockCount
}
func (dv *DuoVUE) GetBlockHash(blockHeight int) string{
	hcmd := json.GetBlockHashCmd{
		Index: int64(blockHeight),
	}
	hash, err := rpc.HandleGetBlockHash(dv.cx.RPCServer, &hcmd, nil)
	if err != nil {
		dv.PushDuoVUEalert("Error",err.Error(), "error")
	}
	return hash.(string)
}
func (dv *DuoVUE) GetBlock(hash string) (json.GetBlockVerboseResult){
	verbose, verbosetx := true, true
	bcmd := json.GetBlockCmd{
		Hash:      hash,
		Verbose:   &verbose,
		VerboseTx: &verbosetx,
	}
	bl, err := rpc.HandleGetBlock(dv.cx.RPCServer, &bcmd, nil)
	if err != nil {
		dv.PushDuoVUEalert("Error",err.Error(), "error")
	}
	return bl.(json.GetBlockVerboseResult)
}

// func (v *DuoVUEnode) Getblockheader(a *json.GetBlockHeaderCmd) {
// 	r, err := v.cx.RPCServer.HandleGetBlockHeader(v.cx.RPCServer, a, nil)
// 	r = json.GetBlockHeaderVerboseResult{}
// 	return
// }

func (dv *DuoVUE) GetConnectionCount() int32 {
	dv.Data.Status.ConnectionCount = dv.cx.RPCServer.Cfg.ConnMgr.ConnectedCount()
	return dv.Data.Status.ConnectionCount
}

func (dv *DuoVUE) GetDifficulty() float64 {
	c := json.GetDifficultyCmd{}
	r, err := rpc.HandleGetDifficulty(dv.cx.RPCServer, c, nil)
	if err != nil {
		dv.PushDuoVUEalert("Error",err.Error(), "error")
	}
	dv.Data.Status.Difficulty = r.(float64)
	return dv.Data.Status.Difficulty
}

// func (v *DuoVUEnode) Gethashespersec() {
// 	r, err := v.cx.RPCServer.HandleGetHashesPerSec(v.cx.RPCServer, a, nil)
// 	r = int64(0)
// 	return
// }
// func (v *DuoVUEnode) Getheaders(a *json.GetHeadersCmd) {
// 	r, err := v.cx.RPCServer.HandleGetHeaders(v.cx.RPCServer, a, nil)
// 	r = []string{}
// 	return
// }
// func (v *DuoVUEnode) Getinfo() {
// 	r, err := v.cx.RPCServer.HandleGetInfo(v.cx.RPCServer, a, nil)
// 	r = json.InfoChainResult{}
// 	return
// }
// func (v *DuoVUEnode) Getmempoolinfo() {
// 	r, err := v.cx.RPCServer.HandleGetMempoolInfo(v.cx.RPCServer, a, nil)
// 	r = json.GetMempoolInfoResult{}
// 	return
// }
// func (v *DuoVUEnode) Getmininginfo() {
// 	r, err := v.cx.RPCServer.HandleGetMiningInfo(v.cx.RPCServer, a, nil)
// 	r = json.GetMiningInfoResult{}
// 	return
// }
// func (v *DuoVUEnode) Getnettotals() {
// 	r, err := v.cx.RPCServer.HandleGetNetTotals(v.cx.RPCServer, a, nil)
// 	r = json.GetNetTotalsResult{}
// 	return
// }
// func (v *DuoVUEnode) Getnetworkhashps(a *json.GetNetworkHashPSCmd) {
// 	r, err := v.cx.RPCServer.HandleGetNetworkHashPS(v.cx.RPCServer, a, nil)
// 	r = int64(0)
// 	return
// }
func (dV *DuoVUE) GetPeerInfo() []*json.GetPeerInfoResult {
			getPeers, err := rpc.HandleGetPeerInfo(dV.cx.RPCServer, nil, nil)
			if err != nil {
				dV.PushDuoVUEalert("Error",err.Error(), "error")
			}
			dV.Data.Peers = getPeers.([]*json.GetPeerInfoResult)
	return dV.Data.Peers
}

// func (v *DuoVUEnode) Stop() {
// 	r, err := v.cx.RPCServer.HandleStop(v.cx.RPCServer, a, nil)
// 	r = ""
// 	return
// }
func (dv *DuoVUE) Uptime() (r int64) {
	rRaw, err := rpc.HandleUptime(dv.cx.RPCServer, nil, nil)
	if err != nil {
	}
	//rRaw = int64(0)
	dv.Data.Status.UpTime = rRaw.(int64)
	return dv.Data.Status.UpTime
}

// func (v *DuoVUEnode) Validateaddress(a *json.ValidateAddressCmd) {
// 	r, err := v.cx.RPCServer.HandleValidateAddress(v.cx.RPCServer, a, nil)
// 	r = json.ValidateAddressChainResult{}
// 	return
// }
// func (v *DuoVUEnode) Verifychain(a *json.VerifyChainCmd) {
// 	r, err := v.cx.RPCServer.HandleVerifyChain(v.cx.RPCServer, a, nil)
// }
// func (v *DuoVUEnode) Verifymessage(a *json.VerifyMessageCmd) {
// 	r, err := v.cx.RPCServer.HandleVerifyMessage(v.cx.RPCServer, a, nil)
// 	r = ""
// 	return
// }
func (dv *DuoVUE) GetWalletVersion(d DuoVUE) map[string]json.VersionResult {
	v, err := rpc.HandleVersion(dv.cx.RPCServer, nil, nil)
	if err != nil {
	}
	return v.(map[string]json.VersionResult)
}
