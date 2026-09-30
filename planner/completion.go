package main

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// completer produces shell completions from the corpus under the resolved
// root. Every method returns nothing when the root cannot be resolved, so a
// missing configuration never breaks the shell.
type completer struct {
	root func() (string, error)
}

func (c completer) corpus() (*corpus, bool) {
	root, err := c.root()
	if err != nil {
		return nil, false
	}
	corp, err := loadCorpus(root)
	if err != nil {
		return nil, false
	}
	return corp, true
}

// plans offers a unique short name, basename, or path for each plan, with its
// title as the description. Plans already named on the command line are left out.
func (c completer) plans(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	corp, ok := c.corpus()
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	given := map[string]bool{}
	for _, arg := range args {
		given[reduceLinkTarget(arg)] = true
	}
	counts := map[string]int{}
	for _, p := range corp.plans {
		counts[p.name]++
	}
	var out []string
	for _, p := range corp.plans {
		name := p.name
		if counts[name] > 1 {
			name = p.basename
			if len(corp.lookup(name)) > 1 {
				name = p.path
			}
		}
		if given[p.basename] || given[p.name] || !strings.HasPrefix(name, toComplete) {
			continue
		}
		out = append(out, name+"\t"+p.title)
	}
	sort.Strings(out)
	return out, cobra.ShellCompDirectiveNoFileComp
}

// planThenStatus completes a plan for the first argument and a status value
// for the second.
func (c completer) planThenStatus(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return c.plans(cmd, args, toComplete)
	case 1:
		return completeValues(statusValues)(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// planFirst completes a plan for the first argument and nothing after it.
func (c completer) planFirst(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return c.plans(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// planThenParent completes a plan for the first argument and a parent for
// the second.
func (c completer) planThenParent(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return c.plans(cmd, args, toComplete)
	case 1:
		return c.parents(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// parents offers plans, minus any already named on the command line, and
// lets the shell add files, since a parent may be a path outside the root.
func (c completer) parents(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	out, _ := c.plans(cmd, args, toComplete)
	return out, cobra.ShellCompDirectiveDefault
}

// repos offers every distinct repository label.
func (c completer) repos(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	corp, ok := c.corpus()
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeValues(repoLabels(corp))(cmd, args, toComplete)
}

// completeValues completes from a fixed list.
func completeValues(values []string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var out []string
		for _, value := range values {
			if strings.HasPrefix(value, toComplete) {
				out = append(out, value)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

// mustCompleteFlag registers a flag completion; the flag name is a constant,
// so a failure is a programming error.
func mustCompleteFlag(cmd *cobra.Command, name string, fn cobra.CompletionFunc) {
	if err := cmd.RegisterFlagCompletionFunc(name, fn); err != nil {
		panic(err)
	}
}
