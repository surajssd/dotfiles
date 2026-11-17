package github

import (
	"testing"
)

func TestGithubRepoFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/widgets.git":   "acme/widgets",
		"https://github.com/acme/widgets/":      "acme/widgets",
		"git@github.com:acme/widgets.git":       "acme/widgets",
		"ssh://git@github.com:22/acme/widgets":  "acme/widgets",
		"ssh://github.com/acme/wid.gets":        "acme/wid.gets",
		"https://gitlab.com/acme/widgets.git":   "",
		"https://github.com/acme":               "",
		"https://github.com/acme/..":            "",
		"git@github.com:acme/widgets/extra.git": "",
	}
	for url, want := range cases {
		got, ok := githubRepoFromURL(url)
		if got != want || ok != (want != "") {
			t.Errorf("githubRepoFromURL(%q) = %q, %v", url, got, ok)
		}
	}
}
