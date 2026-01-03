package statemgmt

import (
	"os/exec"
	"testing"
)

// ============================================================================
// Docker Container Module Tests
// ============================================================================

func TestNewDockerContainerModule(t *testing.T) {
	m := NewDockerContainerModule()

	if m.Name() != "docker_container" {
		t.Errorf("expected name 'docker_container', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"running", "stopped", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestDockerContainerModule_Check_MissingName(t *testing.T) {
	m := NewDockerContainerModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "docker_container",
		State:  "running",
		Parameters: map[string]interface{}{
			"image": "nginx:latest",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestDockerContainerModule_Check_MissingImage(t *testing.T) {
	m := NewDockerContainerModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "docker_container",
		State:  "running",
		Parameters: map[string]interface{}{
			"name": "test-container",
		},
	}

	// This will fail at Check if Docker is available but image is missing
	// If Docker is not available, it will fail with docker not available error
	_, err := m.Check(nil, decl)
	if err == nil {
		t.Error("expected error for missing image parameter")
	}
}

func TestDockerContainerModule_Check_AbsentNoImage(t *testing.T) {
	m := NewDockerContainerModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "docker_container",
		State:  "absent",
		Parameters: map[string]interface{}{
			"name": "test-container",
		},
	}

	// For absent state, image is not required
	// Check will fail if docker is not available, which is expected
	_, err := m.Check(nil, decl)
	if err != nil && err.Error() != "docker is not available: exec: \"docker\": executable file not found in $PATH" {
		// If it's any error other than docker not available, that's fine for this test
		// The point is that we don't get "image parameter is required" error
		if err.Error() == "image parameter is required for state absent" {
			t.Error("image should not be required for absent state")
		}
	}
}

// ============================================================================
// Docker Image Module Tests
// ============================================================================

func TestNewDockerImageModule(t *testing.T) {
	m := NewDockerImageModule()

	if m.Name() != "docker_image" {
		t.Errorf("expected name 'docker_image', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestDockerImageModule_Check_MissingName(t *testing.T) {
	m := NewDockerImageModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "docker_image",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

// ============================================================================
// Docker Network Module Tests
// ============================================================================

func TestNewDockerNetworkModule(t *testing.T) {
	m := NewDockerNetworkModule()

	if m.Name() != "docker_network" {
		t.Errorf("expected name 'docker_network', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestDockerNetworkModule_Check_MissingName(t *testing.T) {
	m := NewDockerNetworkModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "docker_network",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

// ============================================================================
// Docker Volume Module Tests
// ============================================================================

func TestNewDockerVolumeModule(t *testing.T) {
	m := NewDockerVolumeModule()

	if m.Name() != "docker_volume" {
		t.Errorf("expected name 'docker_volume', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestDockerVolumeModule_Check_MissingName(t *testing.T) {
	m := NewDockerVolumeModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "docker_volume",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

// ============================================================================
// Podman Container Module Tests
// ============================================================================

func TestNewPodmanContainerModule(t *testing.T) {
	m := NewPodmanContainerModule()

	if m.Name() != "podman_container" {
		t.Errorf("expected name 'podman_container', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"running", "stopped", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestPodmanContainerModule_Check_MissingName(t *testing.T) {
	m := NewPodmanContainerModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "podman_container",
		State:  "running",
		Parameters: map[string]interface{}{
			"image": "nginx:latest",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestPodmanContainerModule_Check_MissingImage(t *testing.T) {
	m := NewPodmanContainerModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "podman_container",
		State:  "running",
		Parameters: map[string]interface{}{
			"name": "test-container",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil {
		t.Error("expected error for missing image parameter")
	}
}

// ============================================================================
// Podman Image Module Tests
// ============================================================================

func TestNewPodmanImageModule(t *testing.T) {
	m := NewPodmanImageModule()

	if m.Name() != "podman_image" {
		t.Errorf("expected name 'podman_image', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestPodmanImageModule_Check_MissingName(t *testing.T) {
	m := NewPodmanImageModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "podman_image",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

// ============================================================================
// Podman Network Module Tests
// ============================================================================

func TestNewPodmanNetworkModule(t *testing.T) {
	m := NewPodmanNetworkModule()

	if m.Name() != "podman_network" {
		t.Errorf("expected name 'podman_network', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestPodmanNetworkModule_Check_MissingName(t *testing.T) {
	m := NewPodmanNetworkModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "podman_network",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

// ============================================================================
// Podman Volume Module Tests
// ============================================================================

func TestNewPodmanVolumeModule(t *testing.T) {
	m := NewPodmanVolumeModule()

	if m.Name() != "podman_volume" {
		t.Errorf("expected name 'podman_volume', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestPodmanVolumeModule_Check_MissingName(t *testing.T) {
	m := NewPodmanVolumeModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "podman_volume",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

// ============================================================================
// Container Runtime Detector Tests
// ============================================================================

func TestDetectContainerRuntime(t *testing.T) {
	runtime := DetectContainerRuntime()

	// The result depends on what's installed on the test machine
	// Just verify it returns a valid enum value
	switch runtime {
	case ContainerRuntimeDocker, ContainerRuntimePodman, ContainerRuntimeUnknown:
		// Valid
	default:
		t.Errorf("unexpected runtime: %s", runtime)
	}
}

func TestGetContainerRuntimeVersion_Unknown(t *testing.T) {
	_, err := GetContainerRuntimeVersion(ContainerRuntimeUnknown)
	if err == nil {
		t.Error("expected error for unknown runtime")
	}
}

func TestGetContainerRuntimeVersion_Docker(t *testing.T) {
	// Check if docker is available
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		t.Skip("docker is not available")
	}

	version, err := GetContainerRuntimeVersion(ContainerRuntimeDocker)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if version == "" {
		t.Error("expected non-empty version")
	}
}

func TestGetContainerRuntimeVersion_Podman(t *testing.T) {
	// Check if podman is available
	cmd := exec.Command("podman", "version", "--format", "{{.Version}}")
	if err := cmd.Run(); err != nil {
		t.Skip("podman is not available")
	}

	version, err := GetContainerRuntimeVersion(ContainerRuntimePodman)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if version == "" {
		t.Error("expected non-empty version")
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestGetEnvParameters(t *testing.T) {
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{
			"env": map[string]interface{}{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
	}

	envs := getEnvParameters(decl)
	if len(envs) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(envs))
	}
	if envs["FOO"] != "bar" {
		t.Errorf("expected FOO=bar, got %s", envs["FOO"])
	}
	if envs["BAZ"] != "qux" {
		t.Errorf("expected BAZ=qux, got %s", envs["BAZ"])
	}
}

func TestGetEnvParameters_Empty(t *testing.T) {
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{},
	}

	envs := getEnvParameters(decl)
	if len(envs) != 0 {
		t.Errorf("expected 0 env vars, got %d", len(envs))
	}
}

func TestGetDriverOpts(t *testing.T) {
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{
			"opts": map[string]interface{}{
				"type":   "nfs",
				"device": ":/data",
			},
		},
	}

	opts := getDriverOpts(decl)
	if len(opts) != 2 {
		t.Errorf("expected 2 opts, got %d", len(opts))
	}
	if opts["type"] != "nfs" {
		t.Errorf("expected type=nfs, got %s", opts["type"])
	}
}

func TestGetDriverOpts_Empty(t *testing.T) {
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{},
	}

	opts := getDriverOpts(decl)
	if len(opts) != 0 {
		t.Errorf("expected 0 opts, got %d", len(opts))
	}
}

// ============================================================================
// Integration Tests (require Docker/Podman)
// ============================================================================

func TestDockerContainerModule_Integration(t *testing.T) {
	// Check if docker is available
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		t.Skip("docker is not available")
	}

	m := NewDockerContainerModule()

	// Test checking a non-existent container
	decl := &StateDeclaration{
		ID:     "test",
		Module: "docker_container",
		State:  "absent",
		Parameters: map[string]interface{}{
			"name": "kscore-test-nonexistent-12345",
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected container to not exist")
	}
	if !result.Matches {
		t.Error("expected state to match (absent)")
	}
}

func TestDockerImageModule_Integration(t *testing.T) {
	// Check if docker is available
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		t.Skip("docker is not available")
	}

	m := NewDockerImageModule()

	// Test checking a common image that might or might not exist
	decl := &StateDeclaration{
		ID:     "test",
		Module: "docker_image",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "busybox",
			"tag":  "nonexistent-tag-12345",
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	// This specific tag shouldn't exist
	if result.Present {
		t.Log("busybox:nonexistent-tag-12345 exists (unexpected but not an error)")
	}
}

func TestPodmanContainerModule_Integration(t *testing.T) {
	// Check if podman is available
	cmd := exec.Command("podman", "version", "--format", "{{.Version}}")
	if err := cmd.Run(); err != nil {
		t.Skip("podman is not available")
	}

	m := NewPodmanContainerModule()

	// Test checking a non-existent container
	decl := &StateDeclaration{
		ID:     "test",
		Module: "podman_container",
		State:  "absent",
		Parameters: map[string]interface{}{
			"name": "kscore-test-nonexistent-12345",
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected container to not exist")
	}
	if !result.Matches {
		t.Error("expected state to match (absent)")
	}
}
