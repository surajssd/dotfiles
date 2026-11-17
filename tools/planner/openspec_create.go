package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var openSpecNameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func createOpenSpec(ctx context.Context, deps dependencies, root, name string, opts createOptions, now time.Time) error {
	if !openSpecNameRE.MatchString(name) {
		return errors.New("OpenSpec change names require lowercase letters, numbers, and single hyphens; use --no-openspec for a standalone Markdown plan")
	}
	binary, err := exec.LookPath("openspec")
	if err != nil {
		return fmt.Errorf("OpenSpec command not found on PATH; install openspec or use --no-openspec: %w", err)
	}
	repo, err := resolveRepo(deps)
	if err != nil {
		return err
	}
	c, err := loadCorpus(root)
	if err != nil {
		if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
			return err
		}
		c = &corpus{root: root, byIdentity: map[string][]*plan{}, byPath: map[string]*plan{}}
	}
	store := filepath.Join(c.root, "github.com", filepath.FromSlash(repo))
	changes := filepath.Join(store, "openspec", "changes")
	target := filepath.Join(changes, name)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("OpenSpec change already exists: %s; choose an unused change name", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if strings.HasPrefix(filepath.Base(store), ".") {
		return errors.New("repository name must not start with a dot: hidden directories are not discoverable")
	}
	for _, p := range c.plans {
		if p.directory == "" {
			continue
		}
		rel, err := filepath.Rel(p.directory, target)
		if err == nil && filepath.IsLocal(rel) {
			return fmt.Errorf("change directory is inside managed plan %s", p.directory)
		}
	}
	opts.id = now.Format(timestampLayout) + "-" + name
	if len(c.lookup(opts.id)) > 0 {
		return fmt.Errorf("plan ID already exists: %s", opts.id)
	}
	parent := ""
	if opts.parent != "" {
		parent, err = canonicalReference(root, opts.parent)
		if err != nil {
			return err
		}
	}
	content, err := planContent(deps, parent, opts, now)
	if err != nil {
		return err
	}
	store, err = prepareOpenSpec(ctx, binary, c.root, repo)
	if err != nil {
		return err
	}
	target = filepath.Join(store, "openspec", "changes", name)
	probe, err := invokeOpenSpec(ctx, binary, store, "list", "--json")
	if err != nil {
		return err
	}
	var listing struct {
		Root struct {
			Path string `json:"path"`
		} `json:"root"`
	}
	if err := json.Unmarshal(probe, &listing); err != nil {
		return fmt.Errorf("read OpenSpec root: %w", err)
	}
	if listing.Root.Path != store {
		return fmt.Errorf("OpenSpec selected root %q instead of %s", listing.Root.Path, store)
	}
	output, err := invokeOpenSpec(ctx, binary, store, "new", "change", name, "--json")
	uncertain := func(err error) error {
		return fmt.Errorf("OpenSpec change may exist at %s; inspect it before retrying: %w", target, err)
	}
	if err != nil {
		return uncertain(err)
	}
	var created struct {
		Change struct {
			Path string `json:"path"`
		} `json:"change"`
	}
	if err := json.Unmarshal(output, &created); err != nil {
		return uncertain(fmt.Errorf("read OpenSpec creation result: %w", err))
	}
	if created.Change.Path != target {
		return uncertain(fmt.Errorf("OpenSpec reported unexpected change path %q", created.Change.Path))
	}
	info, err := os.Lstat(target)
	if err != nil {
		return uncertain(err)
	}
	if !info.IsDir() {
		return uncertain(fmt.Errorf("OpenSpec change is not a non-symlinked directory: %s", target))
	}
	metadata := filepath.Join(target, "plan.md")
	file, err := os.OpenFile(metadata, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return uncertain(err)
	}
	if _, err := file.Write(content); err != nil {
		return uncertain(removeCreated(file, metadata, err))
	}
	if err := file.Close(); err != nil {
		return uncertain(removeCreated(nil, metadata, err))
	}
	return writeOutput(deps.stdout, metadata+"\n")
}

func prepareOpenSpec(ctx context.Context, binary, root, repo string) (string, error) {
	missing, err := openSpecTreeMissing(root, repo)
	if err != nil {
		return "", err
	}
	store := filepath.Join(root, "github.com", filepath.FromSlash(repo))
	if missing {
		if err := os.MkdirAll(store, 0o755); err != nil {
			return "", err
		}
	}
	store, err = filepath.EvalSymlinks(store)
	if err != nil {
		return "", err
	}
	if missing {
		if _, err := invokeOpenSpec(ctx, binary, store, "init", "--tools", "none", "--no-animation"); err != nil {
			return "", fmt.Errorf("OpenSpec initialization failed at %s; partial setup may remain, inspect it before retrying: %w", store, err)
		}
		if stillMissing, err := openSpecTreeMissing(root, repo); err != nil {
			return "", err
		} else if stillMissing {
			return "", fmt.Errorf("OpenSpec initialization did not create openspec/changes at %s", store)
		}
	}
	return store, nil
}

func openSpecTreeMissing(root, repo string) (bool, error) {
	current := root
	for _, part := range strings.Split("github.com/"+repo+"/openspec/changes", "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() {
			return false, fmt.Errorf("OpenSpec path must be a non-symlinked directory: %s", current)
		}
	}
	return false, nil
}

func invokeOpenSpec(ctx context.Context, binary, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("openspec %s: %w: %s", strings.Join(args, " "), errors.Join(err, ctx.Err()), strings.TrimSpace(stderr.String()+string(output)))
	}
	return output, nil
}
