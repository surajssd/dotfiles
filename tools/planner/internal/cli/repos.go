package cli

import (
	"slices"
	"strings"

	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
)

const reposHelp = `Print every <org>/<repo> that has a plan under the root, sorted, one per
line, whatever the status of its plans. A plan outside a
github.com/<org>/<repo> folder has no repository and is left out.

--repo on planner get and planner tree matches against these labels.`

// repoLabels returns the distinct repository labels of the corpus, sorted.
func repoLabels(c *plan.Corpus) []string {
	var labels []string
	for _, p := range c.Plans {
		if p.Repo != "" && !slices.Contains(labels, p.Repo) {
			labels = append(labels, p.Repo)
		}
	}
	slices.Sort(labels)
	return labels
}

func runRepos(deps dependencies, root string) error {
	c, err := plan.Load(root)
	if err != nil {
		return err
	}
	labels := repoLabels(c)
	if len(labels) == 0 {
		return nil
	}
	return writeOutput(deps.stdout, strings.Join(labels, "\n")+"\n")
}
