package p9

import (
	"image"
	"image/draw"

	"gioui.org/f32"
	l "gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"golang.org/x/exp/shiny/iconvg"
)

type Icon struct {
	th    *Theme
	color string
	src   []byte
	size  unit.Value
	// Cached values.
	sz       int
	op       paint.ImageOp
	imgSize  int
	imgColor string
}

// Icon returns a new Icon from iconVG data.
func (th *Theme) Icon() *Icon {
	return &Icon{th: th, size: th.TextSize, color: "Black"}
}

// Color sets the color of the icon image. It must be called before creating the image
func (i *Icon) Color(color string) *Icon {
	i.color = color
	return i
}

// Src sets the icon source to draw from
func (i *Icon) Src(data []byte) *Icon {
	_, err := iconvg.DecodeMetadata(data)
	if Check(err) {
		Debug("no image data, crashing")
		panic(err)
		// return nil
	}
	i.src = data
	return i
}

// Scale changes the size relative to the base font size
func (i *Icon) Scale(scale float32) *Icon {
	i.size = i.th.TextSize.Scale(scale * 1)
	return i
}

func (i *Icon) Size(size unit.Value) *Icon {
	i.size = size
	return i
}

// Fn renders the icon
func (i *Icon) Fn(gtx l.Context) l.Dimensions {
	ico := i.image(gtx.Px(i.size))
	if i.src == nil {
		panic("icon is nil")
	}
	ico.Add(gtx.Ops)
	paint.PaintOp{
		Rect: f32.Rectangle{
			Max: toPointF(ico.Size()),
		},
	}.Add(gtx.Ops)
	return l.Dimensions{Size: ico.Size()}
}

func (i *Icon) image(sz int) paint.ImageOp {
	if sz == i.imgSize && i.color == i.imgColor {
		// Debug("reusing old icon")
		return i.op
	}
	m, _ := iconvg.DecodeMetadata(i.src)
	dx, dy := m.ViewBox.AspectRatio()
	img := image.NewRGBA(image.Rectangle{Max: image.Point{X: sz,
		Y: int(float32(sz) * dy / dx)}})
	var ico iconvg.Rasterizer
	ico.SetDstImage(img, img.Bounds(), draw.Src)
	m.Palette[0] = i.th.Colors.Get(i.color)
	if err := iconvg.Decode(&ico, i.src, &iconvg.DecodeOptions{
		Palette: &m.Palette,
	}); Check(err) {
	}
	i.op = paint.NewImageOp(img)
	i.imgSize = sz
	i.imgColor = i.color
	return i.op
}
