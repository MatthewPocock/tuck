package render

import (
	"image"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/note"
)

// AppIcon draws the application icon — a sticky note with a few lines of
// "writing" tucked against a dark edge — at the given size in pixels.
func AppIcon(size int) *image.RGBA {
	c := NewCanvas(size, size)
	u := float64(size) / 32 // the icon is designed on a 32-unit grid
	sw := Palette(note.Yellow, false)

	// The dark "screen edge" the note is tucked against.
	c.FillRoundRect(deck.Rect{X: 23 * u, Y: 2 * u, W: 7 * u, H: 28 * u}, 3.5*u, rgb(0x1C1D21))

	// The note itself.
	body := deck.Rect{X: 2 * u, Y: 5 * u, W: 24 * u, H: 22 * u}
	c.FillRoundRect(body, 5*u, sw.Fill)
	c.StrokeRoundRect(body, 5*u, max(1, 0.6*u), WithAlpha(sw.Ink, 0.22))

	// Lines of writing.
	ink := WithAlpha(sw.Ink, 0.72)
	c.Line(7*u, 12*u, 19*u, 12*u, 2.2*u, ink)
	c.Line(7*u, 17*u, 16*u, 17*u, 2.2*u, ink)
	c.Line(7*u, 22*u, 12*u, 22*u, 2.2*u, ink)
	return c.Img
}

// ColorDot draws a note colour as a filled circle, as shown beside each
// colour in menus, at the given size in pixels. The selected colour gets a
// ring in the menu's text colour, which reads better than the faint
// highlight Windows uses for checked items with a picture.
func ColorDot(size int, col note.Color, selected, dark bool) *image.RGBA {
	c := NewCanvas(size, size)
	sw := Palette(col, false)
	mid := float64(size) / 2
	stroke := max(1, float64(size)/16)
	radius := float64(size) * 0.34
	if selected {
		ring := rgb(0x1F1F1F)
		if dark {
			ring = rgb(0xF2F2F2)
		}
		c.StrokeCircle(mid, mid, float64(size)/2-stroke, 1.5*stroke, ring)
		radius = float64(size) * 0.27
	}
	c.FillCircle(mid, mid, radius, sw.Fill)
	// A hairline in the ink colour keeps light fills visible on light menus.
	c.StrokeCircle(mid, mid, radius, stroke, WithAlpha(sw.Ink, 0.25))
	return c.Img
}
