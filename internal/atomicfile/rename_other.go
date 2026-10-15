//go:build !windows

package atomicfile

import "os"

// Rename is os.Rename. On Windows it also waits out brief locks held by
// other processes.
func Rename(from, to string) error { return os.Rename(from, to) }
