package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSessionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	want := Session{Pinned: []PinnedNote{
		{ID: "20261015-090000-aaaaaa", X: -1200, Y: 40, Width: 450, Height: 525},
		{ID: "shopping list", X: 100, Y: 200, Width: 300, Height: 300},
	}}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadSession = %+v, want %+v", got, want)
	}
}

func TestSessionToleratesMissingAndBadFiles(t *testing.T) {
	dir := t.TempDir()
	if s, err := LoadSession(filepath.Join(dir, "absent.json")); err != nil || len(s.Pinned) != 0 {
		t.Errorf("missing file: %+v, %v", s, err)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err := LoadSession(bad); err == nil || len(s.Pinned) != 0 {
		t.Errorf("corrupt file: %+v, %v", s, err)
	}

	odd := filepath.Join(dir, "odd.json")
	if err := os.WriteFile(odd, []byte(`{"pinned":[{"id":"","width":10,"height":10},{"id":"x","width":0,"height":5},{"id":"ok","width":5,"height":5}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, _ := LoadSession(odd); len(s.Pinned) != 1 || s.Pinned[0].ID != "ok" {
		t.Errorf("invalid entries kept: %+v", s.Pinned)
	}
}
