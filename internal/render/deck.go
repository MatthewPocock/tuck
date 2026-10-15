package render

import (
	"image"
	"image/color"
	"math"

	"github.com/MatthewPocock/tuck/internal/deck"
)

// Deck typography and decoration, in device-independent pixels.
const (
	labelSize    = 13.0
	labelPadding = 14.0
	tabShadow    = 0.15
	addRadius    = 10.0
)

// DrawDeck paints one frame of the deck onto the canvas, which is resized to
// the frame and left with a transparent background.
func DrawDeck(c *Canvas, f deck.Frame, text TextDrawer) {
	c.Resize(f.Width, f.Height)
	s := f.Scale

	drawHotZone(c, f.HotZone)
	drawPill(c, f)

	for _, tab := range f.Tabs {
		sw := Palette(tab.Item.Color, false)
		c.Shadow(tab.Rect, tab.Radius, 6*s, 2*s, tab.Open*(tabShadow+0.10*tab.Lift))
		c.FillRoundRect(tab.Rect, tab.Radius, sw.Fill)
		// A hairline in a darker shade of the fill gives the tab a crisp
		// edge against light backgrounds.
		c.StrokeRoundRect(tab.Rect, tab.Radius, math.Max(1, math.Round(s)), WithAlpha(sw.Ink, 0.16*tab.Open))

		if tab.Label <= 0 || text == nil {
			continue
		}
		title := tab.Item.Title
		ink := sw.Ink
		if title == "" {
			title = "New note"
			ink = WithAlpha(ink, 0.55)
		}
		pad := labelPadding * s
		text.DrawText(c.Img, pixelRect(tab.Rect.Inflate(-pad, 0)), title, TextStyle{
			Size:    labelSize * s,
			Bold:    tab.Active,
			Color:   ink,
			Opacity: tab.Label,
		})
	}

	drawAdd(c, f)
}

// drawHotZone makes the hover target hit-testable. Windows passes the mouse
// straight through fully transparent pixels of a layered window, so the zone
// is filled with the faintest possible alpha: invisible, but solid to the
// pointer.
func drawHotZone(c *Canvas, zone deck.Rect) {
	r := pixelRect(zone).Intersect(c.Img.Bounds())
	faint := color.RGBA{A: 1}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c.Img.SetRGBA(x, y, faint)
		}
	}
}

func drawPill(c *Canvas, f deck.Frame) {
	if f.PillAlpha <= 0 {
		return
	}
	s := f.Scale
	c.Shadow(f.Pill, f.PillRadius, 4*s, s, 0.18*f.PillAlpha)
	c.FillRoundRect(f.Pill, f.PillRadius, WithAlpha(chromeFill, f.PillAlpha))
	c.StrokeRoundRect(f.Pill, f.PillRadius, math.Max(1, math.Round(s)), WithAlpha(chromeRing, f.PillAlpha))

	if len(f.Tabs) == 0 {
		// An empty deck shows a single neutral dash as an invitation.
		w, h := 6*s, 4*s
		dash := deck.Rect{X: f.Pill.X + (f.Pill.W-w)/2, Y: f.Pill.Y + (f.Pill.H-h)/2, W: w, H: h}
		c.FillRoundRect(dash, h/2, WithAlpha(chromeGlyph, 0.5*f.PillAlpha))
	}
}

func drawAdd(c *Canvas, f deck.Frame) {
	if f.AddAlpha <= 0 {
		return
	}
	s := f.Scale
	radius := addRadius * s
	fill := lerpColor(chromeFill, chromeFillHover, f.AddLift)

	c.Shadow(f.Add, radius, 7*s, 2*s, tabShadow*f.AddAlpha)
	c.FillRoundRect(f.Add, radius, WithAlpha(fill, f.AddAlpha))
	c.StrokeRoundRect(f.Add, radius, math.Max(1, math.Round(s)), WithAlpha(chromeRing, f.AddAlpha))

	cx, cy := f.Add.X+f.Add.W/2, f.Add.Y+f.Add.H/2
	arm := 5.5 * s
	glyph := WithAlpha(chromeGlyph, f.AddAlpha)
	c.Line(cx-arm, cy, cx+arm, cy, 1.6*s, glyph)
	c.Line(cx, cy-arm, cx, cy+arm, 1.6*s, glyph)
}

// pixelRect rounds a floating-point rectangle to whole pixels.
func pixelRect(r deck.Rect) image.Rectangle {
	return image.Rect(
		int(math.Round(r.X)), int(math.Round(r.Y)),
		int(math.Round(r.X+r.W)), int(math.Round(r.Y+r.H)),
	)
}

// lerpColor blends every channel, including alpha, from a to b by t.
func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), f(a.A, b.A)}
}
