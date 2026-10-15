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
	"time"

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
//
// It remembers the version of every file it has read or written, so that it
// can tell which files other programs — a sync client, another computer, a
// text editor — changed behind its back (see Changes), and so that a save
// never silently overwrites such a change (see Save).
type Store struct {
	dir      string
	versions map[string]version
}

// version identifies one state of a note file.
type version struct {
	modified time.Time
	size     int64
}

func versionOf(info fs.FileInfo) version {
	return version{info.ModTime(), info.Size()}
}

// Change describes a note that changed on disk without going through the
// store.
type Change struct {
	// ID is the note's identifier.
	ID string
	// Note is the note as it now is on disk, or nil if its file is gone.
	Note *note.Note
}

// Open returns a Store rooted at dir, creating the directory if needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: create notes directory: %w", err)
	}
	return &Store{dir: dir, versions: map[string]version{}}, nil
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
	// Stat before reading: if the file changes in between, the recorded
	// version is the older one and the next Changes call picks it up again.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("store: read note: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("store: read note: %w", err)
	}
	base := filepath.Base(path)
	n := note.Decode(base[:len(base)-len(filepath.Ext(base))], data)
	s.versions[n.ID] = versionOf(info)

	mod := info.ModTime().UTC()
	if n.Created.IsZero() {
		n.Created = mod
	}
	if n.Updated.IsZero() {
		n.Updated = mod
	}
	return n, nil
}

// Save writes the note to disk, replacing any previous version atomically.
//
// If the file was changed by another program since the store last read or
// wrote it, that version is not lost: it is first kept as a separate conflict
// note, whose ID is returned. conflict is empty otherwise.
func (s *Store) Save(n *note.Note) (conflict string, err error) {
	path, err := s.path(n.ID)
	if err != nil {
		return "", err
	}
	if s.changedOnDisk(n.ID, path) {
		if conflict, err = s.keepConflict(n.ID, path); err != nil {
			return "", err
		}
	}
	if err := atomicfile.Write(path, n.Encode(), 0o644); err != nil {
		return conflict, fmt.Errorf("store: save note %q: %w", n.ID, err)
	}
	s.record(n.ID, path)
	return conflict, nil
}

// changedOnDisk reports whether a note's file exists in a version the store
// has not seen, i.e. one another program wrote.
func (s *Store) changedOnDisk(id, path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	known, ok := s.versions[id]
	return !ok || known != versionOf(info)
}

// keepConflict moves the file of a note aside under a new ID, so that it
// shows up as a note of its own.
func (s *Store) keepConflict(id, path string) (string, error) {
	conflict := conflictID(id, time.Now())
	to, err := s.path(conflict)
	if err != nil {
		return "", err
	}
	if err := atomicfile.Rename(path, to); err != nil {
		return "", fmt.Errorf("store: keep conflicting version of %q: %w", id, err)
	}
	// Deliberately not recorded: the next Changes call reports the copy as
	// a new note, which is how it reaches the deck.
	return conflict, nil
}

// conflictID names the copy of a note that was changed in two places at
// once, after the convention sync clients use for their own conflicts.
func conflictID(id string, now time.Time) string {
	return id + " (conflict " + now.Format("2006-01-02 150405") + ")"
}

// record remembers the version of a file the store just wrote or moved.
func (s *Store) record(id, path string) {
	if info, err := os.Stat(path); err == nil {
		s.versions[id] = versionOf(info)
	}
}

// Changes reports the notes that other programs added, modified or deleted
// since the store last read or wrote them, and remembers their new versions.
// Calling it again without further changes reports nothing.
func (s *Store) Changes() ([]Change, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("store: list notes: %w", err)
	}
	var (
		changes []Change
		errs    []error
		seen    = map[string]bool{}
	)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ext) {
			continue
		}
		id := name[:len(name)-len(filepath.Ext(name))]
		seen[id] = true
		info, err := e.Info()
		if err != nil {
			continue // removed while listing; the next call notices
		}
		if known, ok := s.versions[id]; ok && known == versionOf(info) {
			continue
		}
		n, err := s.load(filepath.Join(s.dir, name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		changes = append(changes, Change{ID: id, Note: n})
	}
	for id := range s.versions {
		if !seen[id] {
			delete(s.versions, id)
			changes = append(changes, Change{ID: id})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].ID < changes[j].ID })
	return changes, errors.Join(errs...)
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
	err = atomicfile.Rename(path, filepath.Join(trash, id+ext))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: trash note %q: %w", id, err)
	}
	delete(s.versions, id)
	return nil
}

// Restore moves a previously trashed note back into the store and returns it.
func (s *Store) Restore(id string) (*note.Note, error) {
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	if err := atomicfile.Rename(filepath.Join(s.dir, trashDir, id+ext), path); err != nil {
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
	delete(s.versions, id)
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
