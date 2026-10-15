package app

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// openSearch opens the search window and types a query into it.
func openSearch(t *testing.T, a *App, query string) *searchWindow {
	t.Helper()
	a.showSearch()
	w := a.search
	if w == nil {
		t.Fatal("the search window did not open")
	}
	t.Cleanup(func() { win32.DestroyWindow(w.hwnd) })
	// Setting the text sends the same change notification as typing.
	win32.SetWindowText(w.input, query)
	return w
}

// selectedText returns the text selected in an editor.
func selectedText(e *editorWindow) string {
	var sel win32.CHARRANGE
	win32.SendMessage(e.edit, win32.EM_EXGETSEL, 0, uintptr(unsafe.Pointer(&sel)))
	runes := []rune(e.text())
	if sel.Min < 0 || int(sel.Max) > len(runes) {
		return ""
	}
	return string(runes[sel.Min:sel.Max])
}

func TestSearchOpensTheMatch(t *testing.T) {
	a := newTestApp(t)
	addNote(t, a, "Groceries\n- [ ] oat milk")
	meeting := addNote(t, a, "Standup\nAsk Sam about the Budget for milk")
	addNote(t, a, "Unrelated")

	w := openSearch(t, a, "budget")
	if got := len(w.state.Results); got != 1 || w.state.Results[0].Note != meeting {
		t.Fatalf("results for budget: %d", got)
	}
	if !w.handleKey(win32.VK_RETURN) {
		t.Fatal("Enter was not handled")
	}
	if win32.IsWindowVisible(w.hwnd) {
		t.Error("the search window stayed open")
	}
	e := a.editorFor(meeting.ID)
	if e == nil {
		t.Fatal("the note did not open")
	}
	if got := selectedText(e); got != "Budget" {
		t.Errorf("selected %q, want the match", got)
	}
}

func TestSearchKeys(t *testing.T) {
	a := newTestApp(t)
	for _, body := range []string{"milk one", "milk two", "milk three"} {
		addNote(t, a, body)
	}
	archived := addNote(t, a, "old milk")
	archived.Archived = true

	w := openSearch(t, a, "milk")
	if len(w.state.Results) != 4 || w.state.Selected != 0 {
		t.Fatalf("%d results, selected %d", len(w.state.Results), w.state.Selected)
	}
	if last := w.state.Results[3].Note; last != archived {
		t.Error("the archived note is not listed last")
	}
	w.handleKey(win32.VK_DOWN)
	w.handleKey(win32.VK_DOWN)
	w.handleKey(win32.VK_UP)
	if w.state.Selected != 1 {
		t.Errorf("selected %d after down, down, up", w.state.Selected)
	}
	w.handleKey(win32.VK_NEXT)
	if w.state.Selected != 3 {
		t.Errorf("Page Down selected %d", w.state.Selected)
	}
	w.handleKey(win32.VK_RETURN)
	if archived.Archived || a.editorFor(archived.ID) == nil {
		t.Error("opening an archived result did not bring it back")
	}

	w = openSearch(t, a, "nothing like this")
	if len(w.state.Results) != 0 || w.layout.Message.H == 0 {
		t.Error("no message for a query without results")
	}
	if w.handleKey(win32.VK_RETURN); !win32.IsWindowVisible(w.hwnd) {
		t.Error("Enter without results closed the window")
	}
	if !w.handleKey(win32.VK_ESCAPE) || win32.IsWindowVisible(w.hwnd) {
		t.Error("Esc did not close the window")
	}
}

func TestSearchDeletesWordWithCtrlBackspace(t *testing.T) {
	a := newTestApp(t)
	w := openSearch(t, a, "oat milk")
	win32.SendMessage(w.input, win32.EM_SETSEL, 8, 8)
	w.deleteWord()
	if got := win32.WindowText(w.input); got != "oat " {
		t.Errorf("after deleting a word: %q", got)
	}
}

func TestFoldingANote(t *testing.T) {
	dir := t.TempDir()
	a := newTestAppIn(t, dir)
	n := addNote(t, a, "Groceries\nmilk")
	a.openNote(n.ID)
	e := a.editor
	win32.SetWindowPos(e.hwnd, 0, 200, 150, 400, 360, win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)

	// Double-clicking the header folds the note, pinning it.
	e.wndProc(e.hwnd, win32.WM_NCLBUTTONDBLCLK, win32.HTCAPTION, 0)
	r := win32.GetWindowRect(e.hwnd)
	if !e.folded || !e.pinned || r.Height() != int32(e.headerLayout().Height()) || r.Width() != 400 {
		t.Fatalf("folded=%v pinned=%v rect %+v", e.folded, e.pinned, r)
	}
	if win32.IsWindowVisible(e.edit) {
		t.Error("the text is still showing")
	}
	s, err := config.LoadSession(a.paths.session)
	if err != nil || len(s.Pinned) != 1 || !s.Pinned[0].Folded || s.Pinned[0].Height != 360 {
		t.Fatalf("session %+v, %v", s, err)
	}

	// It comes back folded after a restart.
	b := newTestAppIn(t, dir)
	b.notes, _ = b.store.List()
	b.syncDeck()
	b.restoreSession()
	restored := b.editorFor(n.ID)
	if restored == nil || !restored.folded {
		t.Fatal("the note did not return folded")
	}
	if got := win32.GetWindowRect(restored.hwnd); got.Height() != int32(restored.headerLayout().Height()) || got.Top != 150 {
		t.Errorf("restored at %+v", got)
	}

	// Opening it from the deck unfolds it to its full height.
	a.openNote(n.ID)
	if r := win32.GetWindowRect(e.hwnd); e.folded || r.Height() != 360 || r.Top != 150 || !win32.IsWindowVisible(e.edit) {
		t.Errorf("after opening: folded=%v rect %+v", e.folded, r)
	}

	// Unpinning a folded note unfolds it.
	e.setFolded(true)
	e.setPinned(false)
	if e.folded {
		t.Error("an unpinned note stayed folded")
	}
}

func TestTickedTaskSinks(t *testing.T) {
	a := openRich(t, "Weekend\n- [ ] milk\n- [ ] bread\n- [x] eggs")
	click := func(sub string) {
		box := int32(strings.Index(a.editor.text(), sub)) - 4
		pt := pointOf(a, box+1)
		win32.SendMessage(a.editor.edit, win32.WM_LBUTTONDOWN, 0, uintptr(uint32(pt.Y)<<16|uint32(pt.X)))
		win32.SendMessage(a.editor.edit, win32.WM_LBUTTONUP, 0, uintptr(uint32(pt.Y)<<16|uint32(pt.X)))
	}
	click("milk")
	if got := a.editor.text(); got != "Weekend\n- [ ] bread\n- [x] milk\n- [x] eggs" {
		t.Fatalf("after ticking milk: %q", got)
	}
	win32.SendMessage(a.editor.edit, win32.EM_UNDO, 0, 0)
	if got := a.editor.text(); got != "Weekend\n- [ ] milk\n- [ ] bread\n- [x] eggs" {
		t.Errorf("one undo left %q", got)
	}

	a.cfg.SinkDoneTasks = false
	click("milk")
	if got := a.editor.text(); got != "Weekend\n- [x] milk\n- [ ] bread\n- [x] eggs" {
		t.Errorf("with sinking off: %q", got)
	}
}
