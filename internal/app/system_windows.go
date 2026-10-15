package app

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/MatthewPocock/tuck/internal/win32"
)

const (
	runKey         = `Software\Microsoft\Windows\CurrentVersion\Run`
	personalizeKey = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	mutexName      = `Local\Tuck.SingleInstance`
	// maxLogSize is the size at which the log file is started afresh.
	maxLogSize = 1 << 20
)

// paths are the files and folders the app uses.
type paths struct {
	dir    string // the app's data directory
	config string
	notes  string // default notes folder; the config may override it
	log    string
	// session remembers the pinned notes between runs.
	session string
}

// resolvePaths locates the app's data directory under %AppData%, creating it
// if necessary.
func resolvePaths() (paths, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return paths{}, fmt.Errorf("locate the AppData folder: %w", err)
	}
	dir := filepath.Join(base, appName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return paths{}, fmt.Errorf("create the data folder: %w", err)
	}
	return paths{
		dir:     dir,
		config:  filepath.Join(dir, "config.json"),
		notes:   filepath.Join(dir, "notes"),
		log:     filepath.Join(dir, "tuck.log"),
		session: filepath.Join(dir, "session.json"),
	}, nil
}

// setupLog sends the standard logger to a file, since a GUI process has no
// console. The returned function closes the file.
func setupLog(path string) func() {
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return func() {}
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	return func() { _ = f.Close() }
}

// acquireSingleInstance claims the per-session mutex that marks a running
// instance. already is true when another instance holds it.
func acquireSingleInstance() (release func(), already bool, err error) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, false, err
	}
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		return func() {}, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("create instance mutex: %w", err)
	}
	return func() { _ = windows.CloseHandle(h) }, false, nil
}

// systemUsesDarkMode reports whether Windows is set to dark app mode.
func systemUsesDarkMode() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	light, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && light == 0
}

// autostartEnabled reports whether the app is registered to start when the
// user signs in.
func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(appName)
	return err == nil
}

// setAutostart adds or removes the app from the user's sign-in programs.
func setAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the Run registry key: %w", err)
	}
	defer k.Close()

	if !enable {
		if err := k.DeleteValue(appName); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove the autostart entry: %w", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the executable: %w", err)
	}
	if err := k.SetStringValue(appName, `"`+exe+`"`); err != nil {
		return fmt.Errorf("write the autostart entry: %w", err)
	}
	return nil
}

// display describes the monitor the deck lives on.
type display struct {
	// work is the usable area of the monitor, excluding the taskbar.
	work win32.RECT
	// scale is the monitor's scale factor (1 at 96 DPI).
	scale float64
}

// primaryDisplay returns the primary monitor's work area and scale.
func primaryDisplay() display {
	mon := win32.MonitorFromPoint(win32.POINT{}, win32.MONITOR_DEFAULTTOPRIMARY)
	mi, ok := win32.GetMonitorInfo(mon)
	if !ok {
		// A sensible fallback; this should never happen on a working
		// desktop.
		return display{work: win32.RECT{Right: 1280, Bottom: 720}, scale: 1}
	}
	return display{work: mi.Work, scale: float64(win32.GetDpiForMonitor(mon)) / win32.USER_DEFAULT_DPI}
}
