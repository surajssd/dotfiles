package main

import (
	"testing"
	"time"
)

func TestProjectURLs(t *testing.T) {
	for _, value := range []string{
		"https://app.asana.com/1/100/project/200",
		"https://app.asana.com/1/100/project/200/",
		"https://app.asana.com/1/100/project/200/list",
		"https://app.asana.com/1/100/project/200/board",
		"https://app.asana.com/1/100/project/200?view=board",
		"https://app.asana.com/1/100/project/200/list?view=list",
		"https://app.asana.com/1/100/project/200/board?view=board",
		"https://app.asana.com/0/200",
		"https://app.asana.com/0/200/",
		"https://app.asana.com/0/200/list",
		"https://app.asana.com/0/200/300?focus=true",
	} {
		t.Run(value, func(t *testing.T) {
			if gid, err := parseProjectURL(value); err != nil || gid != "200" {
				t.Errorf("gid = %q, err = %v", gid, err)
			}
		})
	}
	for _, value := range []string{
		"", "200", "work", "https://app.asana.com", "/0/200/list",
		"http://app.asana.com/0/200/list", "https://example.com/0/200/list",
		"https://app.asana.com.evil.test/0/200/list", "https://app.asana.com:443/0/200/list",
		"https://user@app.asana.com/0/200/list", "https://app.asana.com/0/not-a-gid/list",
		"https://app.asana.com/0//list", "https://app.asana.com/0/200x/list",
		"https://app.asana.com/1/100/project/", "https://app.asana.com/1/100/project/abc",
		"https://app.asana.com/1/workspace/project/200", "https://app.asana.com/1/100/task/200",
		"https://app.asana.com/1/100/project/200/timeline", "https://app.asana.com/1/100/project/200/list/300",
		"https://app.asana.com/1/100/project/200#task", "https://app.asana.com/%zz",
	} {
		t.Run(value, func(t *testing.T) {
			if gid, err := parseProjectURL(value); err == nil {
				t.Errorf("accepted %q as %q", value, gid)
			}
		})
	}
}

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
