package app

import (
	"image/color"
	"log"
	"math"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/note"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/win32"
)

const (
	saveTimer = 1
	// enterTimer and exitTimer drive the animations of a note opening and
	// closing.
	enterTimer = 2
	exitTimer  = 3
	// saveDelay is how long typing must pause before the note is written
	// to disk.
	saveDelay = 400 // milliseconds
	// maxNoteChars caps the size of a note; the rich edit default of 32K
	// characters is too small for long-running notes.
	maxNoteChars = 4 << 20
)

// Editor layout in device-independent pixels.
const (
	editorPadLeft   = 16
	editorPadTop    = 10
	editorPadRight  = 4
	editorPadBottom = 12
	// resizeBorder is the width of the invisible band along the window's
	// edges that resizes it.
	resizeBorder = 6
	// editorGap is the space between the editor and the deck.
	editorGap = 10
	// A note opening slides in this far from the deck as it fades in.
	enterSlide = 16
)

// enterDuration is how long a note takes to open: within Material Design's
// range for a medium-sized element entering the screen.
// Closing is quicker, as Material Design has things leave faster than they
// arrive.
const (
	enterDuration = 300 * time.Millisecond
	exitDuration  = 160 * time.Millisecond
)

// editorWindow is the borderless window in which one note at a time is
// edited. Its header is drawn by the render package; the text area is a
// native edit control, which provides IME support, undo, accessibility and
// every text-editing shortcut users expect.
type editorWindow struct {
	app  *App
	hwnd win32.HWND
	edit win32.HWND
	// rich reports whether edit is a rich edit control. If that DLL is
	// unavailable the editor falls back to a classic EDIT control.
	rich bool
	// doc is the rich edit control's text object model, used to keep
	// restyling out of the undo history.
	doc *win32.TextDocument
	// editProc is the text control's own window procedure (see
	// subclassEdit).
	editProc uintptr

	note   *note.Note
	shown  bool
	pinned bool
	// folded is true while a pinned note is rolled up into its header;
	// fullHeight is then the window's height unfolded.
	folded     bool
	fullHeight int32
	// loading suppresses change notifications while text is set from code.
	loading bool
	dirty   bool

	scale float64
	font  win32.HFONT
	paper win32.HBRUSH
	sw    render.Swatch
	// border outlines the window where Desktop Window Manager does not.
	border win32.HBRUSH
	// drawBorder is true before Windows 11, which neither rounds the window
	// nor colours its border; shadow then does both.
	drawBorder bool
	shadow     *shadowWindow

	header  *render.Canvas
	dib     *win32.DIB
	hover   render.HeaderButton
	pressed render.HeaderButton
	// tracking is true while a WM_MOUSELEAVE notification is pending.
	tracking bool
	// moveStart is the window rectangle when a drag or resize began.
	moveStart win32.RECT

	// title is the note's title as shown in the header.
	title string
	// editRange is the byte range of the lines being edited, where Markdown
	// syntax shows; revealOn is false while the text is not focused.
	editRange [2]int
	revealOn  bool
	// marks are the task boxes and bullets drawn over the text, and
	// markImages their pictures.
	marks      []drawnMark
	markImages map[markKey]*win32.DIB

	// entering is set while the window animates open toward enterRect,
	// starting enterSlide pixels toward the deck (enterDX).
	entering  bool
	enterAt   time.Time
	enterRect win32.RECT
	enterDX   int32
	// exiting is set while the window fades out after closing, sliding back
	// toward the deck from exitFrom.
	exiting  bool
	exitAt   time.Time
	exitFrom win32.RECT
}

func newEditorWindow(a *App) (*editorWindow, error) {
	e := &editorWindow{app: a, scale: 1, header: render.NewCanvas(1, 1)}

	hwnd, err := createWindow(e, a.inst,
		win32.WS_EX_TOOLWINDOW|win32.WS_EX_TOPMOST,
		editorClass, appName,
		win32.WS_POPUP|win32.WS_THICKFRAME|win32.WS_CLIPCHILDREN)
	if err != nil {
		return nil, err
	}
	e.hwnd = hwnd
	win32.DwmSetWindowAttribute(hwnd, win32.DWMWA_WINDOW_CORNER_PREFERENCE, win32.DWMWCP_ROUND)
	// Windows 10 drops the shadow of a window without a frame unless the
	// frame reaches into the client area. One pixel at the top is enough, and
	// the header covers it with opaque pixels.
	win32.DwmExtendFrameIntoClientArea(hwnd, win32.MARGINS{Top: 1})
	e.drawBorder = !win32.IsWindows11()
	if e.drawBorder {
		if e.shadow, err = newShadowWindow(a); err != nil {
			// Only looks suffer; the note still works.
			log.Printf("editor: shadow: %v", err)
			e.shadow = nil
		}
	}

	if err := e.createEdit(); err != nil {
		return nil, err
	}
	return e, nil
}

// createEdit creates the text control, preferring the modern rich edit.
func (e *editorWindow) createEdit() error {
	const style = win32.WS_CHILD | win32.WS_VISIBLE | win32.WS_VSCROLL |
		win32.ES_MULTILINE | win32.ES_AUTOVSCROLL | win32.ES_WANTRETURN | win32.ES_NOHIDESEL

	// Loading the DLL registers the RICHEDIT50W window class.
	if _, err := windows.LoadLibrary("Msftedit.dll"); err == nil {
		if h, err := win32.CreateWindowEx(0, "RICHEDIT50W", "", style, 0, 0, 0, 0, e.hwnd, 0, e.app.inst); err == nil {
			e.edit, e.rich = h, true
			// Rich text mode is only used to style the Markdown on screen
			// (see restyle); the note itself stays plain text.
			win32.SendMessage(h, win32.EM_SETTEXTMODE, win32.TM_RICHTEXT|win32.TM_MULTILEVELUNDO, 0)
			win32.SendMessage(h, win32.EM_EXLIMITTEXT, 0, maxNoteChars)
			win32.SendMessage(h, win32.EM_AUTOURLDETECT, win32.AURL_ENABLEURL, 0)
			win32.SendMessage(h, win32.EM_SETEVENTMASK, 0, win32.ENM_CHANGE|win32.ENM_LINK|win32.ENM_SELCHANGE)
			e.doc = win32.RichEditDocument(h)
			e.subclassEdit()
			return nil
		}
	}

	log.Print("editor: rich edit unavailable, falling back to a plain edit control")
	h, err := win32.CreateWindowEx(0, "EDIT", "", style, 0, 0, 0, 0, e.hwnd, 0, e.app.inst)
	if err != nil {
		return err
	}
	e.edit = h
	return nil
}

// open shows a note in the editor at the given screen rectangle. If the
// editor is pinned and already visible it stays where the user put it.
func (e *editorWindow) open(n *note.Note, at win32.RECT, scale float64) {
	e.finishExit()
	previous := e.note
	if previous != nil && previous != n {
		e.flush()
	}

	e.note = n
	e.setScale(scale)
	e.applyTheme()
	e.setText(n.Body)

	if !(e.shown && e.pinned) {
		win32.SetWindowPos(e.hwnd, win32.HWND_TOPMOST, at.Left, at.Top, at.Width(), at.Height(), win32.SWP_NOACTIVATE)
	}
	e.layout()

	if !e.shown && !e.app.instant {
		e.startEntrance(at)
	}
	e.shown = true
	win32.ShowWindow(e.hwnd, win32.SW_SHOW)
	win32.SetForegroundWindow(e.hwnd)
	win32.SetFocus(e.edit)
	// Put the caret at the end, ready to continue the note.
	end := win32.SendMessage(e.edit, win32.WM_GETTEXTLENGTH, 0, 0)
	win32.SendMessage(e.edit, win32.EM_SETSEL, end, end)

	if previous != nil && previous != n {
		e.app.noteClosed(previous)
	}
}

// startEntrance makes the window, about to be shown at r, fade in while
// sliding out from the deck, as if it came out of its tab.
func (e *editorWindow) startEntrance(r win32.RECT) {
	e.entering, e.enterAt, e.enterRect = true, time.Now(), r
	e.enterDX = e.px(enterSlide)
	if e.app.cfg.Edge == config.EdgeLeft {
		e.enterDX = -e.enterDX
	}
	win32.SetExStyle(e.hwnd, win32.ExStyle(e.hwnd)|win32.WS_EX_LAYERED)
	win32.SetLayeredAlpha(e.hwnd, 0)
	e.shadow.setAlpha(0)
	win32.SetWindowPos(e.hwnd, 0, r.Left+e.enterDX, r.Top, 0, 0, win32.SWP_NOSIZE|win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
	win32.SetTimer(e.hwnd, enterTimer, 10)
}

// stepEntrance moves the opening animation on to the current time.
func (e *editorWindow) stepEntrance() {
	t := float64(time.Since(e.enterAt)) / float64(enterDuration)
	if t >= 1 || !e.shown {
		e.finishEntrance()
		return
	}
	// The window fades in over the first half and keeps settling into
	// place after it is opaque.
	alpha := uint8(math.Round(255 * math.Min(1, t*2)))
	win32.SetLayeredAlpha(e.hwnd, alpha)
	e.shadow.setAlpha(alpha)
	x := e.enterRect.Left + int32(math.Round(float64(e.enterDX)*(1-deck.EmphasizedDecelerate(t))))
	win32.SetWindowPos(e.hwnd, 0, x, e.enterRect.Top, 0, 0, win32.SWP_NOSIZE|win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
}

// finishEntrance ends the opening animation, leaving the window where it
// was headed.
func (e *editorWindow) finishEntrance() {
	if !e.entering {
		return
	}
	e.entering = false
	win32.KillTimer(e.hwnd, enterTimer)
	win32.SetWindowPos(e.hwnd, 0, e.enterRect.Left, e.enterRect.Top, 0, 0, win32.SWP_NOSIZE|win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
	win32.SetExStyle(e.hwnd, win32.ExStyle(e.hwnd)&^win32.WS_EX_LAYERED)
	e.shadow.setAlpha(255)
	win32.InvalidateRect(e.hwnd, nil)
}

// startExit fades the window out while it slides back toward the deck, then
// hides it.
func (e *editorWindow) startExit() {
	if e.app.instant || !win32.IsWindowVisible(e.hwnd) {
		win32.ShowWindow(e.hwnd, win32.SW_HIDE)
		return
	}
	e.exiting, e.exitAt, e.exitFrom = true, time.Now(), win32.GetWindowRect(e.hwnd)
	win32.SetExStyle(e.hwnd, win32.ExStyle(e.hwnd)|win32.WS_EX_LAYERED)
	win32.SetLayeredAlpha(e.hwnd, 255)
	win32.SetTimer(e.hwnd, exitTimer, 10)
}

// stepExit moves the closing animation on to the current time.
func (e *editorWindow) stepExit() {
	t := float64(time.Since(e.exitAt)) / float64(exitDuration)
	if t >= 1 {
		e.finishExit()
		return
	}
	p := deck.EmphasizedAccelerate(t)
	alpha := uint8(math.Round(255 * (1 - math.Min(1, t*1.2))))
	win32.SetLayeredAlpha(e.hwnd, alpha)
	e.shadow.setAlpha(alpha)
	dx := e.px(enterSlide)
	if e.app.cfg.Edge == config.EdgeLeft {
		dx = -dx
	}
	x := e.exitFrom.Left + int32(math.Round(float64(dx)*p))
	win32.SetWindowPos(e.hwnd, 0, x, e.exitFrom.Top, 0, 0, win32.SWP_NOSIZE|win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
}

// finishExit hides a window that is fading out, at once.
func (e *editorWindow) finishExit() {
	if !e.exiting {
		return
	}
	e.exiting = false
	win32.KillTimer(e.hwnd, exitTimer)
	win32.ShowWindow(e.hwnd, win32.SW_HIDE)
	win32.SetExStyle(e.hwnd, win32.ExStyle(e.hwnd)&^win32.WS_EX_LAYERED)
	e.shadow.setAlpha(255)
}

// hide saves and closes the editor.
func (e *editorWindow) hide() {
	if !e.shown {
		return
	}
	e.finishEntrance()
	// Clear shown first: hiding the window deactivates it, which would
	// otherwise re-enter hide.
	e.shown = false
	e.flush()
	e.startExit()
	if e.folded {
		// The next note opens at full size.
		e.folded = false
		win32.ShowWindow(e.edit, win32.SW_SHOW)
	}

	n := e.note
	e.note = nil
	e.pinned = false
	e.hover, e.pressed = render.ButtonNone, render.ButtonNone
	if n != nil {
		e.app.noteClosed(n)
	}
	e.app.editorClosed(e)
}

// restore shows a note pinned at a rectangle, as it was when Tuck last
// ran, without taking focus from whatever the user is doing.
func (e *editorWindow) restore(n *note.Note, at win32.RECT, scale float64) {
	e.finishExit()
	e.note = n
	e.pinned = true
	e.setScale(scale)
	e.applyTheme()
	e.setText(n.Body)
	// Showing the window through SetWindowPos, not ShowWindow, keeps a newly
	// started process from handing it the keyboard.
	win32.SetWindowPos(e.hwnd, win32.HWND_TOPMOST, at.Left, at.Top, at.Width(), at.Height(),
		win32.SWP_NOACTIVATE|win32.SWP_SHOWWINDOW)
	e.layout()
	e.shown = true
}

// focus brings the editor to the front, unfolded, and puts the keyboard in
// it.
func (e *editorWindow) focus() {
	e.setFolded(false)
	win32.SetForegroundWindow(e.hwnd)
	win32.SetFocus(e.edit)
}

// setFolded rolls the note up into its header, or back down. Folding pins
// the note: a folded note is one the user wants to keep at hand.
func (e *editorWindow) setFolded(folded bool) {
	if e.folded == folded || !e.shown {
		return
	}
	r := win32.GetWindowRect(e.hwnd)
	if folded {
		e.flush()
		e.setPinned(true)
		e.folded, e.fullHeight = true, r.Height()
		win32.ShowWindow(e.edit, win32.SW_HIDE)
		win32.SetWindowPos(e.hwnd, 0, 0, 0, r.Width(), int32(e.headerLayout().Height()),
			win32.SWP_NOMOVE|win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
	} else {
		e.folded = false
		// Unfold downwards, unless that would run off the bottom of the
		// screen.
		top := r.Top
		if mi, ok := win32.GetMonitorInfo(win32.MonitorFromWindow(e.hwnd, win32.MONITOR_DEFAULTTONEAREST)); ok {
			top = max(mi.Work.Top, min(top, mi.Work.Bottom-e.fullHeight))
		}
		win32.SetWindowPos(e.hwnd, 0, r.Left, top, r.Width(), e.fullHeight, win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
		e.layout()
		win32.ShowWindow(e.edit, win32.SW_SHOW)
	}
	win32.InvalidateRect(e.hwnd, nil)
	e.app.saveSession()
}

// reveal selects the n bytes of the note's text from byte at, such as a
// search match, and scrolls them into view.
func (e *editorWindow) reveal(at, n int) {
	text := e.text()
	if at < 0 || at+n > len(text) {
		return
	}
	pos := utf16Offsets(text)
	sel := win32.CHARRANGE{Min: pos[at], Max: pos[at+n]}
	win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&sel)))
	win32.SendMessage(e.edit, win32.EM_SCROLLCARET, 0, 0)
}

// reload shows the open note's text again after it changed on disk, keeping
// the caret and selection where they were as far as possible.
func (e *editorWindow) reload() {
	if e.note == nil {
		return
	}
	var sel win32.CHARRANGE
	win32.SendMessage(e.edit, win32.EM_EXGETSEL, 0, uintptr(unsafe.Pointer(&sel)))
	e.applyTheme()
	e.setText(e.note.Body)
	win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&sel)))
}

// showing reports whether the editor is open on the given note.
func (e *editorWindow) showing(id string) bool {
	return e.shown && e.note != nil && e.note.ID == id
}

// flush writes any pending edits to disk immediately.
func (e *editorWindow) flush() {
	win32.KillTimer(e.hwnd, saveTimer)
	if !e.dirty || e.note == nil {
		return
	}
	e.dirty = false
	e.note.Body = e.text()
	e.note.Updated = time.Now().UTC().Truncate(time.Second)
	e.app.saveNote(e.note)
}

// text returns the edit control's content with "\n" line endings.
func (e *editorWindow) text() string {
	s := win32.WindowText(e.edit)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// setText replaces the edit control's content without marking it dirty.
func (e *editorWindow) setText(body string) {
	e.loading = true
	win32.SetWindowText(e.edit, strings.ReplaceAll(body, "\n", "\r\n"))
	e.restyle()
	e.loading = false
	e.dirty = false
	e.updateTitle()
}

// updateTitle shows the note's current first line in the header.
func (e *editorWindow) updateTitle() {
	if title := (&note.Note{Body: e.text()}).Title(); title != e.title {
		e.title = title
		e.invalidateHeader()
	}
}

// setScale adapts fonts and metrics to the display scale factor.
func (e *editorWindow) setScale(scale float64) {
	if scale == e.scale && e.font != 0 {
		return
	}
	e.scale = scale
	e.dropMarkImages()
	if e.font != 0 {
		win32.DeleteObject(win32.HGDIOBJ(e.font))
	}
	// Points are 1/72 inch and a scale of 1 is 96 pixels per inch.
	px := float64(e.app.cfg.FontSize) * 96 / 72 * scale
	e.font = win32.CreateFont(int32(math.Round(px)), win32.FW_NORMAL, win32.CLEARTYPE_QUALITY, uiFont)
	e.loading = true
	win32.SendMessage(e.edit, win32.WM_SETFONT, uintptr(e.font), 1)
	e.restyle()
	e.loading = false
}

// applyFont recreates the font after the font size setting changed.
func (e *editorWindow) applyFont() {
	scale := e.scale
	e.scale = 0 // force setScale to rebuild the font
	e.setScale(scale)
}

// applyTheme recolours the window for the current note and theme.
func (e *editorWindow) applyTheme() {
	if e.note == nil {
		return
	}
	dark := e.app.dark()
	e.sw = render.Palette(e.note.Color, dark)
	e.dropMarkImages()

	if e.paper != 0 {
		win32.DeleteObject(win32.HGDIOBJ(e.paper))
	}
	e.paper = win32.CreateSolidBrush(colorRef(e.sw.Paper))
	if e.border != 0 {
		win32.DeleteObject(win32.HGDIOBJ(e.border))
	}
	e.border = win32.CreateSolidBrush(colorRef(e.sw.Border))

	win32.DwmSetWindowAttribute(e.hwnd, win32.DWMWA_BORDER_COLOR, uint32(colorRef(e.sw.Border)))
	var darkFlag uint32
	theme := "Explorer"
	if dark {
		darkFlag, theme = 1, "DarkMode_Explorer"
	}
	win32.DwmSetWindowAttribute(e.hwnd, win32.DWMWA_USE_IMMERSIVE_DARK_MODE, darkFlag)
	// Gives the scroll bar a matching light or dark appearance.
	win32.SetWindowTheme(e.edit, theme)

	if e.rich {
		win32.SendMessage(e.edit, win32.EM_SETBKGNDCOLOR, 0, uintptr(colorRef(e.sw.Paper)))
	}
	e.loading = true
	e.restyle()
	e.loading = false

	win32.InvalidateRect(e.hwnd, nil)
	win32.InvalidateRect(e.edit, nil)
	if e.shown {
		e.shadow.follow(e, true)
	}
}

// colorRef converts an opaque colour to GDI's representation.
func colorRef(c color.NRGBA) win32.COLORREF {
	return win32.RGB(c.R, c.G, c.B)
}

// px converts device-independent pixels to physical pixels.
func (e *editorWindow) px(dip float64) int32 {
	return int32(math.Round(dip * e.scale))
}

func (e *editorWindow) headerLayout() render.HeaderLayout {
	return render.HeaderLayout{Width: int(win32.GetClientRect(e.hwnd).Width()), Scale: e.scale}
}

// layout positions the text control below the header, inset from the
// window's edges.
func (e *editorWindow) layout() {
	client := win32.GetClientRect(e.hwnd)
	top := int32(e.headerLayout().Height()) + e.px(editorPadTop)
	win32.MoveWindow(e.edit,
		e.px(editorPadLeft), top,
		max(client.Width()-e.px(editorPadLeft)-e.px(editorPadRight), 0),
		max(client.Height()-top-e.px(editorPadBottom), 0),
		true)
}

// paint draws the header and fills the rest of the window with the paper
// colour; the text control paints itself on top.
func (e *editorWindow) paint() {
	var ps win32.PAINTSTRUCT
	dc := win32.BeginPaint(e.hwnd, &ps)
	defer win32.EndPaint(e.hwnd, &ps)

	client := win32.GetClientRect(e.hwnd)
	layout := e.headerLayout()
	if layout.Width <= 0 {
		return
	}
	state := render.HeaderState{Hover: e.hover, Pressed: e.pressed, Pinned: e.pinned, Title: e.title}
	if state.Title == "" {
		state.Title, state.Placeholder = "New note", true
	}
	render.DrawHeader(e.header, layout, e.sw, state, e.app.text)

	w, h := layout.Width, layout.Height()
	if e.dib == nil || e.dib.Width != w || e.dib.Height != h {
		e.dib.Close()
		var err error
		if e.dib, err = win32.NewDIB(w, h); err != nil {
			e.dib = nil
			log.Printf("editor: %v", err)
			return
		}
	}
	e.dib.CopyFrom(e.header.Img)
	win32.BitBlt(dc, 0, 0, int32(w), int32(h), e.dib.DC, 0, 0)

	body := win32.RECT{Top: int32(h), Right: client.Right, Bottom: client.Bottom}
	win32.FillRect(dc, &body, e.paper)

	if e.drawBorder && e.shadow == nil {
		// The text control is inset from every edge, so a hairline drawn
		// here is never covered.
		t := max(1, e.px(1))
		for _, r := range []win32.RECT{
			{Right: client.Right, Bottom: t},
			{Top: client.Bottom - t, Right: client.Right, Bottom: client.Bottom},
			{Bottom: client.Bottom, Right: t},
			{Left: client.Right - t, Right: client.Right, Bottom: client.Bottom},
		} {
			win32.FillRect(dc, &r, e.border)
		}
	}
}

func (e *editorWindow) invalidateHeader() {
	r := win32.RECT{Right: win32.GetClientRect(e.hwnd).Right, Bottom: int32(e.headerLayout().Height())}
	win32.InvalidateRect(e.hwnd, &r)
}

// hitTest implements WM_NCHITTEST for the borderless window: the edges
// resize, the header drags and everything else is ordinary client area.
func (e *editorWindow) hitTest(screen win32.POINT) uintptr {
	r := win32.GetWindowRect(e.hwnd)
	x, y := screen.X-r.Left, screen.Y-r.Top
	border := e.px(resizeBorder)

	left, right := x < border, x >= r.Width()-border
	// A folded note is as tall as its header and only gets wider.
	top, bottom := y < border && !e.folded, y >= r.Height()-border && !e.folded
	switch {
	case top && left:
		return win32.HTTOPLEFT
	case top && right:
		return win32.HTTOPRIGHT
	case bottom && left:
		return win32.HTBOTTOMLEFT
	case bottom && right:
		return win32.HTBOTTOMRIGHT
	case left:
		return win32.HTLEFT
	case right:
		return win32.HTRIGHT
	case top:
		return win32.HTTOP
	case bottom:
		return win32.HTBOTTOM
	}

	layout := e.headerLayout()
	if int(y) < layout.Height() && layout.Hit(int(x), int(y)) == render.ButtonNone {
		return win32.HTCAPTION
	}
	return win32.HTCLIENT
}

// click runs the action of a header button.
func (e *editorWindow) click(b render.HeaderButton) {
	switch b {
	case render.ButtonNew:
		e.app.newNote()
	case render.ButtonMenu:
		if e.note != nil {
			r := e.headerLayout().ButtonRect(render.ButtonMenu)
			origin := win32.GetWindowRect(e.hwnd)
			e.app.showNoteMenu(e.note.ID, win32.POINT{X: origin.Left + int32(r.X), Y: origin.Top + int32(r.Y+r.H)})
		}
	case render.ButtonPin:
		e.setPinned(!e.pinned)
	case render.ButtonClose:
		e.hide()
	}
}

// setPinned controls whether the editor stays open when it loses focus.
func (e *editorWindow) setPinned(pinned bool) {
	if e.pinned == pinned {
		return
	}
	if !pinned {
		e.setFolded(false)
	}
	e.pinned = pinned
	e.invalidateHeader()
	e.app.updateHold()
	e.app.saveSession()
}

// handleKey processes editor-wide keyboard shortcuts before the text control
// sees the key. It reports whether the key was consumed.
func (e *editorWindow) handleKey(vk uintptr) bool {
	ctrl := win32.KeyDown(win32.VK_CONTROL)
	shift := win32.KeyDown(win32.VK_SHIFT)
	alt := win32.KeyDown(win32.VK_MENU)

	if vk == win32.VK_ESCAPE {
		e.hide()
		return true
	}
	if e.rich && (ctrl && !alt && !shift && vk == 'V' || shift && !ctrl && vk == win32.VK_INSERT) {
		// Paste words, never formatting or pictures.
		win32.SendMessage(e.edit, win32.EM_PASTESPECIAL, win32.CF_UNICODETEXT, 0)
		return true
	}
	// AltGr is reported as Ctrl+Alt and types characters on many layouts.
	if !ctrl || alt {
		return false
	}

	switch vk {
	case 'N':
		e.app.newNote()
	case 'F':
		e.app.showSearch()
	case 'W':
		e.hide()
	case 'S':
		e.flush()
	case 'P':
		e.setPinned(!e.pinned)
	case 'B':
		e.wrapSelection("**")
	case 'I':
		e.wrapSelection("_")
	case win32.VK_OEM_PERIOD:
		e.app.cycleColor(e)
	case win32.VK_NEXT:
		e.app.stepNote(e, +1)
	case win32.VK_PRIOR:
		e.app.stepNote(e, -1)
	case win32.VK_BACK:
		if !shift {
			return false
		}
		if e.note != nil {
			e.app.deleteNote(e.note.ID)
		}
	case 'E', 'L', 'R', 'J', '1', '2', '5', 'U', 'A', win32.VK_OEM_PLUS, win32.VK_OEM_4, win32.VK_OEM_6:
		// Rich edit formatting shortcuts, which make no sense for plain
		// text. Plain Ctrl+A still selects everything.
		return e.rich && (vk != 'A' || shift)
	default:
		return false
	}
	return true
}

// wrapSelection surrounds the selected text with a Markdown marker, or
// inserts an empty pair with the caret in between.
func (e *editorWindow) wrapSelection(marker string) {
	if !e.rich {
		return
	}
	var sel win32.CHARRANGE
	win32.SendMessage(e.edit, win32.EM_EXGETSEL, 0, uintptr(unsafe.Pointer(&sel)))

	selected := ""
	if n := sel.Max - sel.Min; n > 0 {
		buf := make([]uint16, n+1)
		win32.SendMessage(e.edit, win32.EM_GETSELTEXT, 0, uintptr(unsafe.Pointer(&buf[0])))
		selected = win32.UTF16ToString(buf)
	}
	const canUndo = 1
	win32.SendMessage(e.edit, win32.EM_REPLACESEL, canUndo, uintptr(unsafe.Pointer(win32.UTF16(marker+selected+marker))))

	if selected == "" {
		caret := win32.CHARRANGE{Min: sel.Min + int32(len(marker)), Max: sel.Min + int32(len(marker))}
		win32.SendMessage(e.edit, win32.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&caret)))
	}
}

// endMove runs when the user finishes dragging or resizing the window.
func (e *editorWindow) endMove() {
	r := win32.GetWindowRect(e.hwnd)
	resized := r.Width() != e.moveStart.Width() || r.Height() != e.moveStart.Height()
	moved := r.Left != e.moveStart.Left || r.Top != e.moveStart.Top

	if resized && e.folded {
		// Only the width can change; keep it for when the note unfolds.
		e.app.cfg.EditorWidth = int(math.Round(float64(r.Width()) / e.scale))
		e.app.saveConfig()
	} else if resized {
		e.app.cfg.EditorWidth = int(math.Round(float64(r.Width()) / e.scale))
		e.app.cfg.EditorHeight = int(math.Round(float64(r.Height()) / e.scale))
		e.app.saveConfig()
	} else if moved {
		// Dragging the note somewhere is a clear sign the user wants it to
		// stay there.
		e.setPinned(true)
	}
}

func (e *editorWindow) wndProc(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case win32.WM_NCCALCSIZE:
		if wp != 0 {
			// Claim the whole window as client area, removing the standard
			// frame while keeping its shadow and resize behaviour.
			return 0
		}

	case win32.WM_NCHITTEST:
		return e.hitTest(win32.PointFromLParam(lp))

	case win32.WM_NCACTIVATE:
		// Passing -1 stops Windows from repainting the (removed) frame.
		return win32.DefWindowProc(hwnd, msg, wp, ^uintptr(0))

	case win32.WM_GETMINMAXINFO:
		info := win32.PointerFromLParam[win32.MINMAXINFO](lp)
		info.MinTrackSize = win32.POINT{X: e.px(config.MinEditorWidth), Y: e.px(config.MinEditorHeight)}
		if e.folded {
			info.MinTrackSize.Y = int32(e.headerLayout().Height())
		}
		return 0

	case win32.WM_NCLBUTTONDBLCLK:
		// Double-clicking the header folds the note up, as in Stickies.
		if wp == win32.HTCAPTION {
			e.setFolded(!e.folded)
			return 0
		}

	case win32.WM_SIZE:
		if e.edit != 0 {
			e.layout()
			win32.InvalidateRect(hwnd, nil)
		}
		e.shadow.clip(e)
		return 0

	case win32.WM_WINDOWPOSCHANGED:
		pos := win32.PointerFromLParam[win32.WINDOWPOS](lp)
		visible := pos.Flags&win32.SWP_HIDEWINDOW == 0 && (win32.IsWindowVisible(hwnd) || pos.Flags&win32.SWP_SHOWWINDOW != 0)
		e.shadow.follow(e, visible)
		// The default handler turns this into WM_SIZE and WM_MOVE.

	case win32.WM_ERASEBKGND:
		return 1

	case win32.WM_PAINT:
		e.paint()
		return 0

	case win32.WM_CTLCOLOREDIT:
		// Only the classic EDIT fallback asks for its colours this way.
		dc := win32.HDC(wp)
		win32.SetTextColor(dc, colorRef(e.sw.Text))
		win32.SetBkColor(dc, colorRef(e.sw.Paper))
		return uintptr(e.paper)

	case win32.WM_SETFOCUS:
		if !e.folded {
			win32.SetFocus(e.edit)
		}
		return 0

	case win32.WM_ACTIVATE:
		if win32.LoWord(wp) == win32.WA_INACTIVE && e.shown && !e.pinned && !e.app.menuOpen {
			// An unpinned note tucks itself away as soon as the user
			// turns to something else.
			e.hide()
		}

	case win32.WM_COMMAND:
		if win32.HiWord(wp) == win32.EN_CHANGE && win32.HWND(lp) == e.edit && !e.loading {
			e.dirty = true
			win32.SetTimer(hwnd, saveTimer, saveDelay)
			e.restyle()
			e.updateTitle()
		}
		return 0

	case win32.WM_NOTIFY:
		if hdr := win32.PointerFromLParam[win32.NMHDR](lp); hdr.From == e.edit && hdr.Code == win32.EN_SELCHANGE {
			e.selectionMoved()
		} else if hdr.From == e.edit && hdr.Code == win32.EN_LINK {
			link := win32.PointerFromLParam[win32.ENLINK](lp)
			if link.Msg == win32.WM_LBUTTONUP {
				e.openLink(link.Range)
			}
		}
		return 0

	case win32.WM_TIMER:
		switch wp {
		case saveTimer:
			e.flush()
			e.app.noteEdited()
		case enterTimer:
			e.stepEntrance()
		case exitTimer:
			e.stepExit()
		}
		return 0

	case win32.WM_MOUSEMOVE:
		pt := win32.PointFromLParam(lp)
		if hover := e.headerLayout().Hit(int(pt.X), int(pt.Y)); hover != e.hover {
			e.hover = hover
			e.invalidateHeader()
		}
		if !e.tracking {
			e.tracking = true
			win32.TrackMouseEvent(&win32.TRACKMOUSEEVENT{Flags: win32.TME_LEAVE, Track: hwnd})
		}
		return 0

	case win32.WM_MOUSELEAVE:
		e.tracking = false
		if e.hover != render.ButtonNone || e.pressed != render.ButtonNone {
			e.hover, e.pressed = render.ButtonNone, render.ButtonNone
			e.invalidateHeader()
		}
		return 0

	case win32.WM_LBUTTONDOWN:
		pt := win32.PointFromLParam(lp)
		e.pressed = e.headerLayout().Hit(int(pt.X), int(pt.Y))
		e.invalidateHeader()
		return 0

	case win32.WM_LBUTTONUP:
		pt := win32.PointFromLParam(lp)
		pressed := e.pressed
		e.pressed = render.ButtonNone
		e.invalidateHeader()
		if b := e.headerLayout().Hit(int(pt.X), int(pt.Y)); b != render.ButtonNone && b == pressed {
			e.click(b)
		}
		return 0

	case win32.WM_ENTERSIZEMOVE:
		e.finishEntrance()
		e.moveStart = win32.GetWindowRect(hwnd)
		return 0

	case win32.WM_EXITSIZEMOVE:
		e.endMove()
		if e.pinned {
			e.app.saveSession()
		}
		return 0

	case msgDiscard:
		win32.DestroyWindow(hwnd)
		return 0

	case win32.WM_DPICHANGED:
		// Windows suggests a rectangle scaled for the new monitor.
		suggested := win32.PointerFromLParam[win32.RECT](lp)
		e.setScale(float64(win32.HiWord(wp)) / win32.USER_DEFAULT_DPI)
		win32.SetWindowPos(hwnd, 0, suggested.Left, suggested.Top, suggested.Width(), suggested.Height(),
			win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
		return 0

	case win32.WM_SYSCOMMAND:
		if wp&0xFFF0 == win32.SC_CLOSE {
			// Alt+F4: the user closes this note.
			e.hide()
			return 0
		}

	case win32.WM_CLOSE:
		// Sent directly, without SC_CLOSE, this is a request to close the
		// app (taskkill, an installer, Restart Manager), which arrives at
		// every window. Quit, so the pinned notes are remembered as they are
		// instead of being closed one by one first.
		e.app.quit()
		return 0

	case win32.WM_DESTROY:
		e.dib.Close()
		e.dib = nil
		e.dropMarkImages()
		if e.shadow != nil {
			win32.DestroyWindow(e.shadow.hwnd)
		}
		if e.font != 0 {
			win32.DeleteObject(win32.HGDIOBJ(e.font))
		}
		if e.paper != 0 {
			win32.DeleteObject(win32.HGDIOBJ(e.paper))
		}
		if e.border != 0 {
			win32.DeleteObject(win32.HGDIOBJ(e.border))
		}
		e.doc.Release()
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wp, lp)
}
