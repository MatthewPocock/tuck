package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MatthewPocock/tuck/internal/note"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "notes"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveAndList(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

	newer := note.New(base.Add(time.Hour), note.Pink)
	newer.Body = "newer"
	older := note.New(base, note.Sky)
	older.Body = "older"

	for _, n := range []*note.Note{newer, older} {
		if _, err := s.Save(n); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("List returned %d notes, want 2", len(got))
	}
	if got[0].Body != "older" || got[1].Body != "newer" {
		t.Errorf("notes not sorted oldest first: %q, %q", got[0].Body, got[1].Body)
	}
	if got[0].Color != note.Sky {
		t.Errorf("Color = %q, want %q", got[0].Color, note.Sky)
	}
}

func TestListIgnoresForeignFilesAndLoadsPlainMarkdown(t *testing.T) {
	s := openTemp(t)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(s.Dir(), name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("shopping list.md", "milk\neggs")
	write("notes.txt", "not a note")
	write(".hidden.md", "hidden")
	if err := os.Mkdir(filepath.Join(s.Dir(), "folder.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("List returned %d notes, want 1", len(got))
	}
	n := got[0]
	if n.ID != "shopping list" || n.Body != "milk\neggs" {
		t.Errorf("got note %q with body %q", n.ID, n.Body)
	}
	if n.Created.IsZero() || n.Updated.IsZero() {
		t.Error("timestamps were not filled in from the file's modification time")
	}
}

func TestTrashAndRestore(t *testing.T) {
	s := openTemp(t)
	n := note.New(time.Now(), note.Yellow)
	n.Body = "keep me"
	if _, err := s.Save(n); err != nil {
		t.Fatal(err)
	}

	if err := s.Trash(n.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.List(); len(got) != 0 {
		t.Fatalf("trashed note is still listed")
	}
	if err := s.Trash(n.ID); err != nil {
		t.Errorf("trashing a missing note returned %v, want nil", err)
	}

	restored, err := s.Restore(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Body != "keep me" {
		t.Errorf("restored body = %q", restored.Body)
	}
	if got, _ := s.List(); len(got) != 1 {
		t.Errorf("restored note is not listed")
	}
}

func TestRemove(t *testing.T) {
	s := openTemp(t)
	n := note.New(time.Now(), note.Yellow)
	if _, err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(n.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(n.ID); err != nil {
		t.Errorf("removing a missing note returned %v, want nil", err)
	}
	if got, _ := s.List(); len(got) != 0 {
		t.Errorf("removed note is still listed")
	}
}

func TestInvalidIDs(t *testing.T) {
	s := openTemp(t)
	for _, id := range []string{"", ".", "..", ".trash", "../escape", `..\escape`, "a/b", "c:evil"} {
		if _, err := s.Save(&note.Note{ID: id}); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Save(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}

// writeExternally changes a note file the way another program would, making
// sure the modification time moves on even on coarse-grained file systems.
func writeExternally(t *testing.T, s *Store, id, content string) {
	t.Helper()
	path := filepath.Join(s.Dir(), id+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestChangesReportsOnlyExternalEdits(t *testing.T) {
	s := openTemp(t)
	n := note.New(time.Now(), note.Yellow)
	n.Body = "mine"
	if _, err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	if changes, err := s.Changes(); err != nil || len(changes) != 0 {
		t.Fatalf("Changes after our own save = %v, %v; want none", changes, err)
	}

	writeExternally(t, s, n.ID, "---\ncolor: mint\n---\n\nfrom another computer\n")
	writeExternally(t, s, "dropped in", "a new file")
	changes, err := s.Changes()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want 2: %+v", len(changes), changes)
	}
	byID := map[string]*note.Note{}
	for _, c := range changes {
		byID[c.ID] = c.Note
	}
	if got := byID[n.ID]; got == nil || got.Body != "from another computer" || got.Color != note.Mint {
		t.Errorf("modified note reported as %+v", got)
	}
	if got := byID["dropped in"]; got == nil || got.Body != "a new file" {
		t.Errorf("new note reported as %+v", got)
	}
	if changes, _ := s.Changes(); len(changes) != 0 {
		t.Errorf("changes reported twice: %+v", changes)
	}

	if err := os.Remove(filepath.Join(s.Dir(), "dropped in.md")); err != nil {
		t.Fatal(err)
	}
	changes, _ = s.Changes()
	if len(changes) != 1 || changes[0].ID != "dropped in" || changes[0].Note != nil {
		t.Errorf("deletion reported as %+v, want a single change with no note", changes)
	}
}

func TestChangesIgnoresTrashAndRemove(t *testing.T) {
	s := openTemp(t)
	a, b := note.New(time.Now(), note.Yellow), note.New(time.Now().Add(time.Second), note.Pink)
	for _, n := range []*note.Note{a, b} {
		n.Body = "text"
		if _, err := s.Save(n); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Trash(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(b.ID); err != nil {
		t.Fatal(err)
	}
	if changes, _ := s.Changes(); len(changes) != 0 {
		t.Errorf("our own trash and remove reported as external changes: %+v", changes)
	}
	if _, err := s.Restore(a.ID); err != nil {
		t.Fatal(err)
	}
	if changes, _ := s.Changes(); len(changes) != 0 {
		t.Errorf("our own restore reported as an external change: %+v", changes)
	}
}

func TestSaveKeepsExternalVersionAsConflict(t *testing.T) {
	s := openTemp(t)
	n := note.New(time.Now(), note.Yellow)
	n.Body = "first"
	if _, err := s.Save(n); err != nil {
		t.Fatal(err)
	}
	// Another computer edits the note before we noticed...
	writeExternally(t, s, n.ID, "edited elsewhere")
	// ...and we save our own edit on top of it.
	n.Body = "edited here"
	conflict, err := s.Save(n)
	if err != nil {
		t.Fatal(err)
	}
	if conflict == "" {
		t.Fatal("Save overwrote an external change without keeping it")
	}
	// The copy reaches the app as a new note.
	changes, _ := s.Changes()
	if len(changes) != 1 || changes[0].ID != conflict {
		t.Errorf("Changes after a conflict = %+v, want just the conflict note", changes)
	}

	notes, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string]string{}
	for _, got := range notes {
		bodies[got.ID] = got.Body
	}
	if bodies[n.ID] != "edited here" {
		t.Errorf("note body = %q, want our edit", bodies[n.ID])
	}
	if bodies[conflict] != "edited elsewhere" {
		t.Errorf("conflict note %q = %q, want the external edit; all notes: %v", conflict, bodies[conflict], bodies)
	}

	// Our next save goes through without another conflict.
	n.Body = "edited here again"
	if conflict, err := s.Save(n); err != nil || conflict != "" {
		t.Errorf("second Save = %q, %v; want no conflict", conflict, err)
	}
}

func TestSaveOfNewNoteOverExistingForeignFileKeepsIt(t *testing.T) {
	s := openTemp(t)
	writeExternally(t, s, "shared", "theirs")
	n := &note.Note{ID: "shared", Color: note.Yellow, Body: "ours"}
	conflict, err := s.Save(n)
	if err != nil {
		t.Fatal(err)
	}
	if conflict == "" {
		t.Error("a file the store never read was overwritten without keeping it")
	}
}
