package model

import (
	"github.com/p9c/pod/cmd/gui/controller"
	"github.com/p9c/pod/pkg/rpc/btcjson"
)

type DuoUIexplorer struct {
	Page        *controller.DuoUIcounter
	PerPage     *controller.DuoUIcounter
	Blocks      []DuoUIblock
	SingleBlock btcjson.GetBlockVerboseResult
}
