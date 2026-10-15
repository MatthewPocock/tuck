package render

import (
	"fmt"
	"image/color"
	"math"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/search"
)

// SearchState is what the search window shows.
type SearchState struct {
	Query   string
	Results []search.Result
	// Selected is the result Enter opens and Hover the one under the
	// pointer; either may be -1.
	Selected, Hover int
}

// Search window typography in device-independent pixels.
const (
	searchTitleSize   = 14.0
	searchSnippetSize = 12.5
	searchHintSize    = 12.0
)

// DrawSearch paints the search window onto the canvas, which is resized to
// the layout. The text box itself is a native control placed over
// l.Input.
func DrawSearch(c *Canvas, l search.Layout, s SearchState, dark bool, text TextDrawer) {
	c.Resize(l.Width, l.Height)
	p := chromeFor(dark)
	c.Fill(p.bg)
	sc := l.Scale
	hairline := math.Max(1, math.Round(sc))
	highlight := WithAlpha(p.accent, 0.55)
	if dark {
		highlight = WithAlpha(p.accent, 0.32)
	}
	draw := func(r deck.Rect, str string, size float64, bold bool, col color.NRGBA, align Align) {
		if text != nil {
			text.DrawText(c.Img, pixelRect(r), str, TextStyle{Size: size * sc, Bold: bold, Color: col, Opacity: 1, Align: align})
		}
	}

	// A magnifying glass in front of the query.
	cx, cy := l.Field.X+26*sc, l.Field.Y+l.Field.H/2
	c.StrokeCircle(cx-1.5*sc, cy-1.5*sc, 6*sc, 1.6*sc, p.muted)
	c.Line(cx+3*sc, cy+3*sc, cx+7.5*sc, cy+7.5*sc, 1.8*sc, p.muted)
	c.FillRect(deck.Rect{Y: l.Field.H - hairline, W: float64(l.Width), H: hairline}, p.line)

	for i, r := range l.Rows {
		n := l.First + i
		res := s.Results[n]
		switch {
		case n == s.Selected:
			c.FillRoundRect(r, controlRadius*sc, p.controlHover)
		case n == s.Hover:
			c.FillRoundRect(r, controlRadius*sc, p.control)
		}
		sw := Palette(res.Note.Color, false)
		c.FillCircle(r.X+18*sc, r.Y+r.H/2, 5*sc, sw.Fill)
		c.StrokeCircle(r.X+18*sc, r.Y+r.H/2, 5*sc, sc, WithAlpha(sw.Ink, 0.25))

		left := r.X + 36*sc
		right := r.X + r.W - 14*sc
		if res.Note.Archived {
			tag := deck.Rect{X: right - 64*sc, Y: r.Y, W: 64 * sc, H: r.H}
			draw(tag, "Archived", searchHintSize, false, p.muted, AlignRight)
			right = tag.X - 8*sc
		}
		title := res.Title
		titleCol := p.text
		if title == "" {
			title, titleCol = "Untitled", p.muted
		}
		hasSnippet := res.Snippet != ""
		titleRect := deck.Rect{X: left, Y: r.Y + 7*sc, W: right - left, H: 22 * sc}
		if !hasSnippet {
			titleRect = deck.Rect{X: left, Y: r.Y, W: right - left, H: r.H}
		}
		titleStyle := TextStyle{Size: searchTitleSize * sc, Bold: true, Color: titleCol, Opacity: 1}
		marks(c, text, titleRect, title, titleStyle, res.TitleMatches, highlight, sc)
		draw(titleRect, title, searchTitleSize, true, titleCol, AlignLeft)
		if hasSnippet {
			snippetRect := deck.Rect{X: left, Y: r.Y + 28*sc, W: right - left, H: 19 * sc}
			style := TextStyle{Size: searchSnippetSize * sc, Color: p.muted, Opacity: 1}
			marks(c, text, snippetRect, res.Snippet, style, res.SnippetMatches, highlight, sc)
			draw(snippetRect, res.Snippet, searchSnippetSize, false, p.muted, AlignLeft)
		}
	}

	if l.Message.H > 0 {
		msg := "No notes yet. Press Ctrl+N to start one."
		if s.Query != "" {
			msg = fmt.Sprintf("No notes contain “%s”", s.Query)
		}
		draw(l.Message, msg, settingsTextSize, false, p.muted, AlignCenter)
	}

	c.FillRect(deck.Rect{Y: l.Footer.Y, W: float64(l.Width), H: hairline}, p.line)
	hints := l.Footer.Inflate(-18*sc, 0)
	draw(hints, "↑↓ to choose   ·   Enter to open   ·   Esc to close", searchHintSize, false, p.muted, AlignLeft)
	if n := len(s.Results); n > 0 {
		count := "1 note"
		if n != 1 {
			count = fmt.Sprintf("%d notes", n)
		}
		draw(hints, count, searchHintSize, false, p.muted, AlignRight)
	}
}

// marks highlights the byte ranges ms of str, as it will be drawn in r,
// when the text drawer can measure text.
func marks(c *Canvas, text TextDrawer, r deck.Rect, str string, style TextStyle, ms [][2]int, col color.NRGBA, sc float64) {
	m, ok := text.(TextMeasurer)
	if !ok || len(ms) == 0 {
		return
	}
	h := style.Size * 1.3
	y := r.Y + (r.H-h)/2
	for _, span := range ms {
		x0 := m.MeasureText(str[:span[0]], style)
		x1 := m.MeasureText(str[:span[1]], style)
		if x0 >= r.W {
			break
		}
		x1 = math.Min(x1, r.W)
		c.FillRoundRect(deck.Rect{X: r.X + x0 - sc, Y: y, W: x1 - x0 + 2*sc, H: h}, 3*sc, col)
	}
}

// SearchColors returns the background and text colours of the search
// window, for the native text box placed on it.
func SearchColors(dark bool) (bg, text color.NRGBA) {
	p := chromeFor(dark)
	return p.bg, p.text
}
