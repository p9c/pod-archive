package p9

import (
	"image/color"
	"math"
	"strings"

	"gioui.org/f32"
	l "gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/p9c/pod/pkg/gui/f32color"
	w "github.com/p9c/pod/pkg/gui/widget"
)

type _button struct {
	th   *Theme
	text string
	// Color is the text color.
	color        color.RGBA
	font         text.Font
	textSize     unit.Value
	background   color.RGBA
	cornerRadius unit.Value
	inset        *l.Inset
	button       *w.Clickable
	shaper       text.Shaper
}

func (b *_button) Text(text string) *_button {
	b.text = text
	return b
}

func (b *_button) Inset(pad float32) *_button {
	b.inset = &l.Inset{
		Top:    unit.Sp(pad),
		Right:  unit.Sp(pad),
		Bottom: unit.Sp(pad),
		Left:   unit.Sp(pad),
	}
	return b
}

func (b *_button) CornerRadius(cornerRadius float32) *_button {
	b.cornerRadius = unit.Sp(cornerRadius)
	return b
}

func (b *_button) Background(background string) *_button {
	b.background = b.th.Colors.Get(background)
	return b
}

func (b *_button) TextScale(scale float32) *_button {
	b.textSize = b.th.textSize.Scale(scale)
	return b
}

func (b *_button) Font(font string) *_button {
	var f text.Font
	for i := range b.th.collection {
		// Debug(th.Collection[i].Font)
		if b.th.collection[i].Font.Typeface == text.Typeface(font) {
			f = b.th.collection[i].Font
		}
	}
	b.font = f
	return b
}

func (b *_button) Color(color string) *_button {
	b.color = b.th.Colors.Get(color)
	return b
}

func (th *Theme) Button(btn *w.Clickable) *_button {
	return &_button{
		th:   th,
		text: strings.ToUpper("text unset"),
		// default sets
		font:  th.collection[0].Font,
		color: th.Colors.Get("ButtonText"),
		// CornerRadius: unit.Dp(4),
		background: th.Colors.Get("Primary"),
		textSize:   th.textSize,
		inset: &l.Inset{
			Top: unit.Sp(8), Bottom: unit.Sp(8),
			Left: unit.Sp(8), Right: unit.Sp(8),
		},
		button: btn,
		shaper: th.shaper,
	}
}

func (b *_button) Fn(gtx l.Context) l.Dimensions {
	bl := &_buttonLayout{
		background:   b.background,
		cornerRadius: b.cornerRadius,
		button:       b.button,
	}
	fn := func(gtx l.Context) l.Dimensions {
		return b.inset.Layout(gtx, func(gtx l.Context) l.Dimensions {
			paint.ColorOp{Color: b.color}.Add(gtx.Ops)
			return widget.Label{Alignment: text.Middle}.
				Layout(gtx, b.shaper, b.font, b.textSize, b.text)
		})
	}
	bl.Widget(fn)
	return bl.Fn(gtx)
}

func drawInk(c l.Context, p widget.Press) {
	// duration is the number of seconds for the completed animation: expand while fading in, then out.
	const (
		expandDuration = float32(0.5)
		fadeDuration   = float32(0.9)
	)
	now := c.Now
	t := float32(now.Sub(p.Start).Seconds())
	end := p.End
	if end.IsZero() {
		// If the press hasn't ended, don't fade-out.
		end = now
	}
	endt := float32(end.Sub(p.Start).Seconds())
	// Compute the fade-in/out position in [0;1].
	var alphat float32
	{
		var haste float32
		if p.Cancelled {
			// If the press was cancelled before the inkwell was fully faded in, fast forward the animation to match the
			// fade-out.
			if h := 0.5 - endt/fadeDuration; h > 0 {
				haste = h
			}
		}
		// Fade in.
		half1 := t/fadeDuration + haste
		if half1 > 0.5 {
			half1 = 0.5
		}
		// Fade out.
		half2 := float32(now.Sub(end).Seconds())
		half2 /= fadeDuration
		half2 += haste
		if half2 > 0.5 {
			// Too old.
			return
		}

		alphat = half1 + half2
	}
	// Compute the expand position in [0;1].
	sizet := t
	if p.Cancelled {
		// Freeze expansion of cancelled presses.
		sizet = endt
	}
	sizet /= expandDuration
	// Animate only ended presses, and presses that are fading in.
	if !p.End.IsZero() || sizet <= 1.0 {
		op.InvalidateOp{}.Add(c.Ops)
	}
	if sizet > 1.0 {
		sizet = 1.0
	}
	if alphat > .5 {
		// Start fadeout after half the animation.
		alphat = 1.0 - alphat
	}
	// Twice the speed to attain fully faded in at 0.5.
	t2 := alphat * 2
	// Beziér ease-in curve.
	alphaBezier := t2 * t2 * (3.0 - 2.0*t2)
	sizeBezier := sizet * sizet * (3.0 - 2.0*sizet)
	size := float32(c.Constraints.Min.X)
	if h := float32(c.Constraints.Min.Y); h > size {
		size = h
	}
	// Cover the entire constraints min rectangle.
	size *= 2 * float32(math.Sqrt(2))
	// Apply curve values to size and color.
	size *= sizeBezier
	alpha := 0.7 * alphaBezier
	const col = 0.8
	ba, bc := byte(alpha*0xff), byte(col*0xff)
	defer op.Push(c.Ops).Pop()
	rgba := f32color.MulAlpha(color.RGBA{A: 0xff, R: bc, G: bc, B: bc}, ba)
	ink := paint.ColorOp{Color: rgba}
	ink.Add(c.Ops)
	rr := size * .5
	op.Offset(p.Position.Add(f32.Point{
		X: -rr,
		Y: -rr,
	})).Add(c.Ops)
	clip.RRect{
		Rect: f32.Rectangle{Max: f32.Point{
			X: size,
			Y: size,
		}},
		NE: rr, NW: rr, SE: rr, SW: rr,
	}.Add(c.Ops)
	paint.PaintOp{Rect: f32.Rectangle{Max: f32.Point{X: size, Y: size}}}.Add(c.Ops)
}