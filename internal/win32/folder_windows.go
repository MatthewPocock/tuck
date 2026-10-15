package win32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32 = windows.NewLazySystemDLL("ole32.dll")

	procCoCreateInstance            = ole32.NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")

	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidFileOpenDialog   = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidShellItem        = windows.GUID{Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE, Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

// Method indices of IFileOpenDialog and IShellItem.
const (
	vtShow           = 3
	vtSetOptions     = 9
	vtSetFolder      = 12
	vtSetTitle       = 17
	vtGetResult      = 20
	vtGetDisplayName = 5
)

// PickFolder shows the system folder picker, starting in start, and returns
// the chosen folder. ok is false if the user cancelled or the picker is
// unavailable.
func PickFolder(owner HWND, title, start string) (folder string, ok bool) {
	// The picker is a COM object and needs COM on this thread; this is a
	// no-op if it is already initialised.
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}

	var dlg unsafe.Pointer
	const inprocServer = 1
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, inprocServer,
		uintptr(unsafe.Pointer(&iidFileOpenDialog)), uintptr(unsafe.Pointer(&dlg)))
	if int32(hr) < 0 || dlg == nil {
		return "", false
	}
	defer comCall(dlg, vtRelease)

	const (
		pickFolders     = 0x00000020
		forceFileSystem = 0x00000040
		pathMustExist   = 0x00000800
	)
	comCall(dlg, vtSetOptions, pickFolders|forceFileSystem|pathMustExist)
	comCall(dlg, vtSetTitle, uintptr(unsafe.Pointer(UTF16(title))))
	if start != "" {
		var item unsafe.Pointer
		hr, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(UTF16(start))), 0,
			uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
		if int32(hr) >= 0 && item != nil {
			comCall(dlg, vtSetFolder, uintptr(item))
			comCall(item, vtRelease)
		}
	}

	if hr := comCall(dlg, vtShow, uintptr(owner)); int32(hr) < 0 {
		return "", false
	}
	var result unsafe.Pointer
	if hr := comCall(dlg, vtGetResult, uintptr(unsafe.Pointer(&result))); int32(hr) < 0 || result == nil {
		return "", false
	}
	defer comCall(result, vtRelease)

	const fileSystemPath = 0x80058000 // SIGDN_FILESYSPATH
	var path *uint16
	if hr := comCall(result, vtGetDisplayName, fileSystemPath, uintptr(unsafe.Pointer(&path))); int32(hr) < 0 || path == nil {
		return "", false
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(path))
	return windows.UTF16PtrToString(path), true
}
