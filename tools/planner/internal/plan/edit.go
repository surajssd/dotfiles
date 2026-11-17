package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParentCycle returns the chain p would close by taking value as its parent:
// p, the new parent, its ancestors, and p again. It is nil when the value is
// not a wikilink or the plan is not among the new parent's ancestors.
func ParentCycle(c *Corpus, p *Plan, value string) []string {
	if ClassifyParent(value) != ParentWikilink {
		return nil
	}
	matches := c.Lookup(ReduceLinkTarget(value))
	if len(matches) != 1 {
		return nil
	}
	chain, _ := ParentChain(c, matches[0])
	for i, identity := range chain {
		if identity == p.Identity {
			return append([]string{p.Identity}, chain[:i+1]...)
		}
	}
	return nil
}

type Change struct {
	Status       string
	Checked      string
	Note         string
	NoteSet      bool
	SupersededBy string
	Parent       string
	Issues       []string
	PullRequests []string
	LogHeading   string
	LogBody      string
}

func EditFile(p *Plan, change Change) (FrontMatter, error) {
	edits := statusEdits(change.Status, change.Note, change.NoteSet, change.Checked)
	if change.SupersededBy != "" {
		edits = append(edits, fieldEdit{"SupersededBy", "SupersededBy: " + quoteYAML(change.SupersededBy)})
	}
	if change.Parent != "" {
		edits = append(edits, fieldEdit{"Parent", parentLine(change.Parent)})
	}
	front := p.Front
	front.Issues = MergeLinks(front.Issues, nil)
	front.PullRequests = MergeLinks(front.PullRequests, nil)
	for _, list := range []struct {
		key             string
		existing, added []string
	}{
		{IssuesKey, front.Issues, change.Issues},
		{PullRequestsKey, front.PullRequests, change.PullRequests},
	} {
		merged := MergeLinks(list.existing, list.added)
		if len(merged) > len(list.existing) {
			edits = append(edits, fieldEdit{list.key, listLines(list.key, merged)})
		}
	}
	if len(edits) == 0 && change.LogHeading == "" {
		return front, nil
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return FrontMatter{}, err
	}
	updated, err := updateFrontMatter(data, edits)
	switch {
	case errors.Is(err, errNoFrontMatter):
		updated = newFrontMatter(data, edits)
	case err != nil:
		return FrontMatter{}, fmt.Errorf("%s: %w", p.RelPath, err)
	}
	if change.LogHeading != "" {
		updated = appendLogEntry(updated, change.LogHeading, change.LogBody)
	}
	front, err = DecodeFrontMatter(updated)
	if err != nil {
		return FrontMatter{}, fmt.Errorf("%s: %w", p.RelPath, err)
	}
	if err := writePlanFile(p.Path, updated); err != nil {
		return FrontMatter{}, err
	}
	return front, nil
}

func writePlanFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".planner-*.md")
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// fieldEdit is one front matter key with its rendered lines, without the
// trailing newline; a list value spans several lines.
type fieldEdit struct {
	key  string
	text string
}

var errNoFrontMatter = errors.New("the plan has no front matter")

// statusEdits renders the status lines a status change touches.
func statusEdits(status, note string, noteSet bool, today string) []fieldEdit {
	var edits []fieldEdit
	if status != "" {
		edits = append(edits, fieldEdit{"ImplementationStatus", "ImplementationStatus: " + status})
	}
	if today != "" {
		edits = append(edits, fieldEdit{"StatusChecked", "StatusChecked: " + today})
	}
	if noteSet {
		edits = append(edits, fieldEdit{"StatusNote", "StatusNote: " + quoteYAML(note)})
	}
	return edits
}

func lineEnding(lines []string) string {
	if len(lines) > 0 && strings.HasSuffix(lines[0], "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// YAML key positions keep quoted keys and blank lines inside scalar values
// from being mistaken for field boundaries. Unedited text stays byte-for-byte.
func updateFrontMatter(data []byte, edits []fieldEdit) ([]byte, error) {
	block, _, hasFront, err := splitFrontMatter(string(data))
	if err != nil {
		return nil, err
	}
	if !hasFront {
		return nil, errNoFrontMatter
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(block), &document); err != nil {
		return nil, err
	}
	var fields []*yaml.Node
	indent := ""
	if len(document.Content) > 0 {
		mapping := document.Content[0]
		if mapping.Kind != yaml.MappingNode || mapping.Style&yaml.FlowStyle != 0 {
			return nil, errors.New("front matter must use a block mapping for in-place updates")
		}
		fields = mapping.Content
		if len(fields) > 0 {
			indent = strings.Repeat(" ", fields[0].Column-1)
		}
	}
	lines := strings.SplitAfter(string(data), "\n")
	eol := lineEnding(lines)
	closing := strings.Count(block, "\n") + 1
	byKey := map[string]fieldEdit{}
	for _, edit := range edits {
		byKey[edit.key] = edit
	}
	out := []string{lines[0]}
	seen := map[string]bool{}
	cursor := 1
	for i := 0; i < len(fields); i += 2 {
		key := fields[i]
		edit, ok := byKey[key.Value]
		if !ok {
			continue
		}
		end := closing
		if i+2 < len(fields) {
			end = fields[i+2].Line
		}
		for end > key.Line+1 {
			line := strings.TrimRight(lines[end-1], "\r\n")
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, indent+"#") {
				break
			}
			end--
		}
		out = append(out, lines[cursor:key.Line]...)
		out = append(out, renderEdit(edit, eol, indent))
		cursor = end
		seen[key.Value] = true
	}
	out = append(out, lines[cursor:closing]...)
	for _, edit := range edits {
		if !seen[edit.key] {
			out = append(out, renderEdit(edit, eol, indent))
		}
	}
	out = append(out, lines[closing:]...)
	updated := []byte(strings.Join(out, ""))
	if _, err := DecodeFrontMatter(updated); err != nil {
		return nil, fmt.Errorf("updated front matter does not decode: %w", err)
	}
	return updated, nil
}

func renderEdit(edit fieldEdit, eol, indent string) string {
	return indent + strings.ReplaceAll(edit.text, "\n", eol+indent) + eol
}

// newFrontMatter puts a fresh block holding Type: plan and the edits in front
// of a file that has none, using the file's own line endings.
func newFrontMatter(data []byte, edits []fieldEdit) []byte {
	eol := lineEnding(strings.SplitAfter(string(data), "\n"))
	lines := []string{"---", "Type: plan"}
	for _, edit := range edits {
		lines = append(lines, edit.text)
	}
	lines = append(lines, "---", "")
	block := strings.Join(lines, eol) + eol
	return append([]byte(block), data...)
}
