package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/MatthewPocock/tuck/internal/markdown"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// formatAt returns the formatting of the character at a rich edit position.
func formatAt(a *App, char int32) win32.CHARFORMAT {
	return formatOf(a, char, char+1)
}

// formatOf returns the formatting of a range of the text, which should be
// uniform.
func formatOf(a *App, min, max int32) win32.CHARFORMAT {
	// Looking works through the selection; keep that from counting as the
	// user moving the caret, which changes what is shown.
	e := a.editor
	e.loading = true
	defer func() { e.loading = false }()
	var sel win32.CHARRANGE
	win32.SendMessage(e.edit, win32.EM_EXGETSEL, 0, uintptr(unsafe.Pointer(&sel)))
	defer win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&sel)))

	r := win32.CHARRANGE{Min: min, Max: max}
	win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&r)))
	var f win32.CHARFORMAT
	f.Size = uint32(unsafe.Sizeof(f))
	win32.SendMessage(e.edit, win32.EM_GETCHARFORMAT, win32.SCF_SELECTION, uintptr(unsafe.Pointer(&f)))
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
	a := openRich(t, "Note\n# Title\nsome **bold** text\n- [x] done")
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
	// Away from the caret the markers are tucked out of sight.
	if f, g := formatAt(a, at("**")), formatAt(a, at("some")); f.Height >= g.Height || f.TextColor != colorRef(a.editor.sw.Paper) {
		t.Errorf("Markdown markers away from the caret are not collapsed: %+v", f)
	}
	// On the line being edited they show, faintly.
	caret := win32.CHARRANGE{Min: at("text"), Max: at("text")}
	win32.SendMessage(a.editor.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&caret)))
	if f, g := formatAt(a, at("**")), formatAt(a, at("some")); f.TextColor == g.TextColor || f.Height != g.Height {
		t.Errorf("Markdown markers on the caret's line are not shown muted: %+v", f)
	}
}

func TestTitleMovesIntoTheHeader(t *testing.T) {
	a := openRich(t, "Groceries\n\n- [ ] milk\n- just a bullet")
	text := a.editor.text()
	if a.editor.title != "Groceries" {
		t.Errorf("header title %q", a.editor.title)
	}
	if f := formatAt(a, 0); f.Height != collapsedTwips {
		t.Errorf("title line is not collapsed while editing elsewhere: %+v", f)
	}
	if len(a.editor.marks) != 2 || a.editor.marks[0].kind != markdown.TaskMark || a.editor.marks[1].kind != markdown.BulletMark {
		t.Errorf("drawn marks %+v", a.editor.marks)
	}
	if f := formatAt(a, int32(strings.Index(text, "[ ]"))); f.TextColor != colorRef(a.editor.sw.Paper) {
		t.Error("the task box characters are visible under the drawn box")
	}
	// Moving the caret to the title line brings it back.
	caret := win32.CHARRANGE{Min: 3, Max: 3}
	win32.SendMessage(a.editor.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&caret)))
	if f := formatAt(a, 0); f.Height == collapsedTwips {
		t.Error("the title line stays collapsed with the caret on it")
	}
	// Deleting the line after the title leaves the title alone.
	line := win32.CHARRANGE{Min: int32(strings.Index(text, "- [ ]")), Max: int32(strings.Index(text, "- just"))}
	win32.SendMessage(a.editor.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&line)))
	win32.SendMessage(a.editor.edit, win32.EM_REPLACESEL, 1, uintptr(unsafe.Pointer(win32.UTF16(""))))
	if got := a.editor.text(); got != "Groceries\n\n- just a bullet" {
		t.Errorf("after deleting a line: %q", got)
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

func TestEmojiKeepTheEmojiFont(t *testing.T) {
	a := openRich(t, "ship it 🚀 **now** 👍🏽")
	text := a.editor.text()
	for _, emoji := range []string{"🚀", "👍🏽"} {
		start := strings.Index(text, emoji)
		pos := utf16Offsets(text)
		f := formatOf(a, pos[start], pos[start+len(emoji)])
		if face := win32.UTF16ToString(f.FaceName[:]); face != emojiFont {
			t.Errorf("%s is drawn with %q; it would show as a box", emoji, face)
		}
	}
	plain := formatAt(a, 0)
	if face := win32.UTF16ToString(plain.FaceName[:]); face != uiFont {
		t.Errorf("text is drawn with %q", face)
	}
}

func TestEmojiRuns(t *testing.T) {
	text := "a 👍🏽 b ❤️ c"
	runs := emojiRuns(text)
	if len(runs) != 2 || text[runs[0][0]:runs[0][1]] != "👍🏽" || text[runs[1][0]:runs[1][1]] != "❤️" {
		t.Errorf("emojiRuns(%q) = %v", text, runs)
	}
}

func TestAddItemRow(t *testing.T) {
	a := openRich(t, "Groceries\n- [ ] milk\n  - [x] eggs\n\nAfter the list")
	e := a.editor
	win32.UpdateWindow(e.edit)
	if len(e.addRows) != 1 {
		t.Fatalf("%d add rows, want one under the list", len(e.addRows))
	}
	r := e.addRows[0].rect
	if r.Height() <= 0 {
		t.Fatalf("the add row was not drawn: %+v", r)
	}
	pt := win32.POINT{X: r.Left + 30, Y: r.Top + r.Height()/2}
	win32.SendMessage(e.edit, win32.WM_LBUTTONDOWN, 0, uintptr(uint32(pt.Y)<<16|uint32(pt.X)))
	win32.SendMessage(e.edit, win32.WM_LBUTTONUP, 0, uintptr(uint32(pt.Y)<<16|uint32(pt.X)))
	if got := e.text(); got != "Groceries\n- [ ] milk\n  - [x] eggs\n  - [ ] \n\nAfter the list" {
		t.Errorf("after adding an item: %q", got)
	}
	var sel win32.CHARRANGE
	win32.SendMessage(e.edit, win32.EM_EXGETSEL, 0, uintptr(unsafe.Pointer(&sel)))
	if want := int32(strings.Index(e.text(), "  - [ ] \n") + len("  - [ ] ")); sel.Min != want {
		t.Errorf("caret at %d, want %d, ready to type the item", sel.Min, want)
	}
}
