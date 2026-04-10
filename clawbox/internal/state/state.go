package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

// EnsureDirs creates the host-side directory tree for a session.
func EnsureDirs(sessionName string) error {
	configDir := filepath.Join(config.StateDir(), sessionName, "config")

	dirs := []string{
		filepath.Join(configDir, "identity"),
		filepath.Join(configDir, "agents", "main", "agent"),
		filepath.Join(configDir, "agents", "main", "sessions"),
		filepath.Join(configDir, "workspace"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	return nil
}

// ConfigJSONPath returns the path to the openclaw.json config file for a session.
func ConfigJSONPath(sessionName string) string {
	return filepath.Join(config.StateDir(), sessionName, "config", "openclaw.json")
}

// ConfigExists checks if the openclaw.json config file exists for a session.
func ConfigExists(sessionName string) bool {
	_, err := os.Stat(ConfigJSONPath(sessionName))
	return err == nil
}

// openclawConfig represents the relevant fields from openclaw.json.
type openclawConfig struct {
	Gateway struct {
		Auth struct {
			Token string `json:"token"`
		} `json:"auth"`
	} `json:"gateway"`
}

// ReadGatewayToken reads the gateway auth token from openclaw.json.
// Returns an empty string if the file doesn't exist or the token is not set.
func ReadGatewayToken(sessionName string) string {
	data, err := os.ReadFile(ConfigJSONPath(sessionName))
	if err != nil {
		return ""
	}

	var cfg openclawConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}

	return cfg.Gateway.Auth.Token
}
