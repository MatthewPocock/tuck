package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/MatthewPocock/tuck/internal/atomicfile"
)

// Session is what Tuck restores when it starts again that is not a setting:
// which notes are pinned to the screen, and where. It is kept apart from the
// config because window positions only make sense on one computer, while the
// config may be copied between them.
type Session struct {
	Pinned []PinnedNote `json:"pinned"`
}

// PinnedNote is a note pinned to the screen, with its window rectangle in
// physical pixels. A folded note shows only its title bar; Height is then
// its height unfolded.
type PinnedNote struct {
	ID     string `json:"id"`
	X      int32  `json:"x"`
	Y      int32  `json:"y"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
	Folded bool   `json:"folded,omitempty"`
}

// LoadSession reads the session at path. A missing or unreadable file yields
// an empty session: losing it only means the pinned notes are not reopened.
func LoadSession(path string) (Session, error) {
	var s Session
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("session: read: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}, fmt.Errorf("session: parse %s: %w", path, err)
	}
	valid := s.Pinned[:0]
	for _, p := range s.Pinned {
		if p.ID != "" && p.Width > 0 && p.Height > 0 {
			valid = append(valid, p)
		}
	}
	s.Pinned = valid
	return s, nil
}

// Save writes the session to path atomically.
func (s Session) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("session: encode: %w", err)
	}
	if err := atomicfile.Write(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("session: save: %w", err)
	}
	return nil
}
