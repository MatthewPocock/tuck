package search

import (
	"testing"
	"time"

	"github.com/MatthewPocock/tuck/internal/note"
)

func mk(body string, updated int, archived bool) *note.Note {
	n := note.New(time.Unix(0, 0), note.Yellow)
	n.ID = body[:min(len(body), 8)]
	n.Body = body
	n.Updated = time.Unix(int64(updated), 0)
	n.Archived = archived
	return n
}

func titles(rs []Result) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Title)
	}
	return out
}

func TestFind(t *testing.T) {
	notes := []*note.Note{
		mk("Groceries\n- [ ] Oat milk\n- [ ] coffee", 10, false),
		mk("Meeting notes\nAsk about the milk budget", 20, false),
		mk("Milk run\nTuesday", 5, false),
		mk("Old milk recipe", 30, true),
		mk("   ", 40, false),
		mk("Café list\nCAFÉ Nero", 1, false),
	}

	got := titles(Find(notes, "MILK"))
	want := []string{"Milk run", "Meeting notes", "Groceries", "Old milk recipe"}
	if !equal(got, want) {
		t.Errorf("milk: got %q, want %q", got, want)
	}

	if got := titles(Find(notes, "milk budget")); !equal(got, []string{"Meeting notes"}) {
		t.Errorf("all words must match: got %q", got)
	}
	if got := titles(Find(notes, "")); !equal(got, []string{"Meeting notes", "Groceries", "Milk run", "Café list"}) {
		t.Errorf("empty query: got %q", got)
	}
	if got := titles(Find(notes, "café")); !equal(got, []string{"Café list"}) {
		t.Errorf("non-ASCII case folding: got %q", got)
	}

	r := Find(notes, "budget")[0]
	if r.Snippet != "Ask about the milk budget" || len(r.SnippetMatches) != 1 ||
		r.Snippet[r.SnippetMatches[0][0]:r.SnippetMatches[0][1]] != "budget" {
		t.Errorf("snippet %q %v", r.Snippet, r.SnippetMatches)
	}
	if r.Note.Body[r.At:r.At+r.AtLen] != "budget" {
		t.Errorf("At points at %q", r.Note.Body[r.At:])
	}

	g := Find(notes, "oat")[0]
	if g.Snippet != "☐ Oat milk" {
		t.Errorf("snippet is shown without Markdown: %q", g.Snippet)
	}
}

func TestShorten(t *testing.T) {
	long := ""
	for range 40 {
		long += "word "
	}
	long += "needle"
	for range 40 {
		long += " more"
	}
	got := shorten(long, len(long)/2)
	if []rune(got)[0] != '…' || len([]rune(got)) > maxSnippet+2 {
		t.Errorf("shorten: %q", got)
	}
}

func TestLayoutScrollsToSelection(t *testing.T) {
	l := NewLayout(1, 20, 10, 0)
	if len(l.Rows) != MaxRows || l.First != 10-MaxRows+1 {
		t.Fatalf("rows %d first %d", len(l.Rows), l.First)
	}
	if got := l.Hit(int(l.Rows[MaxRows-1].X+5), int(l.Rows[MaxRows-1].Y+5)); got != 10 {
		t.Errorf("hit %d", got)
	}
	if l := NewLayout(1, 0, -1, 0); l.Message.H == 0 || len(l.Rows) != 0 {
		t.Errorf("empty layout %+v", l)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
