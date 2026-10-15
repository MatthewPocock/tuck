package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Default() {
		t.Errorf("Load = %+v, want defaults", cfg)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Default()
	want.Edge = EdgeLeft
	want.Position = 0.2
	want.Theme = ThemeDark
	want.Hotkeys.NewNote = "Win+Shift+N"

	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestLoadPartialFileKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"edge": "left"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.Edge = EdgeLeft
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestLoadNormalizesInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"edge": "top", "position": 7, "theme": "neon", "editor_width": 1, "editor_height": 99999, "font_size": 300}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	def := Default()
	if got.Edge != def.Edge || got.Theme != def.Theme || got.FontSize != def.FontSize {
		t.Errorf("invalid enum values were not reset: %+v", got)
	}
	if got.Position != 1 {
		t.Errorf("Position = %v, want it clamped to 1", got.Position)
	}
	if got.EditorWidth != MinEditorWidth || got.EditorHeight != maxEditorSize {
		t.Errorf("editor size = %dx%d, want it clamped", got.EditorWidth, got.EditorHeight)
	}
}

func TestLoadCorruptFileReturnsDefaultsAndError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err == nil {
		t.Fatal("Load of a corrupt file returned no error")
	}
	if got != Default() {
		t.Errorf("Load = %+v, want defaults", got)
	}
}
