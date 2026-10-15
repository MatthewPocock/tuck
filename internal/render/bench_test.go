package render

import (
	"testing"
	"time"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/note"
)

// BenchmarkDeckFrame draws one frame of a deck of ten notes at 125%
// scaling with a card half-way through growing, the busiest moment.
func BenchmarkDeckFrame(b *testing.B) {
	text := &monoText{}
	d := deck.New(deck.Right)
	d.SetScale(1.25)
	d.SetCardMeasure(func(it deck.Item, s float64) float64 { return CardHeight(it, s, text) })
	var items []deck.Item
	for i := range 10 {
		items = append(items, deck.Item{ID: string(rune('a' + i)), Title: "Groceries for the weekend", Meta: "2 of 5 done · 3h ago",
			Color: note.Colors[i%len(note.Colors)], Body: "Title\n- [ ] oat milk\n- [ ] sourdough\n- [x] eggs\n\nRemember the **bags**."})
	}
	d.SetItems(items)
	d.SetHeld(true)
	for range 100 {
		d.Advance(16 * time.Millisecond)
	}
	r := d.Frame().Tabs[3].Rect
	d.Pointer(int(r.X+r.W/2), int(r.Y+r.H/2), true)
	for d.Peeked() == "" {
		d.Advance(8 * time.Millisecond)
	}
	d.Advance(60 * time.Millisecond)
	f := d.Frame()
	c := NewCanvas(1, 1)
	b.Logf("frame %dx%d", f.Width, f.Height)
	b.ResetTimer()
	for range b.N {
		DrawDeck(c, f, text)
	}
}
