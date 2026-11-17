package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/surajssd/dotfiles/tools/planner/internal/github"
	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
)

const checkHelp = `Report problems in the plans under the root as a table with one row per
finding: FILE, REPO, SEVERITY, RULE, and DETAIL, sorted by file, then rule.
The exit status is 1 when any error finding exists and 0 when there are only
advisories or no findings.

With plan names (a path, a [[wikilink]], a basename, the short NAME shown by
planner get, or a URL the plan lists) only the findings of those plans are
shown. A name that matches no plan is reported after the table and the exit
status is 1.

--github asks gh pr view for the state of every pull request the plans in
scope list (the active plans, or the named ones) and adds the two rules
marked below. It needs gh on PATH, logged in, and network access.

Front matter keys: Type, ImplementationStatus, StatusChecked, and
StatusNote are required; Parent, SupersededBy, Issues, and PullRequests
are optional. Parent and SupersededBy hold a "[[wikilink]]", a path, or a
URL; Issues and PullRequests hold lists of URLs. Directory plans also require
ID (twelve digits, a hyphen, and a reference-safe name).

Recognised ImplementationStatus values:

  NotImplemented, InProgress, PartiallyImplemented, ImplementedUnmerged,
  Implemented, Superseded, Abandoned

Rules:

  invalid-id            error     a directory plan has a missing or invalid ID
  duplicate-id          error     an effective identity identifies several plans

  no-front-matter       advisory  the first line is not ---
  invalid-front-matter  error     no closing --- or the YAML does not decode
  missing-field         error     Type, ImplementationStatus, StatusChecked,
                                  or StatusNote is absent or empty
  unknown-key           advisory  a front matter key outside the schema
  unknown-status        error     ImplementationStatus is not a recognised value
  invalid-date          error     StatusChecked is not YYYY-MM-DD
  invalid-link          error     an Issues or PullRequests entry is not an
                                  http(s) URL, a github.com PullRequests entry
                                  is not a pull request, or a github.com Issues
                                  entry is a pull request
  parent-not-found      error     a wikilink parent matches zero or several
                                  plans, or a path parent does not exist
  parent-cycle          error     following parent links returns to the plan
  successor-not-found   error     the same for SupersededBy
  missing-successor     advisory  ImplementationStatus is Superseded but
                                  SupersededBy is empty
  dead-link             error     a link in the body or in StatusNote names a
                                  plan-style file (twelve digits, a hyphen, a
                                  name) that is not under the root
  undumped-reference    advisory  Parent, SupersededBy, or a link in the body
                                  or in StatusNote points into .claude/plans/
  duplicate-title       advisory  two or more plans in one repository folder
                                  share an H1 without replacing one another
                                  through SupersededBy
  stale-pr-note         advisory  (--github) a sentence of StatusNote calls a
                                  listed pull request open, by URL or #<n>,
                                  while GitHub says CLOSED or MERGED
  pr-state-unknown      advisory  (--github) gh could not report the state of
                                  a listed pull request URL`

type checkOptions struct {
	github bool
}

func runCheck(deps dependencies, root string, names []string, opts checkOptions) error {
	c, err := plan.Load(root)
	if err != nil {
		return err
	}
	findings := plan.Check(c)
	var scope []*plan.Plan
	var missing error
	if len(names) > 0 {
		scope, missing = plan.FindAll(c, names)
		if len(scope) == 0 {
			return missing
		}
		findings = plan.FindingsFor(findings, scope)
	} else {
		for _, p := range c.Plans {
			if p.Valid() && p.Active() {
				scope = append(scope, p)
			}
		}
	}
	if opts.github {
		states := github.FetchPRStates(pullRequestURLs(scope))
		findings = plan.SortFindings(append(findings, githubFindings(scope, states)...))
	}
	if len(findings) == 0 {
		if err := writeOutput(deps.stderr, "No findings.\n"); err != nil {
			return err
		}
		return missing
	}
	table := [][]string{{"FILE", "REPO", "SEVERITY", "RULE", "DETAIL"}}
	errorCount, advisoryCount := 0, 0
	for _, f := range findings {
		repo := f.Plan.Repo
		if repo == "" {
			repo = emptyCell
		}
		file := filepath.Base(f.Plan.RelPath)
		if f.Plan.Directory != "" {
			file = f.Plan.RelPath
		}
		table = append(table, []string{file, repo, f.Severity.String(), f.Rule, f.Detail})
		if f.Severity == plan.Failure {
			errorCount++
		} else {
			advisoryCount++
		}
	}
	if err := writeOutput(deps.stdout, renderTable(table, 0)); err != nil {
		return err
	}
	if errorCount > 0 {
		summary := fmt.Errorf("check: %s, %s", plural(errorCount, "error", "errors"), plural(advisoryCount, "advisory", "advisories"))
		if missing != nil {
			return errors.Join(missing, summary)
		}
		return summary
	}
	return missing
}
