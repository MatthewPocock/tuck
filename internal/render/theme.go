// Package render paints the app's custom-drawn surfaces — the deck, the
// editor header and the app icon — into in-memory images.
//
// Everything here is pure Go with no platform dependencies: the Windows shell
// only has to copy the finished pixels to the screen. That keeps the look of
// the app testable and previewable on any operating system (see
// cmd/tuck-preview).
package render

import (
	"image/color"

	"github.com/MatthewPocock/tuck/internal/note"
)

// Swatch is the set of colours used to draw one note.
type Swatch struct {
	// Fill is the note's signature colour, used for its tab and dash.
	Fill color.NRGBA
	// Ink is the text colour on top of Fill.
	Ink color.NRGBA
	// Paper is the editor background.
	Paper color.NRGBA
	// Text is the editor text colour on top of Paper.
	Text color.NRGBA
	// Muted is used for secondary glyphs on Paper, such as header icons.
	Muted color.NRGBA
	// Border outlines the editor window.
	Border color.NRGBA
}

// Chrome colours shared by every note.
var (
	// chromeFill is the dark translucent surface of the pill and add button.
	chromeFill = color.NRGBA{0x1C, 0x1D, 0x21, 0xD9}
	// chromeFillHover is chromeFill under the pointer.
	chromeFillHover = color.NRGBA{0x34, 0x36, 0x3D, 0xF2}
	chromeRing      = color.NRGBA{0xFF, 0xFF, 0xFF, 0x24}
	chromeGlyph     = color.NRGBA{0xFF, 0xFF, 0xFF, 0xE6}
	shadowColor     = color.NRGBA{0x00, 0x00, 0x00, 0xFF}
)

type tone struct {
	fill, ink, paper, darkPaper color.NRGBA
}

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{uint8(hex >> 16), uint8(hex >> 8), uint8(hex), 0xFF}
}

var tones = map[note.Color]tone{
	note.Yellow: {rgb(0xFFD95A), rgb(0x3D2F00), rgb(0xFFF6CF), rgb(0x2D2A1D)},
	note.Peach:  {rgb(0xFFB688), rgb(0x47240A), rgb(0xFFEBDD), rgb(0x2F261F)},
	note.Pink:   {rgb(0xFF9EBB), rgb(0x4A0F25), rgb(0xFFE6EE), rgb(0x302229)},
	note.Lilac:  {rgb(0xC8B3FF), rgb(0x2A1657), rgb(0xF0EAFF), rgb(0x282437)},
	note.Sky:    {rgb(0x9CCBFF), rgb(0x0B2C52), rgb(0xE4F1FF), rgb(0x1F2935)},
	note.Mint:   {rgb(0x9AE3B3), rgb(0x0C3A1F), rgb(0xE3F8EA), rgb(0x1F2D25)},
	note.Stone:  {rgb(0xD9D6CF), rgb(0x2B2A27), rgb(0xF5F3EF), rgb(0x29292A)},
}

// Palette returns the colours for a note colour in the light or dark theme.
// Unknown colours fall back to the default.
func Palette(c note.Color, dark bool) Swatch {
	t, ok := tones[c]
	if !ok {
		t = tones[note.DefaultColor]
	}
	s := Swatch{Fill: t.fill, Ink: t.ink}
	if dark {
		s.Paper = t.darkPaper
		s.Text = rgb(0xECECEC)
		s.Muted = color.NRGBA{0xFF, 0xFF, 0xFF, 0x99}
		s.Border = mix(t.darkPaper, rgb(0xFFFFFF), 0.14)
	} else {
		s.Paper = t.paper
		s.Text = mix(t.ink, rgb(0x000000), 0.35)
		s.Muted = WithAlpha(t.ink, 0.62)
		s.Border = mix(t.paper, t.ink, 0.18)
	}
	return s
}

// mix blends a toward b by t in [0, 1], ignoring alpha.
func mix(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 0xFF}
}

// WithAlpha scales the alpha of c by opacity in [0, 1].
func WithAlpha(c color.NRGBA, opacity float64) color.NRGBA {
	if opacity < 0 {
		opacity = 0
	} else if opacity > 1 {
		opacity = 1
	}
	c.A = uint8(float64(c.A)*opacity + 0.5)
	return c
}
