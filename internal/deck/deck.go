// Package deck models the note deck that lives at the edge of the screen.
//
// At rest the deck is a thin pill showing one coloured dash per note. When
// the pointer reaches it, each dash grows into a labelled tab, one after the
// other, and shrinks back once the pointer leaves. Resting the pointer on a
// tab grows it into a card that previews the note, pushing the tabs below it
// down.
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
	tabWidth   = 216.0 // width of an expanded tab
	tabHeight  = 48.0  // room for the title and a line of details
	tabGap     = 6.0   // vertical space between tabs
	tabRadius  = 10.0  // corner radius of an expanded tab
	tabLift    = 10.0  // extra width of a hovered or active tab
	edgeGap    = 8.0   // space between expanded tabs and the screen edge
	margin     = 16.0  // transparent border that leaves room for shadows
	addSize    = 36.0  // the square "new note" button below the tabs
	pillWidth  = 12.0
	pillInset  = 3.0 // space between the pill and the screen edge
	pillPadY   = 7.0 // padding above and below the dashes
	dashWidth  = 6.0
	dashHeight = 4.0
	dashGap    = 4.0
	// maxDashes caps the resting pill so a large deck stays a slim handle;
	// notes beyond the cap share the last dash.
	maxDashes  = 12
	hotPad     = 6.0 // how far the hover target extends beyond the pill
	hoverSlack = 12.0
	// TabHeight is the height of an expanded tab, which a preview card keeps
	// as its header.
	TabHeight = tabHeight
	// CardWidth is the width of a tab grown into a preview card.
	CardWidth = 300.0
	// MaxCardHeight caps the height of a preview card, header included.
	MaxCardHeight = 380.0
)

// Animation timings.
const (
	tabDuration   = 0.30 // seconds for one tab to fully expand
	maxStagger    = 0.035
	maxStaggerSum = 0.25 // cap on the total delay so large decks stay snappy
	collapseSpeed = 1.6  // collapsing runs faster than expanding
	// collapseGrace is how long the pointer may stray from the deck before
	// it collapses, forgiving small overshoots.
	collapseGrace = 0.35
	// peekDelay is how long the pointer must rest on a tab before it grows
	// into a card: too short to notice, but long enough that a quick sweep
	// across the deck opens nothing.
	peekDelay = 0.07
	// relaxSpeed is how fast, in device-independent pixels per second, the
	// deck slides back to its home once no card is open.
	relaxSpeed = 1400.0
)

// Item is one note as shown in the deck.
type Item struct {
	ID    string
	Title string
	Color note.Color
	// Meta is the line of details under the title, e.g. "2 of 5 done ·
	// 3h ago".
	Meta string
	// Body is the note's text, shown when its tab grows into a card.
	Body string
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
	// Grow is how far the tab has grown into a preview card, from 0 to 1.
	Grow float64
	// Header is the height of the tab's title area, which stays at the top
	// of the card as it grows.
	Header float64
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
	// Dark asks for preview cards in the dark theme.
	Dark bool

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

	// stale is set when something changed that the next Advance must report
	// even though no animation is running, such as which tab is active.
	stale bool

	dark bool
	// measure returns the height of a note's preview card in pixels. Without
	// it tabs never grow into cards.
	measure func(it Item, scale float64) float64
	// cardHeights caches measure for every item; nil when stale.
	cardHeights []float64
	// grow holds the card animation state per tab, and growV its velocity.
	grow, growV []float64
	// liftV is the velocity of each entry of lift.
	liftV []float64
	// peekID is the note whose tab is, or is growing into, a card.
	peekID string
	// dwellID is the tab the pointer is resting on, and dwell for how long.
	dwellID string
	dwell   float64
	// skipID is a tab that must not grow again until the pointer leaves it,
	// e.g. one just clicked.
	skipID string
	// peekAway is how long the pointer has been away from an open card.
	peekAway float64
	// offset shifts the expanded tabs vertically, in pixels, to keep the
	// card under the pointer still while others open and close, and to keep
	// the card on screen.
	offset float64
	// safeTop and safeBottom bound where a card may be drawn, in window
	// pixels; safeBottom is zero when unset.
	safeTop, safeBottom float64
}

// New returns a collapsed, empty deck on the given edge.
func New(edge Edge) *Deck {
	return &Deck{edge: edge, scale: 1, lift: make([]float64, 1)}
}

// SetItems replaces the notes shown in the deck.
func (d *Deck) SetItems(items []Item) {
	// A tab keeps its card state across updates by note, since notes can
	// change places.
	grown := map[string]float64{}
	for i, it := range d.items {
		if i < len(d.grow) && d.grow[i] > 0 {
			grown[it.ID] = d.grow[i]
		}
	}
	d.grow = make([]float64, len(items))
	d.growV = make([]float64, len(items))
	for i, it := range items {
		d.grow[i] = grown[it.ID]
	}
	d.cardHeights = nil
	d.items = append(d.items[:0], items...)
	// Carry over the hover state of surviving tabs; the add button's state
	// lives in the last slot.
	lift := make([]float64, len(items)+1)
	copy(lift[:len(items)], d.lift[:len(d.lift)-1])
	lift[len(items)] = d.lift[len(d.lift)-1]
	d.lift = lift
	d.liftV = make([]float64, len(lift))
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
	d.offset *= s / d.scale
	d.scale = s
	d.cardHeights = nil
	d.clampScroll()
}

// SetDark chooses the theme of the preview cards.
func (d *Deck) SetDark(dark bool) {
	if dark != d.dark {
		d.stale = true
	}
	d.dark = dark
}

// SetCardMeasure enables preview cards. measure returns the full height in
// pixels of an item's card at a scale factor, header included; heights are
// capped at MaxCardHeight.
func (d *Deck) SetCardMeasure(measure func(it Item, scale float64) float64) {
	d.measure = measure
	d.cardHeights = nil
}

// SetSafeArea limits preview cards to the rows top to bottom of the window,
// typically the monitor's work area, which the window may extend beyond.
func (d *Deck) SetSafeArea(top, bottom int) {
	d.safeTop, d.safeBottom = float64(top), float64(bottom)
}

// Inset is how far, in pixels, the window extends above and below the area
// the deck itself occupies, to leave room for a card to grow. The host
// shifts the window up by this much so that the deck stays where it would
// be without cards.
func (d *Deck) Inset() int {
	return int(d.extra() + 0.5)
}

// Peeked returns the note whose tab is showing a preview card, if any.
func (d *Deck) Peeked() string { return d.peekID }

// Unpeek shrinks an open card back into its tab. The tab will not grow
// again until the pointer has left it.
func (d *Deck) Unpeek() {
	if d.peekID != "" {
		d.skipID = d.peekID
	}
	if d.dwellID != "" {
		d.skipID = d.dwellID
	}
	d.peekID, d.dwellID, d.dwell = "", "", 0
	d.stale = true
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
	if id != d.active {
		d.stale = true
	}
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
	d.Unpeek()
	d.scroll += delta
	d.clampScroll()
}

// Size returns the size of the deck window in pixels. It depends on the
// number of notes, the scale and the maximum height, but not on the
// animation state, so the window only needs to be resized when those change.
func (d *Deck) Size() (width, height int) {
	s := d.scale
	w := tabLift + tabWidth
	if d.measure != nil {
		w = max(w, CardWidth)
	}
	w = (margin + w + edgeGap) * s
	return int(w + 0.5), int(d.homeHeight() + 2*d.extra() + 0.5)
}

// homeHeight is the height of the part of the window the deck occupies
// without cards: the tabs, or the resting pill, plus a margin for shadows.
func (d *Deck) homeHeight() float64 {
	s := d.scale
	h := max(d.contentHeight(), d.restHeight()+2*pillPadY*s) + 2*margin*s
	if d.maxHeight > 0 {
		h = min(h, float64(d.maxHeight))
	}
	return float64(int(h + 0.5))
}

// extra is the room above and below the home area for cards to grow into.
func (d *Deck) extra() float64 {
	if d.measure == nil {
		return 0
	}
	return float64(int((MaxCardHeight-tabHeight)*d.scale + 0.5))
}

// Advance moves the animation forward by dt and reports whether anything
// visible changed, in which case the host should repaint.
func (d *Deck) Advance(dt time.Duration) bool {
	sec := clamp(dt.Seconds(), 0, 0.1)
	// The host ticks slowly while nothing moves, so waiting for the pointer
	// to settle counts real time.
	wait := clamp(dt.Seconds(), 0, 0.5)
	changed := d.stale
	d.stale = false

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
		if i >= len(d.liftV) {
			d.liftV = make([]float64, len(d.lift))
		}
		if fastSpatial.step(&d.lift[i], &d.liftV[i], want, sec) {
			changed = true
		}
	}
	if d.updatePeek(wait, hit) {
		changed = true
	}
	if d.updateCards(sec) {
		changed = true
	}
	return changed
}

// updatePeek decides which tab, if any, should be a card. It reports
// whether that changed.
func (d *Deck) updatePeek(sec float64, hit Hit) bool {
	before := d.peekID
	if !d.open || d.measure == nil {
		d.peekID, d.dwellID, d.skipID = "", "", ""
		return before != ""
	}
	over := ""
	if hit.Kind == HitTab && d.progress(len(d.items)) >= 1 {
		over = d.items[hit.Index].ID
	}
	if over != d.skipID {
		d.skipID = ""
	}
	switch {
	case over != "" && over == d.peekID:
		d.dwellID, d.peekAway = "", 0
	case over != "" && over != d.skipID && over != d.active:
		d.peekAway = 0
		if over != d.dwellID {
			d.dwellID, d.dwell = over, 0
		}
		if d.dwell += sec; d.dwell >= peekDelay {
			d.peekID, d.dwellID = over, ""
		}
	default:
		d.dwellID = ""
		if d.peekID == "" || hit.Kind != HitNone || d.nearContent() {
			d.peekAway = 0
			break
		}
		if d.peekAway += sec; d.peekAway >= collapseGrace {
			d.peekID, d.peekAway = "", 0
		}
	}
	return d.peekID != before
}

// updateCards animates tabs growing into and shrinking from cards,
// keeping the open card where it is while the tabs above it change size.
func (d *Deck) updateCards(sec float64) bool {
	p := d.index(d.peekID)
	anchor := 0.0
	if p >= 0 {
		anchor = d.slotY(p, false)
	}
	changed := false
	for i := range d.grow {
		want := 0.0
		if i == p {
			want = 1
		}
		if defaultSpatial.step(&d.grow[i], &d.growV[i], want, sec) {
			changed = true
		}
	}
	offset := d.offset
	if p >= 0 {
		offset += anchor - d.slotY(p, false)
		offset = d.keepOnScreen(p, offset)
	} else {
		offset = approach(offset, 0, sec*relaxSpeed*d.scale)
	}
	if offset != d.offset {
		d.offset = offset
		changed = true
	}
	return changed
}

// keepOnScreen adjusts offset so that the card of tab i lies within the
// safe area.
func (d *Deck) keepOnScreen(i int, offset float64) float64 {
	_, h := d.Size()
	lo, hi := d.safeTop, d.safeBottom
	if hi <= lo {
		lo, hi = 0, float64(h)
	}
	lo, hi = max(lo, 0)+margin*d.scale, min(hi, float64(h))-margin*d.scale
	top := d.slotY(i, false) - d.offset + offset
	bottom := top + d.height(i, false)
	if bottom > hi {
		offset -= bottom - hi
		top -= bottom - hi
	}
	if top < lo {
		offset += lo - top
	}
	e := d.extra()
	return clamp(offset, -e, e)
}

// nearContent reports whether the pointer is on or close to anything the
// deck shows.
func (d *Deck) nearContent() bool {
	slack := (hoverSlack + margin) * d.scale
	return d.pointerIn && d.contentBounds().Inflate(slack, slack).Contains(d.pointerX, d.pointerY)
}

// contentBounds is the smallest rectangle around the pill, the tabs and the
// add button as currently drawn.
func (d *Deck) contentBounds() Rect {
	b := d.mirror(d.pillRect())
	grow := func(r Rect) {
		x0, y0 := min(b.X, r.X), min(b.Y, r.Y)
		x1, y1 := max(b.X+b.W, r.X+r.W), max(b.Y+b.H, r.Y+r.H)
		b = Rect{x0, y0, x1 - x0, y1 - y0}
	}
	for i := range d.items {
		grow(d.tabRect(i))
	}
	grow(d.addRect())
	return b
}

// index returns the position of the item with the given ID, or -1.
func (d *Deck) index(id string) int {
	if id == "" {
		return -1
	}
	for i, it := range d.items {
		if it.ID == id {
			return i
		}
	}
	return -1
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

	if d.nearContent() {
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
	if d.open || d.held || d.clock > 0 || d.stale {
		return false
	}
	for i, l := range d.lift {
		if i < len(d.liftV) && d.liftV[i] != 0 {
			return false
		}
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
			return d.mirror(d.openRect(i, 1, true)), true
		}
	}
	return Rect{}, false
}

// AddAnchor returns the fully expanded rectangle of the "new note" button.
func (d *Deck) AddAnchor() Rect {
	return d.mirror(d.openAddRect(true))
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
		Dark:       d.dark,
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
			Grow:   d.growth(i),
			Header: tabHeight * d.scale,
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
	return EmphasizedDecelerate((d.clock - float64(i)*d.stagger()) / tabDuration)
}

// contentHeight is the height of the expanded tabs plus the add button.
func (d *Deck) contentHeight() float64 {
	return (float64(len(d.items))*(tabHeight+tabGap) + addSize) * d.scale
}

// restHeight is the height of the dashes shown at rest. An empty deck still
// reserves room for one dash so the pill has something to show.
func (d *Deck) restHeight() float64 {
	n := float64(min(max(len(d.items), 1), maxDashes))
	return (n*dashHeight + (n-1)*dashGap) * d.scale
}

// maxScroll is how far the expanded content overflows its home area.
func (d *Deck) maxScroll() float64 {
	return max(0, d.contentHeight()+2*margin*d.scale-d.homeHeight())
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
	top := float64(i) * (tabHeight + tabGap) * s
	bottom := top + tabHeight*s
	visible := d.homeHeight() - 2*margin*s
	if top < d.scroll {
		d.scroll = top
	} else if bottom > d.scroll+visible {
		d.scroll = bottom - visible
	}
	d.clampScroll()
}

// openTop is the y coordinate of the first expanded tab when no card is
// open.
func (d *Deck) openTop() float64 {
	if d.maxScroll() > 0 {
		return d.extra() + margin*d.scale - d.scroll
	}
	return d.extra() + (d.homeHeight()-d.contentHeight())/2
}

// restRect is the dash that tab i collapses into.
func (d *Deck) restRect(i int) Rect {
	s := d.scale
	w, _ := d.Size()
	top := d.extra() + (d.homeHeight()-d.restHeight())/2
	return Rect{
		X: float64(w) - (pillInset+(pillWidth+dashWidth)/2)*s,
		Y: top + float64(min(i, maxDashes-1))*(dashHeight+dashGap)*s,
		W: dashWidth * s,
		H: dashHeight * s,
	}
}

// openRect is tab i fully expanded and raised by lift. With final it is
// where the tab comes to rest once every card has shrunk away; otherwise it
// is where the tab is now, as far as cards are concerned.
func (d *Deck) openRect(i int, lift float64, final bool) Rect {
	s := d.scale
	w, _ := d.Size()
	width := (tabWidth + tabLift*lift) * s
	if !final {
		width = lerp(width, CardWidth*s, d.growth(i))
	}
	return Rect{
		X: float64(w) - edgeGap*s - width,
		Y: d.slotY(i, final),
		W: width,
		H: d.height(i, final),
	}
}

// slotY is the top of tab i, or of the add button for i == len(items).
func (d *Deck) slotY(i int, final bool) float64 {
	y := d.openTop()
	if !final {
		y += d.offset
	}
	for k := range min(i, len(d.items)) {
		y += d.height(k, final) + tabGap*d.scale
	}
	return y
}

// height is the height of tab i, grown toward its card unless final.
func (d *Deck) height(i int, final bool) float64 {
	h := tabHeight * d.scale
	if final {
		return h
	}
	return lerp(h, d.cardHeight(i), d.growth(i))
}

// growth is how far tab i has grown into its card, eased.
func (d *Deck) growth(i int) float64 {
	if i >= len(d.grow) {
		return 0
	}
	// The spring may overshoot a little past a full card, which reads as
	// the card settling into place, but never shrinks a tab below its size.
	return max(d.grow[i], 0)
}

// cardHeight is the full height of tab i's card in pixels.
func (d *Deck) cardHeight(i int) float64 {
	tab := tabHeight * d.scale
	if d.measure == nil {
		return tab
	}
	if len(d.cardHeights) != len(d.items) {
		d.cardHeights = make([]float64, len(d.items))
		for k, it := range d.items {
			d.cardHeights[k] = clamp(d.measure(it, d.scale), tab, MaxCardHeight*d.scale)
		}
	}
	return d.cardHeights[i]
}

// tabRect is the current, animated rectangle of tab i in window coordinates.
func (d *Deck) tabRect(i int) Rect {
	return d.mirror(lerpRect(d.restRect(i), d.openRect(i, d.lift[i], false), d.progress(i)))
}

func (d *Deck) openAddRect(final bool) Rect {
	s := d.scale
	w, _ := d.Size()
	return Rect{
		X: float64(w) - (edgeGap+addSize)*s,
		Y: d.slotY(len(d.items), final),
		W: addSize * s,
		H: addSize * s,
	}
}

// addRect is the current rectangle of the add button, which slides in from
// the screen edge as it fades.
func (d *Deck) addRect() Rect {
	r := d.openAddRect(false)
	r.X += (1 - d.progress(len(d.items))) * addSize * d.scale / 2
	return d.mirror(r)
}

func (d *Deck) pillRect() Rect {
	s := d.scale
	w, _ := d.Size()
	height := d.restHeight() + 2*pillPadY*s
	return Rect{
		X: float64(w) - (pillInset+pillWidth)*s,
		Y: d.extra() + (d.homeHeight()-height)/2,
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
