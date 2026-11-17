package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
)

const openHelp = `Open a plan file or managed directory in VS Code using the code command on PATH.

<plan> is a path under the root, a [[wikilink]], a basename, the short NAME
shown by planner get, or a URL the plan lists. The reference must match
exactly one plan. Inactive plans and plans with missing or invalid front
matter can be opened too.

The command returns after launching VS Code, without waiting for the file
to be closed. Install the VS Code command-line launcher if code is not on
PATH.`

func runOpen(ctx context.Context, deps dependencies, root, ref string) error {
	c, err := plan.Load(root)
	if err != nil {
		return err
	}
	p, err := plan.Find(c, ref)
	if err != nil {
		return err
	}
	target := p.Path
	if p.Directory != "" {
		target = p.Directory
	}
	cmd := exec.CommandContext(ctx, "code", target)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = deps.stdin, deps.stdout, deps.stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("VS Code command \"code\" not found on PATH; install the VS Code command-line launcher: %w", err)
		}
		return fmt.Errorf("open %s in VS Code: %w", p.Path, err)
	}
	return nil
}
