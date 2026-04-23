package proxy

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/surajssd/dotfiles/clawbox/internal/config"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime"
)

const (
	proxyHealthMaxAttempts = 10
	proxyHealthInterval    = 200 * time.Millisecond
	proxyHealthTimeout     = 500 * time.Millisecond
)

// Manager handles tinyproxy lifecycle and configuration.
type Manager struct {
	VMNetGateway string
	PIDDir       string
	Runtime      runtime.Runtime
}

// NewManager creates a new proxy manager.
func NewManager(vmnetGateway string, rt runtime.Runtime) *Manager {
	return &Manager{
		VMNetGateway: vmnetGateway,
		PIDDir:       config.ProxyPIDDir(),
		Runtime:      rt,
	}
}

// IsEnabled returns true if the proxy is enabled for the session.
func IsEnabled(cfg *config.SessionConfig) bool {
	return cfg.Proxy.Enabled
}

// Port returns the proxy port for the session.
func Port(cfg *config.SessionConfig) int {
	if cfg.Proxy.Port == 0 {
		return config.DefaultProxyPort
	}
	return cfg.Proxy.Port
}

// pidFile returns the path to the PID file for a session.
func (m *Manager) pidFile(sessionName string) string {
	return filepath.Join(m.PIDDir, sessionName+".pid")
}

// configFile returns the path to the tinyproxy config file for a session.
func (m *Manager) configFile(sessionName string) string {
	return filepath.Join(m.PIDDir, sessionName+".conf")
}

// Start starts a tinyproxy instance for the given session.
func (m *Manager) Start(sessionName string, cfg *config.SessionConfig) error {
	if !IsEnabled(cfg) {
		return nil
	}

	if _, err := exec.LookPath("tinyproxy"); err != nil {
		fmt.Println("⚠️  'tinyproxy' not installed — proxy support disabled. Install with: brew install tinyproxy")
		return nil
	}

	if m.VMNetGateway == "" {
		fmt.Println("⚠️  Could not detect vmnet gateway IP — proxy support disabled. Ensure a bridge interface exists.")
		return nil
	}

	port := Port(cfg)
	pidPath := m.pidFile(sessionName)
	confPath := m.configFile(sessionName)

	if err := os.MkdirAll(m.PIDDir, 0o755); err != nil {
		return fmt.Errorf("creating proxy PID directory: %w", err)
	}

	// Check if proxy is already running.
	if pid, err := m.readPID(pidPath); err == nil {
		if processAlive(pid) {
			fmt.Printf("🔌 HTTP proxy already running (PID %d) on %s:%d\n", pid, m.VMNetGateway, port)
			return nil
		}
		// Stale PID file — clean up.
		os.Remove(pidPath)
	}

	// Derive /24 subnet from gateway IP.
	subnet := gatewayToSubnet(m.VMNetGateway)

	// Generate tinyproxy config.
	confContent := fmt.Sprintf(`Port %d
Listen %s
Timeout 600
LogLevel Critical
MaxClients 100
DisableViaHeader Yes
Allow %s
`, port, m.VMNetGateway, subnet)

	if err := os.WriteFile(confPath, []byte(confContent), 0o644); err != nil {
		return fmt.Errorf("writing tinyproxy config: %w", err)
	}

	fmt.Printf("⏳ Starting HTTP proxy on %s:%d...\n", m.VMNetGateway, port)

	// Start tinyproxy in the background.
	// Set Setpgid so tinyproxy gets its own process group and is not killed
	// when the user presses Ctrl+C (SIGINT to the CLI's process group).
	cmd := exec.Command("tinyproxy", "-d", "-c", confPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting tinyproxy: %w", err)
	}

	proxyPID := cmd.Process.Pid

	// Wait for the proxy to become responsive.
	proxyAddr := fmt.Sprintf("http://%s:%d/", m.VMNetGateway, port)
	client := &http.Client{Timeout: proxyHealthTimeout}
	healthy := false

	for range proxyHealthMaxAttempts {
		resp, err := client.Get(proxyAddr)
		if err == nil {
			resp.Body.Close()
			healthy = true
			break
		}
		time.Sleep(proxyHealthInterval)
	}

	if !healthy {
		// Check if the process is still alive as a last resort.
		if !processAlive(proxyPID) {
			fmt.Printf("❌ Failed to start HTTP proxy on %s:%d\n", m.VMNetGateway, port)
			return nil
		}
	}

	// Write PID file.
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(proxyPID)), 0o644); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}

	fmt.Printf("✅ HTTP proxy running (PID %d) on %s:%d\n", proxyPID, m.VMNetGateway, port)
	return nil
}

// Stop stops the tinyproxy instance for a session.
func (m *Manager) Stop(sessionName string) error {
	pidPath := m.pidFile(sessionName)

	pid, err := m.readPID(pidPath)
	if err != nil {
		return nil
	}

	if processAlive(pid) {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		fmt.Printf("✅ HTTP proxy stopped (PID %d)\n", pid)
	}

	os.Remove(pidPath)
	return nil
}

// InjectEnv writes proxy environment variables inside the running container.
// All operations are non-fatal (matching || true in bash).
func (m *Manager) InjectEnv(sessionName, containerName string, cfg *config.SessionConfig) {
	if !IsEnabled(cfg) {
		return
	}

	if _, err := exec.LookPath("tinyproxy"); err != nil || m.VMNetGateway == "" {
		return
	}

	port := Port(cfg)
	proxyURL := fmt.Sprintf("http://%s:%d", m.VMNetGateway, port)
	subnet := gatewayToSubnet(m.VMNetGateway)

	// Write proxy environment file inside the container.
	proxyEnvContent := fmt.Sprintf(`export http_proxy=%s
export HTTP_PROXY=%s
export https_proxy=%s
export HTTPS_PROXY=%s
export no_proxy=localhost,127.0.0.1,%s,*.dev.local
export NO_PROXY=localhost,127.0.0.1,%s,*.dev.local
`, proxyURL, proxyURL, proxyURL, proxyURL, subnet, subnet)

	// Write .proxy_env file.
	writeCmd := []string{"bash", "-c", fmt.Sprintf("cat > /home/node/.proxy_env << 'PROXYEOF'\n%sPROXYEOF", proxyEnvContent)}
	_ = m.Runtime.Exec(containerName, false, writeCmd)

	// Install into profile.d, .profile, and bash.bashrc.
	installCmd := []string{"bash", "-c", `
sudo cp /home/node/.proxy_env /etc/profile.d/proxy.sh 2>/dev/null || true
if ! grep -q "\.proxy_env" /home/node/.profile 2>/dev/null; then
    echo "" >> /home/node/.profile
    echo "# HTTP proxy" >> /home/node/.profile
    echo "[ -f ~/.proxy_env ] && source ~/.proxy_env" >> /home/node/.profile
fi
if ! grep -q "\.proxy_env" /etc/bash.bashrc 2>/dev/null; then
    echo "" | sudo tee -a /etc/bash.bashrc > /dev/null
    echo "# HTTP proxy" | sudo tee -a /etc/bash.bashrc > /dev/null
    echo "[ -f /home/node/.proxy_env ] && . /home/node/.proxy_env" | sudo tee -a /etc/bash.bashrc > /dev/null
fi
`}
	_ = m.Runtime.Exec(containerName, false, installCmd)

	fmt.Printf("🔌 Proxy env injected: %s\n", proxyURL)
}

// Status displays the proxy status for a session.
func (m *Manager) Status(sessionName string, cfg *config.SessionConfig) {
	if !IsEnabled(cfg) {
		fmt.Printf("🔌 Proxy is not enabled for session '%s'\n", sessionName)
		fmt.Printf("   Add 'proxy: { enabled: true }' to %s\n", config.SessionsConfigPath())
		return
	}

	port := Port(cfg)
	pidPath := m.pidFile(sessionName)

	fmt.Println()
	fmt.Printf("  Session:  %s\n", sessionName)
	fmt.Printf("  Bind:     %s:%d\n", m.VMNetGateway, port)

	pid, err := m.readPID(pidPath)
	if err != nil {
		fmt.Println("  Status:   ⚪ Not started")
	} else if processAlive(pid) {
		fmt.Printf("  Status:   🟢 Running (PID %d)\n", pid)
	} else {
		fmt.Printf("  Status:   🔴 Dead (stale PID %d)\n", pid)
	}

	fmt.Println()
}

// readPID reads the PID from a file.
func (m *Manager) readPID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}

	return pid, nil
}

// processAlive checks if a process with the given PID is alive.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// gatewayToSubnet converts a gateway IP (e.g. "192.168.65.1") to a /24 subnet (e.g. "192.168.65.0/24").
func gatewayToSubnet(gateway string) string {
	ip := net.ParseIP(gateway)
	if ip == nil {
		return ""
	}

	ip = ip.To4()
	if ip == nil {
		return ""
	}

	ip[3] = 0
	return fmt.Sprintf("%s/24", ip.String())
}
