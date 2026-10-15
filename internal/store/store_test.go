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
		if err := s.Save(n); err != nil {
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
	if err := s.Save(n); err != nil {
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
	if err := s.Save(n); err != nil {
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
		if err := s.Save(&note.Note{ID: id}); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Save(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}
