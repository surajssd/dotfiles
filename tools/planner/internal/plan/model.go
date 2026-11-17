package plan

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

type FrontMatter struct {
	ID                   string   `yaml:"ID"`
	Type                 string   `yaml:"Type"`
	Parent               string   `yaml:"Parent"`
	Issues               []string `yaml:"Issues"`
	PullRequests         []string `yaml:"PullRequests"`
	ImplementationStatus string   `yaml:"ImplementationStatus"`
	SupersededBy         string   `yaml:"SupersededBy"`
	StatusChecked        string   `yaml:"StatusChecked"`
	StatusNote           string   `yaml:"StatusNote"`
}

// knownKeys are the front matter keys the schema defines; check reports every
// other key as an advisory.
var knownKeys = map[string]bool{
	"ID":                   true,
	"Type":                 true,
	"Parent":               true,
	"Issues":               true,
	"PullRequests":         true,
	"ImplementationStatus": true,
	"SupersededBy":         true,
	"StatusChecked":        true,
	"StatusNote":           true,
}

type Plan struct {
	Path        string
	Directory   string
	Identity    string
	RelPath     string
	Repo        string
	Basename    string
	Name        string
	Title       string
	hasTitle    bool
	Front       FrontMatter
	HasFront    bool
	FrontErr    error
	unknownKeys []string
	links       []string
	noteLinks   []string
}

// Valid reports whether the plan has a front matter block that decoded.
func (p *Plan) Valid() bool {
	return p.HasFront && p.FrontErr == nil
}

func (p *Plan) Active() bool {
	status := p.Front.ImplementationStatus
	return status != "Implemented" && status != "Superseded" && status != "Abandoned"
}

// linksTo reports whether the front matter names the URL as parent,
// successor, issue, or pull request.
func (p *Plan) linksTo(url string) bool {
	url = strings.TrimSpace(url)
	values := []string{p.Front.Parent, p.Front.SupersededBy}
	values = append(values, p.Front.Issues...)
	values = append(values, p.Front.PullRequests...)
	for _, value := range values {
		if strings.TrimSpace(value) == url {
			return true
		}
	}
	return false
}

func (p *Plan) HasPullRequest(url string) bool {
	if !p.Valid() {
		return false
	}
	for _, value := range p.Front.PullRequests {
		if strings.TrimSpace(value) == url {
			return true
		}
	}
	return false
}

type Corpus struct {
	Root       string
	Plans      []*Plan
	byIdentity map[string][]*Plan
	byPath     map[string]*Plan
}

func NewCorpus(root string) *Corpus {
	return &Corpus{Root: root, byIdentity: map[string][]*Plan{}, byPath: map[string]*Plan{}}
}

func (c *Corpus) Lookup(identity string) []*Plan {
	return c.byIdentity[identity]
}

var planPrefixRE = regexp.MustCompile(`^[0-9]{12}-`)

var directoryIDRE = regexp.MustCompile(`^[0-9]{12}-[^\s/\\#|\[\]]+$`)

var idKeyRE = regexp.MustCompile(`(?m)^[ \t]*['"]?ID['"]?[ \t]*:`)

func declaresID(block string) bool {
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(block), &fields); err == nil {
		_, present := fields["ID"]
		return present
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(block), &document); err == nil && len(document.Content) > 0 {
		mapping := document.Content[0]
		if mapping.Kind == yaml.MappingNode {
			for i := 0; i < len(mapping.Content); i += 2 {
				if mapping.Content[i].Value == "ID" {
					return true
				}
			}
		}
	}
	return idKeyRE.MatchString(block)
}

func validDirectoryID(id string) bool {
	return directoryIDRE.MatchString(id) && !strings.HasSuffix(id, ".md")
}

func isPlanStyle(basename string) bool {
	return planPrefixRE.MatchString(basename) && len(basename) > 13
}

func shortName(basename string) string {
	if isPlanStyle(basename) {
		return basename[13:]
	}
	return basename
}

func openSpecLocation(rel string) (inside, tracking bool) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 4 || parts[0] != "github.com" || parts[3] != "openspec" {
		return false, false
	}
	tail := parts[4:]
	tracking = len(tail) == 3 && tail[0] == "changes" && tail[1] != "archive" && tail[2] == "plan.md"
	tracking = tracking || len(tail) == 4 && tail[0] == "changes" && tail[1] == "archive" && tail[3] == "plan.md"
	return true, tracking
}

func Load(root string) (*Corpus, error) {
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
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	c := NewCorpus(root)
	add := func(p *Plan) {
		c.Plans = append(c.Plans, p)
		c.byIdentity[p.Identity] = append(c.byIdentity[p.Identity], p)
		c.byPath[p.Path] = p
		if p.Directory != "" {
			c.byPath[p.Directory] = p
		}
	}
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if filePath != root && strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		inside, tracking := openSpecLocation(rel)
		if inside {
			if !entry.IsDir() && !tracking {
				return nil
			}
			if entry.IsDir() {
				_, tracksDirectory := openSpecLocation(filepath.Join(rel, "plan.md"))
				structural := len(strings.Split(filepath.ToSlash(rel), "/")) == 4 || name == "changes" || name == "archive"
				if !tracksDirectory && !structural {
					return fs.SkipDir
				}
			}
		}
		if entry.IsDir() {
			metadata := filepath.Join(filePath, "plan.md")
			info, err := os.Lstat(metadata)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err == nil && info.Mode().IsRegular() {
				p, err := LoadFile(root, metadata)
				if err != nil {
					return err
				}
				if p.Directory != "" {
					add(p)
					return fs.SkipDir
				}
			}
			if inside {
				_, tracksDirectory := openSpecLocation(filepath.Join(rel, "plan.md"))
				if tracksDirectory {
					return fs.SkipDir
				}
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || !strings.HasSuffix(name, ".md") {
			return nil
		}
		p, err := LoadFile(root, filePath)
		if err != nil {
			return err
		}
		add(p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(c.Plans, func(i, j int) bool { return c.Plans[i].RelPath < c.Plans[j].RelPath })
	return c, nil
}

func LoadFile(root, filePath string) (*Plan, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return nil, err
	}
	relSlash := filepath.ToSlash(rel)
	p := &Plan{
		Path:     filePath,
		RelPath:  relSlash,
		Repo:     repoLabel(path.Dir(relSlash)),
		Basename: strings.TrimSuffix(path.Base(relSlash), ".md"),
	}
	p.Identity = p.Basename
	block, body, hasFront, frontErr := splitFrontMatter(string(data))
	p.HasFront = hasFront
	p.FrontErr = frontErr
	if hasFront && frontErr == nil {
		if err := yaml.Unmarshal([]byte(block), &p.Front); err != nil {
			p.FrontErr = err
		} else {
			p.unknownKeys = unknownFrontMatterKeys(block)
			p.noteLinks = scanLinks(p.Front.StatusNote)
		}
	}
	tracking := declaresID(block)
	if frontErr != nil {
		tracking = idKeyRE.Match(data)
	}
	inside, reserved := openSpecLocation(rel)
	if p.Basename == "plan" && tracking && !inside || reserved {
		p.Directory = filepath.Dir(filePath)
		if p.Front.ID != "" {
			p.Identity = p.Front.ID
		} else {
			p.Identity = p.RelPath
		}
		parts := strings.Split(relSlash, "/")
		if len(parts) > 4 && parts[0] == "github.com" {
			p.Repo = strings.Join(parts[1:3], "/")
		}
	}
	p.Name = shortName(p.Identity)
	p.Title, p.hasTitle, p.links = scanBody(body)
	if !p.hasTitle {
		p.Title = p.Name
	}
	return p, nil
}

// DecodeFrontMatter reads the front matter block at the start of a file.
func DecodeFrontMatter(data []byte) (FrontMatter, error) {
	var front FrontMatter
	block, _, hasFront, err := splitFrontMatter(string(data))
	if err != nil {
		return front, err
	}
	if !hasFront {
		return front, errNoFrontMatter
	}
	err = yaml.Unmarshal([]byte(block), &front)
	return front, err
}

// unknownFrontMatterKeys lists the top-level keys of a decoded block that the
// schema does not define, sorted.
func unknownFrontMatterKeys(block string) []string {
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(block), &fields); err != nil {
		return nil
	}
	var unknown []string
	for key := range fields {
		if !knownKeys[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
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
		links = append(links, lineLinks(line)...)
	}
	return title, hasTitle, links
}

// scanLinks returns the link targets in text that has no fenced blocks, such
// as a status note.
func scanLinks(text string) []string {
	var links []string
	for _, line := range strings.Split(text, "\n") {
		links = append(links, lineLinks(line)...)
	}
	return links
}

func lineLinks(line string) []string {
	var links []string
	stripped := codeSpanRE.ReplaceAllString(line, "")
	for _, m := range mdLinkRE.FindAllStringSubmatch(stripped, -1) {
		links = append(links, m[1])
	}
	for _, m := range wikilinkRE.FindAllStringSubmatch(stripped, -1) {
		if strings.TrimSpace(m[1]) != "" {
			links = append(links, "[["+m[1]+"]]")
		}
	}
	return links
}
