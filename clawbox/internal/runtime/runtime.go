package runtime

// Runtime defines the interface for container runtime operations.
// The current implementation uses the Apple "container" CLI on macOS.
// Future backends (Docker, Podman) can implement this interface.
type Runtime interface {
	// Container lifecycle
	Run(opts RunOpts) error
	RunInteractive(opts RunOpts) error
	Start(name string) error
	Stop(name string) error
	Remove(name string) error
	Exec(name string, interactive bool, cmd []string) error
	LogsFollow(name string) error

	// Queries
	IsRunning(name string) (bool, error)
	Exists(name string) (bool, error)
	ListContainers(prefix string) ([]ContainerInfo, error)
	SystemStatus() error

	// Volumes
	EnsureVolume(name string, size string) error

	// Network
	DetectVMNetGateway() (string, error)
}

// RunOpts holds options for running a container.
type RunOpts struct {
	Name        string
	Image       string
	CPUs        int
	Memory      string
	UID         string
	GID         string
	Detach      bool
	Remove      bool
	Interactive bool
	Ports       []PortMapping
	Mounts      []string // pre-built mount spec strings
	Env         []string // pre-built KEY=VALUE strings
	Command     []string
}

// PortMapping represents a host:container port mapping.
type PortMapping struct {
	Host      int
	Container int
}

// ContainerInfo holds basic information about a container.
type ContainerInfo struct {
	Name   string
	Status string
}
