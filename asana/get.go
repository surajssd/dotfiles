package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func newGetTaskCommand(deps dependencies, configPath *string) *cobra.Command {
	var due, output string
	var projects []string
	var noHeaders bool
	cmd := &cobra.Command{
		Use:   "task",
		Short: "List your incomplete tasks across all workspaces",
		Long: `List incomplete tasks assigned to the authenticated user across every
accessible workspace. By default, include overdue tasks and tasks due today
in local time. Tasks without a due date are excluded.

--due selects tasks due on or before a date; any includes undated tasks,
and none selects only undated tasks. Repeat --project to match any selected
project. Without --project, include all projects and ignore default_project.
Results are sorted by local due date, undated last, then name and GID.`,
		Example: `  asana get task
  asana get task --due tomorrow
  asana get task --due any -o json
  asana get task --due none
  asana get task -p work -p personal -o wide --no-headers`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if output != "table" && output != "wide" && output != "json" {
				return fmt.Errorf("invalid output %q: use table, wide, or json", output)
			}
			now := deps.now()
			cutoff, err := parseGetDue(due, now)
			if err != nil {
				return err
			}
			cfg, err := readConfig(*configPath)
			if err != nil {
				return err
			}
			selected := make(map[string]bool, len(projects))
			for _, project := range projects {
				gid, err := resolveProject(cfg, project)
				if err != nil {
					return err
				}
				selected[gid] = true
			}
			token, err := lookupToken(cmd.Context(), cfg.TokenCommand)
			if err != nil {
				return err
			}
			_, workspaces, err := fetchUser(cmd.Context(), deps, token)
			if err != nil {
				return err
			}
			tasks := make([]task, 0)
			for _, workspace := range workspaces {
				workspaceTasks, err := fetchTasks(cmd.Context(), deps, token, workspace)
				if err != nil {
					return fmt.Errorf("list tasks in workspace %s: %w", workspace, err)
				}
				for _, task := range workspaceTasks {
					if task.DueOn != nil {
						task.dueDate = *task.DueOn
					}
					if task.DueAt != nil {
						dueAt, err := time.Parse(time.RFC3339, *task.DueAt)
						if err != nil {
							return fmt.Errorf("task %s has invalid due_at: %w", task.GID, err)
						}
						task.dueDate = dueAt.In(now.Location()).Format(time.DateOnly)
					}
					matchesDue := true
					switch cutoff {
					case "any":
					case "none":
						matchesDue = task.dueDate == ""
					default:
						matchesDue = task.dueDate != "" && task.dueDate <= cutoff
					}
					if !matchesDue {
						continue
					}
					matches := len(selected) == 0
					for _, project := range task.Projects {
						matches = matches || selected[project.GID]
					}
					if matches {
						if task.Projects == nil {
							task.Projects = []taskProject{}
						}
						tasks = append(tasks, task)
					}
				}
			}
			slices.SortFunc(tasks, func(a, b task) int {
				if a.dueDate == "" && b.dueDate != "" {
					return 1
				}
				if a.dueDate != "" && b.dueDate == "" {
					return -1
				}
				return cmp.Or(cmp.Compare(a.dueDate, b.dueDate), cmp.Compare(a.Name, b.Name), cmp.Compare(a.GID, b.GID))
			})
			return writeTasks(deps, tasks, output, noHeaders)
		},
	}
	cmd.Flags().StringVar(&due, "due", "today", "due on or before YYYY-MM-DD, yesterday, today, tomorrow; any or none")
	cmd.Flags().StringArrayVarP(&projects, "project", "p", nil, "configured project alias or Asana project URL (repeatable)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "output format: table, wide, or json")
	cmd.Flags().BoolVar(&noHeaders, "no-headers", false, "omit table headers (ignored for JSON)")
	return cmd
}

func parseGetDue(value string, now time.Time) (string, error) {
	switch value {
	case "any", "none":
		return value, nil
	case "yesterday":
		return now.AddDate(0, 0, -1).Format(time.DateOnly), nil
	case "today":
		return now.Format(time.DateOnly), nil
	case "tomorrow":
		return now.AddDate(0, 0, 1).Format(time.DateOnly), nil
	default:
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			return "", fmt.Errorf("invalid due date %q: use YYYY-MM-DD, yesterday, today, tomorrow, any, or none", value)
		}
		return value, nil
	}
}

func writeTasks(deps dependencies, tasks []task, output string, noHeaders bool) error {
	if output == "json" {
		encoder := json.NewEncoder(deps.stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(struct {
			Items []task `json:"items"`
		}{Items: tasks})
	}
	if len(tasks) == 0 {
		_, err := fmt.Fprintln(deps.stderr, "No tasks found.")
		return err
	}
	var buf strings.Builder
	table := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	if !noHeaders {
		header := "GID\tPROJECT\tDUE\tNAME"
		if output == "wide" {
			header += "\tURL"
		}
		_, _ = fmt.Fprintln(table, header)
	}
	oneLine := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ", "\v", " ", "\f", " ")
	for _, task := range tasks {
		projects := make([]string, 0, len(task.Projects))
		for _, project := range task.Projects {
			projects = append(projects, project.Name)
		}
		projectNames := "-"
		if len(projects) != 0 {
			projectNames = strings.Join(projects, ",")
		}
		due := task.dueDate
		if due == "" {
			due = "-"
		}
		_, _ = fmt.Fprintf(table, "%s\t%s\t%s\t%s", oneLine.Replace(task.GID), oneLine.Replace(projectNames), oneLine.Replace(due), oneLine.Replace(task.Name))
		if output == "wide" {
			_, _ = fmt.Fprintf(table, "\t%s", oneLine.Replace(task.PermalinkURL))
		}
		_, _ = fmt.Fprintln(table)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprint(deps.stdout, buf.String())
	return err
}
