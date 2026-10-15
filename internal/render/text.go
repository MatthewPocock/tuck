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
}

// TextDrawer draws a single line of text. Implementations left-align the text
// within r, centre it vertically, and truncate it with an ellipsis if it does
// not fit.
//
// The Windows shell provides an implementation backed by the system text
// renderer, which handles font fallback for every script. Package fonttext
// has a portable implementation used by tests and previews.
type TextDrawer interface {
	DrawText(dst *image.RGBA, r image.Rectangle, text string, style TextStyle)
}
