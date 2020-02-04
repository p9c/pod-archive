package duoui

import (
	"github.com/p9c/pod/cmd/gui/models"
	"github.com/p9c/pod/cmd/gui/rcd"
	"github.com/p9c/pod/pkg/conte"
	"github.com/p9c/pod/pkg/gui/layout"
)

func DuoUIsidebar(duo *models.DuoUI, cx *conte.Xt, rc *rcd.RcVar) func(){
	return func(){
		duo.DuoUIcomponents.Sidebar.Layout.Layout(duo.DuoUIcontext,
			layout.Rigid(DuoUImenu(duo, cx, rc)),
		)
	}
}