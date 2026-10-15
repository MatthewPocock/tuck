package app

import (
	"image/color"
	"math"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/MatthewPocock/tuck/internal/markdown"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/win32"
)

const (
	// codeFont is used for `code` spans. Consolas ships with Windows.
	codeFont = "Consolas"
	// emojiFont has the emoji. Setting the font of the whole text, as
	// restyle does, overrides the font the control picked for them itself.
	emojiFont = "Segoe UI Emoji"
	// maxStyledChars bounds the work done on every keystroke: longer notes
	// are shown without Markdown styling.
	maxStyledChars = 100_000
)

// Relative sizes of headings.
var headingScale = map[markdown.Style]float64{
	markdown.Heading1: 1.4,
	markdown.Heading2: 1.2,
	markdown.Heading3: 1.08,
}

// restyle shows the note's Markdown: headings larger, emphasis in bold or
// italics, finished tasks struck through, and the syntax itself fainter. Only
// the display changes; the text stays plain. It also resets any formatting
// that pasting or a stray shortcut may have introduced.
func (e *editorWindow) restyle() {
	if !e.rich || e.note == nil {
		return
	}
	wasLoading := e.loading
	e.loading = true
	defer func() { e.loading = wasLoading }()

	// Styling works on the selection, so remember what the user had
	// selected and where the view was scrolled, and hide the work from the
	// screen and from undo.
	win32.SendMessage(e.edit, win32.WM_SETREDRAW, 0, 0)
	e.doc.SuspendUndo()
	var sel win32.CHARRANGE
	win32.SendMessage(e.edit, win32.EM_EXGETSEL, 0, uintptr(unsafe.Pointer(&sel)))
	var scroll win32.POINT
	win32.SendMessage(e.edit, win32.EM_GETSCROLLPOS, 0, uintptr(unsafe.Pointer(&scroll)))

	e.setFormat(win32.SCF_ALL, e.baseFormat())

	text := e.text()
	pos := utf16Offsets(text)
	if len(text) <= maxStyledChars {
		for _, sp := range markdown.Spans(text) {
			e.formatRange(pos[sp.Start], pos[sp.End], e.spanFormat(sp.Style))
		}
	}
	var emoji win32.CHARFORMAT
	emoji.Mask = win32.CFM_FACE
	copy(emoji.FaceName[:len(emoji.FaceName)-1], utf16.Encode([]rune(emojiFont)))
	for _, run := range emojiRuns(text) {
		e.formatRange(pos[run[0]], pos[run[1]], emoji)
	}

	win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&sel)))
	win32.SendMessage(e.edit, win32.EM_SETSCROLLPOS, 0, uintptr(unsafe.Pointer(&scroll)))
	e.doc.ResumeUndo()
	win32.SendMessage(e.edit, win32.WM_SETREDRAW, 1, 0)
	win32.InvalidateRect(e.edit, nil)
}

func (e *editorWindow) formatRange(min, max int32, f win32.CHARFORMAT) {
	r := win32.CHARRANGE{Min: min, Max: max}
	win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&r)))
	e.setFormat(win32.SCF_SELECTION, f)
}

// emojiRuns returns the byte ranges of text made of emoji, including the
// joiners, variation selectors and modifiers that combine them.
func emojiRuns(text string) [][2]int {
	var runs [][2]int
	start := -1
	for i, r := range text {
		switch {
		case isEmoji(r):
			if start < 0 {
				start = i
			}
		case start >= 0:
			runs = append(runs, [2]int{start, i})
			start = -1
		}
	}
	if start >= 0 {
		runs = append(runs, [2]int{start, len(text)})
	}
	return runs
}

func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF, // pictographs, flags, skin tones
		r >= 0x2600 && r <= 0x27BF,   // symbols and dingbats
		r >= 0x2B00 && r <= 0x2BFF,   // stars, squares, arrows
		r >= 0x2300 && r <= 0x23FF,   // watch, hourglass, media controls
		r == 0x200D,                  // zero width joiner
		r >= 0xFE00 && r <= 0xFE0F,   // variation selectors
		r >= 0xE0020 && r <= 0xE007F: // tag sequences
		return true
	}
	return false
}

func (e *editorWindow) setFormat(scope uintptr, f win32.CHARFORMAT) {
	f.Size = uint32(unsafe.Sizeof(f))
	win32.SendMessage(e.edit, win32.EM_SETCHARFORMAT, scope, uintptr(unsafe.Pointer(&f)))
}

// fontTwips is the editor font size in twentieths of a point, the unit of
// CHARFORMAT. Rich edit scales it for the display itself.
func (e *editorWindow) fontTwips(scale float64) int32 {
	return int32(math.Round(float64(e.app.cfg.FontSize) * 20 * scale))
}

// baseFormat is the plain text formatting every character starts from.
func (e *editorWindow) baseFormat() win32.CHARFORMAT {
	f := win32.CHARFORMAT{
		Mask: win32.CFM_BOLD | win32.CFM_ITALIC | win32.CFM_STRIKEOUT | win32.CFM_UNDERLINE |
			win32.CFM_OFFSET | win32.CFM_COLOR | win32.CFM_SIZE | win32.CFM_FACE,
		Height:    e.fontTwips(1),
		TextColor: colorRef(e.sw.Text),
	}
	copy(f.FaceName[:len(f.FaceName)-1], utf16.Encode([]rune(uiFont)))
	return f
}

// spanFormat is the formatting added on top of the base for a style. Only
// the attributes the style changes are in the mask, so overlapping spans
// combine.
func (e *editorWindow) spanFormat(s markdown.Style) win32.CHARFORMAT {
	var f win32.CHARFORMAT
	set := func(mask, effect uint32) {
		f.Mask |= mask
		f.Effects |= effect
	}
	if s&markdown.Bold != 0 {
		set(win32.CFM_BOLD, win32.CFE_BOLD)
	}
	if s&markdown.Italic != 0 {
		set(win32.CFM_ITALIC, win32.CFE_ITALIC)
	}
	if s&markdown.Strike != 0 {
		set(win32.CFM_STRIKEOUT, win32.CFE_STRIKEOUT)
	}
	if s&markdown.Muted != 0 {
		f.Mask |= win32.CFM_COLOR
		f.TextColor = colorRef(mutedText(e.sw))
	}
	if s&markdown.Code != 0 {
		f.Mask |= win32.CFM_FACE
		copy(f.FaceName[:len(f.FaceName)-1], utf16.Encode([]rune(codeFont)))
	}
	if h := s & markdown.Headings; h != 0 {
		f.Mask |= win32.CFM_SIZE
		f.Height = e.fontTwips(headingScale[h])
	}
	return f
}

// mutedText is the colour of Markdown syntax and finished tasks: the text
// colour halfway to the paper, which stays legible on every note colour.
func mutedText(sw render.Swatch) color.NRGBA {
	mix := func(a, b uint8) uint8 { return uint8((int(a) + int(b) + 1) / 2) }
	return color.NRGBA{mix(sw.Text.R, sw.Paper.R), mix(sw.Text.G, sw.Paper.G), mix(sw.Text.B, sw.Paper.B), 0xFF}
}

// utf16Offsets maps every byte offset of s, including len(s), to the
// position of the same point in the rich edit control, which counts UTF-16
// code units and one character per line break.
func utf16Offsets(s string) []int32 {
	pos := make([]int32, len(s)+1)
	var n int32
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		width := int32(utf16.RuneLen(r))
		if width < 1 {
			width = 1 // invalid UTF-8 reaches the control as U+FFFD
		}
		pos[i] = n
		// Bytes inside a character, never a span boundary, map past it.
		for k := 1; k < size; k++ {
			pos[i+k] = n + width
		}
		n += width
		i += size
	}
	pos[len(s)] = n
	return pos
}

// byteOffset maps a rich edit character position back to a byte offset of
// s, the inverse of utf16Offsets.
func byteOffset(s string, char int) int {
	n := 0
	for i, r := range s {
		if n >= char {
			return i
		}
		n += max(utf16.RuneLen(r), 1)
	}
	return len(s)
}

// taskBoxAt reports the task box under a point in the text control's client
// area, as a byte offset of the text.
func (e *editorWindow) taskBoxAt(pt win32.POINT) (text string, box int, checked, ok bool) {
	if !e.rich {
		return "", 0, false, false
	}
	char := int(win32.SendMessage(e.edit, win32.EM_CHARFROMPOS, 0, uintptr(unsafe.Pointer(&pt))))
	text = e.text()
	box, checked, ok = markdown.TaskBoxAt(text, byteOffset(text, char))
	return text, box, checked, ok
}

// toggleTask ticks or unticks the task whose box starts at byte box of text,
// moving it within its list if finished tasks are set to sink. It is an
// ordinary edit: it can be undone and is saved like typing.
func (e *editorWindow) toggleTask(text string, box int) {
	edit := markdown.ToggleTask(text, box, e.app.cfg.SinkDoneTasks)
	pos := utf16Offsets(text)
	var sel win32.CHARRANGE
	win32.SendMessage(e.edit, win32.EM_EXGETSEL, 0, uintptr(unsafe.Pointer(&sel)))
	target := win32.CHARRANGE{Min: pos[edit.Start], Max: pos[edit.End]}
	win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&target)))
	const canUndo = 1
	win32.SendMessage(e.edit, win32.EM_REPLACESEL, canUndo, uintptr(unsafe.Pointer(win32.UTF16(edit.Text))))
	// Moving lines keeps the length of the text, so the old selection is
	// still a valid one.
	win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&sel)))
}

var (
	editProcOnce sync.Once
	editProcPtr  uintptr
	// editors maps text controls to their editor, for editProc.
	editors = map[win32.HWND]*editorWindow{}
)

// subclassEdit routes the text control's messages through editProc, which
// makes task boxes clickable.
func (e *editorWindow) subclassEdit() {
	editProcOnce.Do(func() { editProcPtr = win32.NewWndProc(editProc) })
	editors[e.edit] = e
	e.editProc = win32.Subclass(e.edit, editProcPtr)
}

func editProc(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr {
	e := editors[hwnd]
	if e == nil {
		return 0
	}
	switch msg {
	case win32.WM_SETCURSOR:
		if win32.LoWord(lp) == win32.HTCLIENT {
			if pt, ok := win32.GetCursorPos(); ok {
				if _, _, _, onBox := e.taskBoxAt(win32.ScreenToClient(hwnd, pt)); onBox {
					win32.SetCursor(win32.LoadCursor(win32.IDC_HAND))
					return 1
				}
			}
		}
	case win32.WM_LBUTTONDOWN:
		if wp&win32.MK_SHIFT == 0 {
			if text, box, _, ok := e.taskBoxAt(win32.PointFromLParam(lp)); ok {
				win32.SetFocus(hwnd)
				e.toggleTask(text, box)
				return 0
			}
		}
	case win32.WM_DESTROY:
		delete(editors, hwnd)
	}
	return win32.CallWindowProc(e.editProc, hwnd, msg, wp, lp)
}

// openLink opens a link the user clicked, if it is a web or mail address.
// Other kinds of URL rich edit recognises, such as file: links, are left
// alone: a note may come from a synced folder someone else can write to.
func (e *editorWindow) openLink(r win32.CHARRANGE) {
	runes := utf16.Encode([]rune(e.text()))
	if r.Min < 0 || int(r.Max) > len(runes) || r.Min >= r.Max {
		return
	}
	url := string(utf16.Decode(runes[r.Min:r.Max]))
	lower := strings.ToLower(url)
	switch {
	case strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "mailto:"):
	case strings.HasPrefix(lower, "www."):
		url = "https://" + url
	default:
		return
	}
	win32.ShellOpen(url)
}
