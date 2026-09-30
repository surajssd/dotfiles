package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionText(t *testing.T) {
	info := &debug.BuildInfo{
		GoVersion: "go1.27.1",
		Main:      debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.time", Value: "2026-09-30T00:00:00Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	want := "Version:  (devel)\nCommit:   abc123\nTree:     dirty\nDate:     2026-09-30T00:00:00Z\nGo:       go1.27.1\n"
	if got := versionText(info, true); got != want {
		t.Errorf("versionText:\n%s\nwant:\n%s", got, want)
	}
	if got := versionText(nil, false); strings.Count(got, "unknown") != 5 {
		t.Errorf("without build information:\n%s", got)
	}
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "version"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(io.stdout.String(), "Version:") {
		t.Errorf("planner version printed:\n%s", io.stdout)
	}
}
