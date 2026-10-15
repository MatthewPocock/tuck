package app

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/note"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/store"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// newTestApp builds an App with real windows but with its data in a
// temporary directory, without a tray icon or global hotkeys.
func newTestApp(t *testing.T) *App {
	t.Helper()
	return newTestAppIn(t, t.TempDir())
}

// newTestAppIn is newTestApp with its data in dir, so that a second app can
// pick up where a first one left off.
func newTestAppIn(t *testing.T, dir string) *App {
	t.Helper()
	// Windows belong to the thread that creates them.
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)

	a := &App{
		version: "test",
		paths: paths{dir: dir, config: filepath.Join(dir, "config.json"), notes: filepath.Join(dir, "notes"),
			session: filepath.Join(dir, "session.json")},
		cfg:  config.Default(),
		inst: win32.ModuleHandle(),
	}
	var err error
	if a.store, err = store.Open(a.paths.notes); err != nil {
		t.Fatal(err)
	}
	if err := a.createWindows(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.editor.pinned = true // keep teardown from re-entering hide
		a.restoring = true     // and from rewriting the session
		for _, e := range a.pinned {
			win32.DestroyWindow(e.hwnd)
		}
		win32.DestroyWindow(a.editor.hwnd)
		win32.DestroyWindow(a.deck.hwnd)
		win32.DestroyWindow(a.controller)
		a.text.close()
	})
	a.syncDeck()
	a.deck.show()
	return a
}

// addNote stores a note with the given body and adds it to the app.
func addNote(t *testing.T, a *App, body string) *note.Note {
	t.Helper()
	n := note.New(time.Now().Add(time.Duration(len(a.notes))*time.Second), note.Sky)
	n.Body = body
	if _, err := a.store.Save(n); err != nil {
		t.Fatal(err)
	}
	a.notes = append(a.notes, n)
	a.syncDeck()
	return n
}

// typeText inserts text into the editor the way typing would, including the
// change notification.
func typeText(a *App, text string) {
	win32.SendMessage(a.editor.edit, win32.EM_REPLACESEL, 1, uintptr(unsafe.Pointer(win32.UTF16(text))))
}

func TestGDITextRendersGlyphs(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	text := newGDIText()
	defer text.close()

	dst := image.NewRGBA(image.Rect(0, 0, 200, 60))
	area := image.Rect(10, 10, 110, 40)
	style := render.TextStyle{Size: 16, Color: color.NRGBA{0, 0, 0, 255}, Opacity: 1}
	text.DrawText(dst, area, "Hello, a rather long label that must be cut short", style)

	inside, outside := 0, 0
	for y := 0; y < 60; y++ {
		for x := 0; x < 200; x++ {
			if dst.RGBAAt(x, y).A == 0 {
				continue
			}
			if image.Pt(x, y).In(area) {
				inside++
			} else {
				outside++
			}
		}
	}
	if inside == 0 {
		t.Error("no text pixels were drawn")
	}
	if outside != 0 {
		t.Errorf("%d text pixels were drawn outside the target rectangle", outside)
	}

	// Drawing the same string again must hit the cache.
	text.DrawText(dst, area, "Hello, a rather long label that must be cut short", style)
	if len(text.masks) != 1 {
		t.Errorf("mask cache has %d entries, want 1", len(text.masks))
	}
}

func TestMakeIcon(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	icon := makeIcon(32)
	if icon == 0 {
		t.Fatal("makeIcon returned no icon")
	}
	win32.DestroyIcon(icon)
}

func TestDeckWindowDocksToEdge(t *testing.T) {
	a := newTestApp(t)
	addNote(t, a, "first")
	addNote(t, a, "second")

	work := primaryDisplay().work
	w, h := a.deck.model.Size()
	r := win32.GetWindowRect(a.deck.hwnd)
	if int(r.Width()) != w || int(r.Height()) != h {
		t.Errorf("window is %dx%d, want the deck's %dx%d", r.Width(), r.Height(), w, h)
	}
	if r.Right != work.Right {
		t.Errorf("window right edge = %d, want the work area's %d", r.Right, work.Right)
	}
	if a.deck.dib == nil || a.deck.dib.Width != w || a.deck.dib.Height != h {
		t.Errorf("deck was not rendered at its window size")
	}

	a.setEdge(config.EdgeLeft)
	if r := win32.GetWindowRect(a.deck.hwnd); r.Left != work.Left {
		t.Errorf("after docking left, window left edge = %d, want %d", r.Left, work.Left)
	}
	saved, err := config.Load(a.paths.config)
	if err != nil || saved.Edge != config.EdgeLeft {
		t.Errorf("edge was not persisted: %+v, %v", saved, err)
	}
}

func TestDeckAnimatesWhileHeld(t *testing.T) {
	a := newTestApp(t)
	addNote(t, a, "first")

	a.deck.model.SetHeld(true)
	a.deck.wake()
	for range 40 {
		time.Sleep(10 * time.Millisecond)
		a.deck.tick()
	}
	f := a.deck.model.Frame()
	if f.Tabs[0].Open != 1 {
		t.Fatalf("tab did not finish expanding: %v", f.Tabs[0].Open)
	}

	// The expanded tab's pixels must have reached the window bitmap. GDI
	// stores them as BGRA.
	tab := f.Tabs[0].Rect
	x, y := int(tab.X+tab.W/2), int(tab.Y+tab.H-3)
	p := a.deck.dib.Pix[(y*a.deck.dib.Width+x)*4:]
	fill := render.Palette(note.Sky, false).Fill
	if p[0] != fill.B || p[1] != fill.G || p[2] != fill.R || p[3] != 255 {
		t.Errorf("pixel in the tab = BGRA %v, want fill %v", p[:4], fill)
	}
}

func TestEditorRoundTripAndAutosave(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "line one\nline two")

	a.openNote(n.ID)
	a.editor.setPinned(true)
	if !a.editor.showing(n.ID) || !win32.IsWindowVisible(a.editor.hwnd) {
		t.Fatal("editor is not showing the note")
	}
	if got := a.editor.text(); got != "line one\nline two" {
		t.Fatalf("editor text = %q", got)
	}
	if a.editor.dirty {
		t.Error("loading a note marked it as modified")
	}

	typeText(a, "\r\nline three ✓")
	if !a.editor.dirty {
		t.Fatal("typing did not mark the note as modified")
	}
	a.editor.flush()

	data, err := os.ReadFile(filepath.Join(a.store.Dir(), n.ID+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "line one\nline two\nline three ✓\n") {
		t.Errorf("saved file does not end with the edited text:\n%s", data)
	}

	a.editor.hide()
	if win32.IsWindowVisible(a.editor.hwnd) {
		t.Error("editor is still visible after hide")
	}
	if a.find(n.ID) == nil {
		t.Error("closing the editor dropped a note that has content")
	}
}

func TestEditorIsPlacedInsideWorkAreaBesideDeck(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "note")
	a.openNote(n.ID)
	a.editor.setPinned(true)

	work := primaryDisplay().work
	r := win32.GetWindowRect(a.editor.hwnd)
	deckRect := win32.GetWindowRect(a.deck.hwnd)
	if r.Left < work.Left || r.Top < work.Top || r.Right > work.Right || r.Bottom > work.Bottom {
		t.Errorf("editor %+v is outside the work area %+v", r, work)
	}
	if r.Right > a.deck.anchor(n.ID).Left {
		t.Errorf("editor %+v overlaps the note's tab (deck window %+v)", r, deckRect)
	}
}

func TestNewEmptyNoteIsDiscardedOnClose(t *testing.T) {
	a := newTestApp(t)
	a.newNote()
	a.editor.setPinned(true)
	if len(a.notes) != 1 || !a.editor.shown {
		t.Fatalf("newNote left %d notes, editor shown = %v", len(a.notes), a.editor.shown)
	}

	a.editor.hide()
	if len(a.notes) != 0 {
		t.Errorf("empty note was kept after closing the editor")
	}
	if entries, _ := os.ReadDir(a.store.Dir()); len(entries) != 0 {
		t.Errorf("empty note left %d files behind", len(entries))
	}
}

func TestNewNoteWithTextIsKept(t *testing.T) {
	a := newTestApp(t)
	a.newNote()
	a.editor.setPinned(true)
	typeText(a, "# Shopping\n- milk")
	a.editor.hide()

	if len(a.notes) != 1 {
		t.Fatalf("have %d notes, want 1", len(a.notes))
	}
	if got := a.notes[0].Title(); got != "Shopping" {
		t.Errorf("title = %q, want %q", got, "Shopping")
	}
	if items := a.deck.model.Items(); len(items) != 1 || items[0].Title != "Shopping" {
		t.Errorf("deck items = %+v", items)
	}
	listed, err := a.store.List()
	if err != nil || len(listed) != 1 {
		t.Errorf("store lists %d notes (%v), want 1", len(listed), err)
	}
}

func TestDeleteRestoreAndArchive(t *testing.T) {
	a := newTestApp(t)
	keep := addNote(t, a, "keep")
	gone := addNote(t, a, "gone")

	a.openNote(gone.ID)
	a.editor.setPinned(true)
	a.deleteNote(gone.ID)
	if a.editor.shown {
		t.Error("deleting the open note left the editor open")
	}
	if a.find(gone.ID) != nil || len(a.deck.model.Items()) != 1 {
		t.Fatal("deleted note is still present")
	}

	a.restoreDeleted()
	if n := a.find(gone.ID); n == nil || n.Body != "gone" {
		t.Fatalf("note was not restored: %+v", n)
	}
	if a.lastTrashed != "" {
		t.Error("restore did not clear the pending undo")
	}

	a.setArchived(keep.ID, true)
	if items := a.deck.model.Items(); len(items) != 1 || items[0].ID != gone.ID {
		t.Errorf("archived note still in the deck: %+v", items)
	}
	listed, _ := a.store.List()
	archived := 0
	for _, n := range listed {
		if n.Archived {
			archived++
		}
	}
	if archived != 1 {
		t.Errorf("%d notes are archived on disk, want 1", archived)
	}
	a.setArchived(keep.ID, false)
	if len(a.deck.model.Items()) != 2 {
		t.Error("unarchived note did not return to the deck")
	}
}

func TestColorChangeIsSavedAndShown(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "colour me")
	a.openNote(n.ID)
	a.editor.setPinned(true)

	a.cycleColor(a.editor)
	want := note.Sky.Next()
	if n.Color != want {
		t.Fatalf("colour = %q, want %q", n.Color, want)
	}
	if a.editor.sw != render.Palette(want, a.dark()) {
		t.Error("editor was not recoloured")
	}
	if a.deck.model.Items()[0].Color != want {
		t.Error("deck tab was not recoloured")
	}
	listed, _ := a.store.List()
	if len(listed) != 1 || listed[0].Color != want {
		t.Errorf("colour was not saved: %+v", listed)
	}
}

func TestStepNoteCyclesThroughDeck(t *testing.T) {
	a := newTestApp(t)
	first := addNote(t, a, "one")
	second := addNote(t, a, "two")

	a.openNote(first.ID)
	a.editor.setPinned(true)
	a.stepNote(a.editor, +1)
	if !a.editor.showing(second.ID) {
		t.Fatal("stepNote(+1) did not move to the next note")
	}
	a.stepNote(a.editor, +1)
	if !a.editor.showing(first.ID) {
		t.Error("stepNote(+1) did not wrap around")
	}
	a.stepNote(a.editor, -1)
	if !a.editor.showing(second.ID) {
		t.Error("stepNote(-1) did not wrap around backwards")
	}
}

func TestEditorHitTest(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "note")
	a.openNote(n.ID)
	a.editor.setPinned(true)

	r := win32.GetWindowRect(a.editor.hwnd)
	layout := a.editor.headerLayout()
	closeRect := layout.ButtonRect(render.ButtonClose)
	tests := []struct {
		name string
		x, y int32
		want uintptr
	}{
		{"top-left corner", r.Left + 1, r.Top + 1, win32.HTTOPLEFT},
		{"bottom-right corner", r.Right - 2, r.Bottom - 2, win32.HTBOTTOMRIGHT},
		{"left edge", r.Left + 1, r.Top + r.Height()/2, win32.HTLEFT},
		{"header", r.Left + r.Width()/2, r.Top + int32(layout.Height())/2, win32.HTCAPTION},
		{"close button", r.Left + int32(closeRect.X+closeRect.W/2), r.Top + int32(closeRect.Y+closeRect.H/2), win32.HTCLIENT},
		{"body", r.Left + r.Width()/2, r.Top + r.Height()/2, win32.HTCLIENT},
	}
	for _, tt := range tests {
		if got := a.editor.hitTest(win32.POINT{X: tt.x, Y: tt.y}); got != tt.want {
			t.Errorf("%s: hitTest = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestEditorShortcuts(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "")
	n.Body = "x" // keep the note from being discarded as empty
	a.openNote(n.ID)
	a.editor.setPinned(true)

	if !a.editor.rich {
		t.Skip("Markdown shortcuts need the rich edit control")
	}
	a.editor.setText("")
	a.editor.wrapSelection("**")
	typeText(a, "bold")
	if got := a.editor.text(); got != "**bold**" {
		t.Errorf("after wrapping and typing, text = %q, want %q", got, "**bold**")
	}

	if !a.editor.handleKey(win32.VK_ESCAPE) || a.editor.shown {
		t.Error("Esc did not close the editor")
	}
}

// clientPoint packs client coordinates into a mouse message's LPARAM.
func clientPoint(x, y float64) uintptr {
	return uintptr(uint16(int16(x))) | uintptr(uint16(int16(y)))<<16
}

func TestClickingTabsOpensAndClosesNotes(t *testing.T) {
	a := newTestApp(t)
	first := addNote(t, a, "one")
	second := addNote(t, a, "two")

	a.deck.model.SetHeld(true)
	for range 60 {
		a.deck.model.Advance(16 * time.Millisecond)
	}
	click := func(r rectCentre) {
		x, y := r.centre()
		win32.SendMessage(a.deck.hwnd, win32.WM_LBUTTONDOWN, 0, clientPoint(x, y))
		win32.SendMessage(a.deck.hwnd, win32.WM_LBUTTONUP, 0, clientPoint(x, y))
	}
	tab := func(i int) rectCentre { return rectCentre(a.deck.model.Frame().Tabs[i].Rect) }

	click(tab(0))
	if !a.editor.showing(first.ID) {
		t.Fatal("clicking the first tab did not open its note")
	}
	click(tab(1))
	if !a.editor.showing(second.ID) {
		t.Fatal("clicking another tab did not switch notes")
	}
	click(tab(1))
	if a.editor.shown {
		t.Fatal("clicking the open note's tab did not close it")
	}

	click(rectCentre(a.deck.model.Frame().Add))
	if !a.editor.shown || len(a.notes) != 3 {
		t.Fatalf("clicking + did not create a note: shown=%v notes=%d", a.editor.shown, len(a.notes))
	}
}

func TestHeaderButtons(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "note")
	a.openNote(n.ID)

	click := func(b render.HeaderButton) {
		x, y := rectCentre(a.editor.headerLayout().ButtonRect(b)).centre()
		win32.SendMessage(a.editor.hwnd, win32.WM_LBUTTONDOWN, 0, clientPoint(x, y))
		win32.SendMessage(a.editor.hwnd, win32.WM_LBUTTONUP, 0, clientPoint(x, y))
	}

	click(render.ButtonPin)
	if !a.editor.pinned {
		t.Fatal("pin button did not pin the editor")
	}
	click(render.ButtonColor)
	if n.Color != note.Sky.Next() {
		t.Errorf("colour button left the colour at %q", n.Color)
	}
	click(render.ButtonClose)
	if a.editor.shown {
		t.Error("close button did not close the editor")
	}
}

func TestUnpinnedEditorClosesWhenDeactivated(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "note")

	a.openNote(n.ID)
	a.editor.setPinned(true)
	win32.SendMessage(a.editor.hwnd, win32.WM_ACTIVATE, win32.WA_INACTIVE, 0)
	if !a.editor.shown {
		t.Fatal("a pinned editor closed when it lost focus")
	}

	a.editor.setPinned(false)
	a.menuOpen = true
	win32.SendMessage(a.editor.hwnd, win32.WM_ACTIVATE, win32.WA_INACTIVE, 0)
	if !a.editor.shown {
		t.Fatal("the editor closed while one of its menus was open")
	}

	a.menuOpen = false
	win32.SendMessage(a.editor.hwnd, win32.WM_ACTIVATE, win32.WA_INACTIVE, 0)
	if a.editor.shown {
		t.Fatal("an unpinned editor stayed open after losing focus")
	}
	a.deck.model.Pointer(0, 0, false)
	for range 200 {
		a.deck.model.Advance(16 * time.Millisecond)
	}
	if !a.deck.model.Idle() {
		t.Error("deck is still held open after the editor closed")
	}
}

func TestDeckHidesWhileSuppressed(t *testing.T) {
	a := newTestApp(t)
	addNote(t, a, "note")
	if !win32.IsWindowVisible(a.deck.hwnd) {
		t.Fatal("deck is not visible to begin with")
	}

	a.deck.setSuppressed(true)
	if win32.IsWindowVisible(a.deck.hwnd) {
		t.Error("deck stayed visible while suppressed")
	}
	a.deck.setSuppressed(false)
	if !win32.IsWindowVisible(a.deck.hwnd) {
		t.Error("deck did not return after suppression ended")
	}

	a.toggleDeck()
	a.deck.setSuppressed(true)
	a.deck.setSuppressed(false)
	if win32.IsWindowVisible(a.deck.hwnd) {
		t.Error("a deck the user hid reappeared when suppression ended")
	}
	a.toggleDeck()
	if !win32.IsWindowVisible(a.deck.hwnd) {
		t.Error("toggling did not bring the deck back")
	}
}

func TestMenuLabel(t *testing.T) {
	if got := menuLabel("R&D\tplan"); got != "R&&D plan" {
		t.Errorf("menuLabel = %q", got)
	}
	long := strings.Repeat("é", 100)
	if got := []rune(menuLabel(long)); len(got) != 41 {
		t.Errorf("long label has %d runes, want 40 plus an ellipsis", len(got))
	}
}

// rectCentre adapts a deck rectangle for click helpers.
type rectCentre deck.Rect

func (r rectCentre) centre() (float64, float64) {
	return r.X + r.W/2, r.Y + r.H/2
}
