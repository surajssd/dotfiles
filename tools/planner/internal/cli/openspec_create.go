package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/surajssd/dotfiles/tools/planner/internal/github"
	"github.com/surajssd/dotfiles/tools/planner/internal/openspec"
	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
)

func createOpenSpec(ctx context.Context, deps dependencies, root, name string, opts createOptions, now time.Time) error {
	creator, err := openspec.NewCreator(name)
	if err != nil {
		return err
	}
	repo, err := github.ResolveRepository(deps.stderr)
	if err != nil {
		return err
	}
	c, err := plan.Load(root)
	if err != nil {
		if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
			return err
		}
		c = plan.NewCorpus(root)
	}
	store := filepath.Join(c.Root, "github.com", filepath.FromSlash(repo))
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
	for _, p := range c.Plans {
		if p.Directory == "" {
			continue
		}
		rel, err := filepath.Rel(p.Directory, target)
		if err == nil && filepath.IsLocal(rel) {
			return fmt.Errorf("change directory is inside managed plan %s", p.Directory)
		}
	}
	opts.ID = now.Format(timestampLayout) + "-" + name
	if len(c.Lookup(opts.ID)) > 0 {
		return fmt.Errorf("plan ID already exists: %s", opts.ID)
	}
	parent := ""
	if opts.parent != "" {
		parent, err = plan.CanonicalReference(root, opts.parent)
		if err != nil {
			return err
		}
	}
	content, err := planContent(deps, parent, opts, now)
	if err != nil {
		return err
	}
	target, err = creator.Create(ctx, c.Root, repo)
	if err != nil {
		return err
	}
	uncertain := func(err error) error {
		return fmt.Errorf("OpenSpec change may exist at %s; inspect it before retrying: %w", target, err)
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
