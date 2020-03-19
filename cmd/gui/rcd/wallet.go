package rcd

import (
	"fmt"

	"github.com/p9c/pod/pkg/rpc/btcjson"
	"github.com/p9c/pod/pkg/rpc/legacy"
	"github.com/p9c/pod/pkg/util"
)

func (r *RcVar) GetDuoUIbalance() {
	L.Trace("getting balance")
	acct := "default"
	minconf := 0
	getBalance, err := legacy.GetBalance(&btcjson.GetBalanceCmd{Account: &acct,
		MinConf: &minconf}, r.cx.WalletServer)
	if err != nil {
		// r.PushDuoUIalert("Error", err.Error(), "error")
	}
	gb, ok := getBalance.(float64)
	if ok {
		bb := fmt.Sprintf("%0.8f", gb)
		r.Status.Wallet.Balance.Store(bb)
	}
	return
}

func (r *RcVar) GetDuoUIunconfirmedBalance() {
	L.Trace("getting unconfirmed balance")
	acct := "default"
	getUnconfirmedBalance, err := legacy.GetUnconfirmedBalance(&btcjson.GetUnconfirmedBalanceCmd{Account: &acct}, r.cx.WalletServer)
	if err != nil {
		// r.PushDuoUIalert("Error", err.Error(), "error")
	}
	ub, ok := getUnconfirmedBalance.(float64)
	if ok {
		ubb := fmt.Sprintf("%0.8f", ub)
		r.Status.Wallet.Unconfirmed.Store(ubb)
	}
	return
}

func (r *RcVar) DuoSend(wp string, ad string, am float64) func() {
	return func() {
		L.Trace("sending", wp, ad, am)
		L.Info("sending", wp, ad, am)
		if am > 0 {
			// getBlockChain, err := rpc.HandleGetBlockChainInfo(r.cx.RPCServer, nil, nil)
			// if err != nil {
			// 	//r.PushDuoUIalert("Error", err.Error(), "error")
			//
			// }
			// result, ok := getBlockChain.(*btcjson.GetBlockChainInfoResult)
			// if !ok {
			// 	result = &btcjson.GetBlockChainInfoResult{}
			// }
			// var defaultNet *netparams.Params
			// switch result.Chain {
			// case "mainnet":
			// 	defaultNet = &netparams.MainNetParams
			// case "testnet":
			// 	defaultNet = &netparams.TestNet3Params
			// case "regtest":
			// 	defaultNet = &netparams.RegressionTestParams
			// default:
			// 	defaultNet = &netparams.MainNetParams
			// }
			amount, _ := util.NewAmount(am)
			addr, err := util.DecodeAddress(ad, r.cx.ActiveNet)
			if err != nil {
				// r.PushDuoUIalert("Error", err.Error(), "error")
			}
			var validateAddr *btcjson.ValidateAddressWalletResult
			if err == nil {
				var va interface{}
				va, err = legacy.ValidateAddress(&btcjson.
				ValidateAddressCmd{Address: addr.String()}, r.cx.WalletServer)
				if err != nil {
					// r.PushDuoUIalert("Error", err.Error(), "error")
				}
				vva := va.(btcjson.ValidateAddressWalletResult)
				validateAddr = &vva
				if validateAddr.IsValid {
					L.Warnf("wallet passphrase '%s'", wp)
					if res, err := legacy.WalletPassphrase(btcjson.NewWalletPassphraseCmd(wp, 5),
						r.cx.WalletServer); L.Check(err) {
					} else {
						L.Warn(res)
					}
					_, err = legacy.SendToAddress(
						&btcjson.SendToAddressCmd{
							Address: addr.EncodeAddress(), Amount: amount.ToDUO(),
						}, r.cx.WalletServer)
					if err != nil {
						// r.PushDuoUIalert("error sending to address:", err.Error(), "error")

					} else {
						// r.PushDuoUIalert("Address OK", "OK", "success")
					}
				} else {
					if err != nil {
						// r.PushDuoUIalert("Invalid address", "INVALID", "error")
					}
				}
				// r.PushDuoUIalert("Payment sent", "PAYMENT", "success")
			}
		} else {
			L.Debug(am)
		}
		r.Sent = true
		return
	}
}
