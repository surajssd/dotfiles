package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestContainerName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dev", "openclaw-dev"},
		{"my-session_1", "openclaw-my-session_1"},
		{"work", "openclaw-work"},
		{"", "openclaw-"},
	}
	for _, tt := range tests {
		if got := ContainerName(tt.input); got != tt.want {
			t.Errorf("ContainerName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHomeVolumeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dev", "openclaw-dev-home"},
		{"work", "openclaw-work-home"},
	}
	for _, tt := range tests {
		if got := HomeVolumeName(tt.input); got != tt.want {
			t.Errorf("HomeVolumeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLinuxbrewVolumeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dev", "openclaw-dev-linuxbrew"},
		{"work", "openclaw-work-linuxbrew"},
	}
	for _, tt := range tests {
		if got := LinuxbrewVolumeName(tt.input); got != tt.want {
			t.Errorf("LinuxbrewVolumeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSessionsConfigPath(t *testing.T) {
	p := SessionsConfigPath()
	if !strings.HasSuffix(p, filepath.Join(".config", "openclaw", "sessions.yaml")) {
		t.Errorf("SessionsConfigPath() = %q, want suffix .config/openclaw/sessions.yaml", p)
	}
}

func TestStateDir(t *testing.T) {
	d := StateDir()
	if !strings.HasSuffix(d, ".custom-openclaw-setup") {
		t.Errorf("StateDir() = %q, want suffix .custom-openclaw-setup", d)
	}
}

func TestProxyPIDDir(t *testing.T) {
	d := ProxyPIDDir()
	if !strings.HasSuffix(d, filepath.Join(".custom-openclaw-setup", "proxy")) {
		t.Errorf("ProxyPIDDir() = %q, want suffix .custom-openclaw-setup/proxy", d)
	}
}

func TestHomeDir(t *testing.T) {
	// Normal case: os.UserHomeDir should work.
	h := homeDir()
	if h == "" {
		t.Error("homeDir() returned empty string")
	}
}

func TestHomeDirFallback(t *testing.T) {
	// Force the fallback by setting HOME and unsetting anything else.
	// This test verifies homeDir returns a non-empty string.
	orig := os.Getenv("HOME")
	t.Setenv("HOME", "/tmp/fakehome")
	h := homeDir()
	if h == "" {
		t.Error("homeDir() returned empty string with HOME set")
	}
	os.Setenv("HOME", orig)
}

func TestValidateSessionName(t *testing.T) {
	sessions := SessionsFile{
		"work":       &SessionConfig{},
		"dev":        &SessionConfig{},
		"my-test_01": &SessionConfig{},
	}

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"work", false},
		{"dev", false},
		{"my-test_01", false},
		{"", true},
		{"-starts-with-hyphen", true},
		{"_starts-with-underscore", true},
		{"has spaces", true},
		{"has!special", true},
		{"has.dots", true},
		{"nonexistent", true},
	}

	for _, tt := range tests {
		err := ValidateSessionName(tt.name, sessions)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateSessionName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestGetSession(t *testing.T) {
	sessions := SessionsFile{
		"valid": &SessionConfig{
			Ports: PortsConfig{Gateway: 18789, Bridge: 18790},
		},
		"no-gateway": &SessionConfig{
			Ports: PortsConfig{Bridge: 18790},
		},
		"no-bridge": &SessionConfig{
			Ports: PortsConfig{Gateway: 18789},
		},
		"no-ports": &SessionConfig{},
	}

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"valid", false},
		{"no-gateway", true},
		{"no-bridge", true},
		{"no-ports", true},
		{"missing", true},
	}

	for _, tt := range tests {
		cfg, err := sessions.GetSession(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("GetSession(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
		if !tt.wantErr && cfg == nil {
			t.Errorf("GetSession(%q) returned nil config with no error", tt.name)
		}
	}
}

func TestSessionNames(t *testing.T) {
	tests := []struct {
		name     string
		sessions SessionsFile
		want     []string
	}{
		{"empty", SessionsFile{}, []string{}},
		{"single", SessionsFile{"work": &SessionConfig{}}, []string{"work"}},
		{"multiple", SessionsFile{"work": &SessionConfig{}, "dev": &SessionConfig{}}, []string{"dev", "work"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sessions.SessionNames()
			sort.Strings(got)
			sort.Strings(tt.want)
			if len(got) != len(tt.want) {
				t.Errorf("SessionNames() returned %d names, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SessionNames()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	t.Run("all empty gets defaults", func(t *testing.T) {
		cfg := &SessionConfig{}
		cfg.applyDefaults()
		if cfg.Image != DefaultImage {
			t.Errorf("Image = %q, want %q", cfg.Image, DefaultImage)
		}
		if cfg.Resources.CPUs != DefaultCPUs {
			t.Errorf("CPUs = %d, want %d", cfg.Resources.CPUs, DefaultCPUs)
		}
		if cfg.Resources.Memory != DefaultMemory {
			t.Errorf("Memory = %q, want %q", cfg.Resources.Memory, DefaultMemory)
		}
		if cfg.Proxy.Port != DefaultProxyPort {
			t.Errorf("Proxy.Port = %d, want %d", cfg.Proxy.Port, DefaultProxyPort)
		}
	})

	t.Run("set values preserved", func(t *testing.T) {
		cfg := &SessionConfig{
			Image:     "custom:latest",
			Resources: ResourcesConfig{CPUs: 8, Memory: "16g"},
			Proxy:     ProxyConfig{Port: 9999},
		}
		cfg.applyDefaults()
		if cfg.Image != "custom:latest" {
			t.Errorf("Image = %q", cfg.Image)
		}
		if cfg.Resources.CPUs != 8 {
			t.Errorf("CPUs = %d", cfg.Resources.CPUs)
		}
		if cfg.Resources.Memory != "16g" {
			t.Errorf("Memory = %q", cfg.Resources.Memory)
		}
		if cfg.Proxy.Port != 9999 {
			t.Errorf("Proxy.Port = %d", cfg.Proxy.Port)
		}
	})
}

func TestLoadSessions(t *testing.T) {
	yamlContent := `work:
  image: ghcr.io/example/image:latest
  resources:
    cpus: 4
    memory: 4g
  ports:
    gateway: 18789
    bridge: 18790
  proxy:
    enabled: true
    port: 8080
  env:
    MY_VAR: hello
  mounts:
    - source: /tmp/src
      target: /container/dst
      readonly: true
  skills:
    - /tmp/skills
dev:
  ports:
    gateway: 19789
    bridge: 19790
`
	tmpFile := t.TempDir() + "/sessions.yaml"
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	sessions, err := LoadSessions(tmpFile)
	if err != nil {
		t.Fatalf("LoadSessions() error: %v", err)
	}

	work := sessions["work"]
	if work.Image != "ghcr.io/example/image:latest" {
		t.Errorf("work.Image = %q", work.Image)
	}
	if work.Resources.CPUs != 4 {
		t.Errorf("work.Resources.CPUs = %d", work.Resources.CPUs)
	}
	if !work.Proxy.Enabled {
		t.Error("work.Proxy.Enabled should be true")
	}
	if work.Env["MY_VAR"] != "hello" {
		t.Errorf("work.Env[MY_VAR] = %q", work.Env["MY_VAR"])
	}
	if len(work.Mounts) != 1 || !work.Mounts[0].ReadOnly {
		t.Errorf("work.Mounts = %+v", work.Mounts)
	}

	dev := sessions["dev"]
	if dev.Image != DefaultImage {
		t.Errorf("dev.Image = %q, want default", dev.Image)
	}
	if dev.Resources.CPUs != DefaultCPUs {
		t.Errorf("dev.Resources.CPUs = %d, want default", dev.Resources.CPUs)
	}
}

func TestLoadSessionsNilEntry(t *testing.T) {
	// A YAML key with no value produces a nil *SessionConfig.
	yamlContent := "empty-session:\n"
	tmpFile := t.TempDir() + "/sessions.yaml"
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	sessions, err := LoadSessions(tmpFile)
	if err != nil {
		t.Fatalf("LoadSessions() error: %v", err)
	}

	cfg, ok := sessions["empty-session"]
	if !ok || cfg == nil {
		t.Fatal("nil entry should be replaced with a defaulted SessionConfig")
	}
	if cfg.Image != DefaultImage {
		t.Errorf("Image = %q, want default", cfg.Image)
	}
}

func TestLoadSessionsFileNotFound(t *testing.T) {
	_, err := LoadSessions("/nonexistent/path.yaml")
	if err == nil {
		t.Error("LoadSessions() should return error for nonexistent file")
	}
}

func TestLoadSessionsInvalidYAML(t *testing.T) {
	tmpFile := t.TempDir() + "/bad.yaml"
	if err := os.WriteFile(tmpFile, []byte("- item1\n- item2\n"), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	_, err := LoadSessions(tmpFile)
	if err == nil {
		t.Error("LoadSessions() should return error for YAML that doesn't match expected schema")
	}
}
