package app

import (
	"testing"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/win32"
)

func TestPinnedNoteStaysWhenAnotherOpens(t *testing.T) {
	a := newTestApp(t)
	first := addNote(t, a, "first")
	second := addNote(t, a, "second")

	a.openNote(first.ID)
	a.editor.setPinned(true)
	pinned := a.editor
	where := win32.GetWindowRect(pinned.hwnd)

	a.openNote(second.ID)

	if a.editorFor(first.ID) != pinned || !win32.IsWindowVisible(pinned.hwnd) {
		t.Fatal("opening another note replaced the pinned one")
	}
	if got := win32.GetWindowRect(pinned.hwnd); got != where {
		t.Errorf("pinned note moved from %+v to %+v", where, got)
	}
	other := a.editorFor(second.ID)
	if other == nil || other == pinned || !win32.IsWindowVisible(other.hwnd) {
		t.Fatal("the second note did not get a window of its own")
	}
	if other.pinned {
		t.Error("the new window started out pinned")
	}

	// A new note gets a window too, leaving both of the others alone.
	a.editor.setPinned(true)
	a.newNote()
	if len(a.pinned) != 2 || !a.editor.shown || a.editor.note == nil || !a.editor.note.Empty() {
		t.Errorf("after a new note: %d pinned windows, quick editor showing %+v", len(a.pinned), a.editor.note)
	}
}

func TestClickingPinnedNotesTabKeepsItOpen(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "pinned")
	a.openNote(n.ID)
	a.editor.setPinned(true)

	a.toggleNote(n.ID)
	if !a.editor.showing(n.ID) {
		t.Error("clicking the tab of a pinned note closed it")
	}
	a.editor.setPinned(false)
	a.toggleNote(n.ID)
	if a.editor.shown {
		t.Error("clicking the tab of an unpinned open note did not close it")
	}
}

func TestClosingAndDeletingPinnedNotes(t *testing.T) {
	a := newTestApp(t)
	keep := addNote(t, a, "keep")
	gone := addNote(t, a, "gone")
	a.openNote(keep.ID)
	a.editor.setPinned(true)
	a.openNote(gone.ID)
	a.editor.setPinned(true)
	a.openNote(keep.ID) // already open: brought to the front, nothing new
	if len(a.pinned) != 1 {
		t.Fatalf("%d retired windows, want 1", len(a.pinned))
	}

	a.deleteNote(gone.ID)
	if a.editorFor(gone.ID) != nil {
		t.Error("deleting a pinned note left its window open")
	}
	e := a.editorFor(keep.ID)
	e.hide()
	if len(a.pinned) != 0 || a.editorFor(keep.ID) != nil {
		t.Errorf("closed pinned window is still tracked: %d", len(a.pinned))
	}
}

func TestPinnedNotesReturnAfterRestart(t *testing.T) {
	dir := t.TempDir()
	a := newTestAppIn(t, dir)
	one, two := addNote(t, a, "one"), addNote(t, a, "two")
	a.openNote(one.ID)
	a.editor.setPinned(true)
	win32.SetWindowPos(a.editor.hwnd, 0, 100, 120, 400, 300, win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
	a.openNote(two.ID)
	a.editor.setPinned(true)
	a.saveSession()

	s, err := config.LoadSession(a.paths.session)
	if err != nil || len(s.Pinned) != 2 {
		t.Fatalf("saved session: %+v, %v", s, err)
	}

	b := newTestAppIn(t, dir)
	notes, err := b.store.List()
	if err != nil {
		t.Fatal(err)
	}
	b.notes = notes
	b.syncDeck()
	b.restoreSession()

	e := b.editorFor(one.ID)
	if e == nil || !e.pinned || !win32.IsWindowVisible(e.hwnd) {
		t.Fatal("the first pinned note did not come back")
	}
	if got := win32.GetWindowRect(e.hwnd); got != (win32.RECT{Left: 100, Top: 120, Right: 500, Bottom: 420}) {
		t.Errorf("restored at %+v", got)
	}
	if b.editorFor(two.ID) == nil {
		t.Error("the second pinned note did not come back")
	}
	if b.editor.shown {
		t.Error("restoring opened the quick editor too")
	}
}

func TestAltF4ClosesOnlyThatNote(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "pinned")
	a.openNote(n.ID)
	a.editor.setPinned(true)

	win32.SendMessage(a.editor.hwnd, win32.WM_SYSCOMMAND, win32.SC_CLOSE, 0)
	if a.editor.shown {
		t.Error("Alt+F4 did not close the note")
	}
	if !win32.IsWindowVisible(a.deck.hwnd) {
		t.Error("Alt+F4 on a note affected the deck")
	}
}
