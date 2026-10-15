package main

import (
	"slices"
	"strings"
)

const reposHelp = `Print every <org>/<repo> that has a plan under the root, sorted, one per
line, whatever the status of its plans. A plan outside a
github.com/<org>/<repo> folder has no repository and is left out.

--repo on planner get and planner tree matches against these labels.`

// repoLabels returns the distinct repository labels of the corpus, sorted.
func repoLabels(c *corpus) []string {
	var labels []string
	for _, p := range c.plans {
		if p.repo != "" && !slices.Contains(labels, p.repo) {
			labels = append(labels, p.repo)
		}
	}
	slices.Sort(labels)
	return labels
}

func runRepos(deps dependencies, root string) error {
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	labels := repoLabels(c)
	if len(labels) == 0 {
		return nil
	}
	return writeOutput(deps.stdout, strings.Join(labels, "\n")+"\n")
}
