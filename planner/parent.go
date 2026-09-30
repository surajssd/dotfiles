package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type parentKind int

const (
	parentNone parentKind = iota
	parentWikilink
	parentPath
	parentURL
	parentUnknown
)

var urlSchemeRE = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`)

func classifyParent(value string) parentKind {
	switch {
	case value == "":
		return parentNone
	case strings.HasPrefix(value, "[[") && strings.HasSuffix(value, "]]"):
		return parentWikilink
	case urlSchemeRE.MatchString(value):
		return parentURL
	case strings.HasPrefix(value, "/"), strings.HasPrefix(value, "~"), strings.HasPrefix(value, "."):
		return parentPath
	default:
		return parentUnknown
	}
}

// reduceLinkTarget turns a wikilink or link target into the basename it names:
// brackets, a |alias or #heading suffix, any directory prefix, and a .md
// extension are removed.
func reduceLinkTarget(target string) string {
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
func resolveParentPlan(c *corpus, p *plan) (*plan, bool) {
	if classifyParent(p.front.Parent) != parentWikilink {
		return nil, false
	}
	matches := c.lookup(reduceLinkTarget(p.front.Parent))
	if len(matches) != 1 {
		return nil, false
	}
	return matches[0], true
}

// expandParentPath resolves a path-form parent stored in a plan that lives in
// planDir: ~ expands to the home directory and a relative path is taken from
// the plan's own folder.
func expandParentPath(value, planDir string) (string, error) {
	expanded, err := expandHome(value)
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

// parentChain follows wikilink parents from p and reports whether the chain
// returns to p. The returned chain lists basenames from p to the repeated plan.
func parentChain(c *corpus, p *plan) (chain []string, cycle bool) {
	seen := map[*plan]bool{p: true}
	chain = []string{p.basename}
	current := p
	for {
		next, ok := resolveParentPlan(c, current)
		if !ok || !next.valid() {
			return chain, false
		}
		chain = append(chain, next.basename)
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

// findPlan resolves a plan reference against the corpus: a path under the
// root, a [[wikilink]], a basename, or the short name the tree prints. A name
// must match exactly one plan.
func findPlan(c *corpus, ref string) (*plan, error) {
	isPath := classifyParent(ref) == parentPath ||
		(classifyParent(ref) != parentWikilink && strings.ContainsRune(ref, os.PathSeparator))
	if isPath {
		expanded, err := expandHome(ref)
		if err != nil {
			return nil, err
		}
		absolute, err := filepath.Abs(expanded)
		if err != nil {
			return nil, err
		}
		if p, ok := c.byPath[absolute]; ok {
			return p, nil
		}
		return nil, fmt.Errorf("plan not found: %s is not a plan under %s", absolute, c.root)
	}
	name := reduceLinkTarget(ref)
	matches := c.lookup(name)
	if len(matches) == 0 {
		for _, p := range c.plans {
			if p.name == name {
				matches = append(matches, p)
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("plan not found: no plan under %s matches %q", c.root, ref)
	default:
		var paths []string
		for _, m := range matches {
			paths = append(paths, m.relPath)
		}
		return nil, fmt.Errorf("plan reference %q is ambiguous: %s", ref, strings.Join(paths, ", "))
	}
}
