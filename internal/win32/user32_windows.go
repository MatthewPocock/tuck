package win32

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")

	procAppendMenuW                   = user32.NewProc("AppendMenuW")
	procBeginPaint                    = user32.NewProc("BeginPaint")
	procCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	procCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")
	procDestroyMenu                   = user32.NewProc("DestroyMenu")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procDrawTextW                     = user32.NewProc("DrawTextW")
	procEndPaint                      = user32.NewProc("EndPaint")
	procFillRect                      = user32.NewProc("FillRect")
	procFindWindowW                   = user32.NewProc("FindWindowW")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procGetDC                         = user32.NewProc("GetDC")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procGetKeyState                   = user32.NewProc("GetKeyState")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")
	procInvalidateRect                = user32.NewProc("InvalidateRect")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procKillTimer                     = user32.NewProc("KillTimer")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	procMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	procMonitorFromRect               = user32.NewProc("MonitorFromRect")
	procGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	procMoveWindow                    = user32.NewProc("MoveWindow")
	procCallWindowProcW               = user32.NewProc("CallWindowProcW")
	procSetWindowLongPtrW             = user32.NewProc("SetWindowLongPtrW")
	procSetCursor                     = user32.NewProc("SetCursor")
	procScreenToClient                = user32.NewProc("ScreenToClient")
	procMsgWaitForMultipleObjectsEx   = user32.NewProc("MsgWaitForMultipleObjectsEx")
	procPeekMessageW                  = user32.NewProc("PeekMessageW")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procRegisterHotKey                = user32.NewProc("RegisterHotKey")
	procRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	procReleaseDC                     = user32.NewProc("ReleaseDC")
	procSendMessageW                  = user32.NewProc("SendMessageW")
	procSetFocus                      = user32.NewProc("SetFocus")
	procSetMenuItemInfoW              = user32.NewProc("SetMenuItemInfoW")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetTimer                      = user32.NewProc("SetTimer")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procSetWindowRgn                  = user32.NewProc("SetWindowRgn")
	procGetFocus                      = user32.NewProc("GetFocus")
	procGetWindowLongPtrW             = user32.NewProc("GetWindowLongPtrW")
	procSetLayeredWindowAttributes    = user32.NewProc("SetLayeredWindowAttributes")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procTrackMouseEvent               = user32.NewProc("TrackMouseEvent")
	procTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procUnregisterHotKey              = user32.NewProc("UnregisterHotKey")
	procUpdateLayeredWindow           = user32.NewProc("UpdateLayeredWindow")
	procUpdateWindow                  = user32.NewProc("UpdateWindow")
)

// callErr turns the error value returned by LazyProc.Call into a useful Go
// error. Call always returns a non-nil error, which is ERROR_SUCCESS when the
// API failed without setting the thread's last error.
func callErr(api string, err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == 0 {
		return fmt.Errorf("%s failed", api)
	}
	return fmt.Errorf("%s: %w", api, err)
}

// UTF16 converts a Go string to a NUL-terminated UTF-16 string. Interior NUL
// characters, which Win32 cannot represent, end the string early instead of
// causing an error.
func UTF16(s string) *uint16 {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			s = s[:i]
			break
		}
	}
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

// NewWndProc wraps a Go function as a window procedure usable in a
// WNDCLASSEX. The callback is never freed, so call this once per class.
func NewWndProc(fn func(hwnd HWND, msg uint32, wp, lp uintptr) uintptr) uintptr {
	return windows.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
		return fn(HWND(hwnd), uint32(msg), wp, lp)
	})
}

func RegisterClassEx(wc *WNDCLASSEX) error {
	wc.Size = uint32(unsafe.Sizeof(*wc))
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(wc)))
	if atom == 0 {
		return callErr("RegisterClassEx", err)
	}
	return nil
}

func CreateWindowEx(exStyle uint32, class, title string, style uint32, x, y, w, h int32,
	parent HWND, menu HMENU, inst HINSTANCE) (HWND, error) {
	hwnd, _, err := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(UTF16(class))),
		uintptr(unsafe.Pointer(UTF16(title))),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent), uintptr(menu), uintptr(inst), 0,
	)
	if hwnd == 0 {
		return 0, callErr("CreateWindowEx("+class+")", err)
	}
	return HWND(hwnd), nil
}

func DefWindowProc(hwnd HWND, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return r
}

func DestroyWindow(hwnd HWND) {
	procDestroyWindow.Call(uintptr(hwnd))
}

func ShowWindow(hwnd HWND, cmd int32) {
	procShowWindow.Call(uintptr(hwnd), uintptr(cmd))
}

func IsWindowVisible(hwnd HWND) bool {
	r, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	return r != 0
}

// GetMessage retrieves the next message for the calling thread. It returns
// false when WM_QUIT is received or the call fails.
func GetMessage(msg *MSG) bool {
	r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(msg)), 0, 0, 0)
	return int32(r) > 0
}

// Subclass replaces the window procedure of a window, typically a control,
// with proc, which is a callback from NewWndProc. It returns the previous
// procedure, which proc must pass unhandled messages to with CallWindowProc.
func Subclass(hwnd HWND, proc uintptr) uintptr {
	const gwlpWndProc = ^uintptr(3) // GWLP_WNDPROC, -4
	r, _, _ := procSetWindowLongPtrW.Call(uintptr(hwnd), gwlpWndProc, proc)
	return r
}

// CallWindowProc passes a message to a window procedure returned by
// Subclass.
func CallWindowProc(proc uintptr, hwnd HWND, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := procCallWindowProcW.Call(proc, uintptr(hwnd), uintptr(msg), wp, lp)
	return r
}

func SetCursor(c HCURSOR) {
	procSetCursor.Call(uintptr(c))
}

// ScreenToClient converts a point from screen to client coordinates.
func ScreenToClient(hwnd HWND, pt POINT) POINT {
	procScreenToClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))
	return pt
}

// PeekMessage removes the next message for the calling thread from its
// queue, if there is one.
func PeekMessage(msg *MSG) bool {
	const remove = 0x0001 // PM_REMOVE
	r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(msg)), 0, 0, 0, remove)
	return r != 0
}

// WaitForMessageOr blocks until a message arrives for the calling thread or
// the handle, if non-zero, is signalled. It reports whether the handle was.
func WaitForMessageOr(h windows.Handle) bool {
	const (
		infinite         = 0xFFFFFFFF
		qsAllInput       = 0x04FF
		inputAvailable   = 0x0004 // MWMO_INPUTAVAILABLE
		waitObject0      = 0
		waitedForObjects = 1
	)
	var count, handles uintptr
	if h != 0 {
		count, handles = waitedForObjects, uintptr(unsafe.Pointer(&h))
	}
	r, _, _ := procMsgWaitForMultipleObjectsEx.Call(count, handles, infinite, qsAllInput, inputAvailable)
	return h != 0 && r == waitObject0
}

func TranslateMessage(msg *MSG) {
	procTranslateMessage.Call(uintptr(unsafe.Pointer(msg)))
}

func DispatchMessage(msg *MSG) {
	procDispatchMessageW.Call(uintptr(unsafe.Pointer(msg)))
}

func PostQuitMessage(code int32) {
	procPostQuitMessage.Call(uintptr(code))
}

func PostMessage(hwnd HWND, msg uint32, wp, lp uintptr) {
	procPostMessageW.Call(uintptr(hwnd), uintptr(msg), wp, lp)
}

func SendMessage(hwnd HWND, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return r
}

func SetWindowPos(hwnd, insertAfter HWND, x, y, w, h int32, flags uint32) {
	procSetWindowPos.Call(uintptr(hwnd), uintptr(insertAfter),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(flags))
}

func MoveWindow(hwnd HWND, x, y, w, h int32, repaint bool) {
	var flag uintptr
	if repaint {
		flag = 1
	}
	procMoveWindow.Call(uintptr(hwnd), uintptr(x), uintptr(y), uintptr(w), uintptr(h), flag)
}

func GetWindowRect(hwnd HWND) RECT {
	var r RECT
	procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	return r
}

func GetClientRect(hwnd HWND) RECT {
	var r RECT
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	return r
}

// GetCursorPos returns the pointer position in screen coordinates. ok is
// false when the position is unavailable, e.g. on the secure desktop.
func GetCursorPos() (pt POINT, ok bool) {
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt, r != 0
}

func SetTimer(hwnd HWND, id uintptr, millis uint32) {
	procSetTimer.Call(uintptr(hwnd), id, uintptr(millis), 0)
}

func KillTimer(hwnd HWND, id uintptr) {
	procKillTimer.Call(uintptr(hwnd), id)
}

// UpdateLayeredWindow replaces the contents of a layered window with the
// premultiplied 32-bit bitmap selected into src.
func UpdateLayeredWindow(hwnd HWND, size SIZE, src HDC) error {
	return UpdateLayeredWindowAlpha(hwnd, size, src, 255)
}

// UpdateLayeredWindowAlpha is UpdateLayeredWindow with the whole window
// faded to opacity alpha.
func UpdateLayeredWindowAlpha(hwnd HWND, size SIZE, src HDC, alpha uint8) error {
	var origin POINT
	blend := BLENDFUNCTION{BlendOp: AC_SRC_OVER, SourceConstantAlpha: alpha, AlphaFormat: AC_SRC_ALPHA}
	r, _, err := procUpdateLayeredWindow.Call(
		uintptr(hwnd), 0, 0,
		uintptr(unsafe.Pointer(&size)),
		uintptr(src),
		uintptr(unsafe.Pointer(&origin)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		ULW_ALPHA,
	)
	if r == 0 {
		return callErr("UpdateLayeredWindow", err)
	}
	return nil
}

// SetWindowRgn clips a window to a region, or removes the clipping when rgn
// is zero. The system takes ownership of the region.
func SetWindowRgn(hwnd HWND, rgn HRGN, redraw bool) {
	var r uintptr
	if redraw {
		r = 1
	}
	procSetWindowRgn.Call(uintptr(hwnd), uintptr(rgn), r)
}

// ExStyle returns a window's extended style.
func ExStyle(hwnd HWND) uint32 {
	const gwlExStyle = ^uintptr(19) // GWL_EXSTYLE, -20
	r, _, _ := procGetWindowLongPtrW.Call(uintptr(hwnd), gwlExStyle)
	return uint32(r)
}

// SetExStyle changes a window's extended style.
func SetExStyle(hwnd HWND, style uint32) {
	const gwlExStyle = ^uintptr(19) // GWL_EXSTYLE, -20
	procSetWindowLongPtrW.Call(uintptr(hwnd), gwlExStyle, uintptr(style))
}

// SetLayeredAlpha fades a layered window that paints itself normally to
// opacity alpha.
func SetLayeredAlpha(hwnd HWND, alpha uint8) {
	const lwaAlpha = 2
	procSetLayeredWindowAttributes.Call(uintptr(hwnd), 0, uintptr(alpha), lwaAlpha)
}

// GetFocus returns the window of this thread that has the keyboard focus.
func GetFocus() HWND {
	r, _, _ := procGetFocus.Call()
	return HWND(r)
}

func GetDC(hwnd HWND) HDC {
	r, _, _ := procGetDC.Call(uintptr(hwnd))
	return HDC(r)
}

func ReleaseDC(hwnd HWND, dc HDC) {
	procReleaseDC.Call(uintptr(hwnd), uintptr(dc))
}

func BeginPaint(hwnd HWND, ps *PAINTSTRUCT) HDC {
	r, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(ps)))
	return HDC(r)
}

func EndPaint(hwnd HWND, ps *PAINTSTRUCT) {
	procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(ps)))
}

// InvalidateRect marks part of a window, or all of it if r is nil, as
// needing a repaint.
func InvalidateRect(hwnd HWND, r *RECT) {
	procInvalidateRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(r)), 0)
}

// UpdateWindow paints a window's invalid region immediately instead of
// waiting for the message loop to get to it.
func UpdateWindow(hwnd HWND) {
	procUpdateWindow.Call(uintptr(hwnd))
}

func FillRect(dc HDC, r *RECT, brush HBRUSH) {
	procFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(r)), uintptr(brush))
}

// DrawText draws a NUL-terminated string into r using the font selected into
// dc.
func DrawText(dc HDC, text string, r *RECT, format uint32) {
	// Pass -1 as the length so the string is read up to its terminator.
	procDrawTextW.Call(uintptr(dc), uintptr(unsafe.Pointer(UTF16(text))), ^uintptr(0),
		uintptr(unsafe.Pointer(r)), uintptr(format))
}

// LoadCursor loads one of the predefined system cursors (IDC_*).
func LoadCursor(id uintptr) HCURSOR {
	r, _, _ := procLoadCursorW.Call(0, id)
	return HCURSOR(r)
}

func TrackMouseEvent(tme *TRACKMOUSEEVENT) {
	tme.Size = uint32(unsafe.Sizeof(*tme))
	procTrackMouseEvent.Call(uintptr(unsafe.Pointer(tme)))
}

func MonitorFromPoint(pt POINT, flags uint32) HMONITOR {
	// POINT is passed by value, packed into a single 64-bit argument.
	packed := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	r, _, _ := procMonitorFromPoint.Call(packed, uintptr(flags))
	return HMONITOR(r)
}

// MonitorFromRect returns the monitor a rectangle mostly lies on.
func MonitorFromRect(r RECT, flags uint32) HMONITOR {
	h, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), uintptr(flags))
	return HMONITOR(h)
}

func GetForegroundWindow() HWND {
	h, _, _ := procGetForegroundWindow.Call()
	return HWND(h)
}

func MonitorFromWindow(hwnd HWND, flags uint32) HMONITOR {
	r, _, _ := procMonitorFromWindow.Call(uintptr(hwnd), uintptr(flags))
	return HMONITOR(r)
}

func GetMonitorInfo(mon HMONITOR) (MONITORINFO, bool) {
	var mi MONITORINFO
	mi.Size = uint32(unsafe.Sizeof(mi))
	r, _, _ := procGetMonitorInfoW.Call(uintptr(mon), uintptr(unsafe.Pointer(&mi)))
	return mi, r != 0
}

// GetDpiForWindow returns the DPI of the monitor a window is on, or the
// default of 96 on versions of Windows that predate per-monitor DPI.
func GetDpiForWindow(hwnd HWND) uint32 {
	if procGetDpiForWindow.Find() != nil {
		return USER_DEFAULT_DPI
	}
	r, _, _ := procGetDpiForWindow.Call(uintptr(hwnd))
	if r == 0 {
		return USER_DEFAULT_DPI
	}
	return uint32(r)
}

// SetProcessDpiAwarenessContext opts the process into a DPI awareness mode.
// It is a no-op where unsupported, or when the mode was already fixed by the
// application manifest.
func SetProcessDpiAwarenessContext(context uintptr) {
	if procSetProcessDpiAwarenessContext.Find() != nil {
		return
	}
	procSetProcessDpiAwarenessContext.Call(context)
}

func GetSystemMetrics(index int32) int32 {
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int32(r)
}

func CreatePopupMenu() HMENU {
	r, _, _ := procCreatePopupMenu.Call()
	return HMENU(r)
}

// AppendMenu adds an item to a menu. For MF_POPUP items, id is the submenu
// handle.
func AppendMenu(menu HMENU, flags uint32, id uintptr, text string) {
	var p *uint16
	if flags&MF_SEPARATOR == 0 {
		p = UTF16(text)
	}
	procAppendMenuW.Call(uintptr(menu), uintptr(flags), id, uintptr(unsafe.Pointer(p)))
}

// SetMenuItemBitmap shows a bitmap beside a menu item, identified by its
// command ID. A 32-bit bitmap with premultiplied alpha is drawn blended. The
// menu does not take ownership of the bitmap.
func SetMenuItemBitmap(menu HMENU, id uintptr, bmp HBITMAP) {
	info := MENUITEMINFO{Mask: MIIM_BITMAP, BitmapItem: bmp}
	info.Size = uint32(unsafe.Sizeof(info))
	procSetMenuItemInfoW.Call(uintptr(menu), id, 0, uintptr(unsafe.Pointer(&info)))
}

// TrackPopupMenu shows a popup menu at a screen position and returns the ID
// of the chosen item, or 0 if the menu was dismissed. flags must include
// TPM_RETURNCMD.
func TrackPopupMenu(menu HMENU, flags uint32, x, y int32, owner HWND) uint32 {
	r, _, _ := procTrackPopupMenu.Call(uintptr(menu), uintptr(flags), uintptr(x), uintptr(y), 0, uintptr(owner), 0)
	return uint32(r)
}

func DestroyMenu(menu HMENU) {
	procDestroyMenu.Call(uintptr(menu))
}

func SetForegroundWindow(hwnd HWND) bool {
	r, _, _ := procSetForegroundWindow.Call(uintptr(hwnd))
	return r != 0
}

func SetFocus(hwnd HWND) {
	procSetFocus.Call(uintptr(hwnd))
}

// KeyDown reports whether a virtual key is currently held down, as of the
// message being processed.
func KeyDown(vk int32) bool {
	r, _, _ := procGetKeyState.Call(uintptr(vk))
	return int16(r) < 0
}

func RegisterHotKey(hwnd HWND, id int32, mods, vk uint32) error {
	r, _, err := procRegisterHotKey.Call(uintptr(hwnd), uintptr(id), uintptr(mods), uintptr(vk))
	if r == 0 {
		return callErr("RegisterHotKey", err)
	}
	return nil
}

func UnregisterHotKey(hwnd HWND, id int32) {
	procUnregisterHotKey.Call(uintptr(hwnd), uintptr(id))
}

func CreateIconIndirect(info *ICONINFO) HICON {
	r, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(info)))
	return HICON(r)
}

func DestroyIcon(icon HICON) {
	procDestroyIcon.Call(uintptr(icon))
}

// RegisterWindowMessage returns a system-wide unique message ID for name.
func RegisterWindowMessage(name string) uint32 {
	r, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(UTF16(name))))
	return uint32(r)
}

// FindWindow returns a top-level window of the given class, or 0.
func FindWindow(class string) HWND {
	r, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(UTF16(class))), 0)
	return HWND(r)
}

func MessageBox(owner HWND, text, caption string, flags uint32) {
	procMessageBoxW.Call(uintptr(owner),
		uintptr(unsafe.Pointer(UTF16(text))), uintptr(unsafe.Pointer(UTF16(caption))), uintptr(flags))
}

// WindowText returns the full text of a window or edit control.
func WindowText(hwnd HWND) string {
	n := SendMessage(hwnd, WM_GETTEXTLENGTH, 0, 0)
	if n == 0 {
		return ""
	}
	// Rich edit controls may report the length with bare CR line endings
	// but return CR LF, so leave room for every character to double.
	buf := make([]uint16, 2*n+1)
	got := SendMessage(hwnd, WM_GETTEXT, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return windows.UTF16ToString(buf[:min(got, uintptr(len(buf)))])
}

// SetWindowText replaces the text of a window or edit control.
func SetWindowText(hwnd HWND, text string) {
	SendMessage(hwnd, WM_SETTEXT, 0, uintptr(unsafe.Pointer(UTF16(text))))
}

// UTF16ToString converts a NUL-terminated UTF-16 buffer to a Go string.
func UTF16ToString(buf []uint16) string {
	return windows.UTF16ToString(buf)
}

// PointerFromLParam interprets a message parameter as a pointer to a
// structure owned by Windows, as many messages (WM_GETMINMAXINFO,
// WM_DPICHANGED, ...) require.
func PointerFromLParam[T any](lp uintptr) *T {
	// Reading the parameter's bits as a pointer, rather than converting the
	// integer, is the form the unsafe rules allow. The memory is not managed
	// by the Go runtime, so the garbage collector is not involved.
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&lp)))
}
