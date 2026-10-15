package markdown

import (
	"slices"
	"strings"
)

// Edit replaces the bytes Start to End of a text with Text.
type Edit struct {
	Start, End int
	Text       string
}

// ToggleTask ticks or unticks the task whose box starts at byte box of text,
// as found by TaskBoxAt.
//
// With sink set, the task also moves within its list so that open tasks stay
// on top: a ticked task goes to just below the last open one, where the
// finished tasks begin, and an unticked one returns to the end of the open
// tasks. Lines indented under a task move with it. The list is the run of
// tasks at the same indent around the toggled one; anything else, including
// a blank line, ends it.
func ToggleTask(text string, box int, sink bool) Edit {
	mark := "x"
	if text[box+1] != ' ' {
		mark = " "
	}
	toggle := Edit{box + 1, box + 2, mark}
	if !sink {
		return toggle
	}

	lines := strings.Split(text, "\n")
	starts := make([]int, len(lines)+1)
	for i, l := range lines {
		starts[i+1] = starts[i] + len(l) + 1
	}
	at := 0
	for at+1 < len(lines) && starts[at+1] <= box {
		at++
	}
	indent := indentOf(lines[at])
	isTask := func(i int) bool {
		_, _, ok := taskBox(lines[i])
		return ok && indentOf(lines[i]) == indent
	}
	isChild := func(i int) bool {
		return strings.TrimSpace(lines[i]) != "" && indentOf(lines[i]) > indent
	}

	first := at
	for i := at - 1; i >= 0 && (isTask(i) || isChild(i)); i-- {
		if isTask(i) {
			first = i
		}
	}
	last := at
	for last+1 < len(lines) && (isTask(last+1) || isChild(last+1)) {
		last++
	}

	// Group the list into tasks, each with the lines indented under it.
	type task struct {
		lines []string
		done  bool
	}
	var tasks []task
	moved := -1
	for i := first; i <= last; i++ {
		if !isTask(i) {
			tasks[len(tasks)-1].lines = append(tasks[len(tasks)-1].lines, lines[i])
			continue
		}
		line := lines[i]
		if i == at {
			moved = len(tasks)
			b := box - starts[i]
			line = line[:b+1] + mark + line[b+2:]
		}
		_, done, _ := taskBox(line)
		tasks = append(tasks, task{lines: []string{line}, done: done})
	}

	t := tasks[moved]
	rest := append(append([]task{}, tasks[:moved]...), tasks[moved+1:]...)
	to := 0
	for i, r := range rest {
		if !r.done {
			to = i + 1
		}
	}
	if to == moved {
		return toggle
	}
	order := append(append(append([]task{}, rest[:to]...), t), rest[to:]...)
	var out []string
	for _, o := range order {
		out = append(out, o.lines...)
	}
	return Edit{starts[first], starts[last+1] - 1, strings.Join(out, "\n")}
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// Plain returns a note's text without its Markdown syntax, for pasting where
// Markdown is not understood: emphasis and heading markers are dropped,
// bullets become "•" and task boxes "☐" or "☑".
func Plain(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = plainLine(line)
	}
	return strings.Join(lines, "\n")
}

func plainLine(line string) string {
	indent := indentOf(line)
	rest := line[indent:]
	prefix, from := "", indent
	switch box, checked, ok := taskBox(line); {
	case headingLevel(rest) > 0:
		from = indent + headingLevel(rest) + 1
	case ok:
		prefix, from = "☐ ", min(box+4, len(line))
		if checked {
			prefix = "☑ "
		}
	case bulletAt(rest):
		prefix, from = "• ", indent+2
	}

	var b strings.Builder
	b.WriteString(line[:indent])
	b.WriteString(prefix)
	// Only the markers are removed. Nested emphasis lists its markers after
	// those around it, so put them in order first.
	var marks []Span
	for _, sp := range appendInline(nil, line, 0, from) {
		if sp.Style == Muted {
			marks = append(marks, sp)
		}
	}
	slices.SortFunc(marks, func(a, b Span) int { return a.Start - b.Start })
	i := from
	for _, sp := range marks {
		b.WriteString(line[i:sp.Start])
		i = sp.End
	}
	b.WriteString(line[i:])
	return b.String()
}
