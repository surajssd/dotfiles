package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// versionText formats the build information kubectl version style: module
// version, commit, tree state, commit date, and Go version. A binary without
// build information prints unknown for each.
func versionText(info *debug.BuildInfo, ok bool) string {
	version, commit, tree, date, goVersion := "unknown", "unknown", "unknown", "unknown", "unknown"
	if ok {
		if info.Main.Version != "" {
			version = info.Main.Version
		}
		if info.GoVersion != "" {
			goVersion = info.GoVersion
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = setting.Value
			case "vcs.time":
				date = setting.Value
			case "vcs.modified":
				tree = "clean"
				if setting.Value == "true" {
					tree = "dirty"
				}
			}
		}
	}
	var out strings.Builder
	for _, field := range [][2]string{{"Version", version}, {"Commit", commit}, {"Tree", tree}, {"Date", date}, {"Go", goVersion}} {
		fmt.Fprintf(&out, "%-10s%s\n", field[0]+":", field[1])
	}
	return out.String()
}
