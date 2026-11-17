package cli

import (
	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
)

// linkLabel is the describe heading for a list key.
func linkLabel(key string) string {
	if key == plan.PullRequestsKey {
		return "Pull Requests"
	}
	return "Issues"
}
