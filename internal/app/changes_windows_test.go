package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// editExternally rewrites a note file as another program would.
func editExternally(t *testing.T, a *App, id, content string) {
	t.Helper()
	path := filepath.Join(a.store.Dir(), id+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func deckTitles(a *App) []string {
	var titles []string
	for _, it := range a.deck.model.Items() {
		titles = append(titles, it.Title)
	}
	return titles
}

func TestExternalEditOfClosedNoteReachesDeck(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "before")
	editExternally(t, a, n.ID, "---\ncolor: pink\n---\n\nafter\n")
	editExternally(t, a, "from elsewhere", "brand new")

	a.pickUpChanges()

	if got := a.find(n.ID); got != n || got.Body != "after" || got.Color != "pink" {
		t.Errorf("note after external edit: %+v", got)
	}
	titles := deckTitles(a)
	if len(titles) != 2 || titles[0] != "after" {
		t.Errorf("deck titles = %q, want the edited note and the new one", titles)
	}
}

func TestExternalEditOfOpenNoteReloadsEditor(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "before")
	a.openNote(n.ID)
	a.editor.setPinned(true)

	editExternally(t, a, n.ID, "after")
	a.pickUpChanges()

	if got := a.editor.text(); got != "after" {
		t.Errorf("editor text = %q, want the external version", got)
	}
	if a.editor.dirty {
		t.Error("reloading marked the note as modified")
	}
}

func TestEditsOnBothSidesKeepBoth(t *testing.T) {
	a := newTestApp(t)
	n := addNote(t, a, "start")
	a.openNote(n.ID)
	a.editor.setPinned(true)

	typeText(a, " typed here")
	editExternally(t, a, n.ID, "start edited elsewhere")
	a.pickUpChanges()

	if got := a.editor.text(); got != "start typed here" {
		t.Errorf("editor text = %q; the user's typing must survive", got)
	}
	var bodies []string
	for _, other := range a.notes {
		bodies = append(bodies, other.Body)
	}
	if len(a.notes) != 2 || a.notes[1].Body != "start edited elsewhere" {
		t.Errorf("notes = %q, want ours plus the external version as a conflict note", bodies)
	}
}

func TestExternalDeleteRemovesNote(t *testing.T) {
	a := newTestApp(t)
	keep := addNote(t, a, "keep")
	gone := addNote(t, a, "gone")
	a.openNote(gone.ID)
	a.editor.setPinned(true)

	if err := os.Remove(filepath.Join(a.store.Dir(), gone.ID+".md")); err != nil {
		t.Fatal(err)
	}
	a.pickUpChanges()

	if a.find(gone.ID) != nil {
		t.Error("note deleted elsewhere is still in the app")
	}
	if a.editor.shown {
		t.Error("editor still shows a note that was deleted elsewhere")
	}
	if titles := deckTitles(a); len(titles) != 1 || titles[0] != keep.Body {
		t.Errorf("deck titles = %q", titles)
	}
	if _, err := os.Stat(filepath.Join(a.store.Dir(), gone.ID+".md")); err == nil {
		t.Error("closing the editor wrote the deleted note back to disk")
	}
}
