package spv

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	
	"github.com/p9c/pod/cmd/spv/cache"
	"github.com/p9c/pod/cmd/spv/cache/lru"
	"github.com/p9c/pod/cmd/spv/filterdb"
	"github.com/p9c/pod/pkg/chainhash"
	"github.com/p9c/pod/pkg/gcs"
	"github.com/p9c/pod/pkg/gcs/builder"
)

var (
	bigOne = big.NewInt(1)
	// blockDataFile is the path to a file containing the first 256 blocks of the block chain.
	//
	// blockDataFile = filepath.Join("tstdata", "blocks1-256.bz2")
	// blockDataNet is the expected network in the test block data.
	// blockDataNet = wire.MainNet
	//
	// maxPowLimit is used as the max block target to ensure all PoWs are valid.
	//
	// maxPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)
)

// TestBigFilterEvictsEverything creates a cache big enough to hold a large filter and inserts many smaller filters
// into. Then it inserts the big filter and verifies that it's the only one remaining.
func TestBigFilterEvictsEverything(t *testing.T) {
	// Create different sized filters.
	b1, f1, _ := genRandFilter(1, t)
	b2, f2, _ := genRandFilter(3, t)
	b3, f3, s3 := genRandFilter(10, t)
	cs := &ChainService{
		FilterCache: lru.NewCache(s3),
	}
	// Insert the smaller filters.
	assertEqual(t, cs.FilterCache.Len(), 0, "")
	e := cs.putFilterToCache(b1, filterdb.RegularFilter, f1)
	if e != nil {
		D.Ln(e)
	}
	assertEqual(t, cs.FilterCache.Len(), 1, "")
	e = cs.putFilterToCache(b2, filterdb.RegularFilter, f2)
	if e != nil {
		D.Ln(e)
	}
	assertEqual(t, cs.FilterCache.Len(), 2, "")
	// Insert the big filter and check all previous filters are evicted.
	e = cs.putFilterToCache(b3, filterdb.RegularFilter, f3)
	if e != nil {
		D.Ln(e)
	}
	assertEqual(t, cs.FilterCache.Len(), 1, "")
	assertEqual(t, getFilter(cs, b3, t), f3, "")
}

// // TestBlockCache checks that blocks are inserted and fetched from the cache
// // before peers are queried.
// func TestBlockCache(// 	t *testing.T) {
// 	t.Parallel()
// 	// Load the first 255 blocks from disk.
// 	blocks, e := loadBlocks(t, blockDataFile, blockDataNet)
// 	if e != nil  {
// 		t.Fatalf("loadBlocks: Unexpected error: %v", e)
// 	}
// 	// We'll use a simple mock header store since the GetBlocks method
// 	// assumes we only query for blocks with an already known header.
// 	headers := newMockBlockHeaderStore()
// 	// Iterate through the blocks, calculating the size of half of them,
// 	// and writing them to the header store.
// 	var size uint64
// 	for i, b := range blocks {
// 		header := headerfs.BlockHeader{
// 			BlockHeader: &b.WireBlock().Header,
// 			Height:      uint32(i),
// 		}
// 		headers.WriteHeaders(header)
// 		sz, _ := (&cache.CacheableBlock{Block: b}).Size()
// 		if i < len(blocks)/2 {
// 			size += sz
// 		}
// 	}
// 	// Set up a ChainService with a BlockCache that can fit the first half
// 	// of the blocks.
// 	cs := &ChainService{
// 		BlockCache:   lru.NewCache(size),
// 		BlockHeaders: headers,
// 		chainParams: chaincfg.Params{
// 			PowLimit: maxPowLimit,
// 		},
// 		timeSource: blockchain.NewMedianTime(),
// 	}
// 	// We'll set up the queryPeers method to make sure we are only querying
// 	// for blocks, and send the block hashes queried over the queries
// 	// channel.
// 	queries := make(chan chainhash.Hash, 1)
// 	cs.queryPeers = func(msg wire.Message, f func(*ServerPeer,
// 		wire.Message, chan<- struct{}), qo ...QueryOption) {
// 		getData, ok := msg.(*wire.MsgGetData)
// 		if !ok {
// 			t.Fatalf("unexpected type: %T", msg)
// 		}
// 		if len(getData.InvList) != 1 {
// 			t.Fatalf("expected 1 element in inv list, found %v",
// 				len(getData.InvList))
// 		}
// 		inv := getData.InvList[0]
// 		if inv.Type != wire.InvTypeWitnessBlock {
// 			t.Fatalf("unexpected inv type: %v", inv.Type)
// 		}
// 		// Serve the block that matches the requested block header.
// 		for _, b := range blocks {
// 			if *b.Hash() == inv.Hash {
// 				// Execute the callback with the found block,
// 				// and wait for the quit channel to be closed.
// 				quit := qu.T()
// 				f(nil, b.WireBlock(), quit)
// 				select {
// 				case <-quit:
// 				case <-time.After(1 * time.Second):
// 					t.Fatalf("channel not closed")
// 				}
// 				// Notify the test about the query.
// 				select {
// 				case queries <- inv.Hash:
// 				case <-time.After(1 * time.Second):
// 					t.Fatalf("query was not handled")
// 				}
// 				return
// 			}
// 		}
// 		t.Fatalf("queried for unknown block: %v", inv.Hash)
// 	}
// 	// fetchAndAssertPeersQueried calls GetBlock and makes sure the block
// 	// is fetched from the peers.
// 	fetchAndAssertPeersQueried := func(hash chainhash.Hash) {
// 		found, e := cs.GetBlock(hash)
// 		if e != nil  {
// 			t.Fatalf("error getting block: %v", e)
// 		}
// 		if *found.Hash() != hash {
// 			t.Fatalf("requested block with hash %v, got %v",
// 				hash, found.Hash())
// 		}
// 		select {
// 		case q := <-queries:
// 			if q != hash {
// 				t.Fatalf("expected hash %v to be queried, "+
// 					"got %v", hash, q)
// 			}
// 		case <-time.After(1 * time.Second):
// 			t.Fatalf("did not query peers for block")
// 		}
// 	}
// 	// fetchAndAssertInCache calls GetBlock and makes sure the block is not
// 	// fetched from the peers.
// 	fetchAndAssertInCache := func(hash chainhash.Hash) {
// 		found, e := cs.GetBlock(hash)
// 		if e != nil  {
// 			t.Fatalf("error getting block: %v", e)
// 		}
// 		if *found.Hash() != hash {
// 			t.Fatalf("requested block with hash %v, got %v",
// 				hash, found.Hash())
// 		}
// 		// Make sure we didn't query the peers for this block.
// 		select {
// 		case q := <-queries:
// 			t.Fatalf("did not expect query for block %v", q)
// 		default:
// 		}
// 	}
// 	// Get the first half of the blocks. Since this is the first time we
// 	// request them, we expect them all to be fetched from peers.
// 	for _, b := range blocks[:len(blocks)/2] {
// 		fetchAndAssertPeersQueried(*b.Hash())
// 	}
// 	// Get the first half of the blocks again. This time we expect them all
// 	// to be fetched from the cache.
// 	for _, b := range blocks[:len(blocks)/2] {
// 		fetchAndAssertInCache(*b.Hash())
// 	}
// 	// Get the second half of the blocks. These have not been fetched
// 	// before, and we expect them to be fetched from peers.
// 	for _, b := range blocks[len(blocks)/2:] {
// 		fetchAndAssertPeersQueried(*b.Hash())
// 	}
// 	// Since the cache only had capacity for the first half of the blocks,
// 	// some of these should now have been evicted. We only check the first
// 	// one, since we cannot know for sure how many because of the variable
// 	// size.
// 	b := blocks[0]
// 	fetchAndAssertPeersQueried(*b.Hash())
// }

// TestCacheBigEnoughHoldsAllFilter creates a cache big enough to hold all filters, then gets them in random order and
// makes sure they are always there.
func TestCacheBigEnoughHoldsAllFilter(t *testing.T) {
	// Create different sized filters.
	b1, f1, s1 := genRandFilter(1, t)
	b2, f2, s2 := genRandFilter(10, t)
	b3, f3, s3 := genRandFilter(100, t)
	cs := &ChainService{
		FilterCache: lru.NewCache(s1 + s2 + s3),
	}
	// Insert those filters into the cache making sure nothing gets evicted.
	assertEqual(t, cs.FilterCache.Len(), 0, "")
	e := cs.putFilterToCache(b1, filterdb.RegularFilter, f1)
	if e != nil {
		D.Ln(e)
	}
	assertEqual(t, cs.FilterCache.Len(), 1, "")
	e = cs.putFilterToCache(b2, filterdb.RegularFilter, f2)
	if e != nil {
		D.Ln(e)
	}
	assertEqual(t, cs.FilterCache.Len(), 2, "")
	e = cs.putFilterToCache(b3, filterdb.RegularFilter, f3)
	if e != nil {
		D.Ln(e)
	}
	assertEqual(t, cs.FilterCache.Len(), 3, "")
	// Chk that we can get those filters back independent of Get order.
	assertEqual(t, getFilter(cs, b1, t), f1, "")
	assertEqual(t, getFilter(cs, b2, t), f2, "")
	assertEqual(t, getFilter(cs, b3, t), f3, "")
	assertEqual(t, getFilter(cs, b2, t), f2, "")
	assertEqual(t, getFilter(cs, b3, t), f3, "")
	assertEqual(t, getFilter(cs, b1, t), f1, "")
	assertEqual(t, getFilter(cs, b3, t), f3, "")
	assertEqual(t, cs.FilterCache.Len(), 3, "")
}
func assertEqual(t *testing.T, a interface{}, b interface{}, message string) {
	if a == b {
		return
	}
	if len(message) == 0 {
		message = fmt.Sprintf("%v != %v", a, b)
	}
	t.Fatal(message)
}

// getRandFilter generates a random GCS filter that contains numElements. It will then convert that filter into
// CacheableFilter to compute it's size for convenience. It will return the filter along with it's size and randomly
// generated block hash. testing.T is passed in as a convenience to deal with errors in this method and making the test
// code more straigthforward. Method originally taken from filterdb/db_test.go.
func genRandFilter(numElements uint32, t *testing.T) (
	*chainhash.Hash, *gcs.Filter, uint64,
) {
	var e error
	elements := make([][]byte, numElements)
	for i := uint32(0); i < numElements; i++ {
		var elem [20]byte
		if _, e = rand.Read(elem[:]); E.Chk(e) {
			t.Fatalf("unable to create random filter: %v", e)
			return nil, nil, 0
		}
		elements[i] = elem[:]
	}
	var key [16]byte
	if _, e = rand.Read(key[:]); E.Chk(e) {
		t.Fatalf("unable to create random filter: %v", e)
		return nil, nil, 0
	}
	filter, e := gcs.BuildGCSFilter(
		builder.DefaultP, builder.DefaultM, key, elements,
	)
	if e != nil {
		t.Fatalf("unable to create random filter: %v", e)
		return nil, nil, 0
	}
	// Convert into CacheableFilter and compute Size.
	c := &cache.CacheableFilter{Filter: filter}
	s, e := c.Size()
	if e != nil {
		t.Fatalf("unable to create random filter: %v", e)
		return nil, nil, 0
	}
	return genRandomBlockHash(), filter, s
}

// genRandomBlockHash generates a random block hash using math/rand.
func genRandomBlockHash() *chainhash.Hash {
	var seed [32]byte
	rand.Read(seed[:])
	hash := chainhash.Hash(seed)
	return &hash
}

// getFilter is a convenience method which will extract a value from the cache and handle errors, it makes the test code
// easier to follow.
func getFilter(cs *ChainService, b *chainhash.Hash, t *testing.T) *gcs.Filter {
	val, e := cs.getFilterFromCache(b, filterdb.RegularFilter)
	if e != nil {
		t.Fatal(e)
	}
	return val
}

// // loadBlocks loads the blocks contained in the tstdata directory and returns
// // a slice of them.
// //
// // NOTE: copied from btcsuite/btcd/database/ffldb/interface_test.go
// func loadBlocks(t *testing.T, dataFile string, network wire.BitcoinNet) (
// 	[]*util.Block, error) {
// 	// Open the file that contains the blocks for reading.
// 	fi, e := os.Open(dataFile)
// 	if e != nil  {
// 		t.Errorf("failed to open file %v, err %v", dataFile, e)
// 		return nil, e
// 	}
// 	defer func() {
// 		if e := fi.Close(); E.Chk(e) {
// 			t.Errorf("failed to close file %v %v", dataFile,
// 				err)
// 		}
// 	}()
// 	dr := bzip2.NewReader(fi)
// 	// Set the first block as the genesis block.
// 	blocks := make([]*util.Block, 0, 256)
// 	genesis := util.NewBlock(chaincfg.MainNetParams.GenesisBlock)
// 	blocks = append(blocks, genesis)
// 	// Load the remaining blocks.
// 	for height := 1; ; height++ {
// 		var net uint32
// 		e := binary.Read(dr, binary.LittleEndian, &net)
// 		if e ==  io.EOF {
// 			// Hit end of file at the expected offset.  No error.
// 			break
// 		}
// 		if e != nil  {
// 			t.Errorf("Failed to load network type for block %d: %v",
// 				height, e)
// 			return nil, e
// 		}
// 		if net != uint32(network) {
// 			t.Errorf("Block doesn't match network: %v expects %v",
// 				net, network)
// 			return nil, e
// 		}
// 		var blockLen uint32
// 		e = binary.Read(dr, binary.LittleEndian, &blockLen)
// 		if e != nil  {
// 			t.Errorf("Failed to load block size for block %d: %v",
// 				height, e)
// 			return nil, e
// 		}
// 		// Read the block.
// 		blockBytes := make([]byte, blockLen)
// 		_, e = io.ReadFull(dr, blockBytes)
// 		if e != nil  {
// 			t.Errorf("Failed to load block %d: %v", height, err)
// 			return nil, e
// 		}
// 		// Deserialize and store the block.
// 		block, e := util.NewBlockFromBytes(blockBytes)
// 		if e != nil  {
// 			t.Errorf("Failed to parse block %v: %v", height, err)
// 			return nil, e
// 		}
// 		blocks = append(blocks, block)
// 	}
// 	return blocks, nil
// }
