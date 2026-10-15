package render

import (
	"github.com/MatthewPocock/tuck/internal/deck"
)

// HeaderHeight is the height of the editor's custom title bar in
// device-independent pixels.
const HeaderHeight = 40

// HeaderButton identifies a control in the editor header.
type HeaderButton int

// The header controls, from left to right.
const (
	ButtonNone HeaderButton = iota
	// ButtonColor is the colour dot; clicking it cycles the note's colour.
	ButtonColor
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
	case ButtonColor:
		return deck.Rect{X: buttonMargin * s, Y: y, W: size, H: size}
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
var headerButtons = []HeaderButton{ButtonColor, ButtonNew, ButtonMenu, ButtonPin, ButtonClose}

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
}

// DrawHeader paints the editor header onto the canvas, which is resized to
// fit and filled with the note's paper colour.
func DrawHeader(c *Canvas, l HeaderLayout, sw Swatch, state HeaderState) {
	c.Resize(l.Width, l.Height())
	c.Fill(sw.Paper)
	s := l.Scale

	for _, b := range headerButtons {
		r := l.ButtonRect(b)
		switch {
		case state.Pressed == b:
			c.FillRoundRect(r, buttonRadius*s, WithAlpha(sw.Text, 0.16))
		case state.Hover == b:
			c.FillRoundRect(r, buttonRadius*s, WithAlpha(sw.Text, 0.09))
		}

		cx, cy := r.X+r.W/2, r.Y+r.H/2
		stroke := 1.5 * s
		switch b {
		case ButtonColor:
			c.FillCircle(cx, cy, 6.5*s, sw.Fill)
			c.StrokeCircle(cx, cy, 6.5*s, s, WithAlpha(sw.Text, 0.28))
		case ButtonNew:
			arm := 5 * s
			c.Line(cx-arm, cy, cx+arm, cy, stroke, sw.Muted)
			c.Line(cx, cy-arm, cx, cy+arm, stroke, sw.Muted)
		case ButtonMenu:
			for _, dx := range []float64{-5, 0, 5} {
				c.FillCircle(cx+dx*s, cy, 1.4*s, sw.Muted)
			}
		case ButtonPin:
			drawPin(c, cx, cy, s, sw, state.Pinned)
		case ButtonClose:
			arm := 4.5 * s
			c.Line(cx-arm, cy-arm, cx+arm, cy+arm, stroke, sw.Muted)
			c.Line(cx-arm, cy+arm, cx+arm, cy-arm, stroke, sw.Muted)
		}
	}
}

// drawPin draws a pushpin: tilted and outlined when the editor is not
// pinned, upright and solid when it is.
func drawPin(c *Canvas, cx, cy, s float64, sw Swatch, pinned bool) {
	if pinned {
		col := sw.Text
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
	col := sw.Muted
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
