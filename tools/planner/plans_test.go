package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSplitFrontMatter(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		block    string
		body     string
		hasFront bool
		wantErr  bool
	}{
		{"none", "# Title\n", "", "# Title\n", false, false},
		{"plain", "---\nType: plan\n---\n\n# T\n", "Type: plan\n", "\n# T\n", true, false},
		{"crlf", "---\r\nType: plan\r\n---\r\n# T\r\n", "Type: plan\r\n", "# T\r\n", true, false},
		{"unclosed", "---\nType: plan\n# T\n", "", "Type: plan\n# T\n", true, true},
		{"empty block", "---\n---\nbody\n", "", "body\n", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block, body, hasFront, err := splitFrontMatter(tc.text)
			if block != tc.block || body != tc.body || hasFront != tc.hasFront || (err != nil) != tc.wantErr {
				t.Errorf("got (%q, %q, %v, %v)", block, body, hasFront, err)
			}
		})
	}
}

func TestScanBodySkipsCodeAndFindsTitle(t *testing.T) {
	body := "intro\n" +
		"# The title\n" +
		"# Not the title\n" +
		"[a](x/260101000000-a.md) and [[260102000000-b#h]] plus `[[260103000000-code]]` and [c](https://e.com/p) [[ ]]\n" +
		"```\n[[260104000000-fenced]]\n```\n" +
		"````md\n```\n[[260105000000-nested]]\n```\n````\n" +
		"~~~\n[d](260106000000-tilde.md)\n~~~\n" +
		"[[260107000000-after]]\n"
	title, hasTitle, links := scanBody(body)
	if !hasTitle || title != "The title" {
		t.Errorf("title = %q (%v)", title, hasTitle)
	}
	want := []string{"x/260101000000-a.md", "https://e.com/p", "[[260102000000-b#h]]", "[[260107000000-after]]"}
	if !reflect.DeepEqual(links, want) {
		t.Errorf("links = %q, want %q", links, want)
	}
}

func TestReduceLinkTarget(t *testing.T) {
	cases := map[string]string{
		"[[260101000000-a]]":                    "260101000000-a",
		"[[260101000000-a|alias]]":              "260101000000-a",
		"[[260101000000-a#heading]]":            "260101000000-a",
		"[[dir/sub/260101000000-a.md]]":         "260101000000-a",
		"../gadgets/260102000000-gone.md#sec":   "260102000000-gone",
		"/abs/path/260103000000-x.md":           "260103000000-x",
		"https://example.com/260104000000-u.md": "260104000000-u",
		"[[ ]]":                                 "",
		"":                                      "",
		"260105000000-bare":                     "260105000000-bare",
		"[[People/Someone|Someone]]":            "Someone",
	}
	for in, want := range cases {
		if got := reduceLinkTarget(in); got != want {
			t.Errorf("reduceLinkTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortNameAndPlanStyle(t *testing.T) {
	if !isPlanStyle("260929171852-planner-go-cli") || shortName("260929171852-planner-go-cli") != "planner-go-cli" {
		t.Error("twelve-digit prefix was not recognised")
	}
	for _, name := range []string{"2026-09-Sep-29-17-18-52-x", "legacy-name", "260929171852-", "26092917185-x"} {
		if isPlanStyle(name) || shortName(name) != name {
			t.Errorf("%q should not be plan-style", name)
		}
	}
}

func TestCorpusWalkSkipsHiddenSymlinkedAndNonMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "github.com", "a", "b", "260101000000-kept.md"), "# Kept\n")
	writeFile(t, filepath.Join(root, "github.com", "a", "b", "notes.txt"), "x")
	writeFile(t, filepath.Join(root, ".claude", "260102000000-hidden-dir.md"), "# Hidden\n")
	writeFile(t, filepath.Join(root, "github.com", "a", "b", ".260103000000-hidden-file.md"), "# Hidden\n")
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "260104000000-linked.md"), "# Linked\n")
	if err := os.Symlink(outside, filepath.Join(root, "github.com", "linked")); err != nil {
		t.Fatal(err)
	}
	c, err := loadCorpus(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.plans) != 1 || c.plans[0].basename != "260101000000-kept" || c.plans[0].repo != "a/b" {
		t.Fatalf("plans = %#v", c.plans)
	}
}

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
