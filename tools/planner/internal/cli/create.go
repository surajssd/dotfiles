package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/surajssd/dotfiles/tools/planner/internal/github"
	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
)

const timestampLayout = "060102150405"

type createOptions struct {
	noOpenSpec bool
	parent     string
	plan.ContentOptions
}

func runCreate(ctx context.Context, deps dependencies, root string, args []string, opts createOptions) error {
	name, err := planName(args)
	if err != nil {
		return err
	}
	if opts.Status != "" {
		status, ok := plan.CanonicalStatus(opts.Status)
		if !ok {
			return fmt.Errorf("unknown status %q; use one of %s", opts.Status, strings.Join(plan.StatusValues(), ", "))
		}
		if !opts.NoteSet {
			return errors.New("--status needs --note: say what is done and what remains")
		}
		opts.Status = status
	}
	for _, list := range []struct {
		key    string
		values []string
	}{
		{plan.IssuesKey, opts.Issues}, {plan.PullRequestsKey, opts.PullRequests},
	} {
		for _, value := range list.values {
			if problem := plan.LinkProblem(list.key, value); problem != "" {
				return errors.New(problem)
			}
		}
	}
	now := deps.now()
	if !opts.noOpenSpec {
		return createOpenSpec(ctx, deps, root, name, opts, now)
	}
	repo, err := github.ResolveRepository(deps.stderr)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "github.com", filepath.FromSlash(repo))
	planPath := filepath.Join(dir, now.Format(timestampLayout)+"-"+name+".md")
	parentValue := ""
	if opts.parent != "" {
		parentValue, err = plan.CanonicalReference(root, opts.parent)
		if err != nil {
			return err
		}
	}
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

func planContent(deps dependencies, parentValue string, opts createOptions, now time.Time) ([]byte, error) {
	var input []byte
	if !deps.stdinIsTerminal() {
		var err error
		input, err = io.ReadAll(deps.stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
	}
	return plan.RenderContent(input, parentValue, opts.ContentOptions, now)
}
