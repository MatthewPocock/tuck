// Package settings models the settings window: which controls it has, where
// they are and what clicking them means.
//
// Like the deck, it is pure layout and state with no platform dependencies.
// The render package paints a Layout and the Windows shell feeds it clicks
// and keys, so the window can be previewed and tested on any OS.
package settings

import (
	"strconv"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/deck"
)

// Control identifies an interactive element of the window.
type Control int

// The controls, in reading order.
const (
	None Control = iota
	Close
	Edge
	Position
	Theme
	FontSmaller
	FontLarger
	NewNoteKey
	ToggleDeckKey
	Autostart
	ChangeFolder
	OpenFolder
	EditFile
)

// Kind is how a control looks and behaves.
type Kind int

// The kinds of control.
const (
	KindClose Kind = iota
	// KindSegmented is a row of mutually exclusive options.
	KindSegmented
	// KindStep is one of the two buttons of a number stepper.
	KindStep
	// KindHotkey shows a shortcut and records a new one when clicked.
	KindHotkey
	// KindToggle is an on/off switch.
	KindToggle
	KindButton
	KindLink
)

// Position presets offered for the deck, from top to bottom.
var PositionPresets = []float64{0.12, 0.5, 0.88}

// Options are the labels of the segmented controls.
var Options = map[Control][]string{
	Edge:     {"Left", "Right"},
	Position: {"Top", "Middle", "Bottom"},
	Theme:    {"System", "Light", "Dark"},
}

var themes = []config.Theme{config.ThemeSystem, config.ThemeLight, config.ThemeDark}

// Font size limits offered by the stepper, in points.
const (
	MinFontSize = 8
	MaxFontSize = 24
)

// State is everything the window shows.
type State struct {
	Config    config.Config
	Autostart bool
	// NotesDir is the folder notes are stored in.
	NotesDir string
	Version  string

	// Capturing is the hotkey field waiting for a new shortcut, or None.
	Capturing Control
	// Message is shown below the shortcuts, e.g. when a shortcut is taken.
	// It is an error when Problem is set.
	Message string
	Problem bool

	// Hover and Pressed are the control under the pointer and the one being
	// clicked; HoverSegment is the option under the pointer.
	Hover        Control
	HoverSegment int
	Pressed      Control
}

// Selected returns the chosen option of a segmented control, or -1 if the
// current value matches none of them (e.g. a position set by hand).
func (s State) Selected(c Control) int {
	switch c {
	case Edge:
		if s.Config.Edge == config.EdgeLeft {
			return 0
		}
		return 1
	case Position:
		for i, p := range PositionPresets {
			if s.Config.Position == p {
				return i
			}
		}
	case Theme:
		for i, t := range themes {
			if s.Config.Theme == t {
				return i
			}
		}
	}
	return -1
}

// Choose applies option i of a segmented control to cfg.
func Choose(cfg *config.Config, c Control, i int) {
	switch c {
	case Edge:
		cfg.Edge = []config.Edge{config.EdgeLeft, config.EdgeRight}[i]
	case Position:
		cfg.Position = PositionPresets[i]
	case Theme:
		cfg.Theme = themes[i]
	}
}

// StepFont returns the font size after pressing a stepper button.
func StepFont(size int, c Control) int {
	switch c {
	case FontSmaller:
		return max(size-1, MinFontSize)
	case FontLarger:
		return min(size+1, MaxFontSize)
	}
	return size
}

// Layout metrics in device-independent pixels.
const (
	Width        = 420.0
	pad          = 24.0
	titleHeight  = 56.0
	sectionGap   = 10.0
	sectionTitle = 26.0
	rowHeight    = 44.0
	controlH     = 30.0
	segmentedW   = 216.0
	stepW        = 30.0
	stepValueW   = 56.0
	hotkeyW      = 168.0
	toggleW      = 40.0
	toggleH      = 22.0
	buttonW      = 84.0
	buttonGap    = 8.0
	messageH     = 22.0
	pathH        = 22.0
	footerH      = 48.0
)

// Item is one control placed in the window.
type Item struct {
	Control Control
	Kind    Kind
	Rect    deck.Rect
	// Segments are the options of a segmented control.
	Segments []deck.Rect
	// Label is the text of a button or link.
	Label string
}

// Text is a static piece of text placed in the window.
type Text struct {
	Rect deck.Rect
	Text string
	Role TextRole
}

// TextRole selects how static text is drawn.
type TextRole int

// The text roles.
const (
	RoleTitle TextRole = iota
	RoleSection
	RoleLabel
	// RoleValue is the number between the stepper buttons.
	RoleValue
	// RoleMessage is the hint or problem below the shortcuts.
	RoleMessage
	// RolePath is the notes folder.
	RolePath
	RoleFooter
)

// Layout is the window's arrangement at one scale factor.
type Layout struct {
	Scale float64
	// Width and Height are the window size in pixels.
	Width, Height int
	Items         []Item
	Texts         []Text
	// Separator is the y coordinate of the line above the footer.
	Separator float64
	// Title is the band that drags the window.
	Title deck.Rect
}

// NewLayout arranges the window for a scale factor and the given state, which
// supplies the texts that depend on it.
func NewLayout(scale float64, s State) Layout {
	l := Layout{Scale: scale}
	px := func(v float64) float64 { return v * scale }
	w := px(Width)
	right := w - px(pad)
	y := 0.0

	l.Title = deck.Rect{W: w, H: px(titleHeight)}
	l.Texts = append(l.Texts, Text{deck.Rect{X: px(pad), W: w / 2, H: px(titleHeight)}, "Settings", RoleTitle})
	closeSize := px(28)
	l.Items = append(l.Items, Item{Control: Close, Kind: KindClose,
		Rect: deck.Rect{X: w - px(14) - closeSize, Y: (px(titleHeight) - closeSize) / 2, W: closeSize, H: closeSize}})
	y += px(titleHeight)

	section := func(title string) {
		y += px(sectionGap)
		l.Texts = append(l.Texts, Text{deck.Rect{X: px(pad), Y: y, W: w - 2*px(pad), H: px(sectionTitle)}, title, RoleSection})
		y += px(sectionTitle)
	}
	// row adds a labelled row whose control is width wide and right-aligned;
	// it returns the control's rectangle.
	row := func(label string, width, height float64) deck.Rect {
		l.Texts = append(l.Texts, Text{deck.Rect{X: px(pad), Y: y, W: right - px(width) - px(pad) - px(12), H: px(rowHeight)}, label, RoleLabel})
		r := deck.Rect{X: right - px(width), Y: y + (px(rowHeight)-px(height))/2, W: px(width), H: px(height)}
		y += px(rowHeight)
		return r
	}
	segmented := func(label string, c Control) {
		r := row(label, segmentedW, controlH)
		n := len(Options[c])
		segs := make([]deck.Rect, n)
		for i := range segs {
			segs[i] = deck.Rect{X: r.X + float64(i)*r.W/float64(n), Y: r.Y, W: r.W / float64(n), H: r.H}
		}
		l.Items = append(l.Items, Item{Control: c, Kind: KindSegmented, Rect: r, Segments: segs})
	}

	section("Deck")
	segmented("Screen edge", Edge)
	segmented("Position", Position)

	section("Appearance")
	segmented("Theme", Theme)
	r := row("Text size", 2*stepW+stepValueW, controlH)
	l.Items = append(l.Items,
		Item{Control: FontSmaller, Kind: KindStep, Rect: deck.Rect{X: r.X, Y: r.Y, W: px(stepW), H: r.H}, Label: "−"},
		Item{Control: FontLarger, Kind: KindStep, Rect: deck.Rect{X: r.X + r.W - px(stepW), Y: r.Y, W: px(stepW), H: r.H}, Label: "+"})
	l.Texts = append(l.Texts, Text{deck.Rect{X: r.X + px(stepW), Y: r.Y, W: px(stepValueW), H: r.H},
		strconv.Itoa(s.Config.FontSize) + " pt", RoleValue})

	section("Shortcuts")
	l.Items = append(l.Items, Item{Control: NewNoteKey, Kind: KindHotkey, Rect: row("New note", hotkeyW, controlH)})
	l.Items = append(l.Items, Item{Control: ToggleDeckKey, Kind: KindHotkey, Rect: row("Show or hide the deck", hotkeyW, controlH)})
	message := s.Message
	if message == "" {
		message = "Click a shortcut, then press the new keys. Backspace turns it off."
	}
	l.Texts = append(l.Texts, Text{deck.Rect{X: px(pad), Y: y, W: w - 2*px(pad), H: px(messageH)}, message, RoleMessage})
	y += px(messageH)

	section("General")
	l.Items = append(l.Items, Item{Control: Autostart, Kind: KindToggle, Rect: row("Start with Windows", toggleW, toggleH)})
	r = row("Notes folder", 2*buttonW+buttonGap, controlH)
	l.Items = append(l.Items,
		Item{Control: ChangeFolder, Kind: KindButton, Rect: deck.Rect{X: r.X, Y: r.Y, W: px(buttonW), H: r.H}, Label: "Change…"},
		Item{Control: OpenFolder, Kind: KindButton, Rect: deck.Rect{X: r.X + px(buttonW+buttonGap), Y: r.Y, W: px(buttonW), H: r.H}, Label: "Open"})
	l.Texts = append(l.Texts, Text{deck.Rect{X: px(pad), Y: y - px(6), W: w - 2*px(pad), H: px(pathH)}, s.NotesDir, RolePath})
	y += px(pathH)

	y += px(sectionGap)
	l.Separator = y
	footer := deck.Rect{X: px(pad), Y: y, W: w - 2*px(pad), H: px(footerH)}
	l.Texts = append(l.Texts, Text{footer, "Tuck " + s.Version, RoleFooter})
	linkW := px(132)
	l.Items = append(l.Items, Item{Control: EditFile, Kind: KindLink, Label: "Edit settings file",
		Rect: deck.Rect{X: right - linkW, Y: y + (px(footerH)-px(controlH))/2, W: linkW, H: px(controlH)}})
	y += px(footerH)

	l.Width, l.Height = int(w+0.5), int(y+0.5)
	return l
}

// Hit returns the control under the point (x, y) and, for a segmented
// control, the option under it.
func (l Layout) Hit(x, y int) (Control, int) {
	fx, fy := float64(x), float64(y)
	for _, it := range l.Items {
		if !it.Rect.Contains(fx, fy) {
			continue
		}
		for i, seg := range it.Segments {
			if seg.Contains(fx, fy) {
				return it.Control, i
			}
		}
		return it.Control, -1
	}
	return None, -1
}

// Find returns the item of a control.
func (l Layout) Find(c Control) (Item, bool) {
	for _, it := range l.Items {
		if it.Control == c {
			return it, true
		}
	}
	return Item{}, false
}
