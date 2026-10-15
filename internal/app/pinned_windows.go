package app

import (
	"log"
	"slices"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// msgDiscard asks a retired editor window to destroy itself. Posting it lets
// the window finish handling the message during which it was closed first.
const msgDiscard = win32.WM_APP + 3

// The app can show several notes at once. a.editor is the quick editor that
// opens beside the deck and tucks itself away when the user turns to
// something else. Pinning it keeps it on screen; the next note the user opens
// then gets a fresh quick editor, and the pinned one joins a.pinned, where it
// stays until closed.

// editors returns every editor window, the quick one first.
func (a *App) editors() []*editorWindow {
	return append([]*editorWindow{a.editor}, a.pinned...)
}

// editorFor returns the editor showing a note, or nil.
func (a *App) editorFor(id string) *editorWindow {
	for _, e := range a.editors() {
		if e.showing(id) {
			return e
		}
	}
	return nil
}

// editorOf returns the editor a window belongs to, or nil.
func (a *App) editorOf(hwnd win32.HWND) *editorWindow {
	for _, e := range a.editors() {
		if hwnd == e.hwnd || hwnd == e.edit {
			return e
		}
	}
	return nil
}

// anyEditorShown reports whether any note is on screen.
func (a *App) anyEditorShown() bool {
	return slices.ContainsFunc(a.editors(), func(e *editorWindow) bool { return e.shown })
}

// flushEditors saves pending edits in every editor.
func (a *App) flushEditors() {
	for _, e := range a.editors() {
		e.flush()
	}
}

// hideQuickEditor closes the quick editor unless it is pinned.
func (a *App) hideQuickEditor() {
	if !a.editor.pinned {
		a.editor.hide()
	}
}

// closeEditors closes every note on screen, pinned or not.
func (a *App) closeEditors() {
	for _, e := range a.editors() {
		e.hide()
	}
}

// retireQuickEditor turns a pinned quick editor into an ordinary pinned
// window and makes a fresh quick editor, so that opening another note does
// not replace the pinned one.
func (a *App) retireQuickEditor() {
	if !a.editor.shown || !a.editor.pinned {
		return
	}
	fresh, err := newEditorWindow(a)
	if err != nil {
		// Without a second window, fall back to reusing this one.
		log.Printf("editor: %v", err)
		return
	}
	a.pinned = append(a.pinned, a.editor)
	a.editor = fresh
}

// editorClosed runs after an editor closed. Pinned windows are discarded;
// the quick editor is kept for reuse.
func (a *App) editorClosed(e *editorWindow) {
	if e == a.editor {
		a.saveSession()
		return
	}
	if i := slices.Index(a.pinned, e); i >= 0 {
		a.pinned = slices.Delete(a.pinned, i, i+1)
		win32.PostMessage(e.hwnd, msgDiscard, 0, 0)
	}
	a.saveSession()
}

// saveSession remembers which notes are pinned where.
func (a *App) saveSession() {
	if a.paths.session == "" || a.restoring {
		return
	}
	var s config.Session
	for _, e := range a.editors() {
		if e.shown && e.pinned && e.note != nil && !e.note.Empty() {
			r := win32.GetWindowRect(e.hwnd)
			p := config.PinnedNote{ID: e.note.ID, X: r.Left, Y: r.Top, Width: r.Width(), Height: r.Height(), Folded: e.folded}
			if e.folded {
				p.Height = e.fullHeight
			}
			s.Pinned = append(s.Pinned, p)
		}
	}
	if err := s.Save(a.paths.session); err != nil {
		log.Printf("session: %v", err)
	}
}

// restoreSession reopens the notes that were pinned when Tuck last ran.
// Notes that no longer exist, and windows that would land off every screen,
// are skipped.
func (a *App) restoreSession() {
	s, err := config.LoadSession(a.paths.session)
	if err != nil {
		log.Printf("session: %v", err)
	}
	a.restoring = true
	defer func() { a.restoring = false }()
	for _, p := range s.Pinned {
		n := a.find(p.ID)
		if n == nil || n.Archived || a.editorFor(p.ID) != nil {
			continue
		}
		r := win32.RECT{Left: p.X, Top: p.Y, Right: p.X + p.Width, Bottom: p.Y + p.Height}
		mon := win32.MonitorFromRect(r, win32.MONITOR_DEFAULTTONULL)
		if mon == 0 {
			continue // its monitor is gone
		}
		e, err := newEditorWindow(a)
		if err != nil {
			log.Printf("editor: %v", err)
			return
		}
		a.pinned = append(a.pinned, e)
		e.restore(n, r, float64(win32.GetDpiForMonitor(mon))/win32.USER_DEFAULT_DPI)
		if p.Folded {
			e.setFolded(true)
		}
	}
}
