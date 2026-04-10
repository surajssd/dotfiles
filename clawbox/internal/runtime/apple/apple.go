package apple

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/surajssd/dotfiles/clawbox/internal/runtime"
)

// Runtime implements the runtime.Runtime interface using the Apple "container" CLI.
type Runtime struct{}

// New creates a new Apple container runtime.
func New() *Runtime {
	return &Runtime{}
}

// Run executes a container with stdout/stderr passthrough.
func (r *Runtime) Run(opts runtime.RunOpts) error {
	args := buildRunArgs(opts)
	cmd := exec.Command("container", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunInteractive executes a container with full terminal passthrough.
func (r *Runtime) RunInteractive(opts runtime.RunOpts) error {
	args := buildRunArgs(opts)
	cmd := exec.Command("container", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Start starts a stopped container.
func (r *Runtime) Start(name string) error {
	cmd := exec.Command("container", "start", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Stop stops a running container. Tolerates non-existent containers.
func (r *Runtime) Stop(name string) error {
	cmd := exec.Command("container", "stop", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Tolerate errors (matches || true in bash).
	_ = cmd.Run()
	return nil
}

// Remove removes a container. Tolerates non-existent containers.
func (r *Runtime) Remove(name string) error {
	cmd := exec.Command("container", "rm", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Exec runs a command inside a running container.
func (r *Runtime) Exec(name string, interactive bool, command []string) error {
	args := []string{"exec"}
	if interactive {
		args = append(args, "-it")
	}
	args = append(args, name)
	args = append(args, command...)

	cmd := exec.Command("container", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LogsFollow follows the logs of a container.
func (r *Runtime) LogsFollow(name string) error {
	cmd := exec.Command("container", "logs", "-f", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// containerLsJSON represents a container entry from "container ls --format json".
// The Apple container CLI nests the container ID under configuration.id.
type containerLsJSON struct {
	Configuration struct {
		ID string `json:"id"`
	} `json:"configuration"`
	Status string `json:"status"`
}

// IsRunning checks if a container with the given name is currently running.
func (r *Runtime) IsRunning(name string) (bool, error) {
	out, err := exec.Command("container", "ls", "--format", "json").Output()
	if err != nil {
		return false, nil
	}
	return parseContainerRunning(out, name), nil
}

// parseContainerRunning checks if a container name exists in the JSON output.
func parseContainerRunning(data []byte, name string) bool {
	var containers []containerLsJSON
	if err := json.Unmarshal(data, &containers); err != nil {
		// Fallback: match name as an exact whitespace-delimited token
		// to avoid "foo" matching "foobar".
		return containsExactToken(string(data), name)
	}
	for _, c := range containers {
		if c.Configuration.ID == name {
			return true
		}
	}
	return false
}

// Exists checks if a container with the given name exists (running or stopped).
func (r *Runtime) Exists(name string) (bool, error) {
	out, err := exec.Command("container", "ls", "-a", "--format", "json").Output()
	if err != nil {
		return false, nil
	}
	return parseContainerRunning(out, name), nil
}

// ListContainers lists all containers matching the given prefix.
func (r *Runtime) ListContainers(prefix string) ([]runtime.ContainerInfo, error) {
	out, err := exec.Command("container", "ls", "-a", "--format", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	return parseContainerList(out, prefix)
}

// parseContainerList parses JSON container list output and filters by prefix.
func parseContainerList(data []byte, prefix string) ([]runtime.ContainerInfo, error) {
	var containers []containerLsJSON
	if err := json.Unmarshal(data, &containers); err != nil {
		return nil, fmt.Errorf("parsing container list: %w", err)
	}

	var result []runtime.ContainerInfo
	for _, c := range containers {
		if strings.HasPrefix(c.Configuration.ID, prefix) {
			result = append(result, runtime.ContainerInfo{
				Name:   c.Configuration.ID,
				Status: c.Status,
			})
		}
	}
	return result, nil
}

// SystemStatus checks if the container system is running.
func (r *Runtime) SystemStatus() error {
	cmd := exec.Command("container", "system", "status")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildRunArgs converts RunOpts into container CLI arguments.
func buildRunArgs(opts runtime.RunOpts) []string {
	args := []string{"run"}

	if opts.Detach {
		args = append(args, "-d")
	}
	if opts.Remove {
		args = append(args, "--rm")
	}
	if opts.Interactive {
		args = append(args, "-it")
	}
	if opts.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%d", opts.CPUs))
	}
	if opts.Memory != "" {
		args = append(args, "--memory", opts.Memory)
	}
	if opts.UID != "" {
		args = append(args, "--uid", opts.UID)
	}
	if opts.GID != "" {
		args = append(args, "--gid", opts.GID)
	}
	for _, p := range opts.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", p.Host, p.Container))
	}
	for _, m := range opts.Mounts {
		args = append(args, "--mount", m)
	}
	for _, e := range opts.Env {
		args = append(args, "-e", e)
	}
	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}
	if opts.Image != "" {
		args = append(args, opts.Image)
	}
	args = append(args, opts.Command...)

	return args
}

// DetectVMNetGateway detects the vmnet gateway IP from the container network.
func (r *Runtime) DetectVMNetGateway() (string, error) {
	out, err := exec.Command("container", "network", "ls", "--format", "json").Output()
	if err != nil {
		return "", nil
	}
	return parseVMNetGateway(out)
}

// parseVMNetGateway parses the vmnet gateway from JSON network list output.
func parseVMNetGateway(data []byte) (string, error) {
	var networks []networkLsJSON
	if err := json.Unmarshal(data, &networks); err != nil {
		// Fallback: parse text output (second row, third column for subnet).
		return parseVMNetGatewayText(string(data)), nil
	}

	if len(networks) == 0 {
		return "", nil
	}

	// The gateway IP is directly available in the JSON output.
	if networks[0].Status.IPv4Gateway != "" {
		return networks[0].Status.IPv4Gateway, nil
	}

	// Fallback: derive from subnet.
	if networks[0].Status.IPv4Subnet != "" {
		return subnetToGateway(networks[0].Status.IPv4Subnet), nil
	}

	return "", nil
}

// networkLsJSON represents a network entry from "container network ls --format json".
type networkLsJSON struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Status struct {
		IPv4Subnet  string `json:"ipv4Subnet"`
		IPv4Gateway string `json:"ipv4Gateway"`
	} `json:"status"`
}

// subnetToGateway converts a subnet (e.g. "192.168.65.0/24") to a gateway IP (e.g. "192.168.65.1").
func subnetToGateway(subnet string) string {
	ip, _, err := net.ParseCIDR(subnet)
	if err != nil {
		return ""
	}

	ip = ip.To4()
	if ip == nil {
		return ""
	}

	// Set the last octet to 1.
	ip[3] = 1
	return ip.String()
}

// parseVMNetGatewayText is a fallback parser for text output of "container network ls".
func parseVMNetGatewayText(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return ""
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 3 {
		return ""
	}

	return subnetToGateway(fields[2])
}

// containsExactToken checks if name appears as an exact whitespace-delimited
// token in text. This avoids "foo" matching "foobar" in text fallback output.
func containsExactToken(text, name string) bool {
	for _, line := range strings.Split(text, "\n") {
		for _, field := range strings.Fields(line) {
			if field == name {
				return true
			}
		}
	}
	return false
}
