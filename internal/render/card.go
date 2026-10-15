package render

import (
	"image"
	"math"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/markdown"
)

// Preview card typography and spacing, in device-independent pixels.
const (
	cardPadX      = 16.0
	cardPadTop    = 6.0
	cardPadBottom = 2.0
	cardRow       = 22.0
	cardBlank     = 8.0
	cardTextSize  = 12.5
	cardHeadSize  = 13.5
	cardIndent    = 16.0 // per level of nesting
	cardMarkWidth = 22.0 // room for a task box or bullet before the text
	cardBoxSize   = 13.0
	cardFooter    = 30.0
	cardMaxRows   = 13 // keeps the card within deck.MaxCardHeight
	metaSize      = 11.0
)

// cardLine is one row of text in a preview card. Coordinates are in pixels,
// relative to the card's left edge and the top of its body.
type cardLine struct {
	y, h  float64
	x     float64 // where the text starts
	kind  markdown.BlockKind
	first bool // the first row of its block, which carries the box or bullet
	done  bool
	segs  []cardSeg
}

type cardSeg struct {
	text  string
	bold  bool
	x, w  float64
	muted bool
}

type cardLayout struct {
	lines []cardLine
	// more is set when the note does not fit and was cut short.
	more bool
	// body is the height of the rows.
	body float64
}

// CardHeight returns the full height in pixels of an item's preview card at
// a scale factor, header included. It measures with text if that can
// measure, and estimates otherwise.
func CardHeight(it deck.Item, scale float64, text TextDrawer) float64 {
	l := cardLayoutFor(it.Body, scale, text)
	return (deck.TabHeight+cardPadTop+cardPadBottom+cardFooter)*scale + l.body
}

var cardCache struct {
	sync.Mutex
	m map[cardKey]cardLayout
}

type cardKey struct {
	body  string
	scale float64
	text  TextDrawer
}

// cardLayoutFor lays out a note's preview, reusing the previous result for
// the same note: cards are drawn every frame while they animate.
func cardLayoutFor(body string, scale float64, text TextDrawer) cardLayout {
	key := cardKey{body, scale, text}
	cardCache.Lock()
	defer cardCache.Unlock()
	if l, ok := cardCache.m[key]; ok {
		return l
	}
	if cardCache.m == nil || len(cardCache.m) >= 128 {
		cardCache.m = map[cardKey]cardLayout{}
	}
	m, _ := text.(TextMeasurer)
	l := layoutCard(body, scale, m)
	cardCache.m[key] = l
	return l
}

// previewBlocks returns what a card shows of a note: everything after its
// title line, with runs of blank lines squeezed to one.
func previewBlocks(body string) []markdown.Block {
	var out []markdown.Block
	titled := false
	for _, b := range markdown.Blocks(body) {
		if b.Kind == markdown.Blank {
			if len(out) > 0 && out[len(out)-1].Kind != markdown.Blank {
				out = append(out, b)
			}
			continue
		}
		if !titled {
			titled = true
			continue
		}
		out = append(out, b)
	}
	for len(out) > 0 && out[len(out)-1].Kind == markdown.Blank {
		out = out[:len(out)-1]
	}
	return out
}

// layoutCard wraps a note's preview into rows that fit the card.
func layoutCard(body string, s float64, m TextMeasurer) cardLayout {
	var l cardLayout
	y := 0.0
	rows := 0
	for _, b := range previewBlocks(body) {
		if b.Kind == markdown.Blank {
			y += cardBlank * s
			continue
		}
		size, bold := cardTextSize, false
		if b.Kind == markdown.Heading {
			size, bold = cardHeadSize, true
		}
		x := (cardPadX + float64(b.Level)*cardIndent) * s
		switch b.Kind {
		case markdown.Task, markdown.Bullet:
			x += cardMarkWidth * s
		case markdown.Quote:
			x += 12 * s
		}
		width := deck.CardWidth*s - cardPadX*s - x
		wrapped := wrap(b.Runs, width, size*s, bold, m)
		for i, segs := range wrapped {
			if rows == cardMaxRows {
				l.more = true
				l.body = y
				return l
			}
			for k := range segs {
				segs[k].x += x
				segs[k].muted = b.Done
			}
			l.lines = append(l.lines, cardLine{
				y: y, h: cardRow * s, x: x, kind: b.Kind, first: i == 0, done: b.Done, segs: segs,
			})
			y += cardRow * s
			rows++
		}
	}
	l.body = y
	return l
}

// wrap breaks styled runs into rows no wider than width, at spaces. Segment
// x positions are relative to the start of the row.
func wrap(runs []markdown.Run, width, size float64, bold bool, m TextMeasurer) [][]cardSeg {
	measure := func(text string, b bool) float64 {
		if m != nil {
			return m.MeasureText(text, TextStyle{Size: size, Bold: b})
		}
		return float64(utf8.RuneCountInString(text)) * size * 0.52
	}
	var rows [][]cardSeg
	var row []cardSeg
	x := 0.0
	add := func(text string, b bool) {
		if n := len(row); n > 0 && row[n-1].bold == b {
			row[n-1].text += text
			row[n-1].w = measure(row[n-1].text, b)
			x = row[n-1].x + row[n-1].w
			return
		}
		w := measure(text, b)
		row = append(row, cardSeg{text: text, bold: b, x: x, w: w})
		x += w
	}
	for _, r := range runs {
		b := bold || r.Bold
		for _, word := range words(r.Text) {
			w := measure(word, b)
			if x > 0 && x+w > width && strings.TrimSpace(word) != "" {
				rows = append(rows, row)
				row, x = nil, 0
			}
			if x == 0 {
				word = strings.TrimLeft(word, " ")
				if word == "" {
					continue
				}
			}
			add(word, b)
		}
	}
	if len(row) > 0 || len(rows) == 0 {
		rows = append(rows, row)
	}
	return rows
}

// words splits text after each run of spaces, keeping the spaces with the
// word before them.
func words(text string) []string {
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == ' ' && (i+1 == len(text) || text[i+1] != ' ') {
			out = append(out, text[start:i+1])
			start = i + 1
		}
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	return out
}

// drawCard paints a tab that is growing into a preview card: the tab's
// colour as a header and the note's paper below it, with the note's text.
func drawCard(c *Canvas, tab deck.Tab, s float64, dark bool, text TextDrawer) {
	r := tab.Rect
	tabSw := Palette(tab.Item.Color, false)
	sw := Palette(tab.Item.Color, dark)
	radius := tab.Radius

	c.FillRoundRect(r, radius, sw.Paper)
	head := r
	head.H = min(tab.Header+radius, r.H)
	c.FillRoundRect(head, radius, tabSw.Fill)
	if r.H > tab.Header {
		c.FillRect(deck.Rect{X: r.X, Y: r.Y + tab.Header, W: r.W, H: min(radius, r.H-tab.Header)}, sw.Paper)
	}

	op := clampUnit((tab.Grow - 0.35) / 0.65)
	if op <= 0 || text == nil {
		return
	}
	bodyTop := r.Y + tab.Header + cardPadTop*s
	clip := pixelRect(deck.Rect{X: r.X, Y: r.Y + tab.Header, W: r.W, H: r.H - tab.Header})
	dst, ok := c.Img.SubImage(clip.Intersect(c.Img.Bounds())).(*image.RGBA)
	if !ok {
		return
	}
	inside := func(y, h float64) bool { return y+h <= float64(clip.Max.Y) }

	ink := WithAlpha(sw.Text, op)
	muted := WithAlpha(sw.Text, 0.5*op)
	l := cardLayoutFor(tab.Item.Body, s, text)
	for _, line := range l.lines {
		y := bodyTop + line.y
		if !inside(y, line.h) {
			break
		}
		size := cardTextSize * s
		if line.kind == markdown.Heading {
			size = cardHeadSize * s
		}
		col := ink
		if line.done {
			col = muted
		}
		if line.first {
			drawMark(c, line, r.X, y, s, op, tabSw, sw)
		}
		if line.kind == markdown.Quote {
			c.FillRect(deck.Rect{X: r.X + line.x - 12*s, Y: y + 3*s, W: 2 * s, H: line.h - 6*s}, WithAlpha(sw.Text, 0.25*op))
		}
		for _, seg := range line.segs {
			box := deck.Rect{X: r.X + seg.x, Y: y, W: seg.w + 3*s, H: line.h}
			text.DrawText(dst, pixelRect(box), seg.text, TextStyle{Size: size, Bold: seg.bold, Color: col, Opacity: 1})
			if line.done {
				mid := y + line.h/2 + 0.5*s
				c.Line(r.X+seg.x, mid, r.X+seg.x+seg.w, mid, math.Max(1, s), WithAlpha(sw.Text, 0.45*op))
			}
		}
	}

	// A footer says what clicking does, and that there is more to read.
	fy := bodyTop + l.body + cardPadBottom*s
	if !inside(fy, cardFooter*s) {
		return
	}
	c.FillRect(deck.Rect{X: r.X + cardPadX*s, Y: fy, W: r.W - 2*cardPadX*s, H: math.Max(1, math.Round(s/2))}, WithAlpha(sw.Text, 0.12*op))
	hint := "Click to open"
	if l.more {
		hint = "Click to read more"
	}
	if len(l.lines) == 0 && strings.TrimSpace(tab.Item.Body) == "" {
		hint = "Click to write"
	}
	text.DrawText(dst, pixelRect(deck.Rect{X: r.X + cardPadX*s, Y: fy, W: r.W - 2*cardPadX*s, H: cardFooter * s}),
		hint, TextStyle{Size: metaSize * s, Color: muted, Opacity: 1})
}

// drawMark draws the task box or bullet at the start of a line.
func drawMark(c *Canvas, line cardLine, left, y, s, op float64, tabSw, sw Swatch) {
	cy := y + line.h/2
	switch line.kind {
	case markdown.Task:
		b := cardBoxSize * s
		box := deck.Rect{X: left + line.x - cardMarkWidth*s, Y: cy - b/2, W: b, H: b}
		if line.done {
			c.FillRoundRect(box, 3.5*s, WithAlpha(tabSw.Fill, op))
			x, yy := box.X, box.Y
			w := math.Max(1.2, 1.6*s)
			c.Line(x+3.2*s, yy+6.8*s, x+5.6*s, yy+9.2*s, w, WithAlpha(tabSw.Ink, op))
			c.Line(x+5.6*s, yy+9.2*s, x+10*s, yy+4.2*s, w, WithAlpha(tabSw.Ink, op))
			return
		}
		c.StrokeRoundRect(box, 3.5*s, math.Max(1, 1.3*s), WithAlpha(sw.Text, 0.45*op))
	case markdown.Bullet:
		c.FillCircle(left+line.x-cardMarkWidth*s+6*s, cy, 2.2*s, WithAlpha(sw.Text, 0.55*op))
	}
}

func clampUnit(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
