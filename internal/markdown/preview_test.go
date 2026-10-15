package markdown

import (
	"reflect"
	"testing"
)

func TestBlocks(t *testing.T) {
	got := Blocks("# Plan\n- [ ] buy **oat** milk\n  - [x] eggs\n* note `a*b*c`\n> quoted _words_\n\nplain")
	want := []Block{
		{Kind: Heading, Runs: []Run{{Text: "Plan"}}},
		{Kind: Task, Runs: []Run{{Text: "buy "}, {Text: "oat", Bold: true}, {Text: " milk"}}},
		{Kind: Task, Level: 1, Done: true, Runs: []Run{{Text: "eggs"}}},
		{Kind: Bullet, Runs: []Run{{Text: "note "}, {Text: "a*b*c", Code: true}}},
		{Kind: Quote, Runs: []Run{{Text: "quoted words"}}},
		{Kind: Blank},
		{Kind: Paragraph, Runs: []Run{{Text: "plain"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Blocks:\n got %+v\nwant %+v", got, want)
	}
}

func TestBlocksKeepsUnmatchedMarkers(t *testing.T) {
	got := Blocks("2 * 3 = **6")
	if len(got) != 1 || len(got[0].Runs) != 1 || got[0].Runs[0].Text != "2 * 3 = **6" {
		t.Errorf("got %+v", got)
	}
}

func TestTasks(t *testing.T) {
	done, total := Tasks("Title\n- [ ] a\n- [x] b\n  * [X] c\n- [] not a task\ntext")
	if done != 2 || total != 3 {
		t.Errorf("Tasks = %d of %d, want 2 of 3", done, total)
	}
}
