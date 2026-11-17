package plan

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type ParentKind int

const (
	ParentNone ParentKind = iota
	ParentWikilink
	ParentPath
	ParentURL
	ParentUnknown
)

var urlSchemeRE = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`)

func ClassifyParent(value string) ParentKind {
	switch {
	case value == "":
		return ParentNone
	case strings.HasPrefix(value, "[[") && strings.HasSuffix(value, "]]"):
		return ParentWikilink
	case urlSchemeRE.MatchString(value):
		return ParentURL
	case strings.HasPrefix(value, "/"), strings.HasPrefix(value, "~"), strings.HasPrefix(value, "."):
		return ParentPath
	default:
		return ParentUnknown
	}
}

// ReduceLinkTarget turns a wikilink or link target into the basename it names:
// brackets, a |alias or #heading suffix, any directory prefix, and a .md
// extension are removed.
func ReduceLinkTarget(target string) string {
	s := strings.TrimSpace(target)
	s = strings.TrimPrefix(s, "[[")
	s = strings.TrimSuffix(s, "]]")
	if i := strings.IndexAny(s, "|#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = path.Base(filepath.ToSlash(s))
	if s == "." || s == "/" {
		return ""
	}
	return strings.TrimSuffix(s, ".md")
}

// resolveParentPlan returns the plan a wikilink parent names when exactly one
// plan matches.
func resolveParentPlan(c *Corpus, p *Plan) (*Plan, bool) {
	if ClassifyParent(p.Front.Parent) != ParentWikilink {
		return nil, false
	}
	matches := c.Lookup(ReduceLinkTarget(p.Front.Parent))
	if len(matches) != 1 {
		return nil, false
	}
	return matches[0], true
}

// expandParentPath resolves a path-form parent stored in a plan that lives in
// planDir: ~ expands to the home directory and a relative path is taken from
// the plan's own folder.
func expandParentPath(value, planDir string) (string, error) {
	expanded, err := ExpandHome(value)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(planDir, expanded)
	}
	return filepath.Clean(expanded), nil
}

func isUndumpedReference(value string) bool {
	return strings.Contains(value, ".claude/plans/")
}

// ParentChain follows wikilink parents from p and reports whether the chain
// returns to p. The returned chain lists effective identities from p to the repeated plan.
func ParentChain(c *Corpus, p *Plan) (chain []string, cycle bool) {
	seen := map[*Plan]bool{p: true}
	chain = []string{p.Identity}
	current := p
	for {
		next, ok := resolveParentPlan(c, current)
		if !ok {
			return chain, false
		}
		chain = append(chain, next.Identity)
		if next == p {
			return chain, true
		}
		if seen[next] {
			return chain, false
		}
		seen[next] = true
		current = next
	}
}

// matchPlans resolves a plan reference against the corpus. A path under the
// root, a [[wikilink]], a basename, or the short name the tree prints must
// match exactly one plan. A URL selects every plan whose front matter lists
// it as parent, successor, issue, or pull request.
func matchPlans(c *Corpus, ref string) ([]*Plan, error) {
	kind := ClassifyParent(ref)
	if kind == ParentURL {
		var matches []*Plan
		for _, p := range c.Plans {
			if p.Valid() && p.linksTo(ref) {
				matches = append(matches, p)
			}
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("plan not found: no plan under %s links to %s", c.Root, ref)
		}
		return matches, nil
	}
	isPath := kind == ParentPath || (kind != ParentWikilink && strings.ContainsRune(ref, os.PathSeparator))
	if isPath {
		expanded, err := ExpandHome(ref)
		if err != nil {
			return nil, err
		}
		absolute, err := filepath.Abs(expanded)
		if err != nil {
			return nil, err
		}
		if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = resolved
		}
		if p, ok := c.byPath[absolute]; ok {
			return []*Plan{p}, nil
		}
		return nil, fmt.Errorf("plan not found: %s is not a plan under %s", absolute, c.Root)
	}
	name := ReduceLinkTarget(ref)
	matches := c.Lookup(name)
	if len(matches) == 0 {
		for _, p := range c.Plans {
			if p.Name == name {
				matches = append(matches, p)
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches, nil
	case 0:
		return nil, fmt.Errorf("plan not found: no plan under %s matches %q", c.Root, ref)
	default:
		return nil, ambiguousReference(ref, matches)
	}
}

func ambiguousReference(ref string, matches []*Plan) error {
	var paths []string
	for _, m := range matches {
		paths = append(paths, m.RelPath)
	}
	return fmt.Errorf("plan reference %q is ambiguous: %s", ref, strings.Join(paths, ", "))
}

// Find resolves a reference that must name exactly one plan.
func Find(c *Corpus, ref string) (*Plan, error) {
	matches, err := matchPlans(c, ref)
	if err != nil {
		return nil, err
	}
	if len(matches) > 1 {
		return nil, ambiguousReference(ref, matches)
	}
	return matches[0], nil
}

// FindAll resolves every reference and returns the plans found, in the
// order given, together with one error per reference that resolved to
// nothing, so a caller can print what exists and still fail.
func FindAll(c *Corpus, refs []string) ([]*Plan, error) {
	var plans []*Plan
	var errs []error
	for _, ref := range refs {
		matches, err := matchPlans(c, ref)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		plans = append(plans, matches...)
	}
	return plans, errors.Join(errs...)
}
