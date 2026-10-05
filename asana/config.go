package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type config struct {
	TokenCommand   []string          `yaml:"token_command"`
	DefaultProject string            `yaml:"default_project"`
	Projects       map[string]string `yaml:"projects"`
}

func readConfig(path string) (config, error) {
	var cfg config
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return cfg, err
		}
		path = filepath.Join(home, ".asana.yaml")
	}
	file, err := os.Open(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	defer func() { _ = file.Close() }()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("config %s must contain one YAML document", path)
	}
	if len(cfg.Projects) == 0 {
		return cfg, errors.New("config projects must not be empty")
	}
	for alias, projectURL := range cfg.Projects {
		if strings.TrimSpace(alias) == "" {
			return cfg, errors.New("config project aliases must not be blank")
		}
		if _, err := parseProjectURL(projectURL); err != nil {
			return cfg, fmt.Errorf("config project %q: %w", alias, err)
		}
	}
	if cfg.DefaultProject != "" {
		if _, ok := cfg.Projects[cfg.DefaultProject]; !ok {
			return cfg, fmt.Errorf("default_project %q is not a configured project alias", cfg.DefaultProject)
		}
	}
	return cfg, nil
}
