package openspec

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

var nameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type Creator struct {
	binary string
	name   string
}

func NewCreator(name string) (*Creator, error) {
	if !nameRE.MatchString(name) {
		return nil, errors.New("OpenSpec change names require lowercase letters, numbers, and single hyphens; use --no-openspec for a standalone Markdown plan")
	}
	binary, err := exec.LookPath("openspec")
	if err != nil {
		return nil, fmt.Errorf("OpenSpec command not found on PATH; install openspec or use --no-openspec: %w", err)
	}
	return &Creator{binary: binary, name: name}, nil
}

func (c *Creator) Create(ctx context.Context, root, repo string) (string, error) {
	store, err := prepare(ctx, c.binary, root, repo)
	if err != nil {
		return "", err
	}
	target := filepath.Join(store, "openspec", "changes", c.name)
	probe, err := invoke(ctx, c.binary, store, "list", "--json")
	if err != nil {
		return "", err
	}
	var listing struct {
		Root struct {
			Path string `json:"path"`
		} `json:"root"`
	}
	if err := json.Unmarshal(probe, &listing); err != nil {
		return "", fmt.Errorf("read OpenSpec root: %w", err)
	}
	if listing.Root.Path != store {
		return "", fmt.Errorf("OpenSpec selected root %q instead of %s", listing.Root.Path, store)
	}
	output, err := invoke(ctx, c.binary, store, "new", "change", c.name, "--json")
	uncertain := func(err error) error {
		return fmt.Errorf("OpenSpec change may exist at %s; inspect it before retrying: %w", target, err)
	}
	if err != nil {
		return "", uncertain(err)
	}
	var created struct {
		Change struct {
			Path string `json:"path"`
		} `json:"change"`
	}
	if err := json.Unmarshal(output, &created); err != nil {
		return "", uncertain(fmt.Errorf("read OpenSpec creation result: %w", err))
	}
	if created.Change.Path != target {
		return "", uncertain(fmt.Errorf("OpenSpec reported unexpected change path %q", created.Change.Path))
	}
	info, err := os.Lstat(target)
	if err != nil {
		return "", uncertain(err)
	}
	if !info.IsDir() {
		return "", uncertain(fmt.Errorf("OpenSpec change is not a non-symlinked directory: %s", target))
	}
	return target, nil
}

func prepare(ctx context.Context, binary, root, repo string) (string, error) {
	missing, err := treeMissing(root, repo)
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
		if _, err := invoke(ctx, binary, store, "init", "--tools", "none", "--no-animation"); err != nil {
			return "", fmt.Errorf("OpenSpec initialization failed at %s; partial setup may remain, inspect it before retrying: %w", store, err)
		}
		if stillMissing, err := treeMissing(root, repo); err != nil {
			return "", err
		} else if stillMissing {
			return "", fmt.Errorf("OpenSpec initialization did not create openspec/changes at %s", store)
		}
	}
	return store, nil
}

func treeMissing(root, repo string) (bool, error) {
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

func invoke(ctx context.Context, binary, dir string, args ...string) ([]byte, error) {
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
