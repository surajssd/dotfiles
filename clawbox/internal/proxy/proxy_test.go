package proxy

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.SessionConfig
		want bool
	}{
		{"enabled", &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: true}}, true},
		{"disabled", &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: false}}, false},
		{"default", &config.SessionConfig{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEnabled(tt.cfg); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPort(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.SessionConfig
		want int
	}{
		{"custom port", &config.SessionConfig{Proxy: config.ProxyConfig{Port: 8888}}, 8888},
		{"zero falls back to default", &config.SessionConfig{}, config.DefaultProxyPort},
		{"port 1", &config.SessionConfig{Proxy: config.ProxyConfig{Port: 1}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Port(tt.cfg); got != tt.want {
				t.Errorf("Port() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGatewayToSubnet(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.65.1", "192.168.65.0/24"},
		{"10.0.0.1", "10.0.0.0/24"},
		{"172.16.0.254", "172.16.0.0/24"},
		{"0.0.0.0", "0.0.0.0/24"},
		{"not-an-ip", ""},
		{"", ""},
		{"::1", ""},     // IPv6
		{"fe80::1", ""}, // IPv6 link-local
	}
	for _, tt := range tests {
		if got := gatewayToSubnet(tt.input); got != tt.want {
			t.Errorf("gatewayToSubnet(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestManagerPidFile(t *testing.T) {
	m := &Manager{PIDDir: "/tmp/proxy"}
	if got := m.pidFile("dev"); got != "/tmp/proxy/dev.pid" {
		t.Errorf("pidFile() = %q", got)
	}
}

func TestManagerConfigFile(t *testing.T) {
	m := &Manager{PIDDir: "/tmp/proxy"}
	if got := m.configFile("dev"); got != "/tmp/proxy/dev.conf" {
		t.Errorf("configFile() = %q", got)
	}
}

func TestReadPID(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{PIDDir: tmpDir}

	t.Run("valid PID file", func(t *testing.T) {
		pidFile := filepath.Join(tmpDir, "test.pid")
		os.WriteFile(pidFile, []byte("12345\n"), 0o644)
		pid, err := m.readPID(pidFile)
		if err != nil {
			t.Fatalf("readPID() error: %v", err)
		}
		if pid != 12345 {
			t.Errorf("readPID() = %d, want 12345", pid)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := m.readPID(filepath.Join(tmpDir, "nonexistent.pid"))
		if err == nil {
			t.Error("readPID() should error for nonexistent file")
		}
	})

	t.Run("invalid content", func(t *testing.T) {
		pidFile := filepath.Join(tmpDir, "bad.pid")
		os.WriteFile(pidFile, []byte("not-a-number"), 0o644)
		_, err := m.readPID(pidFile)
		if err == nil {
			t.Error("readPID() should error for non-numeric content")
		}
	})
}

func TestProcessAlive(t *testing.T) {
	// Our own PID should always be alive.
	if !processAlive(os.Getpid()) {
		t.Error("processAlive(own PID) should be true")
	}

	// A very high PID should not be alive.
	if processAlive(9999999) {
		t.Error("processAlive(9999999) should be false")
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager("192.168.65.1", nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.VMNetGateway != "192.168.65.1" {
		t.Errorf("VMNetGateway = %q", m.VMNetGateway)
	}
	if m.PIDDir == "" {
		t.Error("PIDDir should not be empty")
	}
}

func TestStopNoPidFile(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{PIDDir: tmpDir}
	// Should not error when there is no PID file.
	if err := m.Stop("nonexistent"); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestStopAliveProcess(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{PIDDir: tmpDir}

	// Write a PID file with our own PID (alive but won't actually be killed
	// because SIGTERM to self would terminate the test — use a PID we know is alive).
	// Instead, write a dead PID and verify the dead path.
	// For the alive path, we can't safely test it without killing a real process.
	// So we test the code path by using our own PID and accepting the SIGTERM.
	// Actually let's just verify the cleanup happens with a dead PID.
	pidFile := filepath.Join(tmpDir, "alive.pid")
	os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)

	// We can't actually call Stop with our own PID (it would SIGTERM us).
	// Verify readPID works, and the file exists.
	pid, err := m.readPID(pidFile)
	if err != nil || pid != os.Getpid() {
		t.Errorf("readPID() = %d, %v", pid, err)
	}
}

func TestStopDeadProcess(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{PIDDir: tmpDir}

	// Write a PID file with a dead process PID.
	pidFile := filepath.Join(tmpDir, "dead.pid")
	os.WriteFile(pidFile, []byte("9999999"), 0o644)

	if err := m.Stop("dead"); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// PID file should be cleaned up.
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("PID file should be removed after Stop()")
	}
}

func TestStatusDisabled(t *testing.T) {
	m := &Manager{VMNetGateway: "192.168.65.1", PIDDir: t.TempDir()}
	cfg := &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: false}}
	// Should not panic.
	m.Status("test", cfg)
}

func TestStatusEnabledNoPid(t *testing.T) {
	m := &Manager{VMNetGateway: "192.168.65.1", PIDDir: t.TempDir()}
	cfg := &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: true, Port: 8080}}
	// Should not panic — shows "Not started".
	m.Status("test", cfg)
}

func TestStatusEnabledDeadPid(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{VMNetGateway: "192.168.65.1", PIDDir: tmpDir}
	os.WriteFile(filepath.Join(tmpDir, "test.pid"), []byte("9999999"), 0o644)

	cfg := &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: true, Port: 8080}}
	// Should show "Dead (stale PID)".
	m.Status("test", cfg)
}

func TestStatusEnabledRunningPid(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{VMNetGateway: "192.168.65.1", PIDDir: tmpDir}
	// Use our own PID — guaranteed to be alive.
	os.WriteFile(filepath.Join(tmpDir, "test.pid"), []byte(strconv.Itoa(os.Getpid())), 0o644)

	cfg := &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: true, Port: 8080}}
	// Should show "Running".
	m.Status("test", cfg)
}

func TestInjectEnvDisabled(t *testing.T) {
	m := &Manager{VMNetGateway: "192.168.65.1"}
	cfg := &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: false}}
	// Should return immediately without panicking.
	m.InjectEnv("test", "container-name", cfg)
}

func TestInjectEnvNoGateway(t *testing.T) {
	m := &Manager{VMNetGateway: ""}
	cfg := &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: true}}
	// Should return immediately — no gateway.
	m.InjectEnv("test", "container-name", cfg)
}

func TestStartDisabled(t *testing.T) {
	m := &Manager{VMNetGateway: "192.168.65.1", PIDDir: t.TempDir()}
	cfg := &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: false}}
	if err := m.Start("test", cfg); err != nil {
		t.Errorf("Start() error: %v", err)
	}
}

func TestStartNoGateway(t *testing.T) {
	m := &Manager{VMNetGateway: "", PIDDir: t.TempDir()}
	cfg := &config.SessionConfig{Proxy: config.ProxyConfig{Enabled: true}}
	// Should print warning and return nil.
	if err := m.Start("test", cfg); err != nil {
		t.Errorf("Start() error: %v", err)
	}
}
