// Package deck models the note deck that lives at the edge of the screen.
//
// At rest the deck is a thin pill showing one coloured dash per note. When
// the pointer reaches it, each dash grows into a labelled tab, one after the
// other, and shrinks back once the pointer leaves.
//
// The package is pure geometry and timing: it knows nothing about windows or
// drawing. The host feeds it pointer positions and elapsed time and paints
// the Frame it produces, which keeps all of the interaction logic portable
// and unit-testable.
package deck

import (
	"time"

	"github.com/MatthewPocock/tuck/internal/note"
)

// Edge is the side of the screen the deck is attached to.
type Edge int

// The supported edges.
const (
	Right Edge = iota
	Left
)

// Layout metrics in device-independent pixels; they are multiplied by the
// display scale factor before use.
const (
	tabWidth   = 208.0 // width of an expanded tab
	tabHeight  = 36.0
	tabGap     = 6.0  // vertical space between tabs
	tabRadius  = 10.0 // corner radius of an expanded tab
	tabLift    = 10.0 // extra width of a hovered or active tab
	edgeGap    = 8.0  // space between expanded tabs and the screen edge
	margin     = 16.0 // transparent border that leaves room for shadows
	addSize    = 36.0 // the square "new note" button below the tabs
	pillWidth  = 12.0
	pillInset  = 3.0 // space between the pill and the screen edge
	pillPadY   = 7.0 // padding above and below the dashes
	dashWidth  = 6.0
	dashHeight = 4.0
	dashGap    = 4.0
	hotPad     = 6.0 // how far the hover target extends beyond the pill
	hoverSlack = 12.0
)

// Animation timings.
const (
	tabDuration   = 0.22 // seconds for one tab to fully expand
	maxStagger    = 0.035
	maxStaggerSum = 0.25 // cap on the total delay so large decks stay snappy
	collapseSpeed = 1.6  // collapsing runs faster than expanding
	liftDuration  = 0.10
	// collapseGrace is how long the pointer may stray from the deck before
	// it collapses, forgiving small overshoots.
	collapseGrace = 0.35
)

// Item is one note as shown in the deck.
type Item struct {
	ID    string
	Title string
	Color note.Color
}

// HitKind identifies what part of the deck a point falls on.
type HitKind int

// The possible hit targets.
const (
	HitNone HitKind = iota
	HitTab
	HitAdd
)

// Hit is the result of a hit test. Index is the tab index for HitTab.
type Hit struct {
	Kind  HitKind
	Index int
}

// Tab is the drawable state of one note tab.
type Tab struct {
	Item   Item
	Rect   Rect
	Radius float64
	// Open is how far the tab has expanded from a dash, from 0 to 1.
	Open float64
	// Label is the opacity of the title text, from 0 to 1.
	Label float64
	// Lift is how far the tab is raised toward the pointer, from 0 to 1.
	Lift float64
	// Active marks the tab whose note is open in the editor.
	Active bool
}

// Frame is everything needed to paint the deck at one instant.
type Frame struct {
	// Width and Height are the size of the deck window in pixels.
	Width, Height int
	// Scale is the display scale factor (1 at 96 DPI).
	Scale float64
	Edge  Edge

	// Pill is the resting container behind the dashes.
	Pill       Rect
	PillRadius float64
	PillAlpha  float64

	// HotZone is the area that should accept the pointer even though
	// nothing visible is drawn there, so that slamming the pointer into the
	// screen edge reliably opens the deck.
	HotZone Rect

	Tabs []Tab

	// Add is the "new note" button.
	Add      Rect
	AddAlpha float64
	AddLift  float64
}

// Deck is the interaction state of the deck. The zero value is not usable;
// call New.
type Deck struct {
	items     []Item
	edge      Edge
	scale     float64
	maxHeight int
	active    string

	// open is the state the deck is animating toward.
	open bool
	// held keeps the deck open regardless of the pointer, e.g. while a note
	// is being edited or a menu is showing.
	held bool
	// clock is the position on the expansion timeline in seconds: 0 is
	// fully collapsed and timelineEnd is fully expanded.
	clock float64
	// lift holds the hover animation state per tab, plus one trailing entry
	// for the add button.
	lift []float64
	// away is how long the pointer has been off the deck, in seconds. A
	// negative value delays the collapse (see Peek).
	away float64
	// scroll is how far an overflowing deck is scrolled, in pixels.
	scroll float64

	pointerX, pointerY float64
	pointerIn          bool
}

// New returns a collapsed, empty deck on the given edge.
func New(edge Edge) *Deck {
	return &Deck{edge: edge, scale: 1, lift: make([]float64, 1)}
}

// SetItems replaces the notes shown in the deck.
func (d *Deck) SetItems(items []Item) {
	d.items = append(d.items[:0], items...)
	// Carry over the hover state of surviving tabs; the add button's state
	// lives in the last slot.
	lift := make([]float64, len(items)+1)
	copy(lift[:len(items)], d.lift[:len(d.lift)-1])
	lift[len(items)] = d.lift[len(d.lift)-1]
	d.lift = lift
	d.clock = min(d.clock, d.timelineEnd())
	d.clampScroll()
}

// Items returns the notes currently shown in the deck.
func (d *Deck) Items() []Item { return d.items }

// SetEdge moves the deck to another screen edge.
func (d *Deck) SetEdge(e Edge) { d.edge = e }

// SetScale sets the display scale factor (DPI / 96).
func (d *Deck) SetScale(s float64) {
	if s <= 0 {
		s = 1
	}
	d.scroll *= s / d.scale
	d.scale = s
	d.clampScroll()
}

// Scale returns the display scale factor.
func (d *Deck) Scale() float64 { return d.scale }

// SetMaxHeight limits the deck's height in pixels, typically to the height of
// the monitor's work area. Zero removes the limit. A deck with more tabs than
// fit can be scrolled.
func (d *Deck) SetMaxHeight(px int) {
	d.maxHeight = px
	d.clampScroll()
}

// SetActive marks the note that is open in the editor, or none if id is
// empty, and scrolls it into view.
func (d *Deck) SetActive(id string) {
	d.active = id
	for i, it := range d.items {
		if it.ID == id {
			d.reveal(i)
			return
		}
	}
}

// SetHeld keeps the deck expanded regardless of where the pointer is. It is
// used while a note is being edited or a context menu is open.
func (d *Deck) SetHeld(held bool) {
	d.held = held
	if held {
		d.open = true
	}
	d.away = 0
}

// Peek expands the deck briefly without the pointer being near it, to show
// the user where it is.
func (d *Deck) Peek(hold time.Duration) {
	d.open = true
	d.away = -hold.Seconds()
}

// Pointer reports the pointer position in window pixels. present is false
// when the position is unknown, which the deck treats as "far away".
func (d *Deck) Pointer(x, y int, present bool) {
	d.pointerX, d.pointerY, d.pointerIn = float64(x), float64(y), present
}

// Scroll moves an overflowing deck by delta pixels; positive values reveal
// tabs further down.
func (d *Deck) Scroll(delta float64) {
	d.scroll += delta
	d.clampScroll()
}

// Size returns the size of the deck window in pixels. It depends on the
// number of notes, the scale and the maximum height, but not on the
// animation state, so the window only needs to be resized when those change.
func (d *Deck) Size() (width, height int) {
	s := d.scale
	w := (margin + tabLift + tabWidth + edgeGap) * s
	h := max(d.contentHeight(), d.restHeight()+2*pillPadY*s) + 2*margin*s
	if d.maxHeight > 0 {
		h = min(h, float64(d.maxHeight))
	}
	return int(w + 0.5), int(h + 0.5)
}

// Advance moves the animation forward by dt and reports whether anything
// visible changed, in which case the host should repaint.
func (d *Deck) Advance(dt time.Duration) bool {
	sec := clamp(dt.Seconds(), 0, 0.1)
	changed := false

	wasOpen := d.open
	d.updateOpen(sec)
	if d.open != wasOpen {
		changed = true
	}

	target := 0.0
	speed := collapseSpeed
	if d.open {
		target, speed = d.timelineEnd(), 1
	}
	if clock := approach(d.clock, target, sec*speed); clock != d.clock {
		d.clock = clock
		changed = true
	}

	hit := Hit{}
	if d.open && d.pointerIn {
		hit = d.HitTest(int(d.pointerX), int(d.pointerY))
	}
	for i := range d.lift {
		want := 0.0
		isTab := i < len(d.items)
		switch {
		case isTab && hit.Kind == HitTab && hit.Index == i,
			isTab && d.open && d.items[i].ID == d.active,
			!isTab && hit.Kind == HitAdd:
			want = 1
		}
		if v := approach(d.lift[i], want, sec/liftDuration); v != d.lift[i] {
			d.lift[i] = v
			changed = true
		}
	}
	return changed
}

// updateOpen decides whether the deck should be expanding or collapsing
// based on where the pointer is.
func (d *Deck) updateOpen(sec float64) {
	if d.held {
		d.open = true
		return
	}
	if !d.open {
		if d.pointerIn && d.touches(d.pointerX, d.pointerY) {
			d.open = true
			d.away = 0
		}
		return
	}

	w, h := d.Size()
	slack := hoverSlack * d.scale
	bounds := Rect{0, 0, float64(w), float64(h)}.Inflate(slack, slack)
	if d.pointerIn && bounds.Contains(d.pointerX, d.pointerY) {
		d.away = 0
		return
	}
	d.away += sec
	if d.away >= collapseGrace {
		d.open = false
		d.away = 0
	}
}

// touches reports whether a point is on the resting pill's hover target or on
// a tab that is still visible while collapsing.
func (d *Deck) touches(x, y float64) bool {
	if d.hotZone().Contains(x, y) {
		return true
	}
	return d.clock > 0 && d.HitTest(int(x), int(y)).Kind != HitNone
}

// Idle reports whether the deck is fully collapsed with no animation in
// flight, meaning the host can stop its animation timer.
func (d *Deck) Idle() bool {
	if d.open || d.held || d.clock > 0 {
		return false
	}
	for _, l := range d.lift {
		if l > 0 {
			return false
		}
	}
	return true
}

// Open reports whether the deck is expanded or expanding.
func (d *Deck) Open() bool { return d.open }

// HitTest returns what lies under the point (x, y) in window pixels.
func (d *Deck) HitTest(x, y int) Hit {
	if d.clock <= 0 {
		return Hit{}
	}
	fx, fy := float64(x), float64(y)
	// Extend each tab over half of the gap around it so that moving down
	// the deck never passes through a dead zone.
	pad := tabGap * d.scale / 2
	for i := range d.items {
		if d.progress(i) < 0.5 {
			continue
		}
		if d.tabRect(i).Inflate(0, pad).Contains(fx, fy) {
			return Hit{HitTab, i}
		}
	}
	if d.progress(len(d.items)) >= 0.5 && d.addRect().Inflate(pad, pad).Contains(fx, fy) {
		return Hit{Kind: HitAdd}
	}
	return Hit{}
}

// TabAnchor returns the fully expanded rectangle of the tab for the given
// note, which the host uses to position the editor next to it.
func (d *Deck) TabAnchor(id string) (Rect, bool) {
	for i, it := range d.items {
		if it.ID == id {
			return d.mirror(d.openRect(i, 1)), true
		}
	}
	return Rect{}, false
}

// AddAnchor returns the fully expanded rectangle of the "new note" button.
func (d *Deck) AddAnchor() Rect {
	return d.mirror(d.openAddRect())
}

// Frame returns the drawable state of the deck.
func (d *Deck) Frame() Frame {
	w, h := d.Size()
	f := Frame{
		Width:      w,
		Height:     h,
		Scale:      d.scale,
		Edge:       d.edge,
		Pill:       d.mirror(d.pillRect()),
		PillRadius: pillWidth * d.scale / 2,
		PillAlpha:  1 - d.progress(0),
		HotZone:    d.hotZone(),
		Tabs:       make([]Tab, len(d.items)),
	}
	for i, it := range d.items {
		p := d.progress(i)
		f.Tabs[i] = Tab{
			Item:   it,
			Rect:   d.tabRect(i),
			Radius: lerp(dashHeight/2, tabRadius, p) * d.scale,
			Open:   p,
			Label:  clamp((p-0.5)*2, 0, 1),
			Lift:   d.lift[i],
			Active: it.ID == d.active,
		}
	}
	n := len(d.items)
	f.Add = d.addRect()
	f.AddAlpha = d.progress(n)
	f.AddLift = d.lift[n]
	return f
}

// The layout helpers below work in "right edge" coordinates, where the
// screen edge is at x = width. mirror flips the result for a left-edge deck.

func (d *Deck) mirror(r Rect) Rect {
	if d.edge == Left {
		w, _ := d.Size()
		r.X = float64(w) - r.X - r.W
	}
	return r
}

// stagger is the delay between consecutive tabs starting to expand.
func (d *Deck) stagger() float64 {
	n := len(d.items)
	if n == 0 {
		return 0
	}
	return min(maxStagger, maxStaggerSum/float64(n))
}

// timelineEnd is the clock value at which every element is fully expanded.
func (d *Deck) timelineEnd() float64 {
	return d.stagger()*float64(len(d.items)) + tabDuration
}

// progress returns how far element i has expanded, from 0 to 1. Index
// len(items) refers to the add button.
func (d *Deck) progress(i int) float64 {
	return easeOutCubic((d.clock - float64(i)*d.stagger()) / tabDuration)
}

// contentHeight is the height of the expanded tabs plus the add button.
func (d *Deck) contentHeight() float64 {
	return (float64(len(d.items))*(tabHeight+tabGap) + addSize) * d.scale
}

// restHeight is the height of the dashes shown at rest. An empty deck still
// reserves room for one dash so the pill has something to show.
func (d *Deck) restHeight() float64 {
	n := float64(max(len(d.items), 1))
	return (n*dashHeight + (n-1)*dashGap) * d.scale
}

// maxScroll is how far the expanded content overflows the window.
func (d *Deck) maxScroll() float64 {
	_, h := d.Size()
	return max(0, d.contentHeight()+2*margin*d.scale-float64(h))
}

func (d *Deck) clampScroll() {
	d.scroll = clamp(d.scroll, 0, d.maxScroll())
}

// reveal scrolls just enough to bring tab i fully into view.
func (d *Deck) reveal(i int) {
	if d.maxScroll() == 0 {
		return
	}
	s := d.scale
	_, h := d.Size()
	top := float64(i) * (tabHeight + tabGap) * s
	bottom := top + tabHeight*s
	visible := float64(h) - 2*margin*s
	if top < d.scroll {
		d.scroll = top
	} else if bottom > d.scroll+visible {
		d.scroll = bottom - visible
	}
	d.clampScroll()
}

// openTop is the y coordinate of the first expanded tab.
func (d *Deck) openTop() float64 {
	_, h := d.Size()
	if d.maxScroll() > 0 {
		return margin*d.scale - d.scroll
	}
	return (float64(h) - d.contentHeight()) / 2
}

// restRect is the dash that tab i collapses into.
func (d *Deck) restRect(i int) Rect {
	s := d.scale
	w, h := d.Size()
	top := (float64(h) - d.restHeight()) / 2
	return Rect{
		X: float64(w) - (pillInset+(pillWidth+dashWidth)/2)*s,
		Y: top + float64(i)*(dashHeight+dashGap)*s,
		W: dashWidth * s,
		H: dashHeight * s,
	}
}

// openRect is tab i fully expanded and raised by lift.
func (d *Deck) openRect(i int, lift float64) Rect {
	s := d.scale
	w, _ := d.Size()
	width := (tabWidth + tabLift*lift) * s
	return Rect{
		X: float64(w) - edgeGap*s - width,
		Y: d.openTop() + float64(i)*(tabHeight+tabGap)*s,
		W: width,
		H: tabHeight * s,
	}
}

// tabRect is the current, animated rectangle of tab i in window coordinates.
func (d *Deck) tabRect(i int) Rect {
	return d.mirror(lerpRect(d.restRect(i), d.openRect(i, d.lift[i]), d.progress(i)))
}

func (d *Deck) openAddRect() Rect {
	s := d.scale
	w, _ := d.Size()
	return Rect{
		X: float64(w) - (edgeGap+addSize)*s,
		Y: d.openTop() + float64(len(d.items))*(tabHeight+tabGap)*s,
		W: addSize * s,
		H: addSize * s,
	}
}

// addRect is the current rectangle of the add button, which slides in from
// the screen edge as it fades.
func (d *Deck) addRect() Rect {
	r := d.openAddRect()
	r.X += (1 - d.progress(len(d.items))) * addSize * d.scale / 2
	return d.mirror(r)
}

func (d *Deck) pillRect() Rect {
	s := d.scale
	w, h := d.Size()
	height := d.restHeight() + 2*pillPadY*s
	return Rect{
		X: float64(w) - (pillInset+pillWidth)*s,
		Y: (float64(h) - height) / 2,
		W: pillWidth * s,
		H: height,
	}
}

// hotZone is the pill grown outward and extended to the very edge of the
// screen, in window coordinates.
func (d *Deck) hotZone() Rect {
	s := d.scale
	w, _ := d.Size()
	r := d.pillRect().Inflate(0, hotPad*s)
	r.X -= hotPad * s
	r.W = float64(w) - r.X
	return d.mirror(r)
}
