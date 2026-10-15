package markdown

import (
	"strings"
	"testing"
)

func TestToggleTask(t *testing.T) {
	tests := []struct {
		name string
		text string
		// line is the line whose box is clicked, counting from 0.
		line int
		sink bool
		want string
	}{
		{"tick without sinking", "- [ ] a\n- [ ] b", 0, false, "- [x] a\n- [ ] b"},
		{"untick without sinking", "- [X] a", 0, false, "- [ ] a"},
		{"ticked task sinks below open ones", "- [ ] a\n- [ ] b\n- [ ] c", 0, true, "- [ ] b\n- [ ] c\n- [x] a"},
		{"lands above earlier finished tasks", "- [ ] a\n- [ ] b\n- [x] c", 0, true, "- [ ] b\n- [x] a\n- [x] c"},
		{"last open task stays put", "- [ ] a\n- [ ] b\n- [x] c", 1, true, "- [ ] a\n- [x] b\n- [x] c"},
		{"unticked task rejoins the open ones", "- [ ] a\n- [x] b\n- [x] c", 2, true, "- [ ] a\n- [ ] c\n- [x] b"},
		{"children move along", "- [ ] a\n  more about a\n  - [ ] sub\n- [ ] b", 0, true, "- [ ] b\n- [x] a\n  more about a\n  - [ ] sub"},
		{"subtasks sort among themselves", "- [ ] a\n  - [ ] x\n  - [ ] y\n- [ ] b", 1, true, "- [ ] a\n  - [ ] y\n  - [x] x\n- [ ] b"},
		{"a blank line ends the list", "- [ ] a\n\n- [ ] b", 0, true, "- [x] a\n\n- [ ] b"},
		{"other lines end the list", "Shopping\n- [ ] a\n- [ ] b\nLater\n- [ ] c", 1, true, "Shopping\n- [ ] b\n- [x] a\nLater\n- [ ] c"},
		{"plain bullets end the list", "- [ ] a\n- note\n- [ ] b", 0, true, "- [x] a\n- note\n- [ ] b"},
		{"emoji before the list", "🛒 list\n- [ ] a\n- [ ] b", 1, true, "🛒 list\n- [ ] b\n- [x] a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.text, "\n")
			start := len(strings.Join(lines[:tt.line], "\n"))
			if tt.line > 0 {
				start++
			}
			box := start + strings.Index(lines[tt.line], "[")
			if _, _, ok := TaskBoxAt(tt.text, box+1); !ok {
				t.Fatalf("no task box at line %d", tt.line)
			}
			e := ToggleTask(tt.text, box, tt.sink)
			if got := tt.text[:e.Start] + e.Text + tt.text[e.End:]; got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestPlain(t *testing.T) {
	in := "# Groceries\n\nWe need **oat _milk_** and `beans`.\n- [ ] eggs\n  - [x] bread\n* apples\nsnake_case stays\n> quote"
	want := "Groceries\n\nWe need oat milk and beans.\n☐ eggs\n  ☑ bread\n• apples\nsnake_case stays\n> quote"
	if got := Plain(in); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
