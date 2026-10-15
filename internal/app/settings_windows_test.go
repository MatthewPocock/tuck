package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/hotkey"
	"github.com/MatthewPocock/tuck/internal/settings"
	"github.com/MatthewPocock/tuck/internal/win32"
)

func openSettings(t *testing.T) (*App, *settingsWindow) {
	t.Helper()
	a := newTestApp(t)
	a.showSettings()
	if a.settings == nil || !win32.IsWindowVisible(a.settings.hwnd) {
		t.Fatal("settings window did not open")
	}
	t.Cleanup(func() {
		a.unregisterHotkeys()
		win32.DestroyWindow(a.settings.hwnd)
	})
	return a, a.settings
}

// clickSetting clicks a control, or option i of a segmented control, the way
// the mouse would.
func clickSetting(t *testing.T, w *settingsWindow, c settings.Control, option int) {
	t.Helper()
	it, ok := w.layout.Find(c)
	if !ok {
		t.Fatalf("no control %v", c)
	}
	r := it.Rect
	if option >= 0 {
		r = it.Segments[option]
	}
	lp := clientPoint(r.X+r.W/2, r.Y+r.H/2)
	win32.SendMessage(w.hwnd, win32.WM_LBUTTONDOWN, 0, lp)
	win32.SendMessage(w.hwnd, win32.WM_LBUTTONUP, 0, lp)
}

func savedConfig(t *testing.T, a *App) config.Config {
	t.Helper()
	cfg, err := config.Load(a.paths.config)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestSettingsChangesApplyAndSave(t *testing.T) {
	a, w := openSettings(t)

	clickSetting(t, w, settings.Theme, 2)
	clickSetting(t, w, settings.Edge, 0)
	clickSetting(t, w, settings.Position, 0)
	clickSetting(t, w, settings.FontLarger, -1)

	if a.cfg.Theme != config.ThemeDark || a.cfg.Edge != config.EdgeLeft || a.cfg.Position != settings.PositionPresets[0] {
		t.Errorf("config after clicks: %+v", a.cfg)
	}
	if a.cfg.FontSize != config.Default().FontSize+1 {
		t.Errorf("font size = %d", a.cfg.FontSize)
	}
	if saved := savedConfig(t, a); saved.Theme != config.ThemeDark || saved.Edge != config.EdgeLeft {
		t.Errorf("saved config: %+v", saved)
	}
	if w.state.Selected(settings.Theme) != 2 {
		t.Error("the window does not show the new theme as selected")
	}

	clickSetting(t, w, settings.Close, -1)
	if win32.IsWindowVisible(w.hwnd) {
		t.Error("close button left the window open")
	}
}

func TestRecordingAShortcut(t *testing.T) {
	a, w := openSettings(t)
	clickSetting(t, w, settings.NewNoteKey, -1)
	if w.state.Capturing != settings.NewNoteKey {
		t.Fatal("clicking the field did not start recording")
	}

	// An unusual combination, so the test does not collide with other apps.
	w.captureKey(0x10, hotkey.ModCtrl) // Shift alone: keep waiting
	if w.state.Capturing != settings.NewNoteKey {
		t.Fatal("a modifier on its own ended the recording")
	}
	w.captureKey(0x7A, hotkey.ModCtrl|hotkey.ModAlt|hotkey.ModShift) // F11
	if w.state.Capturing != settings.None {
		t.Fatal("recording did not end")
	}
	if got := a.cfg.Hotkeys.NewNote; got != "Ctrl+Alt+Shift+F11" && !w.state.Problem {
		t.Errorf("new note shortcut = %q, message %q", got, w.state.Message)
	}
	if saved := savedConfig(t, a); saved.Hotkeys.NewNote != a.cfg.Hotkeys.NewNote {
		t.Errorf("saved shortcut %q, in memory %q", saved.Hotkeys.NewNote, a.cfg.Hotkeys.NewNote)
	}
}

func TestRecordingRejectsBadShortcuts(t *testing.T) {
	a, w := openSettings(t)
	before := a.cfg.Hotkeys

	w.startCapture(settings.NewNoteKey)
	w.captureKey('K', 0)
	if !w.state.Problem || a.cfg.Hotkeys != before {
		t.Errorf("a key without modifiers was accepted: %+v, %q", a.cfg.Hotkeys, w.state.Message)
	}

	w.startCapture(settings.NewNoteKey)
	other, _ := hotkey.Parse(before.ToggleDeck)
	w.captureKey(other.Key, other.Mods)
	if !w.state.Problem || a.cfg.Hotkeys != before {
		t.Errorf("the other shortcut's keys were accepted: %+v", a.cfg.Hotkeys)
	}

	w.startCapture(settings.ToggleDeckKey)
	w.captureKey(win32.VK_ESCAPE, 0)
	if w.state.Capturing != settings.None || a.cfg.Hotkeys != before {
		t.Error("Esc did not cancel recording without changes")
	}

	w.startCapture(settings.ToggleDeckKey)
	w.captureKey(win32.VK_BACK, 0)
	if a.cfg.Hotkeys.ToggleDeck != "" {
		t.Errorf("Backspace did not turn the shortcut off: %q", a.cfg.Hotkeys.ToggleDeck)
	}
}

func TestSwitchingNotesFolder(t *testing.T) {
	a, _ := openSettings(t)
	addNote(t, a, "in the old folder")

	dir := filepath.Join(t.TempDir(), "synced")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "elsewhere.md"), []byte("in the new folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.setNotesDir(dir)

	if a.store.Dir() != dir || a.cfg.NotesDir != dir || savedConfig(t, a).NotesDir != dir {
		t.Fatalf("notes folder not switched: store %q, config %q", a.store.Dir(), a.cfg.NotesDir)
	}
	if titles := deckTitles(a); len(titles) != 1 || titles[0] != "in the new folder" {
		t.Errorf("deck shows %q", titles)
	}

	a.setNotesDir(a.paths.notes)
	if a.cfg.NotesDir != "" {
		t.Errorf("returning to the default folder left notes_dir = %q", a.cfg.NotesDir)
	}
}
