// Package note defines the note model and its on-disk representation.
//
// A note is stored as a plain Markdown file with an optional front matter
// block, so notes stay readable and editable with any other tool:
//
//	---
//	color: yellow
//	created: 2026-10-14T09:30:00Z
//	updated: 2026-10-14T09:42:10Z
//	---
//
//	Buy oat milk
//
// Files without front matter are valid notes too; missing fields simply take
// their defaults.
package note

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"
)

// Color names a note colour. The concrete RGB values live in the render
// package so that the model stays independent of presentation.
type Color string

// The built-in note colours, in the order they are cycled through.
const (
	Yellow Color = "yellow"
	Peach  Color = "peach"
	Pink   Color = "pink"
	Lilac  Color = "lilac"
	Sky    Color = "sky"
	Mint   Color = "mint"
	Stone  Color = "stone"
)

// Colors lists every built-in colour in cycling order.
var Colors = []Color{Yellow, Peach, Pink, Lilac, Sky, Mint, Stone}

// DefaultColor is used when a note does not specify a (valid) colour.
const DefaultColor = Yellow

// Valid reports whether c is one of the built-in colours.
func (c Color) Valid() bool {
	for _, k := range Colors {
		if c == k {
			return true
		}
	}
	return false
}

// Next returns the colour after c in cycling order, wrapping around.
func (c Color) Next() Color {
	for i, k := range Colors {
		if c == k {
			return Colors[(i+1)%len(Colors)]
		}
	}
	return DefaultColor
}

// Label returns the human-readable name of the colour, e.g. "Yellow".
func (c Color) Label() string {
	if c == "" {
		return ""
	}
	return strings.ToUpper(string(c[:1])) + string(c[1:])
}

// Note is a single sticky note.
type Note struct {
	// ID is the note's stable identifier and the base name of its file.
	ID string
	// Color is the note's colour.
	Color Color
	// Created and Updated are stored in UTC with second precision.
	Created time.Time
	Updated time.Time
	// Archived notes are kept on disk but hidden from the deck.
	Archived bool
	// Body is the note's Markdown text, using "\n" line endings.
	Body string

	// extra holds front matter lines this version does not understand, so
	// that they survive a load/save round trip.
	extra []string
}

// New returns an empty note created at now with a freshly generated ID.
func New(now time.Time, c Color) *Note {
	now = now.UTC().Truncate(time.Second)
	return &Note{ID: NewID(now), Color: c, Created: now, Updated: now}
}

// NewID returns a new identifier that sorts chronologically and is safe to
// use as a file name, e.g. "20261014-093000-3fa9c1".
func NewID(now time.Time) string {
	var suffix [3]byte
	// crypto/rand.Read never returns an error on supported platforms.
	_, _ = rand.Read(suffix[:])
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:])
}

// maxTitleRunes bounds Title so a note that is one enormous line does not
// produce an enormous label.
const maxTitleRunes = 80

// Title returns a short single-line label for the note: its first non-blank
// line with leading Markdown markers removed. It returns "" for a blank note.
func (n *Note) Title() string {
	for _, line := range strings.Split(n.Body, "\n") {
		line = stripMarkers(line)
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > maxTitleRunes {
			r := []rune(line)
			line = strings.TrimSpace(string(r[:maxTitleRunes]))
		}
		return line
	}
	return ""
}

// stripMarkers removes leading block-level Markdown syntax (headings, quotes,
// list bullets and task boxes) from a line.
func stripMarkers(line string) string {
	line = strings.TrimSpace(line)
	for {
		before := line
		line = strings.TrimLeft(line, "#>")
		for _, p := range []string{"- [ ] ", "- [x] ", "- [X] ", "* [ ] ", "* [x] ", "- ", "* ", "+ "} {
			line = strings.TrimPrefix(line, p)
		}
		line = strings.TrimSpace(line)
		if line == before {
			return line
		}
	}
}

// Empty reports whether the note has no visible content.
func (n *Note) Empty() bool {
	return strings.TrimSpace(n.Body) == ""
}

const (
	fence      = "---"
	timeLayout = time.RFC3339
)

// Encode serialises the note to its on-disk form.
func (n *Note) Encode() []byte {
	var b bytes.Buffer
	b.WriteString(fence + "\n")
	color := n.Color
	if !color.Valid() {
		color = DefaultColor
	}
	b.WriteString("color: " + string(color) + "\n")
	if !n.Created.IsZero() {
		b.WriteString("created: " + n.Created.UTC().Format(timeLayout) + "\n")
	}
	if !n.Updated.IsZero() {
		b.WriteString("updated: " + n.Updated.UTC().Format(timeLayout) + "\n")
	}
	if n.Archived {
		b.WriteString("archived: true\n")
	}
	for _, line := range n.extra {
		b.WriteString(line + "\n")
	}
	b.WriteString(fence + "\n\n")
	b.WriteString(n.Body)
	if n.Body != "" && !strings.HasSuffix(n.Body, "\n") {
		b.WriteString("\n")
	}
	return b.Bytes()
}

// Decode parses the on-disk form of a note. It never fails: anything that is
// not recognisable front matter is treated as part of the body, so arbitrary
// text files dropped into the notes folder load as notes.
func Decode(id string, data []byte) *Note {
	n := &Note{ID: id, Color: DefaultColor}

	text := string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	header, body, ok := splitFrontMatter(text)
	if !ok {
		n.Body = trimBody(text)
		return n
	}

	for _, line := range strings.Split(header, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		if !found {
			n.extra = append(n.extra, line)
			continue
		}
		switch strings.TrimSpace(key) {
		case "color":
			if c := Color(strings.ToLower(value)); c.Valid() {
				n.Color = c
			}
		case "created":
			n.Created = parseTime(value)
		case "updated":
			n.Updated = parseTime(value)
		case "archived":
			n.Archived = value == "true"
		default:
			n.extra = append(n.extra, line)
		}
	}
	n.Body = trimBody(body)
	return n
}

// splitFrontMatter separates a leading "---" delimited block from the rest of
// the text.
func splitFrontMatter(text string) (header, body string, ok bool) {
	rest, found := strings.CutPrefix(text, fence+"\n")
	if !found {
		return "", "", false
	}
	if after, empty := strings.CutPrefix(rest, fence+"\n"); empty {
		return "", after, true
	}
	if rest == fence {
		return "", "", true
	}
	if header, body, found = strings.Cut(rest, "\n"+fence+"\n"); found {
		return header, body, true
	}
	if header, found = strings.CutSuffix(rest, "\n"+fence); found {
		return header, "", true
	}
	return "", "", false
}

// trimBody removes the blank separator lines after the front matter and the
// trailing newline added by Encode.
func trimBody(body string) string {
	body = strings.TrimLeft(body, "\n")
	return strings.TrimSuffix(body, "\n")
}

func parseTime(s string) time.Time {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
