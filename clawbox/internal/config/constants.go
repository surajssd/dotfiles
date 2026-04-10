package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// ContainerPrefix is the prefix for all container names.
	ContainerPrefix = "openclaw"

	// DefaultImage is the fallback container image if not specified in YAML.
	DefaultImage = "ghcr.io/surajssd/dotfiles/openclaw:latest"

	// ContainerGatewayPort is the port inside the container for the gateway.
	ContainerGatewayPort = 18789

	// ContainerBridgePort is the port inside the container for the bridge.
	ContainerBridgePort = 18790

	// DefaultProxyPort is the default tinyproxy listen port.
	DefaultProxyPort = 11080

	// DefaultCPUs is the default number of CPUs for a session.
	DefaultCPUs = 2

	// DefaultMemory is the default memory limit for a session.
	DefaultMemory = "2g"

	// VolumeSize is the size of each container volume (home, linuxbrew).
	VolumeSize = "20G"
)

// SessionsConfigPath returns the path to the sessions YAML config file.
func SessionsConfigPath() string {
	return filepath.Join(homeDir(), ".config", "openclaw", "sessions.yaml")
}

// StateDir returns the root directory for all persistent per-session state.
func StateDir() string {
	return filepath.Join(homeDir(), ".custom-openclaw-setup")
}

// ProxyPIDDir returns the directory for tinyproxy PID and config files.
func ProxyPIDDir() string {
	return filepath.Join(StateDir(), "proxy")
}

// ContainerName returns the container name for a given session.
func ContainerName(sessionName string) string {
	return fmt.Sprintf("%s-%s", ContainerPrefix, sessionName)
}

// HomeVolumeName returns the volume name for the home directory.
func HomeVolumeName(sessionName string) string {
	return fmt.Sprintf("%s-%s-home", ContainerPrefix, sessionName)
}

// LinuxbrewVolumeName returns the volume name for the linuxbrew directory.
func LinuxbrewVolumeName(sessionName string) string {
	return fmt.Sprintf("%s-%s-linuxbrew", ContainerPrefix, sessionName)
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to $HOME
		return os.Getenv("HOME")
	}
	return home
}
