package markdown

import "strings"

// BlockKind is the kind of a line in a note's preview.
type BlockKind int

// The kinds of preview lines.
const (
	Paragraph BlockKind = iota
	Heading
	Bullet
	Task
	Quote
	Blank
)

// Block is one line of a note, ready to be shown without its Markdown.
type Block struct {
	Kind BlockKind
	// Level is how far the line is indented, in steps of two spaces or one
	// tab.
	Level int
	// Done marks a ticked task.
	Done bool
	Runs []Run
}

// Run is a stretch of text with one style.
type Run struct {
	Text string
	Bold bool
	Code bool
}

// Blocks splits a note into lines and strips their Markdown markers, keeping
// what each line is (a task, a bullet, a heading) and which words are bold.
func Blocks(text string) []Block {
	lines := strings.Split(text, "\n")
	blocks := make([]Block, 0, len(lines))
	for _, line := range lines {
		blocks = append(blocks, block(strings.TrimRight(line, "\r")))
	}
	return blocks
}

func block(line string) Block {
	if strings.TrimSpace(line) == "" {
		return Block{Kind: Blank}
	}
	indent := indentOf(line)
	rest := line[indent:]
	b := Block{Kind: Paragraph, Level: levelOf(line[:indent])}
	from := indent
	switch box, checked, ok := taskBox(line); {
	case headingLevel(rest) > 0:
		b.Kind, from = Heading, indent+headingLevel(rest)+1
	case ok:
		b.Kind, b.Done, from = Task, checked, min(box+4, len(line))
	case bulletAt(rest):
		b.Kind, from = Bullet, indent+2
	case strings.HasPrefix(rest, ">"):
		b.Kind, from = Quote, indent+1
		if strings.HasPrefix(rest, "> ") {
			from++
		}
	}
	b.Runs = runs(line, from)
	return b
}

// levelOf counts indentation steps: a tab or two spaces each.
func levelOf(indent string) int {
	n := 0
	spaces := 0
	for _, c := range indent {
		if c == '\t' {
			n++
			spaces = 0
			continue
		}
		if spaces++; spaces == 2 {
			n++
			spaces = 0
		}
	}
	return n
}

// runs returns the text of line from byte from on, without its inline
// markers, split wherever the style changes.
func runs(line string, from int) []Run {
	spans := appendInline(nil, line, 0, from)
	style := make([]Style, len(line))
	for _, sp := range spans {
		for i := sp.Start; i < sp.End && i < len(line); i++ {
			style[i] |= sp.Style
		}
	}
	var out []Run
	var cur strings.Builder
	var have bool
	var bold, code bool
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, Run{Text: cur.String(), Bold: bold, Code: code})
			cur.Reset()
		}
	}
	for i := from; i < len(line); i++ {
		s := style[i]
		if s&Muted != 0 {
			continue // a marker
		}
		b, c := s&Bold != 0, s&Code != 0
		if have && (b != bold || c != code) {
			flush()
		}
		bold, code, have = b, c, true
		cur.WriteByte(line[i])
	}
	flush()
	return out
}

// Tasks counts the task list items in a note and how many are ticked.
func Tasks(text string) (done, total int) {
	for _, line := range strings.Split(text, "\n") {
		if _, checked, ok := taskBox(strings.TrimRight(line, "\r")); ok {
			total++
			if checked {
				done++
			}
		}
	}
	return done, total
}

// MarkKind is the kind of a list marker the editor draws as a picture.
type MarkKind int

// The kinds of list markers.
const (
	TaskMark MarkKind = iota
	BulletMark
)

// Mark is a list marker the editor shows as a drawn box or bullet in place
// of its characters. Offsets are bytes of the text.
type Mark struct {
	Kind MarkKind
	// Start and End are the characters the picture covers: "[ ]" or "[x]"
	// for a task, the bullet character for a bullet.
	Start, End int
	// Hide is the syntax before a task's box, its bullet and space, which
	// the editor hides altogether.
	Hide [2]int
	// Checked marks a ticked task.
	Checked bool
}

// Marks finds the task boxes and bullets of a note.
func Marks(text string) []Mark {
	var marks []Mark
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		line := strings.TrimSuffix(line, "\n")
		indent := indentOf(line)
		if box, checked, ok := taskBox(line); ok {
			marks = append(marks, Mark{
				Kind: TaskMark, Start: offset + box, End: offset + box + 3,
				Hide: [2]int{offset + indent, offset + box}, Checked: checked,
			})
		} else if headingLevel(line[indent:]) == 0 && bulletAt(line[indent:]) {
			marks = append(marks, Mark{Kind: BulletMark, Start: offset + indent, End: offset + indent + 1})
		}
		offset += len(line) + 1
	}
	return marks
}

// TitleLine returns the byte range of a note's first non-blank line, which
// is its title, and the end of the blank lines after it. ok is false for a
// blank note.
func TitleLine(text string) (start, end, blockEnd int, ok bool) {
	offset := 0
	lines := strings.SplitAfter(text, "\n")
	for i, line := range lines {
		bare := strings.TrimSuffix(line, "\n")
		if strings.TrimSpace(bare) == "" {
			offset += len(line)
			continue
		}
		start, end = offset, offset+len(bare)
		blockEnd = offset + len(line)
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) != "" {
				break
			}
			blockEnd += len(next)
		}
		return start, end, blockEnd, true
	}
	return 0, 0, 0, false
}
