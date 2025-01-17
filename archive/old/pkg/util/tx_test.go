package util_test

import (
	"bytes"
	"github.com/p9c/pod/pkg/block"
	"io"
	"reflect"
	"testing"
	
	"github.com/davecgh/go-spew/spew"
	
	"github.com/p9c/pod/pkg/chainhash"
	"github.com/p9c/pod/pkg/util"
)

// TestTx tests the API for Tx.
func TestTx(t *testing.T) {
	testTx := block.Block100000.Transactions[0]
	tx := util.NewTx(testTx)
	// Ensure we get the same data back out.
	if msgTx := tx.MsgTx(); !reflect.DeepEqual(msgTx, testTx) {
		t.Errorf("MsgTx: mismatched MsgTx - got %v, want %v",
			spew.Sdump(msgTx), spew.Sdump(testTx))
	}
	// Ensure transaction index set and get work properly.
	wantIndex := 0
	tx.SetIndex(0)
	if gotIndex := tx.Index(); gotIndex != wantIndex {
		t.Errorf("Index: mismatched index - got %v, want %v",
			gotIndex, wantIndex)
	}
	// Hash for block 100,000 transaction 0.
	wantHashStr := "8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87"
	wantHash, e := chainhash.NewHashFromStr(wantHashStr)
	if e != nil  {
		t.Errorf("NewHashFromStr: %v", e)
	}
	// Request the hash multiple times to test generation and caching.
	for i := 0; i < 2; i++ {
		hash := tx.Hash()
		if !hash.IsEqual(wantHash) {
			t.Errorf("Hash #%d mismatched hash - got %v, want %v", i,
				hash, wantHash)
		}
	}
}

// TestNewTxFromBytes tests creation of a Tx from serialized bytes.
func TestNewTxFromBytes(t *testing.T) {
	// Serialize the test transaction.
	testTx := block.Block100000.Transactions[0]
	var testTxBuf bytes.Buffer
	e := testTx.Serialize(&testTxBuf)
	if e != nil  {
		t.Errorf("Serialize: %v", e)
	}
	testTxBytes := testTxBuf.Bytes()
	// Create a new transaction from the serialized bytes.
	tx, e := util.NewTxFromBytes(testTxBytes)
	if e != nil  {
		t.Errorf("NewTxFromBytes: %v", e)
		return
	}
	// Ensure the generated MsgTx is correct.
	if msgTx := tx.MsgTx(); !reflect.DeepEqual(msgTx, testTx) {
		t.Errorf("MsgTx: mismatched MsgTx - got %v, want %v",
			spew.Sdump(msgTx), spew.Sdump(testTx))
	}
}

// TestTxErrors tests the error paths for the Tx API.
func TestTxErrors(t *testing.T) {
	// Serialize the test transaction.
	testTx := block.Block100000.Transactions[0]
	var testTxBuf bytes.Buffer
	e := testTx.Serialize(&testTxBuf)
	if e != nil  {
		t.Errorf("Serialize: %v", e)
	}
	testTxBytes := testTxBuf.Bytes()
	// Truncate the transaction byte buffer to force errors.
	shortBytes := testTxBytes[:4]
	_, e = util.NewTxFromBytes(shortBytes)
	if e != io.EOF {
		t.Errorf("NewTxFromBytes: did not get expected error - "+
			"got %v, want %v", e, io.EOF)
	}
}
