package note

import (
	"fmt"
	"time"
)

// Ago describes when t happened, relative to now, as briefly as the deck's
// small labels need: "Just now", "5 min ago", "3h ago", "Yesterday", a
// weekday within the last week, then a date.
func Ago(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.In(now.Location())
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "Just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	}
	day := func(x time.Time) time.Time {
		y, m, dd := x.Date()
		return time.Date(y, m, dd, 0, 0, 0, 0, x.Location())
	}
	days := int(day(now).Sub(day(t)).Hours()/24 + 0.5)
	switch {
	case days == 0:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case days == 1:
		return "Yesterday"
	case days < 7:
		return t.Weekday().String()
	case t.Year() == now.Year():
		return t.Format("2 Jan")
	}
	return t.Format("2 Jan 2006")
}
