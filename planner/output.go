package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// planRecord is the machine-readable view of a plan for -o json and -o yaml:
// the identity derived from the path, then the front matter as written. The
// front matter fields are absent when the block is missing or does not
// decode, and front_matter_error says why.
type planRecord struct {
	Name                 string   `json:"name" yaml:"name"`
	Basename             string   `json:"basename" yaml:"basename"`
	Path                 string   `json:"path" yaml:"path"`
	Repo                 string   `json:"repo" yaml:"repo"`
	Title                string   `json:"title" yaml:"title"`
	Type                 string   `json:"type,omitempty" yaml:"type,omitempty"`
	Parent               string   `json:"parent,omitempty" yaml:"parent,omitempty"`
	Issues               []string `json:"issues,omitempty" yaml:"issues,omitempty"`
	PullRequests         []string `json:"pull_requests,omitempty" yaml:"pull_requests,omitempty"`
	ImplementationStatus string   `json:"implementation_status,omitempty" yaml:"implementation_status,omitempty"`
	SupersededBy         string   `json:"superseded_by,omitempty" yaml:"superseded_by,omitempty"`
	StatusChecked        string   `json:"status_checked,omitempty" yaml:"status_checked,omitempty"`
	StatusNote           string   `json:"status_note,omitempty" yaml:"status_note,omitempty"`
	FrontMatterError     string   `json:"front_matter_error,omitempty" yaml:"front_matter_error,omitempty"`
}

type planList struct {
	Items []planRecord `json:"items" yaml:"items"`
}

func newPlanRecord(p *plan) planRecord {
	r := planRecord{Name: p.name, Basename: p.basename, Path: p.path, Repo: p.repo, Title: p.title}
	switch {
	case !p.hasFront:
		r.FrontMatterError = "no front matter"
	case p.frontErr != nil:
		r.FrontMatterError = strings.Join(strings.Fields(p.frontErr.Error()), " ")
	default:
		r.Type = p.front.Type
		r.Parent = p.front.Parent
		r.Issues = p.front.Issues
		r.PullRequests = p.front.PullRequests
		r.ImplementationStatus = p.front.ImplementationStatus
		r.SupersededBy = p.front.SupersededBy
		r.StatusChecked = p.front.StatusChecked
		r.StatusNote = p.front.StatusNote
	}
	return r
}

// renderRecords prints plans in the json, yaml, or name format. With single
// set and exactly one plan the plan is printed on its own, kubectl style;
// otherwise the plans form a list under items.
func renderRecords(plans []*plan, format string, single bool) (string, error) {
	if format == "name" {
		var out strings.Builder
		for _, p := range plans {
			out.WriteString(p.basename + "\n")
		}
		return out.String(), nil
	}
	records := make([]planRecord, 0, len(plans))
	for _, p := range plans {
		records = append(records, newPlanRecord(p))
	}
	var value any = planList{Items: records}
	if single && len(records) == 1 {
		value = records[0]
	}
	var buf bytes.Buffer
	switch format {
	case "json":
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "    ")
		if err := enc.Encode(value); err != nil {
			return "", err
		}
	case "yaml":
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(value); err != nil {
			return "", err
		}
		if err := enc.Close(); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown output format %q", format)
	}
	return buf.String(), nil
}
