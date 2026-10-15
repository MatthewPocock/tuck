package app

import (
	"log"

	"golang.org/x/sys/windows"
)

// folderWatch is signalled when files in the notes folder are created,
// renamed, deleted or written. The message loop waits on it alongside window
// messages, so noticing changes costs nothing while the folder is quiet.
type folderWatch struct {
	handle windows.Handle
}

// watchFolder starts watching dir. If that is impossible, for example on
// some network drives, it returns a watch that never fires and the app falls
// back to polling.
func watchFolder(dir string) *folderWatch {
	const filter = windows.FILE_NOTIFY_CHANGE_FILE_NAME |
		windows.FILE_NOTIFY_CHANGE_SIZE |
		windows.FILE_NOTIFY_CHANGE_LAST_WRITE
	h, err := windows.FindFirstChangeNotification(dir, false, filter)
	if err != nil {
		log.Printf("watch notes folder: %v; polling instead", err)
		return &folderWatch{}
	}
	return &folderWatch{handle: h}
}

// active reports whether the watch delivers notifications.
func (w *folderWatch) active() bool { return w != nil && w.handle != 0 }

// rearm asks for the next notification after one was delivered.
func (w *folderWatch) rearm() {
	if err := windows.FindNextChangeNotification(w.handle); err != nil {
		log.Printf("watch notes folder: %v; polling instead", err)
		w.close()
	}
}

func (w *folderWatch) close() {
	if w.active() {
		_ = windows.FindCloseChangeNotification(w.handle)
		w.handle = 0
	}
}
