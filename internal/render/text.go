package render

import (
	"image"
	"image/color"
)

// TextStyle describes how a run of text is drawn.
type TextStyle struct {
	// Size is the font size in pixels.
	Size float64
	// Bold selects a heavier weight.
	Bold bool
	// Color is the text colour; its alpha is multiplied by Opacity.
	Color color.NRGBA
	// Opacity fades the text, from 0 (invisible) to 1.
	Opacity float64
	// Align places the text horizontally within its rectangle.
	Align Align
	// Path shortens text that does not fit in the middle rather than at the
	// end, keeping the last folder of a path visible.
	Path bool
}

// Align is the horizontal alignment of text.
type Align int

// The supported alignments.
const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// TextDrawer draws a single line of text. Implementations align the text
// within r as the style asks, centre it vertically, and truncate it with an ellipsis if it does
// not fit.
//
// The Windows shell provides an implementation backed by the system text
// renderer, which handles font fallback for every script. Package fonttext
// has a portable implementation used by tests and previews.
type TextDrawer interface {
	DrawText(dst *image.RGBA, r image.Rectangle, text string, style TextStyle)
}

// TextMeasurer is implemented by text drawers that can tell how wide a
// string is drawn, in pixels. It is used to highlight words within a line.
type TextMeasurer interface {
	MeasureText(text string, style TextStyle) float64
}
