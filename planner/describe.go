package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func runDescribe(deps dependencies, root string, names []string) error {
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	_, all := buildForest(c)
	children := map[*plan][]*plan{}
	for _, n := range all {
		if n.parent != nil && n.parent.plan != nil {
			children[n.parent.plan] = append(children[n.parent.plan], n.plan)
		}
	}
	findings := checkCorpus(c)
	home, _ := os.UserHomeDir()
	var out strings.Builder
	for i, name := range names {
		p, err := findPlan(c, name)
		if err != nil {
			return err
		}
		if i > 0 {
			out.WriteString("\n\n")
		}
		describePlan(&out, p, children[p], findings, home, deps.now())
	}
	return writeOutput(deps.stdout, out.String())
}

// describePlan writes one kubectl describe style block: aligned Key: Value
// lines, with list values on indented lines below their key.
func describePlan(out *strings.Builder, p *plan, children []*plan, findings []finding, home string, now time.Time) {
	field := func(key, value string) {
		if value == "" {
			value = emptyCell
		}
		fmt.Fprintf(out, "%-10s%s\n", key+":", value)
	}
	list := func(key string, items []string) {
		if len(items) == 0 {
			field(key, emptyCell)
			return
		}
		fmt.Fprintf(out, "%s:\n", key)
		for _, item := range items {
			fmt.Fprintf(out, "  %s\n", item)
		}
	}
	field("Name", p.name)
	field("Title", p.title)
	field("File", displayPath(p.path, home))
	field("Repo", p.repo)
	field("Type", p.front.Type)
	field("Status", p.front.ImplementationStatus)
	checked := ""
	if strings.TrimSpace(p.front.StatusChecked) != "" {
		checked = fmt.Sprintf("%s (%s)", strings.TrimSpace(p.front.StatusChecked), ageText(p, now))
	}
	field("Checked", checked)
	field("Parent", p.front.Parent)
	var names []string
	for _, child := range children {
		names = append(names, child.name)
	}
	list("Children", names)
	field("Note", strings.TrimSpace(p.front.StatusNote))
	var problems []string
	for _, f := range findings {
		if f.plan == p {
			problems = append(problems, fmt.Sprintf("%-9s %-21s %s", f.severity.String(), f.rule, f.detail))
		}
	}
	list("Findings", problems)
}
