package render

import (
	"image/color"

	"github.com/MatthewPocock/tuck/internal/deck"
)

// HeaderHeight is the height of the editor's custom title bar in
// device-independent pixels.
const HeaderHeight = 42

// HeaderButton identifies a control in the editor header.
type HeaderButton int

// The header controls, from left to right.
const (
	ButtonNone HeaderButton = iota
	// ButtonNew creates another note.
	ButtonNew
	// ButtonMenu opens the note's menu.
	ButtonMenu
	// ButtonPin keeps the editor open when it loses focus.
	ButtonPin
	// ButtonClose closes the editor.
	ButtonClose
)

// Header button metrics in device-independent pixels.
const (
	buttonSize   = 28.0
	buttonGap    = 2.0
	buttonMargin = 6.0
	buttonRadius = 6.0
)

// HeaderLayout positions the header's controls for a given window width. The
// same layout drives both painting and hit testing so they cannot disagree.
type HeaderLayout struct {
	// Width is the header width in pixels.
	Width int
	// Scale is the display scale factor.
	Scale float64
}

// Height returns the header height in pixels.
func (l HeaderLayout) Height() int {
	return int(HeaderHeight*l.Scale + 0.5)
}

// ButtonRect returns the rectangle of a control in pixels.
func (l HeaderLayout) ButtonRect(b HeaderButton) deck.Rect {
	s := l.Scale
	size := buttonSize * s
	y := (HeaderHeight*s - size) / 2
	fromRight := func(slot float64) deck.Rect {
		x := float64(l.Width) - buttonMargin*s - size - slot*(size+buttonGap*s)
		return deck.Rect{X: x, Y: y, W: size, H: size}
	}
	switch b {
	case ButtonClose:
		return fromRight(0)
	case ButtonPin:
		return fromRight(1)
	case ButtonMenu:
		return fromRight(2)
	case ButtonNew:
		return fromRight(3)
	default:
		return deck.Rect{}
	}
}

// headerButtons lists every control for iteration.
var headerButtons = []HeaderButton{ButtonNew, ButtonMenu, ButtonPin, ButtonClose}

// Hit returns the control under the point (x, y), or ButtonNone.
func (l HeaderLayout) Hit(x, y int) HeaderButton {
	for _, b := range headerButtons {
		if l.ButtonRect(b).Contains(float64(x), float64(y)) {
			return b
		}
	}
	return ButtonNone
}

// HeaderState is the interactive state of the header.
type HeaderState struct {
	// Hover is the control under the pointer.
	Hover HeaderButton
	// Pressed is the control being clicked.
	Pressed HeaderButton
	// Pinned reports whether the editor is pinned open.
	Pinned bool
	// Title is the note's title, shown on the left. Placeholder shows it
	// faded, for a note with no title yet.
	Title       string
	Placeholder bool
}

// DrawHeader paints the editor header onto the canvas, which is resized to
// fit and filled with the note's signature colour, the colour of its tab in
// the deck.
func DrawHeader(c *Canvas, l HeaderLayout, sw Swatch, state HeaderState, text TextDrawer) {
	c.Resize(l.Width, l.Height())
	c.Fill(sw.Fill)
	s := l.Scale
	glyph := WithAlpha(sw.Ink, 0.72)

	if state.Title != "" && text != nil {
		right := l.ButtonRect(ButtonNew)
		r := deck.Rect{X: headerPadLeft * s, Y: 0, W: right.X - headerPadLeft*s - 8*s, H: HeaderHeight * s}
		ink := sw.Ink
		if state.Placeholder {
			ink = WithAlpha(ink, 0.55)
		}
		text.DrawText(c.Img, pixelRect(r), state.Title, TextStyle{Size: 14 * s, Bold: true, Color: ink, Opacity: 1})
	}

	for _, b := range headerButtons {
		r := l.ButtonRect(b)
		switch {
		case state.Pressed == b:
			c.FillRoundRect(r, buttonRadius*s, WithAlpha(sw.Ink, 0.18))
		case state.Hover == b:
			c.FillRoundRect(r, buttonRadius*s, WithAlpha(sw.Ink, 0.10))
		}

		cx, cy := r.X+r.W/2, r.Y+r.H/2
		stroke := 1.5 * s
		switch b {
		case ButtonNew:
			arm := 5 * s
			c.Line(cx-arm, cy, cx+arm, cy, stroke, glyph)
			c.Line(cx, cy-arm, cx, cy+arm, stroke, glyph)
		case ButtonMenu:
			for _, dx := range []float64{-5, 0, 5} {
				c.FillCircle(cx+dx*s, cy, 1.4*s, glyph)
			}
		case ButtonPin:
			col := glyph
			if state.Pinned {
				col = sw.Ink
			}
			drawPin(c, cx, cy, s, col, state.Pinned)
		case ButtonClose:
			arm := 4.5 * s
			c.Line(cx-arm, cy-arm, cx+arm, cy+arm, stroke, glyph)
			c.Line(cx-arm, cy+arm, cx+arm, cy-arm, stroke, glyph)
		}
	}
}

// headerPadLeft lines the title up with the text of the note below it.
const headerPadLeft = 18.0

// drawPin draws a pushpin: tilted and outlined when the editor is not
// pinned, upright and solid when it is.
func drawPin(c *Canvas, cx, cy, s float64, col color.NRGBA, pinned bool) {
	if pinned {
		c.Line(cx-3.5*s, cy-5.5*s, cx+3.5*s, cy-5.5*s, 2*s, col) // cap
		c.Line(cx, cy-5*s, cx, cy-0.5*s, 4.2*s, col)             // body
		c.Line(cx-4.5*s, cy, cx+4.5*s, cy, 2*s, col)             // collar
		c.Line(cx, cy+1*s, cx, cy+6.5*s, 1.5*s, col)             // needle
		return
	}
	// The same pin rotated 45° clockwise around its centre.
	const d = 0.70710678
	rot := func(x, y float64) (float64, float64) {
		return cx + (x*d-y*d)*s, cy + (x*d+y*d)*s
	}
	seg := func(x1, y1, x2, y2, w float64) {
		ax, ay := rot(x1, y1)
		bx, by := rot(x2, y2)
		c.Line(ax, ay, bx, by, w*s, col)
	}
	seg(-3.5, -5.5, 3.5, -5.5, 1.5)
	seg(-2, -5.5, -2, 0, 1.5)
	seg(2, -5.5, 2, 0, 1.5)
	seg(-4.5, 0, 4.5, 0, 1.5)
	seg(0, 0, 0, 6.5, 1.5)
}
