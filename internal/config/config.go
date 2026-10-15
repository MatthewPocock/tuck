// Package config loads and saves the user's settings.
//
// Settings live in a small JSON file that is safe to edit by hand; anything
// missing or out of range falls back to its default.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/MatthewPocock/tuck/internal/atomicfile"
)

// Edge is the side of the screen the deck is docked to.
type Edge string

// The supported docking edges.
const (
	EdgeLeft  Edge = "left"
	EdgeRight Edge = "right"
)

// Theme selects the editor's colour scheme.
type Theme string

// The supported themes. ThemeSystem follows the Windows app mode setting.
const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// Config holds every user-configurable setting.
type Config struct {
	// Edge is the screen edge the deck docks to.
	Edge Edge `json:"edge"`
	// Position is where the deck sits along the edge, from 0 (top) to 1
	// (bottom).
	Position float64 `json:"position"`
	// Theme selects the editor colour scheme.
	Theme Theme `json:"theme"`
	// EditorWidth and EditorHeight are the note editor's size in
	// device-independent pixels.
	EditorWidth  int `json:"editor_width"`
	EditorHeight int `json:"editor_height"`
	// FontSize is the editor's font size in points.
	FontSize int `json:"font_size"`
	// NotesDir overrides where notes are stored. Empty means the default
	// location next to the config file.
	NotesDir string `json:"notes_dir,omitempty"`
	// SinkDoneTasks moves a task to the end of its list when it is ticked
	// off, so that the open tasks stay together at the top.
	SinkDoneTasks bool `json:"sink_done_tasks"`
	// Hotkeys are the global keyboard shortcuts. An empty shortcut is
	// disabled.
	Hotkeys Hotkeys `json:"hotkeys"`
}

// Hotkeys lists the global shortcuts, written like "Ctrl+Alt+N".
type Hotkeys struct {
	// NewNote creates a note and opens it for editing.
	NewNote string `json:"new_note"`
	// ToggleDeck hides or shows the deck.
	ToggleDeck string `json:"toggle_deck"`
	// Search opens the search window.
	Search string `json:"search"`
}

// Limits for the editor size, in device-independent pixels.
const (
	MinEditorWidth  = 240
	MinEditorHeight = 200
	maxEditorSize   = 4000
)

// Default returns the settings used on first run.
func Default() Config {
	return Config{
		Edge:          EdgeRight,
		Position:      0.5,
		Theme:         ThemeSystem,
		EditorWidth:   360,
		EditorHeight:  420,
		FontSize:      11,
		SinkDoneTasks: true,
		Hotkeys: Hotkeys{
			NewNote:    "Ctrl+Alt+N",
			ToggleDeck: "Ctrl+Alt+H",
			Search:     "Ctrl+Alt+F",
		},
	}
}

// Load reads the config at path. A missing file yields the defaults and no
// error. If the file exists but cannot be parsed, Load returns the defaults
// together with the error so the caller can report it and carry on.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("config: read: %w", err)
	}
	// Unmarshal onto the defaults so absent keys keep their default value.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("config: parse %s: %w", path, err)
	}
	cfg.normalize()
	return cfg, nil
}

// Save writes the config to path atomically.
func (c Config) Save(path string) error {
	c.normalize()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	if err := atomicfile.Write(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("config: save: %w", err)
	}
	return nil
}

// normalize replaces invalid values with defaults and clamps ranges.
func (c *Config) normalize() {
	def := Default()
	if c.Edge != EdgeLeft && c.Edge != EdgeRight {
		c.Edge = def.Edge
	}
	if c.Theme != ThemeSystem && c.Theme != ThemeLight && c.Theme != ThemeDark {
		c.Theme = def.Theme
	}
	c.Position = clamp(c.Position, 0, 1)
	c.EditorWidth = int(clamp(float64(c.EditorWidth), MinEditorWidth, maxEditorSize))
	c.EditorHeight = int(clamp(float64(c.EditorHeight), MinEditorHeight, maxEditorSize))
	if c.FontSize < 8 || c.FontSize > 32 {
		c.FontSize = def.FontSize
	}
}

func clamp(v, lo, hi float64) float64 {
	// The comparison order also maps NaN to lo.
	if !(v >= lo) {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
