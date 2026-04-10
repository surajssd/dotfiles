package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// SessionsFile maps session names to their configuration.
type SessionsFile map[string]*SessionConfig

// SessionConfig holds the configuration for a single session.
type SessionConfig struct {
	Image     string            `yaml:"image"`
	Resources ResourcesConfig   `yaml:"resources"`
	Ports     PortsConfig       `yaml:"ports"`
	Proxy     ProxyConfig       `yaml:"proxy"`
	Mounts    []MountConfig     `yaml:"mounts"`
	Env       map[string]string `yaml:"env"`
	Skills    []string          `yaml:"skills"`
}

// ResourcesConfig holds CPU and memory settings.
type ResourcesConfig struct {
	CPUs   int    `yaml:"cpus"`
	Memory string `yaml:"memory"`
}

// PortsConfig holds gateway and bridge port mappings.
type PortsConfig struct {
	Gateway int `yaml:"gateway"`
	Bridge  int `yaml:"bridge"`
}

// ProxyConfig holds HTTP proxy settings.
type ProxyConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

// MountConfig holds a single mount definition.
type MountConfig struct {
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"readonly"`
}

var sessionNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// LoadSessions reads and parses the sessions YAML config file.
func LoadSessions(path string) (SessionsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading sessions config: %w", err)
	}

	var sessions SessionsFile
	if err := yaml.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("parsing sessions config: %w", err)
	}

	// Apply defaults to all sessions.
	for name, cfg := range sessions {
		if cfg == nil {
			sessions[name] = &SessionConfig{}
			cfg = sessions[name]
		}
		cfg.applyDefaults()
	}

	return sessions, nil
}

// ValidateSessionName checks that a session name is valid and exists in the config.
func ValidateSessionName(name string, sessions SessionsFile) error {
	if name == "" {
		return fmt.Errorf("session name is required")
	}

	if !sessionNameRegex.MatchString(name) {
		return fmt.Errorf("invalid session name '%s': use only letters, digits, hyphens, and underscores", name)
	}

	if _, ok := sessions[name]; !ok {
		return fmt.Errorf("session '%s' not found in %s", name, SessionsConfigPath())
	}

	return nil
}

// GetSession returns the config for a named session, validating required fields.
func (sf SessionsFile) GetSession(name string) (*SessionConfig, error) {
	cfg, ok := sf[name]
	if !ok {
		return nil, fmt.Errorf("session '%s' not found", name)
	}

	if cfg.Ports.Gateway == 0 {
		return nil, fmt.Errorf("ports.gateway is required for session '%s' in %s", name, SessionsConfigPath())
	}
	if cfg.Ports.Bridge == 0 {
		return nil, fmt.Errorf("ports.bridge is required for session '%s' in %s", name, SessionsConfigPath())
	}

	return cfg, nil
}

// SessionNames returns all session names in the config.
func (sf SessionsFile) SessionNames() []string {
	names := make([]string, 0, len(sf))
	for name := range sf {
		names = append(names, name)
	}
	return names
}

// applyDefaults fills in default values for unset fields.
func (c *SessionConfig) applyDefaults() {
	if c.Image == "" {
		c.Image = DefaultImage
	}
	if c.Resources.CPUs == 0 {
		c.Resources.CPUs = DefaultCPUs
	}
	if c.Resources.Memory == "" {
		c.Resources.Memory = DefaultMemory
	}
	if c.Proxy.Port == 0 {
		c.Proxy.Port = DefaultProxyPort
	}
}
