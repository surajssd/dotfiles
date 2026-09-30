package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

type dependencies struct {
	stdin           io.Reader
	stdout          io.Writer
	stderr          io.Writer
	now             func() time.Time
	stdinIsTerminal func() bool
	stdoutWidth     func() (int, bool)
}

func realDependencies() dependencies {
	return dependencies{
		stdin:           os.Stdin,
		stdout:          os.Stdout,
		stderr:          os.Stderr,
		now:             time.Now,
		stdinIsTerminal: func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
		stdoutWidth: func() (int, bool) {
			width, _, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				return 0, false
			}
			return width, true
		},
	}
}

const configName = ".planner.yaml"

var errNoRoot = errors.New(`no plan root configured: put "root: ~/plans" in ~/` + configName + " or pass --root <dir>")

func newCommand(deps dependencies) *cobra.Command {
	var rootFlag string
	root := &cobra.Command{
		Use:   "planner",
		Short: "Show, check, and create plans with front matter",
		Long: `planner manages a folder of Markdown plans with YAML front matter.

  planner get [<plan>...]      table of active plans, or of the named plans
  planner tree [<plan>...]     the same as an effort tree, or the named subtrees
  planner describe <plan>...   every field of a plan, its children, its findings
  planner check                front matter and link findings; exit 1 on errors
  planner new <name...>    create a plan for the current repository
  planner update status    change a plan's status, checked date, and note

Plans are Markdown files named <YYMMDDHHMMSS>-<name>.md under
<root>/github.com/<org>/<repo>/ with a YAML front matter block that holds
type, implementation_status, status_checked, status_note, and an optional
parent. The plan root comes from --root or from the root key in
~/.planner.yaml:

  root: ~/plans

Active means every implementation_status except Implemented and Superseded.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetIn(deps.stdin)
	root.SetOut(deps.stdout)
	root.SetErr(deps.stderr)
	root.PersistentFlags().StringVar(&rootFlag, "root", "", "plan root directory (overrides root in ~/.planner.yaml)")

	var getOpts listOptions
	get := &cobra.Command{
		Use:   "get [<plan>...]",
		Short: "List plans as a table",
		Long: `Print the plans under the root as a table: NAME, REPO, STATUS, AGE, and
TITLE, one row per plan, sorted by repository then filename.

Without --all only active plans (every implementation_status except
Implemented and Superseded) are shown; --all also lists plans without valid
front matter, with - in STATUS and AGE. AGE counts calendar days since
status_checked; ! marks an active plan older than 7 days. --wide drops REPO,
adds NOTE and PATH (home shown as ~), and never truncates. --repo keeps plans
whose <org>/<repo> contains the value, ignoring case.

With plan names (a path, [[wikilink]], basename, or the short NAME shown in
the table) only those plans are printed, whatever their status.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolveRoot(rootFlag)
			if err != nil {
				return err
			}
			return runGet(deps, rootDir, getOpts, args)
		},
	}
	addListFlags(get, &getOpts)

	var treeOpts listOptions
	tree := &cobra.Command{
		Use:   "tree [<plan>...]",
		Short: "Show plans as an effort tree",
		Long: `Print the plans under the root as a table with tree connectors in NAME:
children sit under their parent plan, and one group row per external parent
(a path or URL) collects the plans that point at it.

Without --all only active plans are shown, together with the ancestors needed
to place them. With plan names, each named plan is printed as a root with its
descendants. The columns and the other flags are those of planner get.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolveRoot(rootFlag)
			if err != nil {
				return err
			}
			return runTree(deps, rootDir, treeOpts, args)
		},
	}
	addListFlags(tree, &treeOpts)

	describe := &cobra.Command{
		Use:   "describe <plan>...",
		Short: "Show every field of a plan, its children, and its findings",
		Long: `Print one block per named plan, kubectl describe style: the name, title,
file (home shown as ~), repository, type, status, checked date and age, parent,
children, the full status note, and the planner check findings for that plan.

<plan> is a path, a [[wikilink]], a basename, or the short NAME shown by
planner get.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolveRoot(rootFlag)
			if err != nil {
				return err
			}
			return runDescribe(deps, rootDir, args)
		},
	}

	check := &cobra.Command{
		Use:   "check",
		Short: "Report front matter and link problems",
		Long:  checkHelp,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolveRoot(rootFlag)
			if err != nil {
				return err
			}
			return runCheck(deps, rootDir)
		},
	}

	var newOpts newOptions
	create := &cobra.Command{
		Use:   "new [--parent <ref>] [--status <status> --note <text>] <name...>",
		Short: "Create a plan for the current repository and print its path",
		Long: `Create <root>/github.com/<org>/<repo>/<YYMMDDHHMMSS>-<name>.md and print its
absolute path.

The words of <name> are joined with hyphens; a trailing .md is dropped. The
repository comes from the upstream remote, then from gh repo view on the
origin remote (which maps a fork to its parent), then from origin itself.

When stdin is a terminal the file holds only the generated front matter. When
stdin is piped, its content follows the generated front matter, unless it
already starts with a --- front matter block, which is kept as is.

--parent accepts a dumped plan (as [[wikilink]], basename, or the short name
shown by planner get), a file path, or a URL. A plan under the root is stored
as the wikilink of its basename; a file outside the root is stored as a path;
a URL is stored as is.

--status sets implementation_status (default NotImplemented; any case, with
or without hyphens) and needs --note, which sets status_note (default
"Implementation has not started."). With piped front matter the flags replace
those lines and set status_checked to today. An existing file is never
overwritten.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolveRoot(rootFlag)
			if err != nil {
				return err
			}
			newOpts.noteSet = cmd.Flags().Changed("note")
			return runNew(deps, rootDir, args, newOpts)
		},
	}
	create.Flags().StringVar(&newOpts.parent, "parent", "", "parent plan: [[wikilink]], basename, file path, or URL")
	create.Flags().StringVar(&newOpts.status, "status", "", "initial implementation_status (needs --note)")
	create.Flags().StringVar(&newOpts.note, "note", "", "initial status_note")

	update := &cobra.Command{
		Use:   "update",
		Short: "Update a plan's front matter in place",
		Args:  cobra.NoArgs,
	}
	var note string
	status := &cobra.Command{
		Use:   "status <plan> [<implementation_status>]",
		Short: "Change a plan's status, checked date, and note",
		Long:  statusHelp,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolveRoot(rootFlag)
			if err != nil {
				return err
			}
			return runStatus(deps, rootDir, args, note, cmd.Flags().Changed("note"))
		},
	}
	status.Flags().StringVar(&note, "note", "", "new status_note text")
	update.AddCommand(status)

	root.AddCommand(get, tree, describe, check, create, update)
	return root
}

// addListFlags attaches the flags that planner get and planner tree share.
func addListFlags(cmd *cobra.Command, opts *listOptions) {
	cmd.Flags().BoolVar(&opts.all, "all", false, "show Implemented and Superseded plans too")
	cmd.Flags().BoolVar(&opts.wide, "wide", false, "add NOTE and PATH columns and never truncate")
	cmd.Flags().StringVar(&opts.repo, "repo", "", "only plans whose <org>/<repo> contains this text (case-insensitive)")
}

// resolveRoot returns the absolute plan root from --root or from ~/.planner.yaml.
// The config file is read only when the flag is empty.
func resolveRoot(flagRoot string) (string, error) {
	if flagRoot != "" {
		expanded, err := expandHome(flagRoot)
		if err != nil {
			return "", err
		}
		return filepath.Abs(expanded)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(home, configName)
	data, err := os.ReadFile(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errNoRoot
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", configPath, err)
	}
	var config struct {
		Root string `yaml:"root"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("parse %s: %w", configPath, err)
	}
	if strings.TrimSpace(config.Root) == "" {
		return "", errNoRoot
	}
	expanded, err := expandHome(strings.TrimSpace(config.Root))
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(filepath.Dir(configPath), expanded)
	}
	return filepath.Clean(expanded), nil
}

// expandHome replaces a leading ~ or ~/ with the home directory.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}

func writeOutput(w io.Writer, text string) error {
	_, err := io.WriteString(w, text)
	return err
}
