package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newAddProjectCommand(deps dependencies, configPath *string) *cobra.Command {
	var alias, workspace string
	cmd := &cobra.Command{
		Use:   "project <name>",
		Short: "Create a private project and save its alias",
		Long: `Create a private project without team sharing. --alias is required and
must not already be configured. Use the account's only workspace or select
one with --workspace. Save the alias in the selected configuration file
and print only the project URL. The default project stays unchanged.`,
		Example: `  asana add project "Research" --alias research
  asana add project "Release planning" --alias releases --workspace 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return errors.New("project name must not be blank")
			}
			if strings.TrimSpace(alias) == "" {
				return errors.New("--alias is required; alias must not be blank")
			}
			if cmd.Flags().Changed("workspace") && !isGID(workspace) {
				return errors.New("workspace must be a nonempty numeric GID")
			}
			cfg, err := readConfig(*configPath)
			if err != nil {
				return err
			}
			if _, exists := cfg.Projects[alias]; exists {
				return fmt.Errorf("project alias %q already exists", alias)
			}
			write, err := prepareConfigWrite(&cfg)
			if err != nil {
				return err
			}
			defer write.close()
			token, err := lookupToken(cmd.Context(), cfg.TokenCommand)
			if err != nil {
				return err
			}
			_, workspaces, err := fetchUser(cmd.Context(), deps, token)
			if err != nil {
				return err
			}
			workspace, err = selectWorkspace(workspaces, workspace)
			if err != nil {
				return err
			}
			permalink, err := createProject(cmd.Context(), deps, token, args[0], workspace)
			if err != nil {
				return err
			}
			cfg.Projects[alias] = permalink
			if err := write.save(cfg.Projects); err != nil {
				return fmt.Errorf("%w; project created at %s but alias %q was not saved; register the URL manually in %s; do not repeat project creation", err, permalink, alias, cfg.path)
			}
			if _, err := fmt.Fprintln(deps.stdout, permalink); err != nil {
				return fmt.Errorf("write project URL: %w; project was created at %s and saved as alias %q", err, permalink, alias)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&alias, "alias", "", "required project alias to save in configuration")
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace GID (required when the account has several)")
	return cmd
}

func selectWorkspace(workspaces []string, selected string) (string, error) {
	if selected != "" {
		return selected, nil
	}
	switch len(workspaces) {
	case 0:
		return "", errors.New("no accessible workspaces")
	case 1:
		return workspaces[0], nil
	default:
		return "", fmt.Errorf("several workspaces available: %s; select one with --workspace", strings.Join(workspaces, ", "))
	}
}
