package deck

import (
	"testing"
	"time"

	"github.com/MatthewPocock/tuck/internal/note"
)

const frame = 16 * time.Millisecond

func newDeck(edge Edge, n int) *Deck {
	d := New(edge)
	items := make([]Item, n)
	for i := range items {
		items[i] = Item{ID: string(rune('a' + i)), Title: "note", Color: note.Yellow}
	}
	d.SetItems(items)
	return d
}

// run advances the deck by the given duration in frame-sized steps.
func run(d *Deck, total time.Duration) {
	for elapsed := time.Duration(0); elapsed < total; elapsed += frame {
		d.Advance(frame)
	}
}

// center returns the integer midpoint of a rectangle.
func center(r Rect) (int, int) {
	return int(r.X + r.W/2), int(r.Y + r.H/2)
}

// hoverPill puts the pointer on the resting pill and lets the deck expand.
func hoverPill(d *Deck) {
	x, y := center(d.Frame().Pill)
	d.Pointer(x, y, true)
	run(d, time.Second)
}

func TestStartsCollapsedAndIdle(t *testing.T) {
	d := newDeck(Right, 3)
	if !d.Idle() || d.Open() {
		t.Fatal("new deck is not idle and collapsed")
	}
	f := d.Frame()
	if f.PillAlpha != 1 {
		t.Errorf("PillAlpha = %v, want 1 at rest", f.PillAlpha)
	}
	for i, tab := range f.Tabs {
		if tab.Label != 0 {
			t.Errorf("tab %d label visible at rest", i)
		}
		if !contains(f.Pill, tab.Rect) {
			t.Errorf("dash %d %+v lies outside the pill %+v", i, tab.Rect, f.Pill)
		}
	}
	if got := d.HitTest(center(f.Pill)); got.Kind != HitNone {
		t.Errorf("HitTest at rest = %+v, want none", got)
	}
}

func contains(outer, inner Rect) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.W <= outer.X+outer.W && inner.Y+inner.H <= outer.Y+outer.H
}

func TestPointerOnPillExpands(t *testing.T) {
	d := newDeck(Right, 3)
	hoverPill(d)

	if !d.Open() {
		t.Fatal("deck did not open when the pointer reached the pill")
	}
	f := d.Frame()
	if f.PillAlpha != 0 || f.AddAlpha != 1 {
		t.Errorf("PillAlpha = %v, AddAlpha = %v after expanding", f.PillAlpha, f.AddAlpha)
	}
	for i, tab := range f.Tabs {
		if tab.Label != 1 {
			t.Errorf("tab %d label = %v, want 1", i, tab.Label)
		}
		if tab.Rect.H != tabHeight {
			t.Errorf("tab %d height = %v, want %v", i, tab.Rect.H, tabHeight)
		}
		if i > 0 && tab.Rect.Y <= f.Tabs[i-1].Rect.Y {
			t.Errorf("tab %d is not below tab %d", i, i-1)
		}
	}
}

func TestTabsExpandInOrder(t *testing.T) {
	d := newDeck(Right, 4)
	x, y := center(d.Frame().Pill)
	d.Pointer(x, y, true)
	run(d, 100*time.Millisecond)

	tabs := d.Frame().Tabs
	for i := 1; i < len(tabs); i++ {
		if tabs[i].Rect.W >= tabs[i-1].Rect.W {
			t.Errorf("mid-animation tab %d (w=%.1f) is not narrower than tab %d (w=%.1f)",
				i, tabs[i].Rect.W, i-1, tabs[i-1].Rect.W)
		}
	}
}

func TestCollapsesAfterPointerLeaves(t *testing.T) {
	d := newDeck(Right, 3)
	hoverPill(d)

	d.Pointer(-500, -500, true)
	run(d, 200*time.Millisecond)
	if !d.Open() {
		t.Fatal("deck collapsed before the grace period elapsed")
	}
	run(d, 2*time.Second)
	if d.Open() || !d.Idle() {
		t.Fatal("deck did not collapse after the pointer left")
	}
}

func TestBriefExcursionDoesNotCollapse(t *testing.T) {
	d := newDeck(Right, 3)
	hoverPill(d)
	x, y := center(d.Frame().Tabs[1].Rect)

	for range 10 {
		d.Pointer(-500, -500, true)
		run(d, 200*time.Millisecond)
		d.Pointer(x, y, true)
		run(d, 50*time.Millisecond)
	}
	if !d.Open() {
		t.Fatal("repeated short excursions collapsed the deck")
	}
}

func TestHeldDeckStaysOpen(t *testing.T) {
	d := newDeck(Right, 2)
	d.SetHeld(true)
	d.Pointer(0, 0, false)
	run(d, 3*time.Second)
	if !d.Open() {
		t.Fatal("held deck collapsed")
	}

	d.SetHeld(false)
	run(d, 3*time.Second)
	if !d.Idle() {
		t.Fatal("deck did not collapse after being released")
	}
}

func TestPeekOpensThenCollapses(t *testing.T) {
	d := newDeck(Right, 2)
	d.Peek(time.Second)
	d.Pointer(0, 0, false)
	run(d, 900*time.Millisecond)
	if !d.Open() {
		t.Fatal("deck collapsed during the peek")
	}
	run(d, 3*time.Second)
	if !d.Idle() {
		t.Fatal("deck did not collapse after the peek")
	}
}

func TestHitTestAndLift(t *testing.T) {
	d := newDeck(Right, 3)
	hoverPill(d)

	f := d.Frame()
	x, y := center(f.Tabs[1].Rect)
	if got := d.HitTest(x, y); got != (Hit{HitTab, 1}) {
		t.Errorf("HitTest on tab 1 = %+v", got)
	}
	ax, ay := center(f.Add)
	if got := d.HitTest(ax, ay); got.Kind != HitAdd {
		t.Errorf("HitTest on add button = %+v", got)
	}

	d.Pointer(x, y, true)
	run(d, 500*time.Millisecond)
	f = d.Frame()
	if f.Tabs[1].Lift != 1 || f.Tabs[0].Lift != 0 {
		t.Errorf("lift = %v / %v, want hovered tab raised only", f.Tabs[0].Lift, f.Tabs[1].Lift)
	}
	if f.Tabs[1].Rect.W <= f.Tabs[0].Rect.W {
		t.Error("hovered tab is not wider than its neighbours")
	}
}

func TestNoDeadZoneBetweenTabs(t *testing.T) {
	d := newDeck(Right, 3)
	hoverPill(d)
	f := d.Frame()
	x, _ := center(f.Tabs[0].Rect)
	top := int(f.Tabs[0].Rect.Y)
	bottom := int(f.Tabs[2].Rect.Y + f.Tabs[2].Rect.H)
	for y := top; y < bottom; y++ {
		if d.HitTest(x, y).Kind != HitTab {
			t.Fatalf("no tab under y=%d between the first and last tab", y)
		}
	}
}

func TestLeftEdgeMirrorsRightEdge(t *testing.T) {
	right, left := newDeck(Right, 3), newDeck(Left, 3)
	hoverPill(right)
	hoverPill(left)
	if !left.Open() {
		t.Fatal("left-edge deck did not open from its pill")
	}

	rf, lf := right.Frame(), left.Frame()
	w := float64(rf.Width)
	for i := range rf.Tabs {
		r, l := rf.Tabs[i].Rect, lf.Tabs[i].Rect
		if l.X != w-r.X-r.W || l.Y != r.Y || l.W != r.W {
			t.Errorf("tab %d: left %+v is not the mirror of right %+v", i, l, r)
		}
	}
	if lf.HotZone.X != 0 {
		t.Errorf("left hot zone starts at x=%v, want the screen edge", lf.HotZone.X)
	}
	if rf.HotZone.X+rf.HotZone.W != w {
		t.Errorf("right hot zone ends at x=%v, want the screen edge %v", rf.HotZone.X+rf.HotZone.W, w)
	}
}

func TestScaleScalesLayout(t *testing.T) {
	a, b := newDeck(Right, 3), newDeck(Right, 3)
	b.SetScale(2)
	aw, ah := a.Size()
	bw, bh := b.Size()
	if bw != 2*aw || bh != 2*ah {
		t.Errorf("size at 2x = %dx%d, want double %dx%d", bw, bh, aw, ah)
	}
}

func TestOverflowScrolls(t *testing.T) {
	d := newDeck(Right, 30)
	d.SetMaxHeight(400)
	if _, h := d.Size(); h != 400 {
		t.Fatalf("height = %d, want it capped at 400", h)
	}
	d.SetHeld(true)
	run(d, time.Second)

	first := d.Frame().Tabs[0].Rect.Y
	d.Scroll(100)
	if got := d.Frame().Tabs[0].Rect.Y; got != first-100 {
		t.Errorf("after scrolling 100px the first tab moved to %v, want %v", got, first-100)
	}
	d.Scroll(1e9)
	add := d.Frame().Add
	if add.Y+add.H > 400 {
		t.Errorf("add button bottom at %v is cut off after scrolling to the end", add.Y+add.H)
	}
	d.Scroll(-1e9)
	if got := d.Frame().Tabs[0].Rect.Y; got != first {
		t.Errorf("scrolling back to the top left the first tab at %v, want %v", got, first)
	}

	d.SetActive("z") // not present: must not panic or scroll
	d.SetActive(d.Items()[29].ID)
	last := d.Frame().Tabs[29].Rect
	if last.Y < 0 || last.Y+last.H > 400 {
		t.Errorf("active tab %+v was not scrolled into view", last)
	}
}

func TestSetItemsKeepsStateConsistent(t *testing.T) {
	d := newDeck(Right, 5)
	hoverPill(d)
	d.SetItems(d.Items()[:1])
	run(d, time.Second)
	if got := len(d.Frame().Tabs); got != 1 {
		t.Fatalf("frame has %d tabs, want 1", got)
	}
	d.SetItems(nil)
	run(d, time.Second)
	f := d.Frame()
	if len(f.Tabs) != 0 || f.Pill.Empty() {
		t.Errorf("empty deck frame = %+v, want no tabs and a visible pill", f)
	}
}

func TestAnchorsMatchExpandedLayout(t *testing.T) {
	d := newDeck(Right, 3)
	d.SetHeld(true)
	d.SetActive("b")
	run(d, time.Second)

	anchor, ok := d.TabAnchor("b")
	if !ok {
		t.Fatal("TabAnchor did not find the note")
	}
	if got := d.Frame().Tabs[1].Rect; got != anchor {
		t.Errorf("TabAnchor = %+v, want the active tab's rect %+v", anchor, got)
	}
	if _, ok := d.TabAnchor("missing"); ok {
		t.Error("TabAnchor found a note that is not in the deck")
	}
}
