package container

import (
	"testing"
	"time"
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

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

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
