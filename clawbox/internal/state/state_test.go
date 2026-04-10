package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

func TestEnsureDirs(t *testing.T) {
	// Use a temp dir as the state dir by overriding HOME.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := EnsureDirs("test-session")
	if err != nil {
		t.Fatalf("EnsureDirs() error: %v", err)
	}

	// Verify all expected directories exist.
	stateDir := config.StateDir()
	expectedDirs := []string{
		filepath.Join(stateDir, "test-session", "config", "identity"),
		filepath.Join(stateDir, "test-session", "config", "agents", "main", "agent"),
		filepath.Join(stateDir, "test-session", "config", "agents", "main", "sessions"),
		filepath.Join(stateDir, "test-session", "config", "workspace"),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory %q not created: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("%q is not a directory", dir)
		}
	}
}

func TestConfigJSONPath(t *testing.T) {
	p := ConfigJSONPath("mytest")
	if !strings.HasSuffix(p, filepath.Join("mytest", "config", "openclaw.json")) {
		t.Errorf("ConfigJSONPath() = %q", p)
	}
}

func TestConfigExists(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	session := "exists-test"

	// Should not exist yet.
	if ConfigExists(session) {
		t.Error("ConfigExists() should be false before creating file")
	}

	// Create the file.
	configDir := filepath.Join(config.StateDir(), session, "config")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "openclaw.json"), []byte("{}"), 0o644)

	if !ConfigExists(session) {
		t.Error("ConfigExists() should be true after creating file")
	}
}

func TestReadGatewayToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	session := "token-test"
	configDir := filepath.Join(config.StateDir(), session, "config")
	os.MkdirAll(configDir, 0o755)

	t.Run("file not found", func(t *testing.T) {
		token := ReadGatewayToken("nonexistent")
		if token != "" {
			t.Errorf("ReadGatewayToken() = %q, want empty", token)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		data := map[string]any{
			"gateway": map[string]any{
				"auth": map[string]any{
					"token": "abc123",
				},
			},
		}
		jsonData, _ := json.Marshal(data)
		os.WriteFile(filepath.Join(configDir, "openclaw.json"), jsonData, 0o644)

		token := ReadGatewayToken(session)
		if token != "abc123" {
			t.Errorf("ReadGatewayToken() = %q, want %q", token, "abc123")
		}
	})

	t.Run("no token field", func(t *testing.T) {
		os.WriteFile(filepath.Join(configDir, "openclaw.json"), []byte(`{"gateway":{}}`), 0o644)
		token := ReadGatewayToken(session)
		if token != "" {
			t.Errorf("ReadGatewayToken() = %q, want empty", token)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		os.WriteFile(filepath.Join(configDir, "openclaw.json"), []byte("not json"), 0o644)
		token := ReadGatewayToken(session)
		if token != "" {
			t.Errorf("ReadGatewayToken() = %q, want empty", token)
		}
	})
}
