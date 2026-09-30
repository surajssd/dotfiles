package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// displayPath shortens a path under the home directory to a ~ form.
func displayPath(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return path
}

type listOptions struct {
	all       bool
	output    string
	noHeaders bool
	repo      string
}

func (o listOptions) wide() bool { return o.output == "wide" }

// table reports whether the output is one of the two table formats rather
// than a machine-readable one.
func (o listOptions) table() bool { return o.output == "" || o.output == "wide" }

// matchesRepo reports whether a repository label passes the --repo filter: an
// empty filter matches everything, otherwise a case-insensitive substring.
func matchesRepo(repo, filter string) bool {
	return filter == "" || strings.Contains(strings.ToLower(repo), strings.ToLower(filter))
}

func listHeader(opts listOptions) []string {
	header := []string{"NAME", "REPO", "STATUS", "CHECKED", "TITLE"}
	if opts.wide() {
		header = append(header, "TYPE", "PATH", "NOTE")
	}
	return header
}

// listWidth returns the terminal width to truncate to, or 0 for no truncation.
func listWidth(deps dependencies, opts listOptions) int {
	width, isTerminal := deps.stdoutWidth()
	if opts.wide() || !isTerminal {
		return 0
	}
	return width
}

// writeList prints the output on stdout and the notes about empty output and
// skipped plans on stderr. The empty note is skipped when plans were named,
// because their errors already say what is missing.
func writeList(deps dependencies, output string, skipped int, noteWhenEmpty bool) error {
	if err := writeOutput(deps.stdout, output); err != nil {
		return err
	}
	var notes []string
	if output == "" && noteWhenEmpty {
		notes = append(notes, "No plans found.")
	}
	if skipped > 0 {
		notes = append(notes, fmt.Sprintf("%s without valid front matter hidden %s", plural(skipped, "plan", "plans"), hiddenHint))
	}
	if len(notes) == 0 {
		return nil
	}
	return writeOutput(deps.stderr, strings.Join(notes, "\n")+"\n")
}

// runGet lists plans as a flat table or as records. Named plans are shown
// whatever their status, in the order given, and a name that resolves to no
// plan is reported after the output; otherwise the active filter and --repo
// apply.
func runGet(deps dependencies, root string, opts listOptions, names []string) error {
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	var plans []*plan
	var missing error
	skipped := 0
	if len(names) > 0 {
		plans, missing = findPlans(c, names)
	} else {
		for _, p := range c.plans {
			switch {
			case !matchesRepo(p.repo, opts.repo):
			case opts.all || (p.valid() && p.active()):
				plans = append(plans, p)
			case !p.valid():
				skipped++
			}
		}
		sort.Slice(plans, func(i, j int) bool {
			if plans[i].repo != plans[j].repo {
				return plans[i].repo < plans[j].repo
			}
			return plans[i].basename < plans[j].basename
		})
	}
	output := ""
	switch {
	case opts.table():
		if len(plans) > 0 {
			home, _ := os.UserHomeDir()
			now := deps.now()
			var cells [][]string
			if !opts.noHeaders {
				cells = append(cells, listHeader(opts))
			}
			for _, p := range plans {
				cells = append(cells, row{node: &node{plan: p}, showRepo: true}.cells(opts, now, home))
			}
			output = renderTable(cells, listWidth(deps, opts))
		}
	case len(plans) > 0 || len(names) == 0:
		if output, err = renderRecords(plans, opts.output, len(names) == 1); err != nil {
			return err
		}
	}
	if err := writeList(deps, output, skipped, len(names) == 0); err != nil {
		return err
	}
	return missing
}

type node struct {
	plan     *plan
	group    string
	parent   *node
	children []*node
	problem  string
	visible  bool
}

const staleAfterDays = 7

// runTree prints the forest, or, when plans are named, the subtree under each
// named plan, which is always shown itself.
func runTree(deps dependencies, root string, opts listOptions, names []string) error {
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	roots, all := buildForest(c)
	home, _ := os.UserHomeDir()
	if len(names) > 0 {
		byPlan := map[*plan]*node{}
		for _, n := range all {
			byPlan[n.plan] = n
		}
		found, missing := findPlans(c, names)
		roots = nil
		for _, p := range found {
			n := byPlan[p]
			markVisible(n, opts)
			n.visible = true
			roots = append(roots, n)
		}
		output := ""
		if len(found) > 0 {
			output = renderForest(roots, opts, deps.now(), listWidth(deps, opts), home)
		}
		if err := writeList(deps, output, 0, false); err != nil {
			return err
		}
		return missing
	}
	for _, n := range roots {
		markVisible(n, opts)
	}
	hidden := 0
	for _, n := range all {
		if !n.visible && !n.plan.valid() && matchesRepo(n.plan.repo, opts.repo) {
			hidden++
		}
	}
	return writeList(deps, renderForest(roots, opts, deps.now(), listWidth(deps, opts), home), hidden, true)
}

// buildForest links every plan to its parent. External parents (paths and
// URLs) become group nodes; unresolved parents and cycles turn the plan into a
// root that carries the problem in its header. Plans without valid front
// matter become nodes too, so a legacy parent can hold its children.
func buildForest(c *corpus) (roots, all []*node) {
	nodes := map[*plan]*node{}
	var plans []*node
	for _, p := range c.plans {
		n := &node{plan: p}
		nodes[p] = n
		plans = append(plans, n)
	}
	groups := map[string]*node{}
	for _, n := range plans {
		value := n.plan.front.Parent
		switch classifyParent(value) {
		case parentNone:
		case parentWikilink:
			matches := c.lookup(reduceLinkTarget(value))
			if len(matches) != 1 {
				n.problem = fmt.Sprintf("(parent not found: %s)", value)
			} else {
				n.parent = nodes[matches[0]]
			}
		case parentPath, parentURL:
			group, ok := groups[value]
			if !ok {
				group = &node{group: value}
				groups[value] = group
			}
			n.parent = group
		default:
			n.problem = fmt.Sprintf("(parent not found: %s)", value)
		}
	}
	var inCycle []*node
	for _, n := range plans {
		if _, cycle := parentChain(c, n.plan); cycle {
			inCycle = append(inCycle, n)
		}
	}
	for _, n := range inCycle {
		n.parent = nil
		n.problem = "(parent cycle)"
	}
	for _, g := range groups {
		roots = append(roots, g)
	}
	for _, n := range plans {
		if n.parent == nil {
			roots = append(roots, n)
			continue
		}
		n.parent.children = append(n.parent.children, n)
	}
	sort.Slice(roots, func(i, j int) bool { return lessRoot(roots[i], roots[j]) })
	for _, n := range plans {
		sort.Slice(n.children, func(i, j int) bool { return n.children[i].plan.basename < n.children[j].plan.basename })
	}
	for _, g := range groups {
		sort.Slice(g.children, func(i, j int) bool { return g.children[i].plan.basename < g.children[j].plan.basename })
	}
	return roots, plans
}

func lessRoot(a, b *node) bool {
	if (a.plan == nil) != (b.plan == nil) {
		return a.plan == nil
	}
	if a.plan == nil {
		return a.group < b.group
	}
	if a.plan.repo != b.plan.repo {
		return a.plan.repo < b.plan.repo
	}
	return a.plan.basename < b.plan.basename
}

func markVisible(n *node, opts listOptions) bool {
	childVisible := false
	for _, child := range n.children {
		if markVisible(child, opts) {
			childVisible = true
		}
	}
	self := false
	if n.plan != nil {
		self = (opts.all || (n.plan.valid() && n.plan.active())) && matchesRepo(n.plan.repo, opts.repo)
	}
	n.visible = self || childVisible
	return n.visible
}

type row struct {
	node   *node
	prefix string
	// showRepo is false for a child filed in the same repository as its parent.
	showRepo bool
}

const (
	columnGap    = "   "
	emptyCell    = "-"
	ellipsis     = "..."
	hiddenHint   = "(--all shows them; planner check lists the problems)"
	externalName = "External"
)

// collectRows flattens the visible part of a tree into rows whose NAME cell
// carries the connectors that draw the hierarchy.
func collectRows(n *node, prefix, childPrefix string, rows *[]row) {
	if !n.visible {
		return
	}
	showRepo := n.parent == nil || n.parent.plan == nil || n.parent.plan.repo != n.plan.repo
	*rows = append(*rows, row{node: n, prefix: prefix, showRepo: showRepo})
	var visible []*node
	for _, child := range n.children {
		if child.visible {
			visible = append(visible, child)
		}
	}
	for i, child := range visible {
		connector, continuation := "├── ", "│   "
		if i == len(visible)-1 {
			connector, continuation = "└── ", "    "
		}
		collectRows(child, childPrefix+connector, childPrefix+continuation, rows)
	}
}

func renderForest(roots []*node, opts listOptions, now time.Time, width int, home string) string {
	var rows []row
	for _, r := range roots {
		collectRows(r, "", "", &rows)
	}
	if len(rows) == 0 {
		return ""
	}
	var table [][]string
	if !opts.noHeaders {
		table = append(table, listHeader(opts))
	}
	for _, r := range rows {
		table = append(table, r.cells(opts, now, home))
	}
	return renderTable(table, width)
}

func (r row) cells(opts listOptions, now time.Time, home string) []string {
	name := r.prefix
	if r.node.plan == nil {
		cells := []string{name + r.node.group, emptyCell, externalName, emptyCell, "not a dumped plan"}
		if opts.wide() {
			cells = append(cells, emptyCell, emptyCell, emptyCell)
		}
		return cells
	}
	p := r.node.plan
	name += p.name
	if r.node.problem != "" {
		name += " " + r.node.problem
	}
	repo := ""
	if r.showRepo {
		repo = orDash(p.repo)
	}
	cells := []string{name, repo, orDash(p.front.ImplementationStatus), checkedText(p, now), p.title}
	if opts.wide() {
		cells = append(cells, orDash(p.front.Type), displayPath(p.path, home), orDash(strings.TrimSpace(p.front.StatusNote)))
	}
	return cells
}

func orDash(value string) string {
	if value == "" {
		return emptyCell
	}
	return value
}

// renderTable pads every column but the last to its widest cell, kubectl
// style. When width is positive the last column is cut to fit the terminal.
func renderTable(table [][]string, width int) string {
	if len(table) == 0 {
		return ""
	}
	columns := len(table[0])
	widths := make([]int, columns-1)
	for _, cells := range table {
		for i := 0; i < columns-1; i++ {
			widths[i] = max(widths[i], utf8.RuneCountInString(cells[i]))
		}
	}
	used := len(columnGap) * (columns - 1)
	for _, w := range widths {
		used += w
	}
	var out strings.Builder
	for _, cells := range table {
		var line strings.Builder
		for i := 0; i < columns-1; i++ {
			line.WriteString(cells[i])
			line.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cells[i])))
			line.WriteString(columnGap)
		}
		last := cells[columns-1]
		if width > 0 {
			last = truncate(last, width-used)
		}
		line.WriteString(last)
		out.WriteString(strings.TrimRight(line.String(), " ") + "\n")
	}
	return out.String()
}

// checkedText is the CHECKED cell: calendar days since StatusChecked, with !
// after an active plan that has not been checked for more than a week.
func checkedText(p *plan, now time.Time) string {
	if !p.valid() {
		return emptyCell
	}
	days, ok := daysSince(p.front.StatusChecked, now)
	if !ok {
		return "?d"
	}
	text := fmt.Sprintf("%dd", days)
	if p.active() && days > staleAfterDays {
		text += "!"
	}
	return text
}

// daysSince counts local calendar days from a YYYY-MM-DD date to now. Both
// dates are rebuilt at UTC midnight so a daylight-saving day still counts as
// one day.
func daysSince(checked string, now time.Time) (int, bool) {
	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(checked), now.Location())
	if err != nil {
		return 0, false
	}
	from := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(to.Sub(from).Hours() / 24), true
}

// truncate cuts text to width runes, ending in an ellipsis. A width too small
// to show anything useful leaves the text alone.
func truncate(text string, width int) string {
	if width <= 2*len(ellipsis) || utf8.RuneCountInString(text) <= width {
		return text
	}
	runes := []rune(text)
	return string(runes[:width-len(ellipsis)]) + ellipsis
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
