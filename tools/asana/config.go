package main

import (
	"bytes"
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
	path           string
	document       yaml.Node
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
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("config %s must contain one YAML document", path)
	}
	if err := yaml.Unmarshal(data, &cfg.document); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if len(cfg.document.Content) != 1 || cfg.document.Content[0].Kind != yaml.MappingNode {
		return cfg, errors.New("config must be a YAML mapping")
	}
	cfg.path = path
	if cfg.Projects == nil {
		cfg.Projects = make(map[string]string)
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
