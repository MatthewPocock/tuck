package atomicfile

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// Windows refuses to replace a file that another process has open without
// FILE_SHARE_DELETE, and sync clients, indexers and virus scanners routinely
// hold a note open for a few milliseconds right after it was written. Those
// failures are transient, so retry for a short while before giving up.
const (
	renameAttempts = 8
	renameBackoff  = 15 * time.Millisecond
)

const (
	errorAccessDenied     syscall.Errno = 5
	errorSharingViolation syscall.Errno = 32
	errorLockViolation    syscall.Errno = 33
)

// Rename is os.Rename, retried while another process briefly holds the
// target open.
func Rename(from, to string) error {
	var err error
	for i := range renameAttempts {
		if err = os.Rename(from, to); err == nil || !transient(err) {
			return err
		}
		time.Sleep(renameBackoff * time.Duration(i+1))
	}
	return err
}

func transient(err error) bool {
	return errors.Is(err, errorAccessDenied) ||
		errors.Is(err, errorSharingViolation) ||
		errors.Is(err, errorLockViolation)
}
