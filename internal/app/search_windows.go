package app

import (
	"log"
	"math"
	"strings"
	"unicode"
	"unicode/utf16"
	"unsafe"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/note"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/search"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// searchQuerySize is the font size of the query in device-independent
// pixels.
const searchQuerySize = 17.0

// searchWindow finds notes by their words. It opens in the upper middle of
// the screen with the keyboard in its text box; the arrow keys choose a
// result and Enter opens it. It closes as soon as it loses focus.
type searchWindow struct {
	app   *App
	hwnd  win32.HWND
	input win32.HWND
	scale float64

	state  render.SearchState
	layout search.Layout
	canvas *render.Canvas
	dib    *win32.DIB
	font   win32.HFONT
	bg     win32.HBRUSH
	// tracking is true while a WM_MOUSELEAVE notification is pending.
	tracking bool
	// drawBorder is true before Windows 11, where DWM draws no border.
	drawBorder bool
}

func newSearchWindow(a *App) (*searchWindow, error) {
	w := &searchWindow{app: a, scale: 1, canvas: render.NewCanvas(1, 1)}
	w.state.Selected, w.state.Hover = -1, -1
	hwnd, err := createWindow(w, a.inst, win32.WS_EX_TOOLWINDOW|win32.WS_EX_TOPMOST, searchClass, appName+" search",
		win32.WS_POPUP|win32.WS_CLIPCHILDREN)
	if err != nil {
		return nil, err
	}
	w.hwnd = hwnd
	win32.DwmSetWindowAttribute(hwnd, win32.DWMWA_WINDOW_CORNER_PREFERENCE, win32.DWMWCP_ROUND)
	win32.DwmExtendFrameIntoClientArea(hwnd, win32.MARGINS{Top: 1})
	w.drawBorder = !win32.IsWindows11()

	w.input, err = win32.CreateWindowEx(0, "EDIT", "", win32.WS_CHILD|win32.WS_VISIBLE|win32.ES_AUTOHSCROLL,
		0, 0, 0, 0, hwnd, 0, a.inst)
	if err != nil {
		win32.DestroyWindow(hwnd)
		return nil, err
	}
	const showWhileFocused = 1
	win32.SendMessage(w.input, win32.EM_SETCUEBANNER, showWhileFocused, uintptr(unsafe.Pointer(win32.UTF16("Search notes"))))
	return w, nil
}

// show opens the window with an empty query, or brings it back to the front.
func (w *searchWindow) show() {
	if !win32.IsWindowVisible(w.hwnd) {
		disp := primaryDisplay()
		w.setScale(disp.scale)
		w.state.Hover = -1
		win32.SetWindowText(w.input, "")
		w.update()
		x := disp.work.Left + (disp.work.Width()-int32(w.layout.Width))/2
		y := disp.work.Top + disp.work.Height()/5
		win32.SetWindowPos(w.hwnd, win32.HWND_TOPMOST, x, y, int32(w.layout.Width), int32(w.layout.Height), 0)
		win32.ShowWindow(w.hwnd, win32.SW_SHOW)
	}
	win32.SetForegroundWindow(w.hwnd)
	win32.SetFocus(w.input)
	win32.SendMessage(w.input, win32.EM_SETSEL, 0, ^uintptr(0))
}

func (w *searchWindow) hide() {
	win32.ShowWindow(w.hwnd, win32.SW_HIDE)
}

// setScale adapts the window to a display scale factor.
func (w *searchWindow) setScale(scale float64) {
	if scale == w.scale && w.font != 0 {
		return
	}
	w.scale = scale
	if w.font != 0 {
		win32.DeleteObject(win32.HGDIOBJ(w.font))
	}
	w.font = win32.CreateFont(int32(math.Round(searchQuerySize*scale)), win32.FW_NORMAL, win32.CLEARTYPE_QUALITY, uiFont)
	win32.SendMessage(w.input, win32.WM_SETFONT, uintptr(w.font), 1)
}

// update runs the query and lays the window out for its results.
func (w *searchWindow) update() {
	query := strings.TrimSpace(win32.WindowText(w.input))
	w.state.Query = query
	w.state.Results = search.Find(w.app.notes, query)
	w.state.Selected = min(0, len(w.state.Results)-1)
	w.relayout(0)
}

// relayout recomputes the layout, keeping the selection in view, and fits
// the window to it. The top edge stays put so the text box does not jump.
func (w *searchWindow) relayout(first int) {
	w.layout = search.NewLayout(w.scale, len(w.state.Results), w.state.Selected, first)
	in := w.layout.Input
	h := math.Round(searchQuerySize * w.scale * 1.45)
	win32.MoveWindow(w.input, int32(in.X), int32(math.Round(w.layout.Field.H/2-h/2)), int32(in.W), int32(h), true)
	r := win32.GetWindowRect(w.hwnd)
	if r.Width() != int32(w.layout.Width) || r.Height() != int32(w.layout.Height) {
		win32.SetWindowPos(w.hwnd, 0, 0, 0, int32(w.layout.Width), int32(w.layout.Height),
			win32.SWP_NOZORDER|win32.SWP_NOMOVE|win32.SWP_NOACTIVATE)
	}
	w.applyTheme()
	win32.InvalidateRect(w.hwnd, nil)
}

// applyTheme matches the window frame and the text box to the theme.
func (w *searchWindow) applyTheme() {
	bg, _ := render.SearchColors(w.app.dark())
	if w.bg != 0 {
		win32.DeleteObject(win32.HGDIOBJ(w.bg))
	}
	w.bg = win32.CreateSolidBrush(colorRef(bg))
	var darkFlag uint32
	if w.app.dark() {
		darkFlag = 1
	}
	win32.DwmSetWindowAttribute(w.hwnd, win32.DWMWA_USE_IMMERSIVE_DARK_MODE, darkFlag)
	win32.InvalidateRect(w.input, nil)
}

// move changes the selection by delta results, stopping at either end.
func (w *searchWindow) move(delta int) {
	n := len(w.state.Results)
	if n == 0 {
		return
	}
	w.state.Selected = max(0, min(w.state.Selected+delta, n-1))
	w.relayout(w.layout.First)
}

// open opens a result, with its first match selected, and closes the window.
func (w *searchWindow) open(i int) {
	if i < 0 || i >= len(w.state.Results) {
		return
	}
	r := w.state.Results[i]
	w.hide()
	w.app.openNote(r.Note.ID)
	if e := w.app.editorFor(r.Note.ID); e != nil && r.At >= 0 {
		e.reveal(r.At, r.AtLen)
	}
}

// handleKey processes the keys that drive the window rather than the text
// box. It reports whether the key was consumed.
func (w *searchWindow) handleKey(vk uintptr) bool {
	switch vk {
	case win32.VK_ESCAPE:
		w.hide()
	case win32.VK_RETURN:
		w.open(w.state.Selected)
	case win32.VK_DOWN:
		w.move(1)
	case win32.VK_UP:
		w.move(-1)
	case win32.VK_NEXT:
		w.move(search.MaxRows)
	case win32.VK_PRIOR:
		w.move(-search.MaxRows)
	case win32.VK_TAB:
		if win32.KeyDown(win32.VK_SHIFT) {
			w.move(-1)
		} else {
			w.move(1)
		}
	case win32.VK_BACK:
		if !win32.KeyDown(win32.VK_CONTROL) {
			return false
		}
		// A classic text box types a box character for Ctrl+Backspace
		// instead of deleting the word before the caret.
		w.deleteWord()
	default:
		return false
	}
	return true
}

// deleteWord deletes the word before the caret, or the selection.
func (w *searchWindow) deleteWord() {
	var start, end uint32
	win32.SendMessage(w.input, win32.EM_GETSEL, uintptr(unsafe.Pointer(&start)), uintptr(unsafe.Pointer(&end)))
	if start == end {
		text := utf16.Encode([]rune(win32.WindowText(w.input)))
		i := min(int(start), len(text))
		for i > 0 && unicode.IsSpace(rune(text[i-1])) {
			i--
		}
		for i > 0 && !unicode.IsSpace(rune(text[i-1])) {
			i--
		}
		win32.SendMessage(w.input, win32.EM_SETSEL, uintptr(i), uintptr(end))
	}
	const canUndo = 1
	win32.SendMessage(w.input, win32.EM_REPLACESEL, canUndo, uintptr(unsafe.Pointer(win32.UTF16(""))))
}

func (w *searchWindow) paint() {
	var ps win32.PAINTSTRUCT
	dc := win32.BeginPaint(w.hwnd, &ps)
	defer win32.EndPaint(w.hwnd, &ps)

	dark := w.app.dark()
	render.DrawSearch(w.canvas, w.layout, w.state, dark, w.app.text)
	if w.drawBorder {
		border := render.Palette(note.Stone, dark).Border
		width, height := float64(w.layout.Width), float64(w.layout.Height)
		for _, r := range []deck.Rect{{W: width, H: 1}, {Y: height - 1, W: width, H: 1}, {W: 1, H: height}, {X: width - 1, W: 1, H: height}} {
			w.canvas.FillRect(r, border)
		}
	}
	width, height := w.layout.Width, w.layout.Height
	if w.dib == nil || w.dib.Width != width || w.dib.Height != height {
		w.dib.Close()
		var err error
		if w.dib, err = win32.NewDIB(width, height); err != nil {
			w.dib = nil
			log.Printf("search: %v", err)
			return
		}
	}
	w.dib.CopyFrom(w.canvas.Img)
	win32.BitBlt(dc, 0, 0, int32(width), int32(height), w.dib.DC, 0, 0)
}

func (w *searchWindow) setHover(i int) {
	if i != w.state.Hover {
		w.state.Hover = i
		win32.InvalidateRect(w.hwnd, nil)
	}
}

func (w *searchWindow) wndProc(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case win32.WM_NCCALCSIZE:
		if wp != 0 {
			return 0 // the whole window is client area; see the editor
		}

	case win32.WM_NCHITTEST:
		pt := win32.PointFromLParam(lp)
		r := win32.GetWindowRect(hwnd)
		if float64(pt.Y-r.Top) < w.layout.Field.H && float64(pt.X-r.Left) < w.layout.Input.X {
			return win32.HTCAPTION // the magnifying glass moves the window
		}
		return win32.HTCLIENT

	case win32.WM_NCACTIVATE:
		return win32.DefWindowProc(hwnd, msg, wp, ^uintptr(0))

	case win32.WM_ERASEBKGND:
		return 1

	case win32.WM_PAINT:
		w.paint()
		return 0

	case win32.WM_CTLCOLOREDIT:
		bg, text := render.SearchColors(w.app.dark())
		dc := win32.HDC(wp)
		win32.SetTextColor(dc, colorRef(text))
		win32.SetBkColor(dc, colorRef(bg))
		return uintptr(w.bg)

	case win32.WM_COMMAND:
		if win32.HiWord(wp) == win32.EN_CHANGE && win32.HWND(lp) == w.input {
			w.update()
		}
		return 0

	case win32.WM_SETFOCUS:
		win32.SetFocus(w.input)
		return 0

	case win32.WM_ACTIVATE:
		if win32.LoWord(wp) == win32.WA_INACTIVE {
			w.hide()
		}

	case win32.WM_MOUSEMOVE:
		pt := win32.PointFromLParam(lp)
		w.setHover(w.layout.Hit(int(pt.X), int(pt.Y)))
		if !w.tracking {
			w.tracking = true
			win32.TrackMouseEvent(&win32.TRACKMOUSEEVENT{Flags: win32.TME_LEAVE, Track: hwnd})
		}
		return 0

	case win32.WM_MOUSELEAVE:
		w.tracking = false
		w.setHover(-1)
		return 0

	case win32.WM_SETCURSOR:
		if win32.LoWord(lp) == win32.HTCLIENT && w.state.Hover >= 0 {
			win32.SetCursor(win32.LoadCursor(win32.IDC_HAND))
			return 1
		}

	case win32.WM_LBUTTONUP:
		pt := win32.PointFromLParam(lp)
		w.open(w.layout.Hit(int(pt.X), int(pt.Y)))
		return 0

	case win32.WM_MOUSEWHEEL:
		lines := -int(int16(win32.HiWord(wp))) / 40
		first := search.Scroll(w.layout.First, lines, len(w.state.Results))
		// Keep the selection on screen without letting it drag the list
		// back.
		w.state.Selected = max(first, min(w.state.Selected, first+search.MaxRows-1))
		w.relayout(first)
		return 0

	case win32.WM_DPICHANGED:
		w.setScale(float64(win32.HiWord(wp)) / win32.USER_DEFAULT_DPI)
		w.relayout(w.layout.First)
		return 0

	case win32.WM_CLOSE:
		w.hide()
		return 0

	case win32.WM_DESTROY:
		w.dib.Close()
		w.dib = nil
		if w.font != 0 {
			win32.DeleteObject(win32.HGDIOBJ(w.font))
		}
		if w.bg != 0 {
			win32.DeleteObject(win32.HGDIOBJ(w.bg))
		}
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wp, lp)
}

// showSearch opens the search window, creating it on first use.
func (a *App) showSearch() {
	if a.search == nil {
		w, err := newSearchWindow(a)
		if err != nil {
			log.Printf("search: %v", err)
			return
		}
		a.search = w
	}
	a.search.show()
}
