package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
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

var (
	getFormats  = []string{"wide", "json", "yaml", "name"}
	treeFormats = []string{"wide"}
)

const getHelp = `Print the plans under the root as a table: NAME, REPO, STATUS, CHECKED, and
TITLE, one row per plan, sorted by repository then filename.

Without --all only active plans (every ImplementationStatus except
Implemented and Superseded) are shown; --all also lists plans without valid
front matter, with - in STATUS and CHECKED. CHECKED counts calendar days
since StatusChecked; ! marks an active plan older than 7 days. --repo keeps
plans whose <org>/<repo> contains the value, ignoring case. --status keeps
plans with that ImplementationStatus, typed in any case with or without
hyphens, and shows an Implemented or Superseded status without --all.

-o wide adds TYPE, PATH (home shown as ~), and NOTE, and never truncates.
-o json and -o yaml print every field of each plan, its path included: one
named plan is a single object, anything else a list under items. -o name
prints one basename per line. --no-headers drops the header row.

With plan names (a path, [[wikilink]], basename, or the short NAME shown in
the table) only those plans are printed, whatever their status. A URL selects
every plan that lists it under Issues, PullRequests, Parent, or
SupersededBy. A name that matches no plan is reported after the others and
the exit status is 1.`

const treeHelp = `Print the plans under the root as a table with tree connectors in NAME:
children sit under their parent plan, and one group row per external parent
(a path or URL) collects the plans that point at it.

Without --all only active plans are shown, together with the ancestors needed
to place them. With plan names, each named plan is printed as a root with its
descendants. The columns and the other flags are those of planner get; the
only output format is -o wide.`

const describeHelp = `Print one block per named plan, kubectl describe style: the name, title,
file (home shown as ~), repository, type, status, checked date and age, parent,
successor, children, issues, pull requests, the full status note, and the
planner check findings for that plan.

<plan> is a path, a [[wikilink]], a basename, the short NAME shown by planner
get, or a URL the plan lists. A name that matches no plan is reported after
the others and the exit status is 1.`

const createHelp = `Create <root>/github.com/<org>/<repo>/<YYMMDDHHMMSS>-<name>.md and print its
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

--status sets ImplementationStatus (default NotImplemented; any case, with
or without hyphens) and needs --note, which sets StatusNote (default
"Implementation has not started."). With piped front matter the flags replace
those lines and set StatusChecked to today. An existing file is never
overwritten.

--issue and --pr, each repeatable, fill the Issues and PullRequests lists
with tracker and pull request URLs; with piped front matter they are merged
into the lists it already holds. planner set issue and planner set pr
add more later.`

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
  planner create <name...>     create a plan for the current repository
  planner set status           change a plan's status, checked date, and note
  planner set pr|issue         add pull request or tracker URLs to a plan
  planner version              build information of this binary

Plans are Markdown files named <YYMMDDHHMMSS>-<name>.md under
<root>/github.com/<org>/<repo>/ with a YAML front matter block that holds
Type, ImplementationStatus, StatusChecked, and StatusNote, and optionally
Parent, SupersededBy, Issues, and PullRequests. The plan root comes from
--root or from the root key in ~/.planner.yaml:

  root: ~/plans

Active means every ImplementationStatus except Implemented and Superseded.
planner completion zsh (or bash, fish, powershell) prints a script that
completes commands, flags, plan names, repositories, and statuses.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetIn(deps.stdin)
	root.SetOut(deps.stdout)
	root.SetErr(deps.stderr)
	root.PersistentFlags().StringVar(&rootFlag, "root", "", "plan root directory (overrides root in ~/.planner.yaml)")
	resolve := func() (string, error) { return resolveRoot(rootFlag) }
	complete := completer{root: resolve}

	var getOpts listOptions
	get := &cobra.Command{
		Use:               "get [<plan>...]",
		Short:             "List plans as a table",
		Long:              getHelp,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: complete.plans,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := getOpts.validate(getFormats); err != nil {
				return err
			}
			rootDir, err := resolve()
			if err != nil {
				return err
			}
			return runGet(deps, rootDir, getOpts, args)
		},
	}
	addListFlags(get, &getOpts, complete, getFormats)

	var treeOpts listOptions
	tree := &cobra.Command{
		Use:               "tree [<plan>...]",
		Short:             "Show plans as an effort tree",
		Long:              treeHelp,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: complete.plans,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := treeOpts.validate(treeFormats); err != nil {
				return err
			}
			rootDir, err := resolve()
			if err != nil {
				return err
			}
			return runTree(deps, rootDir, treeOpts, args)
		},
	}
	addListFlags(tree, &treeOpts, complete, treeFormats)

	describe := &cobra.Command{
		Use:               "describe <plan>...",
		Short:             "Show every field of a plan, its children, and its findings",
		Long:              describeHelp,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: complete.plans,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolve()
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
			rootDir, err := resolve()
			if err != nil {
				return err
			}
			return runCheck(deps, rootDir)
		},
	}

	var newOpts createOptions
	create := &cobra.Command{
		Use:   "create [--parent <ref>] [--status <status> --note <text>] <name...>",
		Short: "Create a plan for the current repository and print its path",
		Long:  createHelp,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolve()
			if err != nil {
				return err
			}
			newOpts.noteSet = cmd.Flags().Changed("note")
			return runCreate(deps, rootDir, args, newOpts)
		},
	}
	create.Flags().StringVar(&newOpts.parent, "parent", "", "parent plan: [[wikilink]], basename, file path, or URL")
	create.Flags().StringVar(&newOpts.status, "status", "", "initial ImplementationStatus (needs --note)")
	create.Flags().StringVar(&newOpts.note, "note", "", "initial StatusNote")
	create.Flags().StringArrayVar(&newOpts.issues, "issue", nil, "tracker URL for the Issues list (repeatable)")
	create.Flags().StringArrayVar(&newOpts.pullRequests, "pr", nil, "pull request URL for the PullRequests list (repeatable)")
	mustCompleteFlag(create, "parent", complete.parents)
	mustCompleteFlag(create, "status", completeValues(statusValues))

	set := &cobra.Command{
		Use:   "set",
		Short: "Update a plan's front matter in place",
		Args:  cobra.NoArgs,
	}
	var statusOpts statusOptions
	status := &cobra.Command{
		Use:               "status <plan> [<ImplementationStatus>]",
		Short:             "Change a plan's status, checked date, note, and successor",
		Long:              statusHelp,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: complete.planThenStatus,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolve()
			if err != nil {
				return err
			}
			statusOpts.noteSet = cmd.Flags().Changed("note")
			return runStatus(deps, rootDir, args, statusOpts)
		},
	}
	status.Flags().StringVar(&statusOpts.note, "note", "", "new StatusNote text")
	status.Flags().StringVar(&statusOpts.supersededBy, "superseded-by", "", "plan that replaces this one: [[wikilink]], basename, file path, or URL")
	mustCompleteFlag(status, "superseded-by", complete.parents)
	set.AddCommand(status, linksCommand(deps, resolve, complete, "pr", pullRequestsKey), linksCommand(deps, resolve, complete, "issue", issuesKey))

	version := &cobra.Command{
		Use:   "version",
		Short: "Print the build information of this binary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info, ok := debug.ReadBuildInfo()
			return writeOutput(deps.stdout, versionText(info, ok))
		},
	}

	root.AddCommand(get, tree, describe, check, create, set, version)
	return root
}

// linksCommand builds planner set pr and planner set issue, which differ
// only in the list they append to.
func linksCommand(deps dependencies, resolve func() (string, error), complete completer, name, key string) *cobra.Command {
	return &cobra.Command{
		Use:               name + " <plan> <url>...",
		Short:             "Add URLs to a plan's " + key + " list",
		Long:              fmt.Sprintf(linksHelp, key, linkRules[key]),
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: complete.planFirst,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, err := resolve()
			if err != nil {
				return err
			}
			return runLinks(deps, rootDir, key, args)
		},
	}
}

// addListFlags attaches the flags that planner get and planner tree share.
func addListFlags(cmd *cobra.Command, opts *listOptions, complete completer, formats []string) {
	cmd.Flags().BoolVar(&opts.all, "all", false, "show Implemented and Superseded plans too")
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "output format: "+strings.Join(formats, ", "))
	cmd.Flags().BoolVar(&opts.noHeaders, "no-headers", false, "omit the header row")
	cmd.Flags().StringVar(&opts.repo, "repo", "", "only plans whose <org>/<repo> contains this text (case-insensitive)")
	cmd.Flags().StringVar(&opts.status, "status", "", "only plans with this ImplementationStatus (any case, with or without hyphens)")
	mustCompleteFlag(cmd, "output", completeValues(formats))
	mustCompleteFlag(cmd, "repo", complete.repos)
	mustCompleteFlag(cmd, "status", completeValues(statusValues))
}

// validate rejects an output format the command does not offer and a status
// it does not recognise, and stores the recognised spelling of the status.
func (o *listOptions) validate(formats []string) error {
	if o.output != "" && !slices.Contains(formats, o.output) {
		return fmt.Errorf("unknown output format %q; use one of %s", o.output, strings.Join(formats, ", "))
	}
	if o.status == "" {
		return nil
	}
	status, ok := canonicalStatus(o.status)
	if !ok {
		return fmt.Errorf("unknown status %q; use one of %s", o.status, strings.Join(statusValues, ", "))
	}
	o.status = status
	return nil
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
