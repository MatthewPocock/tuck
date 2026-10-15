// Package store persists notes as individual Markdown files in a directory.
//
// The directory layout is intentionally simple so it can be synced, backed up
// or edited with other tools:
//
//	notes/
//	  20261014-093000-3fa9c1.md
//	  shopping list.md
//	  .trash/
//	    20261001-120000-9bc2aa.md
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MatthewPocock/tuck/internal/atomicfile"
	"github.com/MatthewPocock/tuck/internal/note"
)

const (
	ext      = ".md"
	trashDir = ".trash"
)

// ErrInvalidID is returned when a note ID cannot be used as a file name.
var ErrInvalidID = errors.New("store: invalid note id")

// Store reads and writes notes in a single directory.
type Store struct {
	dir string
}

// Open returns a Store rooted at dir, creating the directory if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: create notes directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the directory the notes are stored in.
func (s *Store) Dir() string { return s.dir }

// List loads every note in the store, oldest first. Files that cannot be read
// are skipped and reported through the returned error, alongside the notes
// that did load.
func (s *Store) List() ([]*note.Note, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("store: list notes: %w", err)
	}

	var (
		notes []*note.Note
		errs  []error
	)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ext) {
			continue
		}
		n, err := s.load(filepath.Join(s.dir, name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		notes = append(notes, n)
	}

	sort.SliceStable(notes, func(i, j int) bool {
		if !notes[i].Created.Equal(notes[j].Created) {
			return notes[i].Created.Before(notes[j].Created)
		}
		return notes[i].ID < notes[j].ID
	})
	return notes, errors.Join(errs...)
}

// load reads one note file, filling in timestamps from the file's
// modification time when the front matter does not provide them.
func (s *Store) load(path string) (*note.Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("store: read note: %w", err)
	}
	base := filepath.Base(path)
	n := note.Decode(base[:len(base)-len(filepath.Ext(base))], data)

	if n.Created.IsZero() || n.Updated.IsZero() {
		if info, err := os.Stat(path); err == nil {
			mod := info.ModTime().UTC()
			if n.Created.IsZero() {
				n.Created = mod
			}
			if n.Updated.IsZero() {
				n.Updated = mod
			}
		}
	}
	return n, nil
}

// Save writes the note to disk, replacing any previous version atomically.
func (s *Store) Save(n *note.Note) error {
	path, err := s.path(n.ID)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(path, n.Encode(), 0o644); err != nil {
		return fmt.Errorf("store: save note %q: %w", n.ID, err)
	}
	return nil
}

// Trash moves a note into the trash folder, from which Restore can bring it
// back. Trashing a note that does not exist is not an error.
func (s *Store) Trash(id string) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	trash := filepath.Join(s.dir, trashDir)
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return fmt.Errorf("store: create trash: %w", err)
	}
	err = os.Rename(path, filepath.Join(trash, id+ext))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: trash note %q: %w", id, err)
	}
	return nil
}

// Restore moves a previously trashed note back into the store and returns it.
func (s *Store) Restore(id string) (*note.Note, error) {
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(filepath.Join(s.dir, trashDir, id+ext), path); err != nil {
		return nil, fmt.Errorf("store: restore note %q: %w", id, err)
	}
	return s.load(path)
}

// Remove permanently deletes a note. Removing a note that does not exist is
// not an error.
func (s *Store) Remove(id string) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: remove note %q: %w", id, err)
	}
	return nil
}

// path maps a note ID to its file, rejecting IDs that would escape the notes
// directory or collide with the store's own hidden files.
func (s *Store) path(id string) (string, error) {
	if id == "" || strings.HasPrefix(id, ".") || strings.ContainsAny(id, `/\:`) || filepath.Base(id) != id {
		return "", fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return filepath.Join(s.dir, id+ext), nil
}
