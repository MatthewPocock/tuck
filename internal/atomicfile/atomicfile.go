// Package atomicfile writes files so that readers never observe a partially
// written file, even if the process dies mid-write.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write replaces the file at path with data. The data is first written to a
// temporary file in the same directory and then renamed over the target, which
// is atomic on every platform Go supports (including Windows, where os.Rename
// replaces an existing file).
func Write(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("atomicfile: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("atomicfile: write: %w", err)
	}
	if err = tmp.Chmod(perm); err != nil {
		return fmt.Errorf("atomicfile: chmod: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("atomicfile: sync: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("atomicfile: close: %w", err)
	}
	if err = Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomicfile: rename: %w", err)
	}
	return nil
}
