package cli

import (
	"testing"
	"time"
)

func TestTomorrowAcrossDaylightSaving(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		now  time.Time
		want string
	}{
		{time.Date(2026, time.March, 8, 0, 30, 0, 0, location), "2026-03-09"},
		{time.Date(2026, time.November, 1, 0, 30, 0, 0, location), "2026-11-02"},
		{time.Date(2026, time.December, 31, 23, 30, 0, 0, location), "2027-01-01"},
	} {
		if got, err := parseDue("tomorrow", tc.now); err != nil || got != tc.want {
			t.Errorf("tomorrow from %s = %q, %v; want %s", tc.now, got, err, tc.want)
		}
	}
}
