package container

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ContainerdDetector detects containerd containers
type ContainerdDetector struct {
	config *Config
}

// NewContainerdDetector creates a new containerd detector
func NewContainerdDetector(config *Config) *ContainerdDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &ContainerdDetector{
		config: config,
	}
}

// Detect attempts to detect containerd environment and collect metadata
func (d *ContainerdDetector) Detect() (*Metadata, error) {
	if !d.IsContainer() {
		return nil, fmt.Errorf("not running in containerd container")
	}

	metadata := &Metadata{
		Runtime:    RuntimeContainerd,
		DetectedAt: time.Now(),
		Labels:     make(map[string]string),
		Env:        make(map[string]string),
	}

	// Get container ID from cgroup
	if containerID, cgroupPath := d.getContainerIDFromCgroup(); containerID != "" {
		metadata.ContainerID = containerID
		metadata.CgroupPath = cgroupPath
	}

	// Get hostname
	if hostname, err := os.Hostname(); err == nil {
		metadata.Hostname = hostname
	}

	// Parse environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			metadata.Env[parts[0]] = parts[1]
		}
	}

	// Try to get namespace from environment (Kubernetes often sets this)
	if namespace := os.Getenv("POD_NAMESPACE"); namespace != "" {
		metadata.Labels["namespace"] = namespace
	}

	if podName := os.Getenv("POD_NAME"); podName != "" {
		metadata.Labels["pod"] = podName
	}

	return metadata, nil
}

// IsContainer checks if running in a containerd container
func (d *ContainerdDetector) IsContainer() bool {
	// Check cgroup for containerd
	if containerID, _ := d.getContainerIDFromCgroup(); containerID != "" {
		return true
	}

	// Check for containerd socket
	if _, err := os.Stat(d.config.ContainerdSocketPath); err == nil {
		// Socket exists, check if we're in a container
		content := readFile("/proc/1/cgroup")
		if strings.Contains(content, "containerd") {
			return true
		}
	}

	return false
}

// GetRuntime returns containerd as the runtime
func (d *ContainerdDetector) GetRuntime() Runtime {
	return RuntimeContainerd
}

// getContainerIDFromCgroup extracts container ID from cgroup file
func (d *ContainerdDetector) getContainerIDFromCgroup() (string, string) {
	content := readFile("/proc/self/cgroup")
	if content == "" {
		return "", ""
	}

	// containerd cgroup formats:
	// 1. /system.slice/containerd.service/kubepods/.../pod<pod-id>/<container-id>
	// 2. /k8s.io/<container-id>
	// 3. /containerd/<container-id>
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "containerd") || strings.Contains(line, "k8s.io") {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				cgroupPath := parts[2]

				// Extract container ID from path
				pathParts := strings.Split(cgroupPath, "/")
				if len(pathParts) > 0 {
					// The last component is usually the container ID
					containerID := pathParts[len(pathParts)-1]

					// Validate it looks like a container ID (64 hex chars)
					if len(containerID) >= 12 && isHexString(containerID[:12]) {
						return containerID, cgroupPath
					}
				}

				// Try to extract from kubepods path
				if strings.Contains(cgroupPath, "kubepods") {
					// Format: .../pod<pod-id>/<container-id>
					for i := len(pathParts) - 1; i >= 0; i-- {
						if strings.HasPrefix(pathParts[i], "pod") {
							// Next element should be container ID
							if i+1 < len(pathParts) {
								containerID := pathParts[i+1]
								if len(containerID) >= 12 {
									return containerID, cgroupPath
								}
							}
						}
					}
				}
			}
		}
	}

	return "", ""
}

// isHexString checks if a string contains only hex characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
