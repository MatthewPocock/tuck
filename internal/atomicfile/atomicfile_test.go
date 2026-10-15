package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")

	for _, want := range []string{"first", "second, longer than the first"} {
		if err := Write(path, []byte(want), 0o600); err != nil {
			t.Fatalf("Write(%q): %v", want, err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("content = %q, want %q", got, want)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries, want only the target file", len(entries))
	}
}

func TestWriteMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "file.txt")
	if err := Write(path, []byte("x"), 0o600); err == nil {
		t.Fatal("Write into a missing directory succeeded, want error")
	}
}
