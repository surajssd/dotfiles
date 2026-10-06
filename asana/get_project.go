package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newGetProjectCommand(deps dependencies, configPath *string) *cobra.Command {
	var workspace, output string
	var refresh, noHeaders bool
	cmd := &cobra.Command{
		Use:   "project",
		Short: "List configured project aliases, optionally refreshing memberships",
		Long: `Read project aliases from configuration without credentials or network access.
--refresh fetches unarchived projects where you are a member and saves
missing projects under their names. Existing aliases remain unchanged.
A name collision fails without writing configuration or partial output.

Refresh uses the account's only workspace; select one with --workspace
when the account has several. --workspace requires --refresh.`,
		Example: `  asana get project
  asana get project -o wide
  asana get project --refresh --workspace 123 -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if output != "table" && output != "wide" && output != "json" {
				return fmt.Errorf("invalid output %q: use table, wide, or json", output)
			}
			if cmd.Flags().Changed("workspace") {
				if !refresh {
					return errors.New("--workspace requires --refresh")
				}
				if !isGID(workspace) {
					return errors.New("workspace must be a nonempty numeric GID")
				}
			}
			cfg, err := readConfig(*configPath)
			if err != nil {
				return err
			}
			if refresh {
				write, err := prepareConfigWrite(&cfg)
				if err != nil {
					return err
				}
				defer write.close()
				token, err := lookupToken(cmd.Context(), cfg.TokenCommand)
				if err != nil {
					return err
				}
				user, workspaces, err := fetchUser(cmd.Context(), deps, token)
				if err != nil {
					return err
				}
				if !isGID(user) {
					return fmt.Errorf("user response has invalid user GID %q", user)
				}
				workspace, err = selectWorkspace(workspaces, workspace)
				if err != nil {
					return err
				}
				projects, err := fetchProjects(cmd.Context(), deps, token, workspace)
				if err != nil {
					return err
				}
				count := len(cfg.Projects)
				if err := registerMemberships(cfg.Projects, projects, user); err != nil {
					return err
				}
				if len(cfg.Projects) != count {
					if err := write.save(cfg.Projects); err != nil {
						return err
					}
				}
			}
			return writeProjects(deps, cfg.Projects, output, noHeaders)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "add missing project memberships to configuration")
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace GID (requires --refresh)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "output format: table, wide, or json")
	cmd.Flags().BoolVar(&noHeaders, "no-headers", false, "omit table headers (ignored for JSON)")
	return cmd
}

func registerMemberships(aliases map[string]string, projects []project, user string) error {
	known := make(map[string]bool)
	for _, projectURL := range aliases {
		gid, err := parseProjectURL(projectURL)
		if err != nil {
			return err
		}
		known[gid] = true
	}
	for _, project := range projects {
		member := false
		for _, candidate := range project.Members {
			if candidate.GID == user {
				member = true
				break
			}
		}
		if !member || known[project.GID] {
			continue
		}
		gid, err := parseProjectURL(project.PermalinkURL)
		if err != nil {
			return fmt.Errorf("project %q (%s): %w", project.Name, project.GID, err)
		}
		if gid != project.GID || strings.TrimSpace(project.Name) == "" {
			return fmt.Errorf("project response has invalid name or GID for %q (%s)", project.Name, project.GID)
		}
		if existing, exists := aliases[project.Name]; exists {
			return fmt.Errorf("project %q (%s) conflicts with alias %q pointing to %s; configuration was not changed", project.Name, project.PermalinkURL, project.Name, existing)
		}
		aliases[project.Name] = project.PermalinkURL
		known[project.GID] = true
	}
	return nil
}

func writeProjects(deps dependencies, projects map[string]string, output string, noHeaders bool) error {
	type item struct {
		Alias string `json:"alias"`
		GID   string `json:"gid"`
		URL   string `json:"url"`
	}
	items := make([]item, 0, len(projects))
	for _, alias := range slices.Sorted(maps.Keys(projects)) {
		gid, err := parseProjectURL(projects[alias])
		if err != nil {
			return err
		}
		items = append(items, item{alias, gid, projects[alias]})
	}
	if output == "json" {
		encoder := json.NewEncoder(deps.stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(struct {
			Items []item `json:"items"`
		}{items})
	}
	if len(items) == 0 {
		_, err := fmt.Fprintln(deps.stderr, "No projects configured.")
		return err
	}
	var buf strings.Builder
	table := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	if !noHeaders {
		header := "ALIAS\tGID"
		if output == "wide" {
			header += "\tURL"
		}
		_, _ = fmt.Fprintln(table, header)
	}
	oneLine := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ", "\v", " ", "\f", " ")
	for _, item := range items {
		_, _ = fmt.Fprintf(table, "%s\t%s", oneLine.Replace(item.Alias), item.GID)
		if output == "wide" {
			_, _ = fmt.Fprintf(table, "\t%s", oneLine.Replace(item.URL))
		}
		_, _ = fmt.Fprintln(table)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprint(deps.stdout, buf.String())
	return err
}
