package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

type Writer struct {
	target string
	file   *os.File
	config *Config
}

func (cfg *Config) PrepareWrite() (*Writer, error) {
	target, err := filepath.EvalSymlinks(cfg.path)
	if err != nil {
		return nil, fmt.Errorf("prepare config: %w", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("prepare config: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(target), ".asana-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("prepare config: %w", err)
	}
	write := &Writer{target: target, file: file, config: cfg}
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		write.Close()
		return nil, fmt.Errorf("prepare config: %w", err)
	}
	return write, nil
}

func (w *Writer) Close() {
	_ = w.file.Close()
	_ = os.Remove(w.file.Name())
}

func (w *Writer) Save() error {
	root := w.config.document.Content[0]
	var projectNode *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "projects" {
			projectNode = root.Content[i+1]
			break
		}
	}
	if projectNode == nil {
		projectNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "projects"}, projectNode)
	} else if projectNode.Kind == yaml.AliasNode {
		alias := *projectNode
		*projectNode = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"}, &alias,
		}}
	} else if projectNode.Tag == "!!null" {
		projectNode.Kind, projectNode.Tag, projectNode.Value = yaml.MappingNode, "!!map", ""
	}
	existing := make(map[string]bool)
	for i := 0; i < len(projectNode.Content); i += 2 {
		existing[projectNode.Content[i].Value] = true
	}
	for _, alias := range slices.Sorted(maps.Keys(w.config.Projects)) {
		if !existing[alias] {
			projectNode.Content = append(projectNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: alias},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: w.config.Projects[alias]})
		}
	}
	encoder := yaml.NewEncoder(w.file)
	encoder.SetIndent(2)
	if err := encoder.Encode(&w.config.document); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := os.Rename(w.file.Name(), w.target); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
