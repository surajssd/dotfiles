package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
)

const logHelp = `Append a dated entry to the Progress log section of a plan's body and set
StatusChecked to today. Use --keep-checked for administrative entries, such
as recording a ticket, that do not verify implementation status.

The entry is a heading, ### <today>: <title>, followed by the text piped on
stdin, if any. It goes at the end of the "## Progress log" section, before
the next heading of level one or two; a plan without that section gets one at
the end of the file. Headings inside fenced code blocks are ignored. The
words of <title> are joined with spaces.

--note replaces StatusNote in the same write, so the entry can carry the
detail while the note stays short. A status change stays with planner set
status.

--issue and --pr, each repeatable, add tracker and pull request URLs in the
same write, skipping duplicates. They use the same URL rules as planner set
issue and planner set pr. Read the entry from a file with < entry.md.

<plan> is a path under the root, a [[wikilink]], a basename, the short name
shown by planner get, or a URL the plan lists. On success the new heading is
printed.`

type logOptions struct {
	note         string
	noteSet      bool
	keepChecked  bool
	issues       []string
	pullRequests []string
}

func runLog(deps dependencies, root string, args []string, opts logOptions) error {
	title := strings.Join(strings.Fields(strings.Join(args[1:], " ")), " ")
	if title == "" {
		return errors.New("provide a nonempty entry title")
	}
	links := []struct {
		key    string
		values []string
	}{{plan.IssuesKey, opts.issues}, {plan.PullRequestsKey, opts.pullRequests}}
	for _, list := range links {
		for _, value := range list.values {
			if problem := plan.LinkProblem(list.key, value); problem != "" {
				return errors.New(problem)
			}
		}
	}
	body := ""
	if !deps.stdinIsTerminal() {
		input, err := io.ReadAll(deps.stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		body = string(input)
	}
	c, err := plan.Load(root)
	if err != nil {
		return err
	}
	p, err := plan.Find(c, args[0])
	if err != nil {
		return err
	}
	switch {
	case !p.HasFront:
		return fmt.Errorf("%s: the plan has no front matter; give it a status and --note with planner set status first", p.RelPath)
	case p.FrontErr != nil:
		return fmt.Errorf("%s: front matter does not decode (%v); fix it before logging to it", p.RelPath, p.FrontErr)
	}
	today := deps.now().Format(plan.DateLayout)
	checked := today
	if opts.keepChecked {
		checked = ""
	}
	heading := fmt.Sprintf("### %s: %s", today, title)
	if _, err := plan.EditFile(p, plan.Change{
		Checked:      checked,
		Note:         opts.note,
		NoteSet:      opts.noteSet,
		Issues:       opts.issues,
		PullRequests: opts.pullRequests,
		LogHeading:   heading,
		LogBody:      body,
	}); err != nil {
		return err
	}
	return writeOutput(deps.stdout, heading+"\n")
}
