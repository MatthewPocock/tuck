package atomicfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// openWithoutDeleteSharing opens path the way many sync clients and virus
// scanners do, which makes replacing the file fail while the handle is open.
func openWithoutDeleteSharing(t *testing.T, path string) syscall.Handle {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil,
		syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestWriteWaitsOutBrieflyLockedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := openWithoutDeleteSharing(t, path)
	go func() {
		time.Sleep(60 * time.Millisecond)
		syscall.CloseHandle(h)
	}()

	if err := Write(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("Write while the file was briefly locked: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Errorf("file contains %q, want %q", got, "new")
	}
}

func TestWriteGivesUpOnPersistentLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := openWithoutDeleteSharing(t, path)
	defer syscall.CloseHandle(h)

	if err := Write(path, []byte("new"), 0o600); err == nil {
		t.Fatal("Write succeeded although the file stayed locked")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".*.tmp"))
	if len(matches) != 0 {
		t.Errorf("temporary files left behind: %v", matches)
	}
}
