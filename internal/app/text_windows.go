package app

import (
	"image"
	"math"

	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// uiFont is the typeface used for labels. Segoe UI ships with every
// supported version of Windows.
const uiFont = "Segoe UI"

// maxTextCache bounds the number of cached text masks. The deck redraws the
// same handful of labels on every animation frame, so the cache is what
// keeps animation cheap; the bound only guards against unbounded growth.
const maxTextCache = 256

// gdiText implements render.TextDrawer with the system text renderer, which
// brings font fallback for every script and emoji for free.
//
// GDI cannot draw onto a surface with an alpha channel, so each string is
// first drawn in white on black into a scratch bitmap. The resulting
// greyscale image is exactly the text's coverage mask, which is then blended
// onto the destination in the requested colour.
type gdiText struct {
	scratch *win32.DIB
	fonts   map[fontKey]win32.HFONT
	masks   map[maskKey]*textMask
}

type fontKey struct {
	height int32
	bold   bool
}

type maskKey struct {
	text          string
	width, height int
	font          fontKey
	align         render.Align
	path          bool
}

// textMask is the per-pixel coverage of a rendered string.
type textMask struct {
	width, height int
	alpha         []uint8
}

func newGDIText() *gdiText {
	return &gdiText{fonts: map[fontKey]win32.HFONT{}, masks: map[maskKey]*textMask{}}
}

// close releases the GDI resources held by the renderer.
func (t *gdiText) close() {
	for _, f := range t.fonts {
		win32.DeleteObject(win32.HGDIOBJ(f))
	}
	t.fonts = map[fontKey]win32.HFONT{}
	t.masks = map[maskKey]*textMask{}
	t.scratch.Close()
	t.scratch = nil
}

// DrawText implements render.TextDrawer.
func (t *gdiText) DrawText(dst *image.RGBA, r image.Rectangle, text string, style render.TextStyle) {
	if r.Empty() || text == "" || style.Opacity <= 0 || style.Size < 1 {
		return
	}
	key := maskKey{text, r.Dx(), r.Dy(), fontKey{int32(math.Round(style.Size)), style.Bold}, style.Align, style.Path}
	mask, ok := t.masks[key]
	if !ok {
		mask = t.rasterize(key)
		if mask == nil {
			return
		}
		if len(t.masks) >= maxTextCache {
			t.masks = map[maskKey]*textMask{}
		}
		t.masks[key] = mask
	}
	blendMask(dst, r.Min, mask, style)
}

// rasterize draws the string with GDI and extracts its coverage mask.
func (t *gdiText) rasterize(key maskKey) *textMask {
	if t.scratch == nil || t.scratch.Width < key.width || t.scratch.Height < key.height {
		w, h := key.width, key.height
		if t.scratch != nil {
			w, h = max(w, t.scratch.Width), max(h, t.scratch.Height)
			t.scratch.Close()
		}
		var err error
		if t.scratch, err = win32.NewDIB(w, h); err != nil {
			t.scratch = nil
			return nil
		}
	}
	dib := t.scratch

	stride := dib.Width * 4
	for y := 0; y < key.height; y++ {
		clear(dib.Pix[y*stride : y*stride+key.width*4])
	}

	previous := win32.SelectObject(dib.DC, win32.HGDIOBJ(t.font(key.font)))
	win32.SetBkMode(dib.DC, win32.TRANSPARENT)
	win32.SetTextColor(dib.DC, win32.RGB(255, 255, 255))
	bounds := win32.RECT{Right: int32(key.width), Bottom: int32(key.height)}
	align := uint32(win32.DT_LEFT)
	switch key.align {
	case render.AlignCenter:
		align = win32.DT_CENTER
	case render.AlignRight:
		align = win32.DT_RIGHT
	}
	ellipsis := uint32(win32.DT_END_ELLIPSIS)
	if key.path {
		ellipsis = win32.DT_PATH_ELLIPSIS
	}
	win32.DrawText(dib.DC, key.text, &bounds,
		align|win32.DT_VCENTER|win32.DT_SINGLELINE|ellipsis|win32.DT_NOPREFIX)
	win32.SelectObject(dib.DC, previous)
	win32.Flush()

	mask := &textMask{width: key.width, height: key.height, alpha: make([]uint8, key.width*key.height)}
	for y := 0; y < key.height; y++ {
		row := dib.Pix[y*stride:]
		for x := 0; x < key.width; x++ {
			// Greyscale anti-aliasing writes equal channels; green is as
			// good as any.
			mask.alpha[y*key.width+x] = row[x*4+1]
		}
	}
	return mask
}

func (t *gdiText) font(key fontKey) win32.HFONT {
	if f, ok := t.fonts[key]; ok {
		return f
	}
	face, weight := uiFont, int32(win32.FW_NORMAL)
	if key.bold {
		weight = win32.FW_SEMIBOLD
	}
	// ClearType would produce coloured fringes that cannot be used as a
	// coverage mask, so ask for greyscale anti-aliasing.
	f := win32.CreateFont(key.height, weight, win32.ANTIALIASED_QUALITY, face)
	t.fonts[key] = f
	return f
}

// blendMask composites a coverage mask onto dst at origin in the style's
// colour. dst uses premultiplied alpha.
func blendMask(dst *image.RGBA, origin image.Point, mask *textMask, style render.TextStyle) {
	opacity := math.Min(style.Opacity, 1) * float64(style.Color.A) / 255
	scale := uint32(opacity*256 + 0.5)
	cr, cg, cb := uint32(style.Color.R), uint32(style.Color.G), uint32(style.Color.B)

	b := dst.Bounds()
	for y := 0; y < mask.height; y++ {
		dy := origin.Y + y
		if dy < b.Min.Y || dy >= b.Max.Y {
			continue
		}
		for x := 0; x < mask.width; x++ {
			dx := origin.X + x
			if dx < b.Min.X || dx >= b.Max.X {
				continue
			}
			a := uint32(mask.alpha[y*mask.width+x]) * scale >> 8
			if a == 0 {
				continue
			}
			p := dst.Pix[dst.PixOffset(dx, dy):]
			inv := 255 - a
			p[0] = uint8((cr*a + uint32(p[0])*inv + 127) / 255)
			p[1] = uint8((cg*a + uint32(p[1])*inv + 127) / 255)
			p[2] = uint8((cb*a + uint32(p[2])*inv + 127) / 255)
			p[3] = uint8((255*a + uint32(p[3])*inv + 127) / 255)
		}
	}
}

// MeasureText implements render.TextMeasurer.
func (t *gdiText) MeasureText(text string, style render.TextStyle) float64 {
	if text == "" || style.Size < 1 {
		return 0
	}
	if t.scratch == nil {
		var err error
		if t.scratch, err = win32.NewDIB(1, 1); err != nil {
			t.scratch = nil
			return 0
		}
	}
	previous := win32.SelectObject(t.scratch.DC, win32.HGDIOBJ(t.font(fontKey{int32(math.Round(style.Size)), style.Bold})))
	defer win32.SelectObject(t.scratch.DC, previous)
	return float64(win32.TextWidth(t.scratch.DC, text))
}
