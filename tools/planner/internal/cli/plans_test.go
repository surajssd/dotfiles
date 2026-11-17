package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSymlinkRootAndPlanReferences(t *testing.T) {
	root, path := statusRoot(t)
	alias := filepath.Join(t.TempDir(), "plans")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{path, filepath.Join(alias, rel), "widgets-plan"} {
		deps, io := testDependencies("", true, 0)
		if err := run(t, deps, "get", "--root", alias, "-o", "name", ref); err != nil {
			t.Error(err)
		}
		if got := io.stdout.String(); got != "260910120000-widgets-plan\n" {
			t.Errorf("%s: output = %q", ref, got)
		}
	}
}

func TestTreeIncludesCyclesWithInvalidMetadata(t *testing.T) {
	root := t.TempDir()
	for name, parent := range map[string]string{"a": "b", "b": "a"} {
		note := "StatusNote: Valid.\n"
		if name == "b" {
			note = "StatusNote: [invalid, type]\n"
		}
		writeFile(t, filepath.Join(root, "260930120000-"+name+".md"), "---\nType: plan\nParent: \"[[260930120000-"+parent+"]]\"\nImplementationStatus: InProgress\nStatusChecked: 2026-09-29\n"+note+"---\n# "+name+"\n")
	}
	for _, names := range [][]string{nil, {"a"}, {"b"}} {
		deps, io := testDependencies("", true, 0)
		if err := run(t, deps, append([]string{"tree", "--root", root, "--all"}, names...)...); err != nil {
			t.Fatal(err)
		}
		if out := io.stdout.String(); !strings.Contains(out, "(parent cycle)") {
			t.Fatalf("cycle disappeared from tree: %q", out)
		}
	}
}

func TestDaysSinceCountsCalendarDaysAcrossDST(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("time zone database unavailable:", err)
	}
	cases := []struct {
		checked string
		now     time.Time
		want    int
	}{
		{"2026-03-07", time.Date(2026, time.March, 9, 1, 0, 0, 0, zone), 2},
		{"2026-03-08", time.Date(2026, time.March, 8, 23, 59, 0, 0, zone), 0},
		{"2026-10-31", time.Date(2026, time.November, 2, 0, 30, 0, 0, zone), 2},
		{"2026-09-29", time.Date(2026, time.September, 29, 0, 0, 0, 0, zone), 0},
	}
	for _, tc := range cases {
		got, ok := daysSince(tc.checked, tc.now)
		if !ok || got != tc.want {
			t.Errorf("daysSince(%q, %v) = %d (%v), want %d", tc.checked, tc.now, got, ok, tc.want)
		}
	}
	if _, ok := daysSince("2026-9-1", fixedNow); ok {
		t.Error("a non ISO date parsed")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdefghij", 8); got != "abcde..." {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("abc", 8); got != "abc" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("äöüßabcdefgh", 8); got != "äöüßa..." {
		t.Errorf("truncate counts runes: %q", got)
	}
	if got := truncate("abcdefghij", 0); got != "abcdefghij" {
		t.Errorf("width 0 must not truncate: %q", got)
	}
}

func TestDisplayPath(t *testing.T) {
	cases := map[string]string{
		"/home/u/plans/a.md": "~/plans/a.md",
		"/home/u":            "~",
		"/home/user2/a.md":   "/home/user2/a.md",
		"/srv/plans/a.md":    "/srv/plans/a.md",
	}
	for in, want := range cases {
		if got := displayPath(in, "/home/u"); got != want {
			t.Errorf("displayPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := displayPath("/x/y.md", ""); got != "/x/y.md" {
		t.Errorf("empty home changed the path: %q", got)
	}
}
