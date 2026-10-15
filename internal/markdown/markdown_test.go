package markdown

import (
	"strings"
	"testing"
)

// styleOf returns the combined style of every span covering the first byte
// of the nth occurrence (from 1) of sub in text.
func styleOf(t *testing.T, text, sub string, nth int) Style {
	t.Helper()
	pos := -1
	for i := 0; i < nth; i++ {
		next := strings.Index(text[pos+1:], sub)
		if next < 0 {
			t.Fatalf("%q does not occur %d times in %q", sub, nth, text)
		}
		pos += 1 + next
	}
	var s Style
	for _, sp := range Spans(text) {
		if sp.Start <= pos && pos < sp.End {
			s |= sp.Style
		}
	}
	return s
}

func TestSpans(t *testing.T) {
	tests := []struct {
		text string
		sub  string
		nth  int
		want Style
	}{
		{"# Title", "#", 1, Bold | Heading1 | Muted},
		{"# Title", "Title", 1, Bold | Heading1},
		{"## Sub", "Sub", 1, Bold | Heading2},
		{"#### Deep", "Deep", 1, Bold | Heading3},
		{"#hashtag", "hashtag", 1, 0},
		{"- [ ] milk", "-", 1, Muted},
		{"- [ ] milk", "[", 1, Box | Bold},
		{"- [ ] milk", "milk", 1, 0},
		{"- [x] milk", "milk", 1, Strike | Muted},
		{"  * [X] nested", "nested", 1, Strike | Muted},
		{"- [ ]not a task", "[", 1, 0},
		{"- plain bullet", "-", 1, Muted},
		{"- plain bullet", "plain", 1, 0},
		{"> quoted", "quoted", 1, Italic | Muted},
		{"a **bold** word", "**", 1, Muted},
		{"a **bold** word", "bold", 1, Bold},
		{"a **bold** word", "word", 1, 0},
		{"some _italic_ text", "italic", 1, Italic},
		{"some *italic* text", "italic", 1, Italic},
		{"~~gone~~", "gone", 1, Strike},
		{"run `go test` now", "go", 1, Code},
		{"`**not bold**`", "not", 1, Code},
		{"**bold _both_**", "both", 1, Bold | Italic},
		{"snake_case_name", "case", 1, 0},
		{"2 * 3 * 4", "3", 1, 0},
		{"a ** b", "b", 1, 0},
		{"unclosed **bold", "bold", 1, 0},
		{"line one\n# Two", "Two", 1, Bold | Heading1},
		{"line one\n# Two", "line", 1, 0},
		{"- [x] done **really**", "really", 1, Strike | Muted | Bold},
	}
	for _, tt := range tests {
		if got := styleOf(t, tt.text, tt.sub, tt.nth); got != tt.want {
			t.Errorf("style of %q in %q = %#x, want %#x", tt.sub, tt.text, got, tt.want)
		}
	}
}

func TestSpansStayInsideText(t *testing.T) {
	for _, text := range []string{"", "\n", "**", "- [", "# ", "_x_\n**y", "日本 **語** ✓", "- [x]"} {
		for _, sp := range Spans(text) {
			if sp.Start < 0 || sp.End > len(text) || sp.Start >= sp.End {
				t.Errorf("Spans(%q) produced %+v", text, sp)
			}
		}
	}
}

func TestTaskBoxAt(t *testing.T) {
	text := "Groceries\n- [ ] oat milk\n  - [x] coffee\n- plain"
	open := strings.Index(text, "[ ]")
	done := strings.Index(text, "[x]")
	tests := []struct {
		pos         int
		wantBox     int
		wantChecked bool
		wantOK      bool
	}{
		{open, open, false, true},
		{open + 1, open, false, true},
		{open + 3, open, false, true}, // just after "]"
		{open + 4, 0, false, false},   // on the text
		{open - 2, 0, false, false},   // on the bullet
		{done + 1, done, true, true},
		{0, 0, false, false},
		{len(text), 0, false, false},
	}
	for _, tt := range tests {
		box, checked, ok := TaskBoxAt(text, tt.pos)
		if box != tt.wantBox || checked != tt.wantChecked || ok != tt.wantOK {
			t.Errorf("TaskBoxAt(%d) = %d, %v, %v; want %d, %v, %v",
				tt.pos, box, checked, ok, tt.wantBox, tt.wantChecked, tt.wantOK)
		}
	}
}
