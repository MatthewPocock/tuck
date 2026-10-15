// Package search finds notes by their words and lays out the search window.
//
// Like the deck and the settings window, it has no platform dependencies:
// the Windows shell feeds it the query and paints its Layout.
package search

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/MatthewPocock/tuck/internal/markdown"
	"github.com/MatthewPocock/tuck/internal/note"
)

// Result is a note that matches a query.
type Result struct {
	Note *note.Note
	// Title is the note's label and Snippet a line of its text worth
	// showing beneath it: the first line with a match, or the line after the
	// title when the query is empty or only the title matches.
	Title, Snippet string
	// TitleMatches and SnippetMatches are the byte ranges of Title and
	// Snippet that match the query.
	TitleMatches, SnippetMatches [][2]int
	// At is the byte offset of the first match in the note's body, or -1.
	At int
	// AtLen is the length of that match in bytes.
	AtLen int
}

// maxSnippet bounds the length of a snippet in runes, and lead how much text
// is kept before the match when a long line is shortened.
const (
	maxSnippet = 160
	lead       = 24
)

// Find returns the notes that contain every word of the query, best match
// first: notes whose title matches come before those that only mention the
// words, and among equals the most recently edited wins. Matching ignores
// case. An empty query lists the notes in the deck, most recent first, so the
// window doubles as a quick switcher.
//
// Archived notes are searched too, but only with a query, and come after
// the notes in the deck.
func Find(notes []*note.Note, query string) []Result {
	terms := strings.Fields(query)
	type ranked struct {
		Result
		inTitle int
	}
	var found []ranked
	for _, n := range notes {
		if n.Empty() || n.Archived && len(terms) == 0 {
			continue
		}
		r := ranked{Result: Result{Note: n, Title: n.Title(), At: -1}}
		matched := true
		for _, t := range terms {
			at := indexFold(n.Body, t, 0)
			if at < 0 {
				matched = false
				break
			}
			if r.At < 0 || at < r.At {
				r.At, r.AtLen = at, matchLen(n.Body[at:], t)
			}
			if indexFold(r.Title, t, 0) >= 0 {
				r.inTitle++
			}
		}
		if !matched {
			continue
		}
		r.TitleMatches = matches(r.Title, terms)
		r.Snippet, r.SnippetMatches = snippet(n.Body, terms)
		found = append(found, r)
	}
	slices.SortStableFunc(found, func(a, b ranked) int {
		if a.Note.Archived != b.Note.Archived {
			if a.Note.Archived {
				return 1
			}
			return -1
		}
		if a.inTitle != b.inTitle {
			return b.inTitle - a.inTitle
		}
		return b.Note.Updated.Compare(a.Note.Updated)
	})
	out := make([]Result, len(found))
	for i, f := range found {
		out[i] = f.Result
	}
	return out
}

// snippet picks the line of body to show under the title and shortens it
// around its first match.
func snippet(body string, terms []string) (string, [][2]int) {
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if line = strings.TrimSpace(markdown.Plain(line)); line != "" {
			lines = append(lines, line)
		}
	}
	// The first line is the title, which is already shown.
	if len(lines) < 2 {
		return "", nil
	}
	lines = lines[1:]
	pick := lines[0]
	at := -1
	for _, line := range lines {
		if m := matches(line, terms); len(m) > 0 {
			pick, at = line, m[0][0]
			break
		}
	}
	pick = shorten(pick, at)
	return pick, matches(pick, terms)
}

// shorten trims a line to maxSnippet runes, keeping the match at byte at in
// view and marking cuts with an ellipsis.
func shorten(line string, at int) string {
	if utf8.RuneCountInString(line) <= maxSnippet {
		return line
	}
	start := 0
	if at > 0 {
		// Step back lead runes from the match.
		start = at
		for i := 0; i < lead && start > 0; i++ {
			_, size := utf8.DecodeLastRuneInString(line[:start])
			start -= size
		}
	}
	runes := []rune(line[start:])
	out := string(runes[:min(len(runes), maxSnippet)])
	if start > 0 {
		out = "…" + strings.TrimLeft(out, " ")
	}
	if len(runes) > maxSnippet {
		out += "…"
	}
	return out
}

// matches returns the byte ranges of s that match any of the terms, in
// order and without overlaps.
func matches(s string, terms []string) [][2]int {
	var out [][2]int
	for _, t := range terms {
		for at := indexFold(s, t, 0); at >= 0; at = indexFold(s, t, at+1) {
			out = append(out, [2]int{at, at + matchLen(s[at:], t)})
		}
	}
	slices.SortFunc(out, func(a, b [2]int) int { return a[0] - b[0] })
	merged := out[:0]
	for _, m := range out {
		if n := len(merged); n > 0 && m[0] <= merged[n-1][1] {
			merged[n-1][1] = max(merged[n-1][1], m[1])
			continue
		}
		merged = append(merged, m)
	}
	return merged
}

// indexFold returns the byte offset of the first case-insensitive match of
// term in s at or after from, or -1.
func indexFold(s, term string, from int) int {
	if term == "" {
		return -1
	}
	for i := from; i < len(s); {
		if matchLen(s[i:], term) > 0 {
			return i
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return -1
}

// matchLen returns how many bytes at the start of s match term, ignoring
// case, or 0 if s does not start with term.
func matchLen(s, term string) int {
	n := 0
	for _, want := range term {
		if n >= len(s) {
			return 0
		}
		got, size := utf8.DecodeRuneInString(s[n:])
		if !equalFold(got, want) {
			return 0
		}
		n += size
	}
	return n
}

func equalFold(a, b rune) bool {
	if a == b {
		return true
	}
	for f := unicode.SimpleFold(a); f != a; f = unicode.SimpleFold(f) {
		if f == b {
			return true
		}
	}
	return false
}
