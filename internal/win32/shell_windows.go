package win32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	uxtheme  = windows.NewLazySystemDLL("uxtheme.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")

	procShellNotifyIconW      = shell32.NewProc("Shell_NotifyIconW")
	procShellExecuteW         = shell32.NewProc("ShellExecuteW")
	procSHQueryUserNotifState = shell32.NewProc("SHQueryUserNotificationState")
	procGetDpiForMonitor      = shcore.NewProc("GetDpiForMonitor")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procDwmExtendFrame        = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
	procSetWindowTheme        = uxtheme.NewProc("SetWindowTheme")
	procGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW")
)

// ModuleHandle returns the instance handle of the running executable.
func ModuleHandle() HINSTANCE {
	r, _, _ := procGetModuleHandleW.Call(0)
	return HINSTANCE(r)
}

// ShellNotifyIcon adds, modifies or deletes a notification area icon.
func ShellNotifyIcon(action uint32, data *NOTIFYICONDATA) bool {
	data.Size = uint32(unsafe.Sizeof(*data))
	r, _, _ := procShellNotifyIconW.Call(uintptr(action), uintptr(unsafe.Pointer(data)))
	return r != 0
}

// ShellOpen opens a file, folder or URL with its default handler.
func ShellOpen(target string) {
	procShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(UTF16("open"))),
		uintptr(unsafe.Pointer(UTF16(target))),
		0, 0, SW_SHOW)
}

// DwmSetWindowAttribute sets a 32-bit Desktop Window Manager attribute. It
// reports whether the attribute was accepted; attributes introduced in newer
// versions of Windows are simply rejected by older ones.
func DwmSetWindowAttribute(hwnd HWND, attr uint32, value uint32) bool {
	if procDwmSetWindowAttribute.Find() != nil {
		return false
	}
	hr, _, _ := procDwmSetWindowAttribute.Call(uintptr(hwnd), uintptr(attr),
		uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
	return int32(hr) >= 0
}

// DwmExtendFrameIntoClientArea extends the window frame, and with it the
// drop shadow Desktop Window Manager draws, into the client area.
func DwmExtendFrameIntoClientArea(hwnd HWND, m MARGINS) {
	if procDwmExtendFrame.Find() != nil {
		return
	}
	procDwmExtendFrame.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&m)))
}

// IsWindows11 reports whether the system is Windows 11 or later, where
// Desktop Window Manager rounds window corners and draws coloured borders.
func IsWindows11() bool {
	return windows.RtlGetVersion().BuildNumber >= 22000
}

// SetWindowTheme applies a visual style, such as "DarkMode_Explorer", to a
// window's standard parts (for example its scroll bars).
func SetWindowTheme(hwnd HWND, app string) {
	if procSetWindowTheme.Find() != nil {
		return
	}
	procSetWindowTheme.Call(uintptr(hwnd), uintptr(unsafe.Pointer(UTF16(app))), 0)
}

// GetDpiForMonitor returns the effective DPI of a monitor, or the default of
// 96 on versions of Windows that predate per-monitor DPI.
func GetDpiForMonitor(mon HMONITOR) uint32 {
	if procGetDpiForMonitor.Find() != nil {
		return USER_DEFAULT_DPI
	}
	const effectiveDPI = 0 // MDT_EFFECTIVE_DPI
	var x, y uint32
	hr, _, _ := procGetDpiForMonitor.Call(uintptr(mon), effectiveDPI,
		uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
	if int32(hr) < 0 || x == 0 {
		return USER_DEFAULT_DPI
	}
	return x
}

// UserIsBusy reports whether the user is in a full-screen application, a
// full-screen game or presentation mode — the states in which Windows itself
// holds back notifications.
func UserIsBusy() bool {
	if procSHQueryUserNotifState.Find() != nil {
		return false
	}
	// QUERY_USER_NOTIFICATION_STATE values.
	const (
		busy              = 2
		runningFullScreen = 3
		presentationMode  = 4
	)
	var state uint32
	hr, _, _ := procSHQueryUserNotifState.Call(uintptr(unsafe.Pointer(&state)))
	if int32(hr) < 0 {
		return false
	}
	return state == busy || state == runningFullScreen || state == presentationMode
}
