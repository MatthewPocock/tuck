// Package markdown finds the lightweight Markdown in a note so the editor can
// style it while keeping the text itself plain.
//
// Only what is common in quick notes is recognised: headings, bullets, task
// lists, quotes and the inline **bold**, _italic_, ~~strike~~ and `code`
// markers. The markers stay in the text, as they do in the saved file; they
// are merely shown fainter than the words they mark up.
package markdown

import "strings"

// Style is a set of text attributes.
type Style uint16

// The attributes a span can carry.
const (
	Bold Style = 1 << iota
	Italic
	Strike
	Code
	// Muted is used for Markdown syntax and for finished tasks.
	Muted
	Heading1
	Heading2
	Heading3
	// Box marks the "[ ]" or "[x]" of a task, which can be clicked.
	Box
)

// Headings is the union of the heading levels.
const Headings = Heading1 | Heading2 | Heading3

// Span applies a style to the bytes Start to End of the text.
type Span struct {
	Start, End int
	Style      Style
}

// Spans returns the styled ranges of a note's text, whose lines end in "\n".
// Ranges may overlap; their styles add up.
func Spans(text string) []Span {
	var spans []Span
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		spans = appendLine(spans, strings.TrimSuffix(line, "\n"), offset)
		offset += len(line)
	}
	return spans
}

func appendLine(spans []Span, line string, at int) []Span {
	add := func(start, end int, s Style) {
		if end > start {
			spans = append(spans, Span{at + start, at + end, s})
		}
	}
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	rest := line[indent:]

	if level := headingLevel(rest); level > 0 {
		heading := []Style{Heading1, Heading2, Heading3}[min(level, 3)-1]
		add(indent, len(line), Bold|heading)
		add(indent, indent+level+1, Muted)
		return appendInline(spans, line, at, indent+level+1)
	}
	if strings.HasPrefix(rest, "> ") || rest == ">" {
		add(indent, len(line), Italic|Muted)
		return spans
	}
	if box, checked, ok := taskBox(line); ok {
		add(indent, box, Muted)   // the bullet
		add(box, box+3, Box|Bold) // [ ] or [x]
		if checked {
			add(min(box+4, len(line)), len(line), Strike|Muted)
		}
		return appendInline(spans, line, at, box+3)
	}
	if bulletAt(rest) {
		add(indent, indent+1, Muted)
		return appendInline(spans, line, at, indent+2)
	}
	return appendInline(spans, line, at, 0)
}

// headingLevel returns the number of leading "#" of an ATX heading, or 0.
func headingLevel(s string) int {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(s) || s[n] != ' ' {
		return 0
	}
	return n
}

func bulletAt(s string) bool {
	return len(s) >= 2 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' '
}

// taskBox locates the box of a task list line such as "- [ ] milk". box is
// the byte offset of its "[".
func taskBox(line string) (box int, checked, ok bool) {
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	rest := line[indent:]
	if !bulletAt(rest) || len(rest) < 5 || rest[2] != '[' || rest[4] != ']' {
		return 0, false, false
	}
	if len(rest) > 5 && rest[5] != ' ' {
		return 0, false, false
	}
	switch rest[3] {
	case ' ':
		return indent + 2, false, true
	case 'x', 'X':
		return indent + 2, true, true
	}
	return 0, false, false
}

// TaskBoxAt reports whether the byte offset pos of text falls on the box of a
// task, and if so where that box starts and whether it is ticked. A click on
// the box toggles it.
func TaskBoxAt(text string, pos int) (box int, checked, ok bool) {
	if pos < 0 || pos > len(text) {
		return 0, false, false
	}
	start := strings.LastIndexByte(text[:pos], '\n') + 1
	end := strings.IndexByte(text[start:], '\n')
	if end < 0 {
		end = len(text) - start
	}
	b, checked, ok := taskBox(text[start : start+end])
	if !ok || pos < start+b || pos > start+b+3 {
		return 0, false, false
	}
	return start + b, checked, true
}

// inline markers, longest first so "**" wins over "*".
var markers = []struct {
	mark  string
	style Style
}{
	{"**", Bold},
	{"__", Bold},
	{"~~", Strike},
	{"`", Code},
	{"*", Italic},
	{"_", Italic},
}

// appendInline styles the inline markers of line from byte from on.
func appendInline(spans []Span, line string, at, from int) []Span {
	for i := from; i < len(line); {
		matched := false
		for _, m := range markers {
			if !strings.HasPrefix(line[i:], m.mark) || !opens(line, i, m.mark) {
				continue
			}
			end := closing(line, i+len(m.mark), m.mark)
			if end < 0 {
				continue
			}
			inner := Span{at + i + len(m.mark), at + end, m.style}
			spans = append(spans,
				Span{at + i, at + i + len(m.mark), Muted},
				inner,
				Span{at + end, at + end + len(m.mark), Muted})
			if m.style != Code {
				// Markers nest, e.g. **bold _and italic_**; code does not.
				spans = appendInline(spans, line[:end], at, i+len(m.mark))
			}
			i = end + len(m.mark)
			matched = true
			break
		}
		if !matched {
			i++
		}
	}
	return spans
}

// opens reports whether the marker at i can start emphasis: it must be
// followed by text, and an underscore must not sit inside a word, so that
// snake_case_names stay plain.
func opens(line string, i int, mark string) bool {
	next := i + len(mark)
	if next >= len(line) || line[next] == ' ' {
		return false
	}
	if mark[0] == '_' && i > 0 && isWordByte(line[i-1]) {
		return false
	}
	return true
}

// closing finds the marker that ends emphasis opened before from, or -1.
func closing(line string, from int, mark string) int {
	for j := from; j+len(mark) <= len(line); j++ {
		if !strings.HasPrefix(line[j:], mark) {
			continue
		}
		if j == from || line[j-1] == ' ' {
			continue // empty, or preceded by a space: not a closing marker
		}
		after := j + len(mark)
		if mark[0] == '_' && after < len(line) && isWordByte(line[after]) {
			continue
		}
		if len(mark) == 1 && mark != "`" && after < len(line) && line[after] == mark[0] {
			continue // part of a double marker
		}
		return j
	}
	return -1
}

func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}
