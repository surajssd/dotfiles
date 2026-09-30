package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type frontMatter struct {
	Type                 string `yaml:"type"`
	Parent               string `yaml:"parent"`
	ImplementationStatus string `yaml:"implementation_status"`
	StatusChecked        string `yaml:"status_checked"`
	StatusNote           string `yaml:"status_note"`
}

type plan struct {
	path     string
	relPath  string
	repo     string
	basename string
	name     string
	title    string
	hasTitle bool
	front    frontMatter
	hasFront bool
	frontErr error
	links    []string
}

// valid reports whether the plan has a front matter block that decoded.
func (p *plan) valid() bool {
	return p.hasFront && p.frontErr == nil
}

func (p *plan) active() bool {
	status := p.front.ImplementationStatus
	return status != "Implemented" && status != "Superseded"
}

type corpus struct {
	root       string
	plans      []*plan
	byBasename map[string][]*plan
	byPath     map[string]*plan
}

func (c *corpus) lookup(basename string) []*plan {
	return c.byBasename[basename]
}

var planPrefixRE = regexp.MustCompile(`^[0-9]{12}-`)

func isPlanStyle(basename string) bool {
	return planPrefixRE.MatchString(basename) && len(basename) > 13
}

func shortName(basename string) string {
	if isPlanStyle(basename) {
		return basename[13:]
	}
	return basename
}

func loadCorpus(root string) (*corpus, error) {
	info, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("plan root does not exist: %s", root)
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("plan root is not a directory: %s", root)
	}
	c := &corpus{root: root, byBasename: map[string][]*plan{}, byPath: map[string]*plan{}}
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filePath == root {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 || !strings.HasSuffix(name, ".md") {
			return nil
		}
		p, err := loadPlan(root, filePath)
		if err != nil {
			return err
		}
		c.plans = append(c.plans, p)
		c.byBasename[p.basename] = append(c.byBasename[p.basename], p)
		c.byPath[p.path] = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(c.plans, func(i, j int) bool { return c.plans[i].relPath < c.plans[j].relPath })
	return c, nil
}

func loadPlan(root, filePath string) (*plan, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return nil, err
	}
	relSlash := filepath.ToSlash(rel)
	p := &plan{
		path:     filePath,
		relPath:  relSlash,
		repo:     repoLabel(path.Dir(relSlash)),
		basename: strings.TrimSuffix(path.Base(relSlash), ".md"),
	}
	p.name = shortName(p.basename)
	block, body, hasFront, frontErr := splitFrontMatter(string(data))
	p.hasFront = hasFront
	p.frontErr = frontErr
	if hasFront && frontErr == nil {
		if err := yaml.Unmarshal([]byte(block), &p.front); err != nil {
			p.frontErr = err
		}
	}
	p.title, p.hasTitle, p.links = scanBody(body)
	if !p.hasTitle {
		p.title = p.name
	}
	return p, nil
}

func repoLabel(dir string) string {
	if dir == "." || dir == "github.com" {
		return ""
	}
	return strings.TrimPrefix(dir, "github.com/")
}

// splitFrontMatter separates a leading --- block from the body. A trailing CR
// on the delimiter lines is tolerated.
func splitFrontMatter(text string) (block, body string, hasFront bool, err error) {
	lines := strings.SplitAfter(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r\n") != "---" {
		return "", text, false, nil
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r\n") == "---" {
			return strings.Join(lines[1:i], ""), strings.Join(lines[i+1:], ""), true, nil
		}
	}
	return "", strings.Join(lines[1:], ""), true, errors.New("front matter has no closing ---")
}

var (
	fenceRE    = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	h1RE       = regexp.MustCompile(`^#[ \t]+(.+?)[ \t]*$`)
	codeSpanRE = regexp.MustCompile("`[^`]*`")
	mdLinkRE   = regexp.MustCompile(`\]\(\s*([^)\s]+)[^)]*\)`)
	wikilinkRE = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
)

// scanBody returns the first H1 and every link target outside fenced code
// blocks and inline code spans. Wikilinks keep their brackets so a report can
// show them as written.
func scanBody(body string) (title string, hasTitle bool, links []string) {
	inFence := false
	fence := ""
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := fenceRE.FindStringSubmatch(line); m != nil {
			if !inFence {
				inFence, fence = true, m[1]
				continue
			}
			if m[1][0] == fence[0] && len(m[1]) >= len(fence) {
				inFence = false
				continue
			}
		}
		if inFence {
			continue
		}
		if !hasTitle {
			if m := h1RE.FindStringSubmatch(line); m != nil {
				title, hasTitle = m[1], true
			}
		}
		stripped := codeSpanRE.ReplaceAllString(line, "")
		for _, m := range mdLinkRE.FindAllStringSubmatch(stripped, -1) {
			links = append(links, m[1])
		}
		for _, m := range wikilinkRE.FindAllStringSubmatch(stripped, -1) {
			if strings.TrimSpace(m[1]) != "" {
				links = append(links, "[["+m[1]+"]]")
			}
		}
	}
	return title, hasTitle, links
}
