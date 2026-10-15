package win32

import (
	"unicode/utf16"
	"unsafe"
)

var (
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")

	procGetTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
)

// SetClipboardText puts text on the clipboard, replacing what was there. It
// reports whether that worked; the clipboard may be held by another app.
func SetClipboardText(owner HWND, text string) bool {
	units := append(utf16.Encode([]rune(text)), 0)
	if r, _, _ := procOpenClipboard.Call(uintptr(owner)); r == 0 {
		return false
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()

	const moveable = 0x0002
	size := uintptr(len(units)) * 2
	mem, _, _ := procGlobalAlloc.Call(moveable, size)
	if mem == 0 {
		return false
	}
	p, _, _ := procGlobalLock.Call(mem)
	if p == 0 {
		procGlobalFree.Call(mem)
		return false
	}
	procRtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&units[0])), size)
	procGlobalUnlock.Call(mem)
	// On success the clipboard owns the memory.
	if r, _, _ := procSetClipboardData.Call(CF_UNICODETEXT, mem); r == 0 {
		procGlobalFree.Call(mem)
		return false
	}
	return true
}

// TextWidth returns the width in pixels of text drawn on dc with its current
// font.
func TextWidth(dc HDC, text string) int32 {
	units := utf16.Encode([]rune(text))
	if len(units) == 0 {
		return 0
	}
	var size SIZE
	procGetTextExtentPoint32W.Call(uintptr(dc), uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)), uintptr(unsafe.Pointer(&size)))
	return size.CX
}
