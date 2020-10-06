package p9

import (
	"image"
	"image/color"

	"gioui.org/f32"
	l "gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/p9c/pod/pkg/gui/f32color"
)

type _progressBar struct {
	color    color.RGBA
	progress int
}

func (th *Theme) ProgressBar() *_progressBar {
	return &_progressBar{
		progress: 0,
		color:    th.Colors.Get("Primary"),
	}
}

// SetProgress sets the progress of the _progressBar
func (p *_progressBar) SetProgress(progress int) *_progressBar {
	p.progress = progress
	return p
}

func (p *_progressBar) SetColor(c color.RGBA) *_progressBar {
	p.color = c
	return p
}

func (p *_progressBar) Fn(gtx l.Context) l.Dimensions {
	shader := func(width float32, color color.RGBA) l.Dimensions {
		maxHeight := unit.Dp(4)
		rr := float32(gtx.Px(unit.Dp(2)))

		d := image.Point{X: int(width), Y: gtx.Px(maxHeight)}
		dr := f32.Rectangle{
			Max: f32.Point{X: float32(d.X), Y: float32(d.Y)},
		}

		clip.RRect{
			Rect: f32.Rectangle{Max: f32.Point{X: width, Y: float32(gtx.Px(maxHeight))}},
			NE:   rr, NW: rr, SE: rr, SW: rr,
		}.Add(gtx.Ops)

		paint.ColorOp{Color: color}.Add(gtx.Ops)
		paint.PaintOp{Rect: dr}.Add(gtx.Ops)

		return l.Dimensions{Size: d}
	}

	progress := p.progress
	if progress > 100 {
		progress = 100
	} else if progress < 0 {
		progress = 0
	}

	progressBarWidth := float32(gtx.Constraints.Max.X)

	return l.Stack{Alignment: l.W}.Layout(gtx,
		l.Stacked(func(gtx l.Context) l.Dimensions {
			// Use a transparent equivalent of progress color.
			bgCol := f32color.MulAlpha(p.color, 150)

			return shader(progressBarWidth, bgCol)
		}),
		l.Stacked(func(gtx l.Context) l.Dimensions {
			fillWidth := (progressBarWidth / 100) * float32(progress)
			fillColor := p.color
			if gtx.Queue == nil {
				fillColor = f32color.MulAlpha(fillColor, 200)
			}
			return shader(fillWidth, fillColor)
		}),
	)
}