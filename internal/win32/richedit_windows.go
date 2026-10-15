package win32

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TextDocument is the ITextDocument COM interface of a rich edit control. Only
// what the editor needs is exposed.
type TextDocument struct {
	ptr unsafe.Pointer
}

var iidTextDocument = windows.GUID{
	Data1: 0x8CC497C0, Data2: 0xA1DF, Data3: 0x11CE,
	Data4: [8]byte{0x80, 0x98, 0x00, 0xAA, 0x00, 0x47, 0xBE, 0x5D},
}

// Method indices in the interfaces' virtual tables.
const (
	vtQueryInterface = 0
	vtRelease        = 2
	vtUndo           = 22 // ITextDocument::Undo
)

// Values ITextDocument::Undo accepts in place of a count.
const (
	tomSuspend = -9999995
	tomResume  = -9999994
)

// comCall invokes method index of the COM object obj.
func comCall(obj unsafe.Pointer, index int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	method := *(*uintptr)(unsafe.Add(vtbl, index*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

// RichEditDocument returns the text object model of a rich edit control, or
// nil if it does not provide one.
func RichEditDocument(edit HWND) *TextDocument {
	var ole unsafe.Pointer
	if SendMessage(edit, EM_GETOLEINTERFACE, 0, uintptr(unsafe.Pointer(&ole))) == 0 || ole == nil {
		return nil
	}
	defer comCall(ole, vtRelease)
	var doc unsafe.Pointer
	if hr := comCall(ole, vtQueryInterface, uintptr(unsafe.Pointer(&iidTextDocument)), uintptr(unsafe.Pointer(&doc))); int32(hr) < 0 || doc == nil {
		return nil
	}
	return &TextDocument{ptr: doc}
}

// SuspendUndo stops recording changes for undo until ResumeUndo, so that
// changes made by the program, such as restyling, cannot be undone by the
// user.
func (d *TextDocument) SuspendUndo() { d.undo(tomSuspend) }

// ResumeUndo records changes for undo again.
func (d *TextDocument) ResumeUndo() { d.undo(tomResume) }

func (d *TextDocument) undo(count int32) {
	if d != nil {
		comCall(d.ptr, vtUndo, uintptr(uint32(count)), 0)
	}
}

// Release frees the interface.
func (d *TextDocument) Release() {
	if d != nil && d.ptr != nil {
		comCall(d.ptr, vtRelease)
		d.ptr = nil
	}
}
