package win32

import (
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

// Popup menus only come in a dark variant through two uxtheme functions that
// Windows exports by ordinal alone. They are what Explorer itself uses, and
// have kept these ordinals since Windows 10 version 1903; earlier builds
// export something else under them, so they are only used from that build on.
const (
	ordSetPreferredAppMode = 135
	ordFlushMenuThemes     = 136
	minDarkMenuBuild       = 18362
)

// Values of the PreferredAppMode enumeration.
const (
	appModeForceDark  = 2
	appModeForceLight = 3
)

var menuTheme = sync.OnceValues(func() (setMode, flush uintptr) {
	if windows.RtlGetVersion().BuildNumber < minDarkMenuBuild {
		return 0, 0
	}
	h, err := windows.LoadLibraryEx("uxtheme.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return 0, 0
	}
	setMode, err1 := windows.GetProcAddressByOrdinal(h, ordSetPreferredAppMode)
	flush, err2 := windows.GetProcAddressByOrdinal(h, ordFlushMenuThemes)
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return setMode, flush
})

// SetDarkMenus switches the process's popup menus between the light and dark
// system appearance. It does nothing on versions of Windows without dark
// menus.
func SetDarkMenus(dark bool) {
	setMode, flush := menuTheme()
	if setMode == 0 {
		return
	}
	mode := uintptr(appModeForceLight)
	if dark {
		mode = appModeForceDark
	}
	syscall.SyscallN(setMode, mode)
	syscall.SyscallN(flush)
}
