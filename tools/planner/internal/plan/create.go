package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DateLayout = "2006-01-02"

type ContentOptions struct {
	ID           string
	Status       string
	Note         string
	NoteSet      bool
	Issues       []string
	PullRequests []string
}

// links returns the URLs given for one list key.
func (o ContentOptions) links(key string) []string {
	if key == PullRequestsKey {
		return o.PullRequests
	}
	return o.Issues
}

func RenderContent(input []byte, parentValue string, opts ContentOptions, now time.Time) ([]byte, error) {
	first, _, _ := bytes.Cut(input, []byte("\n"))
	if strings.TrimRight(string(first), "\r") != "---" {
		return append(generatedFrontMatter(parentValue, opts, now), input...), nil
	}
	content, err := insertParent(input, parentValue)
	if err != nil {
		return nil, err
	}
	var idEdits []fieldEdit
	if opts.ID != "" {
		front, err := DecodeFrontMatter(content)
		if err != nil {
			return nil, fmt.Errorf("piped front matter: %w", err)
		}
		block, _, _, _ := splitFrontMatter(string(content))
		if declaresID(block) && front.ID != opts.ID {
			return nil, errors.New("piped front matter has a conflicting ID")
		}
		if front.ID == "" {
			idEdits = append(idEdits, fieldEdit{"ID", "ID: " + quoteYAML(opts.ID)})
		}
	}
	edits, err := linkEdits(content, opts)
	if err != nil {
		return nil, err
	}
	edits = append(idEdits, edits...)
	if opts.Status != "" || opts.NoteSet {
		edits = append(edits, statusEdits(opts.Status, opts.Note, opts.NoteSet, now.Format(DateLayout))...)
	}
	if len(edits) == 0 {
		return content, nil
	}
	return updateFrontMatter(content, edits)
}

// linkEdits merges the --issue and --pr values into the lists a piped front
// matter block already holds.
func linkEdits(content []byte, opts ContentOptions) ([]fieldEdit, error) {
	if len(opts.Issues)+len(opts.PullRequests) == 0 {
		return nil, nil
	}
	existing, err := DecodeFrontMatter(content)
	if err != nil {
		return nil, fmt.Errorf("piped front matter: %w", err)
	}
	var edits []fieldEdit
	for _, key := range []string{IssuesKey, PullRequestsKey} {
		if len(opts.links(key)) == 0 {
			continue
		}
		merged := MergeLinks(LinksFor(existing, key), opts.links(key))
		edits = append(edits, fieldEdit{key, listLines(key, merged)})
	}
	return edits, nil
}

func generatedFrontMatter(parentValue string, opts ContentOptions, now time.Time) []byte {
	status, note := "NotImplemented", "Implementation has not started."
	if opts.Status != "" {
		status = opts.Status
	}
	if opts.NoteSet {
		note = opts.Note
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	if opts.ID != "" {
		out.WriteString("ID: " + quoteYAML(opts.ID) + "\n")
	}
	out.WriteString("Type: plan\n")
	if parentValue != "" {
		out.WriteString(parentLine(parentValue) + "\n")
	}
	for _, key := range []string{IssuesKey, PullRequestsKey} {
		if values := MergeLinks(nil, opts.links(key)); len(values) > 0 {
			out.WriteString(listLines(key, values) + "\n")
		}
	}
	fmt.Fprintf(&out, "ImplementationStatus: %s\nStatusChecked: %s\nStatusNote: %s\n---\n\n", status, now.Format(DateLayout), quoteYAML(note))
	return out.Bytes()
}

func parentLine(value string) string {
	return "Parent: " + quoteYAML(value)
}

func quoteYAML(value string) string {
	quoted, _ := json.Marshal(value)
	return string(quoted)
}

// insertParent adds the parent line after the opening --- of a piped front
// matter block. A block that already carries Parent is rejected.
func insertParent(input []byte, parentValue string) ([]byte, error) {
	if parentValue == "" {
		return input, nil
	}
	lines := bytes.SplitAfter(input, []byte("\n"))
	for _, line := range lines[1:] {
		trimmed := strings.TrimRight(string(line), "\r\n")
		if trimmed == "---" {
			break
		}
		if strings.HasPrefix(trimmed, "Parent:") {
			return nil, errors.New("piped front matter already has a parent field")
		}
	}
	newline := "\n"
	if bytes.HasSuffix(lines[0], []byte("\r\n")) {
		newline = "\r\n"
	}
	var out bytes.Buffer
	out.Write(lines[0])
	out.WriteString(parentLine(parentValue) + newline)
	for _, line := range lines[1:] {
		out.Write(line)
	}
	return out.Bytes(), nil
}

// CanonicalReference validates a --parent or --superseded-by argument and
// returns the value to store: the wikilink of a plan under the root, a path
// as written or made absolute, or a URL as is.
func CanonicalReference(root, ref string) (string, error) {
	kind := ClassifyParent(ref)
	if kind == ParentURL {
		return ref, nil
	}
	c, err := Load(root)
	if err != nil {
		if _, statErr := os.Stat(root); !errors.Is(statErr, fs.ErrNotExist) {
			return "", err
		}
		c = NewCorpus(root)
	}
	if kind != ParentPath {
		p, err := Find(c, ref)
		if err != nil {
			return "", fmt.Errorf("parent: %w", err)
		}
		return "[[" + p.Identity + "]]", nil
	}
	expanded, err := ExpandHome(ref)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("parent path does not exist: %s", absolute)
		}
		return "", err
	}
	if p, ok := c.byPath[resolved]; ok {
		if matches := c.Lookup(p.Identity); len(matches) != 1 {
			return "", ambiguousReference(ref, matches)
		}
		return "[[" + p.Identity + "]]", nil
	}
	if strings.HasPrefix(ref, "~") || filepath.IsAbs(ref) {
		return ref, nil
	}
	return absolute, nil
}
