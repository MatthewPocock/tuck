package deck

import (
	"math"
	"testing"
	"time"
)

const testCard = 200.0

// cardDeck returns an expanded deck of n notes whose cards are testCard
// pixels tall.
func cardDeck(n int) *Deck {
	d := newDeck(Right, n)
	d.SetCardMeasure(func(Item, float64) float64 { return testCard })
	d.SetHeld(true)
	run(d, time.Second)
	return d
}

// rest puts the pointer on the middle of tab i and waits.
func rest(d *Deck, i int, wait time.Duration) {
	x, y := center(d.Frame().Tabs[i].Rect)
	d.Pointer(x, y, true)
	run(d, wait)
}

func TestRestingOnATabGrowsItsCard(t *testing.T) {
	d := cardDeck(4)
	before := d.Frame().Tabs

	rest(d, 1, 40*time.Millisecond)
	if d.Peeked() != "" {
		t.Fatal("the card opened before the pointer had settled")
	}
	run(d, time.Second)
	if d.Peeked() != "b" {
		t.Fatalf("peeked %q, want b", d.Peeked())
	}
	tabs := d.Frame().Tabs
	if tabs[1].Rect.H != testCard || tabs[1].Rect.W != CardWidth || tabs[1].Grow != 1 {
		t.Errorf("card is %+v, grow %v", tabs[1].Rect, tabs[1].Grow)
	}
	if tabs[0].Rect.Y != before[0].Rect.Y || tabs[1].Rect.Y != before[1].Rect.Y {
		t.Error("the card or the tab above it moved")
	}
	if push := tabs[2].Rect.Y - before[2].Rect.Y; math.Abs(push-(testCard-tabHeight)) > 0.01 {
		t.Errorf("the tab below moved down by %v, want %v", push, testCard-tabHeight)
	}
	if hit := d.HitTest(int(tabs[1].Rect.X+5), int(tabs[1].Rect.Y+testCard-5)); hit != (Hit{HitTab, 1}) {
		t.Errorf("the bottom of the card hits %+v", hit)
	}
}

func TestSweepingAcrossTabsOpensNothing(t *testing.T) {
	d := cardDeck(6)
	// A quick sweep spends a few frames on each tab.
	for i := range 6 {
		rest(d, i, 40*time.Millisecond)
	}
	if d.Peeked() != "" && d.Peeked() != "f" {
		t.Errorf("peeked %q while sweeping", d.Peeked())
	}
	for i := range 5 {
		if d.Frame().Tabs[i].Grow != 0 {
			t.Errorf("tab %d grew", i)
		}
	}
}

func TestMovingToAnotherCardKeepsItUnderThePointer(t *testing.T) {
	d := cardDeck(5)
	rest(d, 0, time.Second)
	if d.Peeked() != "a" {
		t.Fatal("the first card did not open")
	}
	tab := d.Frame().Tabs[3].Rect
	x, y := center(tab)
	d.Pointer(x, y, true)
	for range 100 {
		d.Advance(frame)
		if got := d.Frame().Tabs[3].Rect.Y; math.Abs(got-tab.Y) > 1 {
			t.Fatalf("tab 3 moved from %v to %v under the pointer", tab.Y, got)
		}
	}
	if d.Peeked() != "d" {
		t.Errorf("peeked %q, want d", d.Peeked())
	}
	if g := d.Frame().Tabs[0].Grow; g != 0 {
		t.Errorf("the first card is still grown by %v", g)
	}
}

func TestCardStaysOnScreen(t *testing.T) {
	d := cardDeck(6)
	_, h := d.Size()
	bottom := float64(h) - d.extra()
	d.SetSafeArea(0, int(bottom))
	rest(d, 5, 2*time.Second)
	card := d.Frame().Tabs[5].Rect
	if d.Peeked() != "f" || card.Y+card.H > bottom-margin+0.5 {
		t.Errorf("card %+v reaches below the safe area at %v", card, bottom)
	}
}

func TestClickedCardWaitsForThePointerToLeave(t *testing.T) {
	d := cardDeck(3)
	rest(d, 0, time.Second)
	d.Unpeek()
	run(d, time.Second)
	if d.Peeked() != "" || d.Frame().Tabs[0].Grow != 0 {
		t.Fatal("the card did not shrink back")
	}
	rest(d, 1, 100*time.Millisecond)
	rest(d, 0, time.Second)
	if d.Peeked() != "a" {
		t.Error("the card did not open again after the pointer came back")
	}
}

func TestCardShrinksWhenThePointerLeaves(t *testing.T) {
	d := cardDeck(3)
	rest(d, 0, time.Second)
	d.Pointer(-1000, -1000, true)
	run(d, time.Second)
	if d.Peeked() != "" {
		t.Error("the card stayed open with the pointer far away")
	}
	run(d, time.Second)
	if f := d.Frame(); f.Tabs[0].Rect.H != tabHeight || d.offset != 0 {
		t.Errorf("tab 0 is %v tall with offset %v once settled", f.Tabs[0].Rect.H, d.offset)
	}
}

func TestOpenNoteDoesNotGrowACard(t *testing.T) {
	d := cardDeck(3)
	d.SetActive("b")
	rest(d, 1, time.Second)
	if d.Peeked() != "" {
		t.Error("the note open in the editor grew a card")
	}
}

func TestAnchorIgnoresCards(t *testing.T) {
	d := cardDeck(4)
	want, _ := d.TabAnchor("c")
	rest(d, 0, time.Second)
	if got, _ := d.TabAnchor("c"); got != want {
		t.Errorf("anchor moved from %+v to %+v while a card was open", want, got)
	}
}

func TestInsetKeepsTheDeckInPlace(t *testing.T) {
	plain, cards := newDeck(Right, 3), newDeck(Right, 3)
	cards.SetCardMeasure(func(Item, float64) float64 { return testCard })
	_, ph := plain.Size()
	_, ch := cards.Size()
	if ch != ph+2*cards.Inset() || cards.Inset() == 0 {
		t.Errorf("height %d with cards, %d without, inset %d", ch, ph, cards.Inset())
	}
	inset := float64(cards.Inset())
	if got, want := cards.Frame().Pill.Y-inset, plain.Frame().Pill.Y; got != want {
		t.Errorf("pill at %v within the home area, want %v", got, want)
	}
}
