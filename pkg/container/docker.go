package container

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// DockerDetector detects Docker containers
type DockerDetector struct {
	config *Config
}

// NewDockerDetector creates a new Docker detector
func NewDockerDetector(config *Config) *DockerDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &DockerDetector{
		config: config,
	}
}

// Detect attempts to detect Docker environment and collect metadata
func (d *DockerDetector) Detect() (*Metadata, error) {
	if !d.IsContainer() {
		return nil, fmt.Errorf("not running in Docker container")
	}

	metadata := &Metadata{
		Runtime:    RuntimeDocker,
		DetectedAt: time.Now(),
		Labels:     make(map[string]string),
		Env:        make(map[string]string),
	}

	// Get container ID from cgroup
	if containerID, cgroupPath := d.getContainerIDFromCgroup(); containerID != "" {
		metadata.ContainerID = containerID
		metadata.CgroupPath = cgroupPath
	}

	// Get hostname (often the container ID short form)
	if hostname, err := os.Hostname(); err == nil {
		metadata.Hostname = hostname
	}

	// Try to read Docker-specific environment variables
	metadata.ImageName = os.Getenv("IMAGE_NAME")
	metadata.ContainerName = os.Getenv("CONTAINER_NAME")

	// Parse environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			metadata.Env[parts[0]] = parts[1]
		}
	}

	// Try to get more info from /.dockerenv (exists in Docker containers)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		// Confirmed Docker container
	}

	// Try to parse docker-specific info from /proc/1/mountinfo
	if mounts := d.getMountInfo(); len(mounts) > 0 {
		for _, mount := range mounts {
			metadata.Volumes = append(metadata.Volumes, mount)
		}
	}

	return metadata, nil
}

// IsContainer checks if running in a Docker container
func (d *DockerDetector) IsContainer() bool {
	// Check for /.dockerenv file (Docker-specific)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check cgroup for docker
	if containerID, _ := d.getContainerIDFromCgroup(); containerID != "" {
		return true
	}

	// Check for docker socket mounted
	if _, err := os.Stat(d.config.DockerSocketPath); err == nil {
		// Socket exists, but doesn't necessarily mean we're in a container
		// Check if we're actually in a container by looking at cgroup
		if strings.Contains(readFile("/proc/1/cgroup"), "docker") {
			return true
		}
	}

	return false
}

// GetRuntime returns Docker as the runtime
func (d *DockerDetector) GetRuntime() Runtime {
	return RuntimeDocker
}

// getContainerIDFromCgroup extracts container ID from cgroup file
func (d *DockerDetector) getContainerIDFromCgroup() (string, string) {
	content := readFile("/proc/self/cgroup")
	if content == "" {
		return "", ""
	}

	// Docker cgroup format: /docker/<container-id>
	// Or: /system.slice/docker-<container-id>.scope
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "docker") {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				cgroupPath := parts[2]

				// Extract container ID
				if strings.Contains(cgroupPath, "/docker/") {
					// Format: /docker/<container-id>
					pathParts := strings.Split(cgroupPath, "/docker/")
					if len(pathParts) > 1 {
						containerID := strings.TrimSpace(pathParts[1])
						// Remove any trailing suffixes
						containerID = strings.Split(containerID, ".")[0]
						return containerID, cgroupPath
					}
				} else if strings.Contains(cgroupPath, "docker-") && strings.HasSuffix(cgroupPath, ".scope") {
					// Format: /system.slice/docker-<container-id>.scope
					parts := strings.Split(cgroupPath, "docker-")
					if len(parts) > 1 {
						containerID := strings.TrimSuffix(parts[1], ".scope")
						return containerID, cgroupPath
					}
				}
			}
		}
	}

	return "", ""
}

// getMountInfo parses /proc/1/mountinfo for volume mounts
func (d *DockerDetector) getMountInfo() []VolumeMount {
	var mounts []VolumeMount

	file, err := os.Open("/proc/1/mountinfo")
	if err != nil {
		return mounts
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		if len(fields) < 5 {
			continue
		}

		// mountinfo format:
		// 36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue
		// [0]mount-id [1]parent-id [2]major:minor [3]root [4]mount-point [5]mount-options ...

		mountPoint := fields[4]
		mountOptions := fields[5]

		// Skip system mounts
		if strings.HasPrefix(mountPoint, "/proc") ||
			strings.HasPrefix(mountPoint, "/sys") ||
			strings.HasPrefix(mountPoint, "/dev") ||
			mountPoint == "/" {
			continue
		}

		mode := "rw"
		if strings.Contains(mountOptions, "ro") {
			mode = "ro"
		}

		mounts = append(mounts, VolumeMount{
			Destination: mountPoint,
			Mode:        mode,
			Type:        "bind",
		})
	}

	return mounts
}

// parseDockerJSON parses Docker inspect JSON (if available via socket)
// This is a placeholder for future enhancement
func (d *DockerDetector) parseDockerJSON(containerID string) (*ContainerInfo, error) {
	// This would require Docker API client
	// For now, return minimal info
	return &ContainerInfo{
		State: "running",
	}, nil
}

// Helper functions

func readFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

func parseJSONFile(path string, v interface{}) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(content, v)
}
