package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/MatthewPocock/tuck/internal/win32"
)

// formatAt returns the formatting of the character at a rich edit position.
func formatAt(a *App, char int32) win32.CHARFORMAT {
	r := win32.CHARRANGE{Min: char, Max: char + 1}
	win32.SendMessage(a.editor.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&r)))
	var f win32.CHARFORMAT
	f.Size = uint32(unsafe.Sizeof(f))
	win32.SendMessage(a.editor.edit, win32.EM_GETCHARFORMAT, win32.SCF_SELECTION, uintptr(unsafe.Pointer(&f)))
	return f
}

// pointOf returns where a character is drawn in the text control.
func pointOf(a *App, char int32) win32.POINT {
	var pt win32.POINT
	win32.SendMessage(a.editor.edit, win32.EM_POSFROMCHAR, uintptr(unsafe.Pointer(&pt)), uintptr(char))
	return pt
}

func openRich(t *testing.T, body string) *App {
	t.Helper()
	a := newTestApp(t)
	n := addNote(t, a, body)
	a.openNote(n.ID)
	a.editor.setPinned(true)
	if !a.editor.rich {
		t.Skip("rich edit is unavailable")
	}
	return a
}

func TestMarkdownIsStyled(t *testing.T) {
	a := openRich(t, "# Title\nsome **bold** text\n- [x] done")
	text := a.editor.text()
	at := func(sub string) int32 { return int32(strings.Index(text, sub)) }

	if f := formatAt(a, at("Title")); f.Effects&win32.CFE_BOLD == 0 || f.Height <= formatAt(a, at("some")).Height {
		t.Errorf("heading not bold and larger: %+v", f)
	}
	if f := formatAt(a, at("bold")); f.Effects&win32.CFE_BOLD == 0 {
		t.Error("**bold** is not bold")
	}
	if f := formatAt(a, at("text")); f.Effects&win32.CFE_BOLD != 0 {
		t.Error("plain text after **bold** is bold")
	}
	if f := formatAt(a, at("done")); f.Effects&win32.CFE_STRIKEOUT == 0 {
		t.Error("finished task is not struck through")
	}
	if f, g := formatAt(a, at("**")), formatAt(a, at("some")); f.TextColor == g.TextColor {
		t.Error("Markdown markers are not muted")
	}
}

func TestStylingIsNotUndoable(t *testing.T) {
	a := openRich(t, "start")
	typeText(a, " **bold**")
	win32.SendMessage(a.editor.edit, win32.EM_UNDO, 0, 0)
	if got := a.editor.text(); got != "start" {
		t.Errorf("after one undo the text is %q; restyling must not take undo steps", got)
	}
}

func TestClickingTaskBoxTogglesIt(t *testing.T) {
	a := openRich(t, "Groceries\n- [ ] oat milk")
	box := int32(strings.Index(a.editor.text(), "[ ]"))
	pt := pointOf(a, box+1)
	win32.SendMessage(a.editor.edit, win32.WM_LBUTTONDOWN, 0, uintptr(uint32(pt.Y)<<16|uint32(pt.X)))
	win32.SendMessage(a.editor.edit, win32.WM_LBUTTONUP, 0, uintptr(uint32(pt.Y)<<16|uint32(pt.X)))

	if got := a.editor.text(); got != "Groceries\n- [x] oat milk" {
		t.Fatalf("after clicking the box the text is %q", got)
	}
	if !a.editor.dirty {
		t.Error("ticking a task did not mark the note for saving")
	}
	win32.SendMessage(a.editor.edit, win32.EM_UNDO, 0, 0)
	if got := a.editor.text(); got != "Groceries\n- [ ] oat milk" {
		t.Errorf("undo after ticking left %q", got)
	}
}

func TestClickingTaskTextDoesNotToggle(t *testing.T) {
	a := openRich(t, "- [ ] oat milk")
	pt := pointOf(a, int32(strings.Index(a.editor.text(), "milk")))
	win32.SendMessage(a.editor.edit, win32.WM_LBUTTONDOWN, 0, uintptr(uint32(pt.Y)<<16|uint32(pt.X)))
	win32.SendMessage(a.editor.edit, win32.WM_LBUTTONUP, 0, uintptr(uint32(pt.Y)<<16|uint32(pt.X)))
	if got := a.editor.text(); got != "- [ ] oat milk" {
		t.Errorf("clicking the task's text changed it to %q", got)
	}
}

func TestStyledNoteIsSavedAsPlainMarkdown(t *testing.T) {
	a := openRich(t, "")
	typeText(a, "# Plan\r\n- [ ] **ship** it")
	a.editor.flush()
	data, err := os.ReadFile(filepath.Join(a.store.Dir(), a.editor.note.ID+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n\n# Plan\n- [ ] **ship** it\n") {
		t.Errorf("saved file:\n%s", data)
	}
}
