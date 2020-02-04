package components

import (
	"github.com/p9c/pod/cmd/gui/helpers"
	"github.com/p9c/pod/cmd/gui/models"
	"github.com/p9c/pod/cmd/gui/rcd"
	"github.com/p9c/pod/pkg/gui/widget"
	"github.com/p9c/pod/pkg/conte"
	"github.com/p9c/pod/pkg/gui/layout"
	"github.com/p9c/pod/pkg/gui/text"
	"github.com/p9c/pod/pkg/gui/unit"
)

var (
	topLabel   = "testtopLabel"
	addressLineEditor = &widget.Editor{
		SingleLine: true,
		Submit:     true,
	}
	amountLineEditor = &widget.Editor{
		SingleLine: true,
		Submit:     true,
	}
	list = &layout.List{
		Axis: layout.Vertical,
	}
	ln = layout.UniformInset(unit.Dp(1))
	in = layout.UniformInset(unit.Dp(0))
)

func DuoUIsend(duo *models.DuoUI, cx *conte.Xt, rc *rcd.RcVar) {
	layout.Flex{}.Layout(duo.DuoUIcontext,
		layout.Rigid(func() {
			helpers.DuoUIdrawRectangle(duo.DuoUIcontext, duo.DuoUIconstraints.Width.Max, 180, "ff30cf30", [4]float32{0, 0, 0, 0}, [4]float32{0, 0, 0, 0})

			layout.Flex{
				Axis: layout.Vertical,
			}.Layout(duo.DuoUIcontext,
				layout.Rigid(func() {
					ln.Layout(duo.DuoUIcontext, func() {
						cs := duo.DuoUIcontext.Constraints
						helpers.DuoUIdrawRectangle(duo.DuoUIcontext, cs.Width.Max, 32, "fff4f4f4", [4]float32{0, 0, 0, 0}, [4]float32{0, 0, 0, 0})
						in.Layout(duo.DuoUIcontext, func() {
							helpers.DuoUIdrawRectangle(duo.DuoUIcontext, cs.Width.Max, 30, "ffffffff", [4]float32{0, 0, 0, 0}, [4]float32{0, 0, 0, 0})
							e := duo.DuoUItheme.DuoUIeditor("DUO address", "DUO dva")
							e.Font.Style = text.Italic
							e.Font.Size = unit.Dp(24)
							e.Layout(duo.DuoUIcontext, addressLineEditor)
							for _, e := range addressLineEditor.Events(duo.DuoUIcontext) {
								if e, ok := e.(widget.SubmitEvent); ok {
									topLabel = e.Text
									addressLineEditor.SetText("")
								}
							}
						})
					})
				}),
				layout.Rigid(func() {
					ln.Layout(duo.DuoUIcontext, func() {
						cs := duo.DuoUIcontext.Constraints
						helpers.DuoUIdrawRectangle(duo.DuoUIcontext, cs.Width.Max, 32, "fff4f4f4", [4]float32{0, 0, 0, 0}, [4]float32{0, 0, 0, 0})
						in.Layout(duo.DuoUIcontext, func() {
							helpers.DuoUIdrawRectangle(duo.DuoUIcontext, cs.Width.Max, 30, "ffffffff", [4]float32{0, 0, 0, 0}, [4]float32{0, 0, 0, 0})
							e := duo.DuoUItheme.DuoUIeditor("DUO Amount", "DUO dva")
							e.Font.Style = text.Italic
							e.Font.Size = unit.Dp(24)
							e.Layout(duo.DuoUIcontext, amountLineEditor)
							for _, e := range amountLineEditor.Events(duo.DuoUIcontext) {
								if e, ok := e.(widget.SubmitEvent); ok {
									topLabel = e.Text
									amountLineEditor.SetText("")
								}
							}
						})
					})
				}))
		}),
	)
}