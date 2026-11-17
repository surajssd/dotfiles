package plan

import (
	"errors"
	"strings"
	"testing"
)

func TestLinkProblem(t *testing.T) {
	const (
		prA     = "https://github.com/acme/widgets/pull/1"
		jiraA   = "https://acme.atlassian.net/browse/WID-7"
		ghIssue = "https://github.com/acme/widgets/issues/3"
	)
	cases := []struct{ key, value, want string }{
		{PullRequestsKey, prA, ""},
		{PullRequestsKey, "https://gitlab.com/acme/widgets/-/merge_requests/4", ""},
		{PullRequestsKey, ghIssue, "is not a GitHub pull request URL"},
		{PullRequestsKey, "https://github.com/acme/widgets", "is not a GitHub pull request URL"},
		{PullRequestsKey, "acme/widgets#1", "is not an http(s) URL"},
		{PullRequestsKey, "", "is not an http(s) URL"},
		{IssuesKey, jiraA, ""},
		{IssuesKey, ghIssue, ""},
		{IssuesKey, "https://app.asana.com/0/123/456", ""},
		{IssuesKey, prA, "is a pull request URL"},
		{IssuesKey, "ftp://x/y", "is not an http(s) URL"},
	}
	for _, tc := range cases {
		got := LinkProblem(tc.key, tc.value)
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) {
			t.Errorf("linkProblem(%s, %q) = %q, want %q", tc.key, tc.value, got, tc.want)
		}
	}
}

func TestUpdateFrontMatterReplacesListsAndFlowSequences(t *testing.T) {
	edit := []fieldEdit{{PullRequestsKey, listLines(PullRequestsKey, []string{"c"})}}
	cases := []struct{ name, in, want string }{
		{"column zero items", "---\nPullRequests:\n- a\n- b\nnext: x\n---\n", "---\nPullRequests:\n  - c\nnext: x\n---\n"},
		{"flow sequence", "---\nPullRequests: [a, b]\nnext: x\n---\n", "---\nPullRequests:\n  - c\nnext: x\n---\n"},
		{"indented items", "---\nnext: x\nPullRequests:\n  - a\n---\nbody\n", "---\nnext: x\nPullRequests:\n  - c\n---\nbody\n"},
		{"missing key", "---\nnext: x\n---\n", "---\nnext: x\nPullRequests:\n  - c\n---\n"},
		{"crlf", "---\r\nnext: x\r\n---\r\n", "---\r\nnext: x\r\nPullRequests:\r\n  - c\r\n---\r\n"},
	}
	for _, tc := range cases {
		got, err := updateFrontMatter([]byte(tc.in), edit)
		if err != nil || string(got) != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := updateFrontMatter([]byte("# no front matter\n"), edit); !errors.Is(err, errNoFrontMatter) {
		t.Errorf("error = %v, want errNoFrontMatter", err)
	}
}
