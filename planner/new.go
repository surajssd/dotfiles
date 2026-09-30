package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	timestampLayout = "060102150405"
	dateLayout      = "2006-01-02"
	ghTimeout       = 10 * time.Second
)

type newOptions struct {
	parent       string
	status       string
	note         string
	noteSet      bool
	issues       []string
	pullRequests []string
}

// links returns the URLs given for one list key.
func (o newOptions) links(key string) []string {
	if key == pullRequestsKey {
		return o.pullRequests
	}
	return o.issues
}

func runNew(deps dependencies, root string, args []string, opts newOptions) error {
	name, err := planName(args)
	if err != nil {
		return err
	}
	if opts.status != "" {
		status, ok := canonicalStatus(opts.status)
		if !ok {
			return fmt.Errorf("unknown status %q; use one of %s", opts.status, strings.Join(statusValues, ", "))
		}
		if !opts.noteSet {
			return errors.New("--status needs --note: say what is done and what remains")
		}
		opts.status = status
	}
	for _, key := range []string{issuesKey, pullRequestsKey} {
		for _, value := range opts.links(key) {
			if problem := linkProblem(key, value); problem != "" {
				return errors.New(problem)
			}
		}
	}
	repo, err := resolveRepo(deps)
	if err != nil {
		return err
	}
	parentValue := ""
	if opts.parent != "" {
		parentValue, err = canonicalReference(root, opts.parent)
		if err != nil {
			return err
		}
	}
	now := deps.now()
	dir := filepath.Join(root, "github.com", filepath.FromSlash(repo))
	planPath := filepath.Join(dir, now.Format(timestampLayout)+"-"+name+".md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(planPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("plan already exists: %s", planPath)
	}
	if err != nil {
		return err
	}
	content, err := planContent(deps, parentValue, opts, now)
	if err != nil {
		return removeCreated(file, planPath, err)
	}
	if _, err := file.Write(content); err != nil {
		return removeCreated(file, planPath, err)
	}
	if err := file.Close(); err != nil {
		return removeCreated(nil, planPath, err)
	}
	return writeOutput(deps.stdout, planPath+"\n")
}

// removeCreated undoes the exclusive create after a later failure. Only the
// file this invocation created is removed.
func removeCreated(file *os.File, planPath string, cause error) error {
	if file != nil {
		_ = file.Close()
	}
	if err := os.Remove(planPath); err != nil {
		return fmt.Errorf("%w; removing %s also failed: %v", cause, planPath, err)
	}
	return cause
}

func planName(args []string) (string, error) {
	words := strings.Fields(strings.Join(args, " "))
	if n := len(words); n > 0 {
		last := strings.TrimSuffix(words[n-1], ".md")
		if last == "" {
			words = words[:n-1]
		} else {
			words[n-1] = last
		}
	}
	name := strings.Join(words, "-")
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", errors.New("provide a nonempty plan name without path separators")
	}
	return name, nil
}

func planContent(deps dependencies, parentValue string, opts newOptions, now time.Time) ([]byte, error) {
	if deps.stdinIsTerminal() {
		return generatedFrontMatter(parentValue, opts, now), nil
	}
	input, err := io.ReadAll(deps.stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	first, _, _ := bytes.Cut(input, []byte("\n"))
	if strings.TrimRight(string(first), "\r") != "---" {
		return append(generatedFrontMatter(parentValue, opts, now), input...), nil
	}
	content, err := insertParent(input, parentValue)
	if err != nil {
		return nil, err
	}
	edits, err := linkEdits(content, opts)
	if err != nil {
		return nil, err
	}
	if opts.status != "" || opts.noteSet {
		edits = append(edits, statusEdits(opts.status, opts.note, opts.noteSet, now.Format(dateLayout))...)
	}
	if len(edits) == 0 {
		return content, nil
	}
	return updateFrontMatter(content, edits)
}

// linkEdits merges the --issue and --pr values into the lists a piped front
// matter block already holds.
func linkEdits(content []byte, opts newOptions) ([]fieldEdit, error) {
	if len(opts.issues)+len(opts.pullRequests) == 0 {
		return nil, nil
	}
	existing, err := decodeFrontMatter(content)
	if err != nil {
		return nil, fmt.Errorf("piped front matter: %w", err)
	}
	var edits []fieldEdit
	for _, key := range []string{issuesKey, pullRequestsKey} {
		if len(opts.links(key)) == 0 {
			continue
		}
		merged := mergeLinks(linksFor(existing, key), opts.links(key))
		edits = append(edits, fieldEdit{key, listLines(key, merged)})
	}
	return edits, nil
}

func generatedFrontMatter(parentValue string, opts newOptions, now time.Time) []byte {
	status, note := "NotImplemented", "Implementation has not started."
	if opts.status != "" {
		status = opts.status
	}
	if opts.noteSet {
		note = opts.note
	}
	var out bytes.Buffer
	out.WriteString("---\ntype: plan\n")
	if parentValue != "" {
		out.WriteString(parentLine(parentValue) + "\n")
	}
	for _, key := range []string{issuesKey, pullRequestsKey} {
		if values := mergeLinks(nil, opts.links(key)); len(values) > 0 {
			out.WriteString(listLines(key, values) + "\n")
		}
	}
	fmt.Fprintf(&out, "implementation_status: %s\nstatus_checked: %s\nstatus_note: %s\n---\n\n", status, now.Format(dateLayout), quoteYAML(note))
	return out.Bytes()
}

func parentLine(value string) string {
	return "parent: " + quoteYAML(value)
}

func quoteYAML(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

// insertParent adds the parent line after the opening --- of a piped front
// matter block. A block that already carries parent is rejected.
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
		if strings.HasPrefix(trimmed, "parent:") {
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

// canonicalReference validates a --parent or --superseded-by argument and
// returns the value to store: the wikilink of a plan under the root, a path
// as written or made absolute, or a URL as is.
func canonicalReference(root, ref string) (string, error) {
	kind := classifyParent(ref)
	if kind == parentURL {
		return ref, nil
	}
	c, err := loadCorpus(root)
	if err != nil {
		if _, statErr := os.Stat(root); !errors.Is(statErr, fs.ErrNotExist) {
			return "", err
		}
		c = &corpus{root: root, byBasename: map[string][]*plan{}, byPath: map[string]*plan{}}
	}
	if kind != parentPath {
		p, err := findPlan(c, ref)
		if err != nil {
			return "", fmt.Errorf("parent: %w", err)
		}
		return "[[" + p.basename + "]]", nil
	}
	expanded, err := expandHome(ref)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(absolute); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("parent path does not exist: %s", absolute)
		}
		return "", err
	}
	if p, ok := c.byPath[absolute]; ok {
		return "[[" + p.basename + "]]", nil
	}
	if strings.HasPrefix(ref, "~") || filepath.IsAbs(ref) {
		return ref, nil
	}
	return absolute, nil
}

var (
	sshURLRE = regexp.MustCompile(`^ssh://(?:[^/@\s]+@)?github\.com(?::[0-9]+)?/(.+)$`)
	scpURLRE = regexp.MustCompile(`^[^/@:\s]+@github\.com:(.+)$`)
	repoRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
)

func validGithubRepo(repo string) bool {
	if !repoRE.MatchString(repo) {
		return false
	}
	name := repo[strings.LastIndex(repo, "/")+1:]
	return name != "." && name != ".."
}

func githubRepoFromURL(url string) (string, bool) {
	url = strings.TrimSpace(url)
	var repo string
	switch {
	case strings.HasPrefix(url, "https://github.com/"):
		repo = strings.TrimPrefix(url, "https://github.com/")
	case sshURLRE.MatchString(url):
		repo = sshURLRE.FindStringSubmatch(url)[1]
	case scpURLRE.MatchString(url):
		repo = scpURLRE.FindStringSubmatch(url)[1]
	default:
		return "", false
	}
	repo = strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
	if !validGithubRepo(repo) {
		return "", false
	}
	return repo, true
}

// resolveRepo identifies the GitHub repository of the current checkout: the
// upstream remote, then the origin remote mapped through gh to its fork
// parent, then origin itself with a warning.
func resolveRepo(deps dependencies) (string, error) {
	inside, err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return "", errors.New("run planner new from a Git checkout or worktree")
	}
	if url, err := exec.Command("git", "remote", "get-url", "upstream").Output(); err == nil {
		if repo, ok := githubRepoFromURL(string(url)); ok {
			return repo, nil
		}
	}
	url, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", errors.New("could not resolve a github.com repository from the upstream or origin remote")
	}
	origin, ok := githubRepoFromURL(string(url))
	if !ok {
		return "", errors.New("could not resolve a github.com repository from the upstream or origin remote")
	}
	if repo, ok := lookupForkParent(origin); ok {
		return repo, nil
	}
	_, _ = fmt.Fprintf(deps.stderr, "warning: GitHub lookup failed or is unavailable; using origin (%s); fork parent could not be verified\n", origin)
	return origin, nil
}

func lookupForkParent(origin string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "https://github.com/"+origin, "--json", "isFork,parent,nameWithOwner")
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	var view struct {
		IsFork *bool `json:"isFork"`
		Parent struct {
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
			Name string `json:"name"`
		} `json:"parent"`
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(output, &view); err != nil || view.IsFork == nil {
		return "", false
	}
	repo := view.NameWithOwner
	if *view.IsFork {
		repo = view.Parent.Owner.Login + "/" + view.Parent.Name
	}
	if !validGithubRepo(repo) {
		return "", false
	}
	return repo, true
}
