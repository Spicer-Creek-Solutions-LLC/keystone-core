package container

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestRuntime_String(t *testing.T) {
	tests := []struct {
		runtime  Runtime
		expected string
	}{
		{RuntimeDocker, "docker"},
		{RuntimeContainerd, "containerd"},
		{RuntimeCRIO, "cri-o"},
		{RuntimePodman, "podman"},
		{RuntimeUnknown, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.runtime.String(); got != tt.expected {
			t.Errorf("Runtime.String() = %v, want %v", got, tt.expected)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if config.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", config.Timeout)
	}

	if !config.EnableDocker {
		t.Error("expected Docker to be enabled")
	}

	if !config.EnableContainerd {
		t.Error("expected containerd to be enabled")
	}

	if config.DockerSocketPath != "/var/run/docker.sock" {
		t.Errorf("unexpected Docker socket path: %s", config.DockerSocketPath)
	}

	if config.ContainerdSocketPath != "/run/containerd/containerd.sock" {
		t.Errorf("unexpected containerd socket path: %s", config.ContainerdSocketPath)
	}

	if config.CacheDuration != 5*time.Minute {
		t.Errorf("expected cache duration 5m, got %v", config.CacheDuration)
	}
}

func TestNewDetector(t *testing.T) {
	detector := NewDetector(nil)

	if detector == nil {
		t.Fatal("NewDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}

	if detector.detectors == nil {
		t.Error("detector detectors map is nil")
	}

	// Should have Docker and containerd detectors registered
	if len(detector.detectors) != 2 {
		t.Errorf("expected 2 detectors, got %d", len(detector.detectors))
	}
}

func TestNewDetector_CustomConfig(t *testing.T) {
	config := &Config{
		Timeout:          10 * time.Second,
		EnableDocker:     true,
		EnableContainerd: false,
		CacheDuration:    1 * time.Minute,
	}

	detector := NewDetector(config)

	if detector == nil {
		t.Fatal("NewDetector returned nil")
	}

	// Should only have Docker detector
	if len(detector.detectors) != 1 {
		t.Errorf("expected 1 detector, got %d", len(detector.detectors))
	}

	if _, ok := detector.detectors[RuntimeDocker]; !ok {
		t.Error("Docker detector not registered")
	}
}

func TestMultiRuntimeDetector_GetRuntime(t *testing.T) {
	detector := NewDetector(nil)

	// In non-container environment, should return unknown
	runtime := detector.GetRuntime()

	if runtime != RuntimeUnknown {
		t.Errorf("expected RuntimeUnknown in non-container env, got %v", runtime)
	}
}

func TestMultiRuntimeDetector_IsContainer(t *testing.T) {
	detector := NewDetector(nil)

	// In normal test environment (not container), should return false
	isContainer := detector.IsContainer()

	if isContainer {
		t.Error("expected false in non-container environment")
	}
}

func TestMultiRuntimeDetector_Cache(t *testing.T) {
	detector := NewDetector(nil)

	// Initially cache should be invalid
	if detector.isCacheValid() {
		t.Error("expected cache to be invalid initially")
	}

	// Set cache
	detector.mu.Lock()
	detector.cache = &Metadata{
		Runtime:    RuntimeDocker,
		DetectedAt: time.Now(),
	}
	detector.cacheTime = time.Now()
	detector.mu.Unlock()

	// Cache should be valid now
	if !detector.isCacheValid() {
		t.Error("expected cache to be valid")
	}

	// Clear cache
	detector.ClearCache()

	// Cache should be invalid again
	if detector.isCacheValid() {
		t.Error("expected cache to be invalid after clear")
	}
}

func TestMultiRuntimeDetector_CacheExpiration(t *testing.T) {
	config := &Config{
		Timeout:          5 * time.Second,
		EnableDocker:     true,
		EnableContainerd: true,
		CacheDuration:    100 * time.Millisecond, // Very short for testing
	}

	detector := NewDetector(config)

	// Set cache
	detector.mu.Lock()
	detector.cache = &Metadata{
		Runtime:    RuntimeDocker,
		DetectedAt: time.Now(),
	}
	detector.cacheTime = time.Now()
	detector.mu.Unlock()

	// Cache should be valid immediately
	if !detector.isCacheValid() {
		t.Error("expected cache to be valid immediately")
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return !detector.isCacheValid(), nil
	}); err != nil {
		t.Fatalf("expected cache to be invalid after expiration: %v", err)
	}

	// Cache should be invalid now
	if detector.isCacheValid() {
		t.Error("expected cache to be invalid after expiration")
	}
}

func TestGetDefaultDetector(t *testing.T) {
	detector1 := GetDefaultDetector()
	detector2 := GetDefaultDetector()

	// Should return the same instance
	if detector1 != detector2 {
		t.Error("GetDefaultDetector should return the same instance")
	}
}

func TestDockerDetector_New(t *testing.T) {
	detector := NewDockerDetector(nil)

	if detector == nil {
		t.Fatal("NewDockerDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}
}

func TestDockerDetector_GetRuntime(t *testing.T) {
	detector := NewDockerDetector(nil)

	if detector.GetRuntime() != RuntimeDocker {
		t.Error("expected RuntimeDocker")
	}
}

func TestContainerdDetector_New(t *testing.T) {
	detector := NewContainerdDetector(nil)

	if detector == nil {
		t.Fatal("NewContainerdDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}
}

func TestContainerdDetector_GetRuntime(t *testing.T) {
	detector := NewContainerdDetector(nil)

	if detector.GetRuntime() != RuntimeContainerd {
		t.Error("expected RuntimeContainerd")
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"0123456789abcdef", true},
		{"0123456789ABCDEF", true},
		{"0123456789abcdefg", false},
		{"not-hex", false},
		{"", true}, // Empty string is technically valid hex
	}

	for _, tt := range tests {
		got := isHexString(tt.input)
		if got != tt.expected {
			t.Errorf("isHexString(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestPortMapping(t *testing.T) {
	port := PortMapping{
		ContainerPort: 8080,
		HostPort:      80,
		Protocol:      "tcp",
		HostIP:        "0.0.0.0",
	}

	if port.ContainerPort != 8080 {
		t.Errorf("unexpected container port: %d", port.ContainerPort)
	}

	if port.HostPort != 80 {
		t.Errorf("unexpected host port: %d", port.HostPort)
	}
}

func TestVolumeMount(t *testing.T) {
	mount := VolumeMount{
		Source:      "/host/path",
		Destination: "/container/path",
		Mode:        "rw",
		Type:        "bind",
	}

	if mount.Source != "/host/path" {
		t.Errorf("unexpected source: %s", mount.Source)
	}

	if mount.Mode != "rw" {
		t.Errorf("unexpected mode: %s", mount.Mode)
	}
}

func TestMetadata(t *testing.T) {
	now := time.Now()
	metadata := Metadata{
		Runtime:       RuntimeDocker,
		Version:       "20.10.0",
		ContainerID:   "abc123def456",
		ContainerName: "test-container",
		ImageID:       "sha256:abc123",
		ImageName:     "nginx:latest",
		ImageDigest:   "sha256:digest123",
		Labels:        map[string]string{"env": "test"},
		Env:           map[string]string{"FOO": "bar"},
		Hostname:      "test-host",
		NetworkMode:   "bridge",
		IPAddress:     "172.17.0.2",
		Ports: []PortMapping{
			{ContainerPort: 80, HostPort: 8080, Protocol: "tcp", HostIP: "0.0.0.0"},
		},
		Volumes: []VolumeMount{
			{Source: "/host", Destination: "/container", Mode: "rw", Type: "bind"},
		},
		CgroupPath: "/docker/abc123",
		PID:        1234,
		CreatedAt:  now,
		StartedAt:  now,
		DetectedAt: now,
	}

	if metadata.Runtime != RuntimeDocker {
		t.Errorf("unexpected runtime: %v", metadata.Runtime)
	}
	if metadata.Version != "20.10.0" {
		t.Errorf("unexpected version: %s", metadata.Version)
	}
	if metadata.ContainerID != "abc123def456" {
		t.Errorf("unexpected container ID: %s", metadata.ContainerID)
	}
	if metadata.ContainerName != "test-container" {
		t.Errorf("unexpected container name: %s", metadata.ContainerName)
	}
	if metadata.ImageID != "sha256:abc123" {
		t.Errorf("unexpected image ID: %s", metadata.ImageID)
	}
	if metadata.ImageName != "nginx:latest" {
		t.Errorf("unexpected image name: %s", metadata.ImageName)
	}
	if metadata.ImageDigest != "sha256:digest123" {
		t.Errorf("unexpected image digest: %s", metadata.ImageDigest)
	}
	if metadata.Labels["env"] != "test" {
		t.Errorf("unexpected label: %v", metadata.Labels)
	}
	if metadata.Env["FOO"] != "bar" {
		t.Errorf("unexpected env: %v", metadata.Env)
	}
	if metadata.Hostname != "test-host" {
		t.Errorf("unexpected hostname: %s", metadata.Hostname)
	}
	if metadata.NetworkMode != "bridge" {
		t.Errorf("unexpected network mode: %s", metadata.NetworkMode)
	}
	if metadata.IPAddress != "172.17.0.2" {
		t.Errorf("unexpected IP address: %s", metadata.IPAddress)
	}
	if len(metadata.Ports) != 1 {
		t.Errorf("unexpected ports count: %d", len(metadata.Ports))
	}
	if len(metadata.Volumes) != 1 {
		t.Errorf("unexpected volumes count: %d", len(metadata.Volumes))
	}
	if metadata.CgroupPath != "/docker/abc123" {
		t.Errorf("unexpected cgroup path: %s", metadata.CgroupPath)
	}
	if metadata.PID != 1234 {
		t.Errorf("unexpected PID: %d", metadata.PID)
	}
}

func TestContainerInfo(t *testing.T) {
	info := Info{
		State:        "running",
		Status:       "Up 2 hours",
		RestartCount: 3,
		OOMKilled:    false,
		ExitCode:     0,
		Resources: &ResourceLimits{
			CPUShares:         1024,
			CPUQuota:          100000,
			CPUPeriod:         100000,
			MemoryLimit:       1073741824,
			MemoryReservation: 536870912,
			MemorySwap:        2147483648,
			PidsLimit:         100,
		},
		HealthCheck: &HealthStatus{
			Status:        "healthy",
			FailingStreak: 0,
			Log: []HealthCheckResult{
				{
					Start:    time.Now(),
					End:      time.Now(),
					ExitCode: 0,
					Output:   "OK",
				},
			},
		},
	}

	if info.State != "running" {
		t.Errorf("unexpected state: %s", info.State)
	}
	if info.Status != "Up 2 hours" {
		t.Errorf("unexpected status: %s", info.Status)
	}
	if info.RestartCount != 3 {
		t.Errorf("unexpected restart count: %d", info.RestartCount)
	}
	if info.OOMKilled {
		t.Error("expected OOMKilled to be false")
	}
	if info.ExitCode != 0 {
		t.Errorf("unexpected exit code: %d", info.ExitCode)
	}
}

func TestResourceLimits(t *testing.T) {
	limits := ResourceLimits{
		CPUShares:         1024,
		CPUQuota:          100000,
		CPUPeriod:         100000,
		MemoryLimit:       1073741824, // 1GB
		MemoryReservation: 536870912,  // 512MB
		MemorySwap:        2147483648, // 2GB
		PidsLimit:         100,
	}

	if limits.CPUShares != 1024 {
		t.Errorf("unexpected CPU shares: %d", limits.CPUShares)
	}
	if limits.CPUQuota != 100000 {
		t.Errorf("unexpected CPU quota: %d", limits.CPUQuota)
	}
	if limits.CPUPeriod != 100000 {
		t.Errorf("unexpected CPU period: %d", limits.CPUPeriod)
	}
	if limits.MemoryLimit != 1073741824 {
		t.Errorf("unexpected memory limit: %d", limits.MemoryLimit)
	}
	if limits.MemoryReservation != 536870912 {
		t.Errorf("unexpected memory reservation: %d", limits.MemoryReservation)
	}
	if limits.MemorySwap != 2147483648 {
		t.Errorf("unexpected memory swap: %d", limits.MemorySwap)
	}
	if limits.PidsLimit != 100 {
		t.Errorf("unexpected pids limit: %d", limits.PidsLimit)
	}
}

func TestHealthStatus(t *testing.T) {
	now := time.Now()
	status := HealthStatus{
		Status:        "healthy",
		FailingStreak: 0,
		Log: []HealthCheckResult{
			{
				Start:    now,
				End:      now.Add(100 * time.Millisecond),
				ExitCode: 0,
				Output:   "Health check passed",
			},
		},
	}

	if status.Status != "healthy" {
		t.Errorf("unexpected status: %s", status.Status)
	}
	if status.FailingStreak != 0 {
		t.Errorf("unexpected failing streak: %d", status.FailingStreak)
	}
	if len(status.Log) != 1 {
		t.Errorf("unexpected log count: %d", len(status.Log))
	}
	if status.Log[0].Output != "Health check passed" {
		t.Errorf("unexpected output: %s", status.Log[0].Output)
	}
}

func TestHealthCheckResult(t *testing.T) {
	start := time.Now()
	end := start.Add(50 * time.Millisecond)

	result := HealthCheckResult{
		Start:    start,
		End:      end,
		ExitCode: 0,
		Output:   "OK",
	}

	if result.Start != start {
		t.Errorf("unexpected start: %v", result.Start)
	}
	if result.End != end {
		t.Errorf("unexpected end: %v", result.End)
	}
	if result.ExitCode != 0 {
		t.Errorf("unexpected exit code: %d", result.ExitCode)
	}
	if result.Output != "OK" {
		t.Errorf("unexpected output: %s", result.Output)
	}
}

func TestHealthCheckResult_Failed(t *testing.T) {
	start := time.Now()
	end := start.Add(100 * time.Millisecond)

	result := HealthCheckResult{
		Start:    start,
		End:      end,
		ExitCode: 1,
		Output:   "Connection refused",
	}

	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
	if result.Output != "Connection refused" {
		t.Errorf("unexpected output: %s", result.Output)
	}
}

func TestConfig(t *testing.T) {
	config := Config{
		Timeout:              10 * time.Second,
		EnableDocker:         true,
		EnableContainerd:     false,
		EnableCRIO:           false,
		EnablePodman:         true,
		DockerSocketPath:     "/custom/docker.sock",
		ContainerdSocketPath: "/custom/containerd.sock",
		CacheDuration:        10 * time.Minute,
	}

	if config.Timeout != 10*time.Second {
		t.Errorf("unexpected timeout: %v", config.Timeout)
	}
	if !config.EnableDocker {
		t.Error("expected Docker to be enabled")
	}
	if config.EnableContainerd {
		t.Error("expected containerd to be disabled")
	}
	if config.EnableCRIO {
		t.Error("expected CRI-O to be disabled")
	}
	if !config.EnablePodman {
		t.Error("expected Podman to be enabled")
	}
	if config.DockerSocketPath != "/custom/docker.sock" {
		t.Errorf("unexpected Docker socket path: %s", config.DockerSocketPath)
	}
	if config.ContainerdSocketPath != "/custom/containerd.sock" {
		t.Errorf("unexpected containerd socket path: %s", config.ContainerdSocketPath)
	}
	if config.CacheDuration != 10*time.Minute {
		t.Errorf("unexpected cache duration: %v", config.CacheDuration)
	}
}

func TestRuntimeValues(t *testing.T) {
	// Test all runtime values are distinct
	runtimes := []Runtime{RuntimeUnknown, RuntimeDocker, RuntimeContainerd, RuntimeCRIO, RuntimePodman}
	seen := make(map[Runtime]bool)

	for _, r := range runtimes {
		if seen[r] {
			t.Errorf("duplicate runtime value: %v", r)
		}
		seen[r] = true
	}
}

func TestDockerDetector_CustomConfig(t *testing.T) {
	config := &Config{
		Timeout:          30 * time.Second,
		DockerSocketPath: "/custom/docker.sock",
	}

	detector := NewDockerDetector(config)

	if detector.config.Timeout != 30*time.Second {
		t.Errorf("unexpected timeout: %v", detector.config.Timeout)
	}
	if detector.config.DockerSocketPath != "/custom/docker.sock" {
		t.Errorf("unexpected socket path: %s", detector.config.DockerSocketPath)
	}
}

func TestContainerdDetector_CustomConfig(t *testing.T) {
	config := &Config{
		Timeout:              30 * time.Second,
		ContainerdSocketPath: "/custom/containerd.sock",
	}

	detector := NewContainerdDetector(config)

	if detector.config.Timeout != 30*time.Second {
		t.Errorf("unexpected timeout: %v", detector.config.Timeout)
	}
	if detector.config.ContainerdSocketPath != "/custom/containerd.sock" {
		t.Errorf("unexpected socket path: %s", detector.config.ContainerdSocketPath)
	}
}

func TestNewDetector_DisableAll(t *testing.T) {
	config := &Config{
		EnableDocker:     false,
		EnableContainerd: false,
		EnableCRIO:       false,
		EnablePodman:     false,
	}

	detector := NewDetector(config)

	if len(detector.detectors) != 0 {
		t.Errorf("expected 0 detectors, got %d", len(detector.detectors))
	}
}

func TestMultiRuntimeDetector_DetectNoContainer(t *testing.T) {
	detector := NewDetector(nil)

	// In non-container environment, should return error
	_, err := detector.Detect()

	if err == nil {
		t.Error("expected error in non-container environment")
	}
}

func TestDockerDetector_IsContainer_NoContainer(t *testing.T) {
	detector := NewDockerDetector(nil)

	// In normal test environment, should return false
	if detector.IsContainer() {
		t.Error("expected IsContainer to return false in non-container env")
	}
}

func TestContainerdDetector_IsContainer_NoContainer(t *testing.T) {
	detector := NewContainerdDetector(nil)

	// In normal test environment, should return false
	if detector.IsContainer() {
		t.Error("expected IsContainer to return false in non-container env")
	}
}

func TestDockerDetector_Detect_NoContainer(t *testing.T) {
	detector := NewDockerDetector(nil)

	// In non-container environment, should return error
	_, err := detector.Detect()

	if err == nil {
		t.Error("expected error in non-container environment")
	}
}

func TestContainerdDetector_Detect_NoContainer(t *testing.T) {
	detector := NewContainerdDetector(nil)

	// In non-container environment, should return error
	_, err := detector.Detect()

	if err == nil {
		t.Error("expected error in non-container environment")
	}
}

func TestConvenienceFunction_IsContainer(t *testing.T) {
	// In normal test environment, should return false
	if IsContainer() {
		t.Error("expected IsContainer() to return false")
	}
}

func TestConvenienceFunction_GetRuntime(t *testing.T) {
	// In normal test environment, should return unknown
	if GetRuntime() != RuntimeUnknown {
		t.Errorf("expected RuntimeUnknown, got %v", GetRuntime())
	}
}

func TestConvenienceFunction_Detect(t *testing.T) {
	// In non-container environment, should return error
	_, err := Detect()

	if err == nil {
		t.Error("expected error in non-container environment")
	}
}

func TestParseJSONFile_InvalidPath(t *testing.T) {
	var result map[string]interface{}
	err := parseJSONFile("/nonexistent/path.json", &result)

	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadFile_InvalidPath(t *testing.T) {
	content := readFile("/nonexistent/path")

	if content != "" {
		t.Errorf("expected empty string for nonexistent file, got: %s", content)
	}
}

func TestDockerDetector_parseDockerJSON(t *testing.T) {
	detector := NewDockerDetector(nil)

	// This should return minimal info (placeholder implementation)
	info, err := detector.parseDockerJSON("test-container-id")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil Info")
	}
	if info.State != "running" {
		t.Errorf("expected state 'running', got '%s'", info.State)
	}
}

func TestPortMapping_AllFields(t *testing.T) {
	tests := []struct {
		name      string
		mapping   PortMapping
		wantPort  int
		wantHost  int
		wantProto string
	}{
		{
			name: "TCP port",
			mapping: PortMapping{
				ContainerPort: 80,
				HostPort:      8080,
				Protocol:      "tcp",
				HostIP:        "0.0.0.0",
			},
			wantPort:  80,
			wantHost:  8080,
			wantProto: "tcp",
		},
		{
			name: "UDP port",
			mapping: PortMapping{
				ContainerPort: 53,
				HostPort:      5353,
				Protocol:      "udp",
				HostIP:        "127.0.0.1",
			},
			wantPort:  53,
			wantHost:  5353,
			wantProto: "udp",
		},
		{
			name: "SCTP port",
			mapping: PortMapping{
				ContainerPort: 3868,
				HostPort:      3869,
				Protocol:      "sctp",
				HostIP:        "0.0.0.0",
			},
			wantPort:  3868,
			wantHost:  3869,
			wantProto: "sctp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mapping.ContainerPort != tt.wantPort {
				t.Errorf("ContainerPort = %d, want %d", tt.mapping.ContainerPort, tt.wantPort)
			}
			if tt.mapping.HostPort != tt.wantHost {
				t.Errorf("HostPort = %d, want %d", tt.mapping.HostPort, tt.wantHost)
			}
			if tt.mapping.Protocol != tt.wantProto {
				t.Errorf("Protocol = %s, want %s", tt.mapping.Protocol, tt.wantProto)
			}
		})
	}
}

func TestVolumeMount_AllTypes(t *testing.T) {
	tests := []struct {
		name  string
		mount VolumeMount
	}{
		{
			name: "bind mount",
			mount: VolumeMount{
				Source:      "/host/path",
				Destination: "/container/path",
				Mode:        "rw",
				Type:        "bind",
			},
		},
		{
			name: "volume mount",
			mount: VolumeMount{
				Source:      "my-volume",
				Destination: "/data",
				Mode:        "rw",
				Type:        "volume",
			},
		},
		{
			name: "tmpfs mount",
			mount: VolumeMount{
				Source:      "",
				Destination: "/tmp",
				Mode:        "rw",
				Type:        "tmpfs",
			},
		},
		{
			name: "readonly bind mount",
			mount: VolumeMount{
				Source:      "/host/readonly",
				Destination: "/container/readonly",
				Mode:        "ro",
				Type:        "bind",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mount.Destination == "" {
				t.Error("Destination should not be empty")
			}
			if tt.mount.Mode != "rw" && tt.mount.Mode != "ro" {
				t.Errorf("Mode should be 'rw' or 'ro', got %s", tt.mount.Mode)
			}
			if tt.mount.Type != "bind" && tt.mount.Type != "volume" && tt.mount.Type != "tmpfs" {
				t.Errorf("Type should be 'bind', 'volume', or 'tmpfs', got %s", tt.mount.Type)
			}
		})
	}
}

func TestMultiRuntimeDetector_CacheWithDetect(t *testing.T) {
	config := &Config{
		Timeout:          5 * time.Second,
		EnableDocker:     true,
		EnableContainerd: true,
		CacheDuration:    1 * time.Hour,
	}

	detector := NewDetector(config)

	// Manually set a cached value
	cachedMetadata := &Metadata{
		Runtime:    RuntimeDocker,
		DetectedAt: time.Now(),
		Labels:     make(map[string]string),
	}

	detector.mu.Lock()
	detector.cache = cachedMetadata
	detector.cacheTime = time.Now()
	detector.mu.Unlock()

	// Detect should return cached value
	result, err := detector.Detect()
	if err != nil {
		t.Errorf("unexpected error with cache: %v", err)
	}

	if result != cachedMetadata {
		t.Error("expected cached metadata to be returned")
	}
}

func TestIsHexString_EdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"", true},                 // Empty string
		{"0", true},                // Single char
		{"a", true},                // Single hex
		{"A", true},                // Uppercase single hex
		{"g", false},               // Just past 'f'
		{"G", false},               // Just past 'F'
		{"abcdef0123456789", true}, // All valid hex chars
		{"ABCDEF0123456789", true}, // All uppercase
		{"AbCdEf0123456789", true}, // Mixed case
		{"abc123xyz", false},       // Contains invalid chars
		{"123-456", false},         // Contains dash
		{"abc 123", false},         // Contains space
		{"abc123def456789012345678901234567890123456789012345678901234567890", true}, // 64 chars (typical container ID length)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isHexString(tt.input)
			if got != tt.expected {
				t.Errorf("isHexString(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHealthStatus_Unhealthy(t *testing.T) {
	now := time.Now()
	status := HealthStatus{
		Status:        "unhealthy",
		FailingStreak: 5,
		Log: []HealthCheckResult{
			{
				Start:    now.Add(-4 * time.Minute),
				End:      now.Add(-4 * time.Minute).Add(100 * time.Millisecond),
				ExitCode: 1,
				Output:   "Connection refused",
			},
			{
				Start:    now.Add(-3 * time.Minute),
				End:      now.Add(-3 * time.Minute).Add(100 * time.Millisecond),
				ExitCode: 1,
				Output:   "Connection refused",
			},
			{
				Start:    now.Add(-2 * time.Minute),
				End:      now.Add(-2 * time.Minute).Add(100 * time.Millisecond),
				ExitCode: 1,
				Output:   "Connection refused",
			},
		},
	}

	if status.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %s", status.Status)
	}
	if status.FailingStreak != 5 {
		t.Errorf("expected failing streak 5, got %d", status.FailingStreak)
	}
	if len(status.Log) != 3 {
		t.Errorf("expected 3 log entries, got %d", len(status.Log))
	}
}

func TestContainerInfo_OOMKilled(t *testing.T) {
	info := Info{
		State:        "exited",
		Status:       "Exited (137)",
		RestartCount: 1,
		OOMKilled:    true,
		ExitCode:     137,
	}

	if !info.OOMKilled {
		t.Error("expected OOMKilled to be true")
	}
	if info.ExitCode != 137 {
		t.Errorf("expected exit code 137, got %d", info.ExitCode)
	}
	if info.State != "exited" {
		t.Errorf("expected state 'exited', got %s", info.State)
	}
}

func TestContainerInfo_NilResources(t *testing.T) {
	info := Info{
		State:     "running",
		Resources: nil,
	}

	if info.Resources != nil {
		t.Error("expected nil Resources")
	}
}

func TestContainerInfo_NilHealthCheck(t *testing.T) {
	info := Info{
		State:       "running",
		HealthCheck: nil,
	}

	if info.HealthCheck != nil {
		t.Error("expected nil HealthCheck")
	}
}
