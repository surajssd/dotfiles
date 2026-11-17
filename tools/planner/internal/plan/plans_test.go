package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
		if got := ReduceLinkTarget(in); got != want {
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
	writePlanFileForTest(t, filepath.Join(root, "github.com", "a", "b", "260101000000-kept.md"), "# Kept\n")
	writePlanFileForTest(t, filepath.Join(root, "github.com", "a", "b", "notes.txt"), "x")
	writePlanFileForTest(t, filepath.Join(root, ".claude", "260102000000-hidden-dir.md"), "# Hidden\n")
	writePlanFileForTest(t, filepath.Join(root, "github.com", "a", "b", ".260103000000-hidden-file.md"), "# Hidden\n")
	outside := t.TempDir()
	writePlanFileForTest(t, filepath.Join(outside, "260104000000-linked.md"), "# Linked\n")
	if err := os.Symlink(outside, filepath.Join(root, "github.com", "linked")); err != nil {
		t.Fatal(err)
	}
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Plans) != 1 || c.Plans[0].Basename != "260101000000-kept" || c.Plans[0].Repo != "a/b" {
		t.Fatalf("plans = %#v", c.Plans)
	}
}

func writePlanFileForTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
