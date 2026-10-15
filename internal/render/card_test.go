package render

import (
	"image"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/note"
)

// monoText draws nothing but measures every character as 7 pixels wide, a
// little more for bold.
type monoText struct{ stubText }

func (m *monoText) MeasureText(text string, style TextStyle) float64 {
	w := 7.0
	if style.Bold {
		w = 8
	}
	return float64(utf8.RuneCountInString(text)) * w
}

func TestCardSkipsTheTitleAndWraps(t *testing.T) {
	long := strings.Repeat("word ", 30)
	l := layoutCard("Title\n\n- [ ] milk\n\n\n"+long, 1, &monoText{})
	if len(l.lines) < 3 {
		t.Fatalf("%d lines", len(l.lines))
	}
	if got := l.lines[0].segs[0].text; got != "milk" {
		t.Errorf("first line %q, want the task after the title", got)
	}
	width := deck.CardWidth - cardPadX
	for i, line := range l.lines {
		for _, seg := range line.segs {
			if seg.x+seg.w > width+0.01 {
				t.Errorf("line %d runs to %v, past %v", i, seg.x+seg.w, width)
			}
			if strings.HasPrefix(seg.text, " ") && seg.x == line.x {
				t.Errorf("line %d starts with a space", i)
			}
		}
	}
	// Three blank lines collapse into one gap.
	if gap := l.lines[1].y - l.lines[0].y - l.lines[0].h; gap != cardBlank {
		t.Errorf("gap after the task is %v, want %v", gap, cardBlank)
	}
}

func TestCardIsCutShort(t *testing.T) {
	body := "Title" + strings.Repeat("\nline", 40)
	l := layoutCard(body, 1, &monoText{})
	if !l.more || len(l.lines) != cardMaxRows {
		t.Errorf("more=%v with %d lines", l.more, len(l.lines))
	}
	it := deck.Item{Body: body}
	if h := CardHeight(it, 1, &monoText{}); h > deck.MaxCardHeight {
		t.Errorf("card is %v tall, above the %v cap", h, deck.MaxCardHeight)
	}
}

func TestDrawCard(t *testing.T) {
	text := &monoText{}
	d := deck.New(deck.Right)
	d.SetCardMeasure(func(it deck.Item, s float64) float64 { return CardHeight(it, s, text) })
	d.SetItems([]deck.Item{
		{ID: "a", Title: "Groceries", Meta: "1 of 2 done", Color: note.Mint, Body: "Groceries\n- [ ] oat milk\n- [x] eggs"},
		{ID: "b", Title: "Other", Color: note.Sky},
	})
	d.SetHeld(true)
	for range 100 {
		d.Advance(16 * time.Millisecond)
	}
	r := d.Frame().Tabs[0].Rect
	d.Pointer(int(r.X+r.W/2), int(r.Y+r.H/2), true)
	for range 100 {
		d.Advance(16 * time.Millisecond)
	}
	f := d.Frame()
	if f.Tabs[0].Grow != 1 {
		t.Fatal("the card did not grow")
	}
	c := NewCanvas(1, 1)
	DrawDeck(c, f, text)
	for _, want := range []string{"Groceries", "1 of 2 done", "oat milk", "eggs", "Click to open", "Other"} {
		if !slices.Contains(text.drawn, want) {
			t.Errorf("%q not drawn; drew %q", want, text.drawn)
		}
	}
	// The body is the note's paper colour, below the header in its fill.
	card := f.Tabs[0].Rect
	paper := Palette(note.Mint, false).Paper
	at := image.Pt(int(card.X+card.W-8), int(card.Y+card.H-6))
	if got := c.Img.RGBAAt(at.X, at.Y); got.R != paper.R || got.G != paper.G || got.B != paper.B {
		t.Errorf("card body is %v, want paper %v", got, paper)
	}
}
