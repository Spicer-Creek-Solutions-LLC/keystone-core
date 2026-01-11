// Package container provides container runtime detection and metadata extraction
// for Docker, containerd, CRI-O, and Podman environments.
//
// The package automatically detects whether the Keystone Core agent is running
// inside a container and extracts runtime metadata such as container ID, image
// information, labels, network configuration, and resource limits.
//
// # Supported Runtimes
//
// The detector supports the following container runtimes:
//   - Docker (via /var/run/docker.sock)
//   - containerd (via /run/containerd/containerd.sock)
//   - CRI-O (via cgroup parsing)
//   - Podman (via Docker-compatible API or cgroup parsing)
//
// # Detection Methods
//
// Container detection uses multiple methods:
//   - Socket-based API queries (Docker, containerd)
//   - Cgroup v1/v2 path parsing (/proc/self/cgroup)
//   - Environment variable inspection
//   - Filesystem markers (/.dockerenv, /run/.containerenv)
//
// # Usage
//
// Basic detection:
//
//	detector := container.NewDetector(container.DefaultConfig())
//	metadata, err := detector.Detect()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if metadata.Runtime != container.RuntimeUnknown {
//	    fmt.Printf("Running in %s container: %s\n", metadata.Runtime, metadata.ContainerID)
//	}
//
// # Metadata
//
// When running inside a container, the detector extracts:
//   - Container ID and name
//   - Image ID, name, and digest
//   - Labels and environment variables
//   - Network configuration (IP, ports, mode)
//   - Volume mounts
//   - Resource limits (CPU, memory)
//   - Health check status
//
// # Caching
//
// Detection results are cached for the duration specified in Config.CacheDuration
// (default 5 minutes) to avoid repeated API calls.
package container

import "time"

// Runtime represents a container runtime
type Runtime int

const (
	// RuntimeUnknown represents an unknown runtime
	RuntimeUnknown Runtime = iota
	// RuntimeDocker represents Docker
	RuntimeDocker
	// RuntimeContainerd represents containerd
	RuntimeContainerd
	// RuntimeCRIO represents CRI-O
	RuntimeCRIO
	// RuntimePodman represents Podman
	RuntimePodman
)

func (r Runtime) String() string {
	switch r {
	case RuntimeDocker:
		return "docker"
	case RuntimeContainerd:
		return "containerd"
	case RuntimeCRIO:
		return "cri-o"
	case RuntimePodman:
		return "podman"
	default:
		return "unknown"
	}
}

// Metadata represents container runtime metadata
type Metadata struct {
	// Runtime is the detected container runtime
	Runtime Runtime

	// Version is the runtime version
	Version string

	// ContainerID is the current container ID (if running in container)
	ContainerID string

	// ContainerName is the current container name
	ContainerName string

	// ImageID is the image ID
	ImageID string

	// ImageName is the image name
	ImageName string

	// ImageDigest is the image digest (SHA256)
	ImageDigest string

	// Labels are container labels
	Labels map[string]string

	// Env are container environment variables
	Env map[string]string

	// Hostname is the container hostname
	Hostname string

	// NetworkMode is the container network mode (bridge, host, none, etc.)
	NetworkMode string

	// IPAddress is the container IP address
	IPAddress string

	// Ports are exposed ports
	Ports []PortMapping

	// Volumes are mounted volumes
	Volumes []VolumeMount

	// CgroupPath is the cgroup path
	CgroupPath string

	// PID is the container init process PID
	PID int

	// CreatedAt is when the container was created
	CreatedAt time.Time

	// StartedAt is when the container was started
	StartedAt time.Time

	// DetectedAt is when the metadata was collected
	DetectedAt time.Time
}

// PortMapping represents a container port mapping
type PortMapping struct {
	// ContainerPort is the port inside the container
	ContainerPort int

	// HostPort is the port on the host
	HostPort int

	// Protocol is the protocol (tcp, udp, sctp)
	Protocol string

	// HostIP is the host IP to bind to
	HostIP string
}

// VolumeMount represents a container volume mount
type VolumeMount struct {
	// Source is the host path or volume name
	Source string

	// Destination is the container path
	Destination string

	// Mode is the mount mode (ro, rw)
	Mode string

	// Type is the mount type (bind, volume, tmpfs)
	Type string
}

// Detector is the interface for container runtime detection
type Detector interface {
	// Detect attempts to detect container runtime and collect metadata
	Detect() (*Metadata, error)

	// IsContainer checks if running in a container
	IsContainer() bool

	// GetRuntime returns the detected runtime
	GetRuntime() Runtime
}

// Config holds configuration for container detection
type Config struct {
	// Timeout for API requests
	Timeout time.Duration

	// EnableDocker enables Docker detection
	EnableDocker bool

	// EnableContainerd enables containerd detection
	EnableContainerd bool

	// EnableCRIO enables CRI-O detection
	EnableCRIO bool

	// EnablePodman enables Podman detection
	EnablePodman bool

	// DockerSocketPath is the Docker socket path
	DockerSocketPath string

	// ContainerdSocketPath is the containerd socket path
	ContainerdSocketPath string

	// CacheDuration is how long to cache metadata
	CacheDuration time.Duration
}

// DefaultConfig returns the default container detection configuration
func DefaultConfig() *Config {
	return &Config{
		Timeout:              5 * time.Second,
		EnableDocker:         true,
		EnableContainerd:     true,
		EnableCRIO:           true,
		EnablePodman:         true,
		DockerSocketPath:     "/var/run/docker.sock",
		ContainerdSocketPath: "/run/containerd/containerd.sock",
		CacheDuration:        5 * time.Minute,
	}
}

// ContainerInfo is additional container information
type ContainerInfo struct {
	// State is the container state (created, running, paused, stopped, etc.)
	State string

	// Status is additional status information
	Status string

	// RestartCount is how many times the container has restarted
	RestartCount int

	// OOMKilled indicates if the container was killed by OOM
	OOMKilled bool

	// ExitCode is the last exit code
	ExitCode int

	// Resources are resource limits
	Resources *ResourceLimits

	// HealthCheck is the health check status
	HealthCheck *HealthStatus
}

// ResourceLimits represents container resource limits
type ResourceLimits struct {
	// CPUShares are CPU shares
	CPUShares int64

	// CPUQuota is the CPU quota in microseconds
	CPUQuota int64

	// CPUPeriod is the CPU period in microseconds
	CPUPeriod int64

	// MemoryLimit is the memory limit in bytes
	MemoryLimit int64

	// MemoryReservation is the memory soft limit in bytes
	MemoryReservation int64

	// MemorySwap is the memory + swap limit in bytes
	MemorySwap int64

	// PidsLimit is the maximum number of PIDs
	PidsLimit int64
}

// HealthStatus represents container health check status
type HealthStatus struct {
	// Status is the health status (healthy, unhealthy, starting, none)
	Status string

	// FailingStreak is consecutive failures
	FailingStreak int

	// Log is the recent health check log
	Log []HealthCheckResult
}

// HealthCheckResult represents a single health check result
type HealthCheckResult struct {
	// Start is when the check started
	Start time.Time

	// End is when the check ended
	End time.Time

	// ExitCode is the health check exit code
	ExitCode int

	// Output is the health check output
	Output string
}
