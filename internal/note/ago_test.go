package note

import (
	"testing"
	"time"
)

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, ""},
		{now.Add(-20 * time.Second), "Just now"},
		{now.Add(5 * time.Second), "Just now"},
		{now.Add(-5 * time.Minute), "5 min ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{time.Date(2026, 10, 14, 23, 0, 0, 0, time.UTC), "Yesterday"},
		{time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC), "Monday"},
		{time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC), "2 Oct"},
		{time.Date(2025, 12, 31, 9, 0, 0, 0, time.UTC), "31 Dec 2025"},
	} {
		if got := Ago(c.t, now); got != c.want {
			t.Errorf("Ago(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}
