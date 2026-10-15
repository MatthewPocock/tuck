// Package fonttext provides a portable render.TextDrawer backed by the Go
// fonts embedded in golang.org/x/image.
//
// It exists for tests and previews on platforms without the Windows text
// renderer. It lives outside the render package so that the fonts are not
// linked into the Windows binary.
package fonttext

import (
	"image"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/MatthewPocock/tuck/internal/render"
)

// FontText is a render.TextDrawer that renders with the Go fonts. It only covers Latin, Greek and Cyrillic, which is
// enough for tests and previews but not for the real app.
type FontText struct {
	mu    sync.Mutex
	faces map[faceKey]font.Face
}

type faceKey struct {
	size float64
	bold bool
}

// New returns a ready-to-use FontText.
func New() *FontText {
	return &FontText{faces: make(map[faceKey]font.Face)}
}

var (
	regularFont = sync.OnceValue(func() *opentype.Font { return mustParse(goregular.TTF) })
	mediumFont  = sync.OnceValue(func() *opentype.Font { return mustParse(gomedium.TTF) })
)

func mustParse(ttf []byte) *opentype.Font {
	f, err := opentype.Parse(ttf)
	if err != nil {
		// The fonts are compiled in, so a failure here is a programming
		// error rather than a runtime condition.
		panic("render: parse embedded font: " + err.Error())
	}
	return f
}

func (t *FontText) face(style render.TextStyle) font.Face {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := faceKey{style.Size, style.Bold}
	if f, ok := t.faces[key]; ok {
		return f
	}
	src := regularFont()
	if style.Bold {
		src = mediumFont()
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: style.Size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic("render: create font face: " + err.Error())
	}
	t.faces[key] = f
	return f
}

// DrawText implements render.TextDrawer.
func (t *FontText) DrawText(dst *image.RGBA, r image.Rectangle, text string, style render.TextStyle) {
	r = r.Intersect(dst.Bounds())
	if r.Empty() || text == "" || style.Opacity <= 0 || style.Size <= 0 {
		return
	}
	face := t.face(style)
	text = ellipsize(face, text, fixed.I(r.Dx()))

	// Centre on the cap height, which looks better balanced than centring
	// on the full ascent + descent.
	m := face.Metrics()
	capHeight := m.CapHeight
	if capHeight == 0 {
		capHeight = m.Ascent
	}
	baseline := fixed.I(r.Min.Y) + (fixed.I(r.Dy())+capHeight)/2

	clipped, ok := dst.SubImage(r).(*image.RGBA)
	if !ok {
		return
	}
	x := fixed.I(r.Min.X)
	switch slack := fixed.I(r.Dx()) - font.MeasureString(face, text); style.Align {
	case render.AlignCenter:
		x += slack / 2
	case render.AlignRight:
		x += slack
	}
	d := font.Drawer{
		Dst:  clipped,
		Src:  image.NewUniform(render.WithAlpha(style.Color, style.Opacity)),
		Face: face,
		Dot:  fixed.Point26_6{X: x, Y: baseline},
	}
	d.DrawString(text)
}

// ellipsize shortens text until it fits within width, marking the cut with
// an ellipsis.
func ellipsize(face font.Face, text string, width fixed.Int26_6) string {
	if font.MeasureString(face, text) <= width {
		return text
	}
	const ellipsis = "…"
	runes := []rune(text)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		// Drop trailing spaces so the ellipsis hugs the last word.
		for len(runes) > 0 && runes[len(runes)-1] == ' ' {
			runes = runes[:len(runes)-1]
		}
		if candidate := string(runes) + ellipsis; font.MeasureString(face, candidate) <= width {
			return candidate
		}
	}
	return ""
}

// MeasureText implements render.TextMeasurer.
func (t *FontText) MeasureText(text string, style render.TextStyle) float64 {
	if style.Size <= 0 {
		return 0
	}
	return float64(font.MeasureString(t.face(style), text)) / 64
}
