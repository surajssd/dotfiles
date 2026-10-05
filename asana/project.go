package main

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

func parseProjectURL(value string) (string, error) {
	u, err := url.Parse(value)
	if err == nil && u.Scheme == "https" && u.Host == "app.asana.com" && u.User == nil && u.Fragment == "" {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")
		if len(parts) >= 2 && parts[0] == "0" && isGID(parts[1]) {
			return parts[1], nil
		}
		if (len(parts) == 4 || len(parts) == 5 && (parts[4] == "list" || parts[4] == "board")) &&
			parts[0] == "1" && isGID(parts[1]) && parts[2] == "project" && isGID(parts[3]) {
			return parts[3], nil
		}
	}
	return "", fmt.Errorf("invalid Asana project URL %q", value)
}

func isGID(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func resolveProject(cfg config, value string) (string, error) {
	if projectURL, ok := cfg.Projects[value]; ok {
		return parseProjectURL(projectURL)
	}
	if gid, err := parseProjectURL(value); err == nil {
		return gid, nil
	}
	aliases := make([]string, 0, len(cfg.Projects))
	for alias := range cfg.Projects {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	if value == "" {
		return "", fmt.Errorf("no project selected: set default_project or pass --project; configured aliases: %s", strings.Join(aliases, ", "))
	}
	return "", fmt.Errorf("unknown project %q: use a configured alias or an Asana project URL; configured aliases: %s", value, strings.Join(aliases, ", "))
}
