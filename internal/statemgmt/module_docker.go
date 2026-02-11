package statemgmt

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// ============================================================================
// Docker Container Module
// ============================================================================

// DockerContainerModule manages Docker containers.
type DockerContainerModule struct {
	*BaseModule
}

// NewDockerContainerModule creates a new DockerContainerModule.
func NewDockerContainerModule() *DockerContainerModule {
	return &DockerContainerModule{
		BaseModule: NewBaseModule("docker_container", []string{"running", "stopped", "absent"}),
	}
}

// Check checks if a Docker container matches the desired state.
func (m *DockerContainerModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	image := getStringParameter(decl, "image", "")
	if image == "" && decl.State != "absent" {
		return nil, fmt.Errorf("image parameter is required for state %s", decl.State)
	}

	// Check if docker is available
	if err := m.checkDockerAvailable(ctx); err != nil {
		return nil, err
	}

	// Get container info
	info, exists := m.getContainerInfo(ctx, name)

	result := &ModuleCheckResult{
		Present:  exists,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.CurrentState = info["State"].(string)
		result.Metadata["container_id"] = info["Id"]
		result.Metadata["image"] = info["Image"]
	}

	switch decl.State {
	case "running":
		if exists && result.CurrentState == "running" {
			result.Matches = true
		}
	case "stopped":
		if exists && result.CurrentState == "exited" {
			result.Matches = true
		}
	case "absent":
		if !exists {
			result.Matches = true
		}
	}

	return result, nil
}

// Apply applies the desired Docker container state.
func (m *DockerContainerModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Container '%s' already in desired state", getStringParameter(decl, "name", "")),
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	image := getStringParameter(decl, "image", "")

	switch decl.State {
	case "running":
		return m.ensureRunning(ctx, name, image, decl)
	case "stopped":
		return m.ensureStopped(ctx, name, image, decl)
	case "absent":
		return m.ensureAbsent(ctx, name, decl)
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: false,
		Comment: fmt.Sprintf("Unknown state: %s", decl.State),
	}, nil
}

// Test runs the module in test mode.
func (m *DockerContainerModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

func (m *DockerContainerModule) checkDockerAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker is not available: %w", err)
	}
	return nil
}

func (m *DockerContainerModule) getContainerInfo(ctx context.Context, name string) (map[string]interface{}, bool) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--type", "container", name)
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var containers []map[string]interface{}
	if err := json.Unmarshal(output, &containers); err != nil {
		return nil, false
	}

	if len(containers) == 0 {
		return nil, false
	}

	info := containers[0]
	// Extract state
	if stateMap, ok := info["State"].(map[string]interface{}); ok {
		if status, ok := stateMap["Status"].(string); ok {
			info["State"] = status
		}
	}

	return info, true
}

func (m *DockerContainerModule) ensureRunning(ctx context.Context, name, image string, decl *StateDeclaration) (*StateResult, error) {
	// Check if container exists
	info, exists := m.getContainerInfo(ctx, name)

	if exists {
		// Container exists, check if running
		state := info["State"].(string)
		if state == "running" {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: true,
				Changed: false,
				Comment: fmt.Sprintf("Container '%s' is already running", name),
			}, nil
		}

		// Start the container
		cmd := exec.CommandContext(ctx, "docker", "start", name)
		if err := cmd.Run(); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to start container: %v", err),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Started container '%s'", name),
		}, nil
	}

	// Container doesn't exist, create and start it
	args := []string{"run", "-d", "--name", name}

	// Add ports
	if ports := getStringSliceParameter(decl, "ports"); len(ports) > 0 {
		for _, port := range ports {
			args = append(args, "-p", port)
		}
	}

	// Add volumes
	if volumes := getStringSliceParameter(decl, "volumes"); len(volumes) > 0 {
		for _, vol := range volumes {
			args = append(args, "-v", vol)
		}
	}

	// Add environment variables
	if envs := getEnvParameters(decl); len(envs) > 0 {
		for k, v := range envs {
			args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Add networks
	if network := getStringParameter(decl, "network", ""); network != "" {
		args = append(args, "--network", network)
	}

	// Add restart policy
	if restart := getStringParameter(decl, "restart", ""); restart != "" {
		args = append(args, "--restart", restart)
	}

	// Add image
	args = append(args, image)

	// Add command if specified
	if command := getStringParameter(decl, "command", ""); command != "" {
		args = append(args, strings.Fields(command)...)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Failed to create container: %v - %s", err, string(output)),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Created and started container '%s'", name),
	}, nil
}

func (m *DockerContainerModule) ensureStopped(ctx context.Context, name, image string, decl *StateDeclaration) (*StateResult, error) {
	info, exists := m.getContainerInfo(ctx, name)

	if !exists {
		// Create container but don't start it
		args := []string{"create", "--name", name, image}
		cmd := exec.CommandContext(ctx, "docker", args...)
		if err := cmd.Run(); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to create container: %v", err),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Created container '%s' (stopped)", name),
		}, nil
	}

	state := info["State"].(string)
	if state == "exited" {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Container '%s' is already stopped", name),
		}, nil
	}

	// Stop the container
	cmd := exec.CommandContext(ctx, "docker", "stop", name)
	if err := cmd.Run(); err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Failed to stop container: %v", err),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Stopped container '%s'", name),
	}, nil
}

func (m *DockerContainerModule) ensureAbsent(ctx context.Context, name string, decl *StateDeclaration) (*StateResult, error) {
	_, exists := m.getContainerInfo(ctx, name)

	if !exists {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Container '%s' does not exist", name),
		}, nil
	}

	// Force remove (stops if running)
	force := getBoolParameter(decl, "force", true)
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)

	cmd := exec.CommandContext(ctx, "docker", args...)
	if err := cmd.Run(); err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Failed to remove container: %v", err),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Removed container '%s'", name),
	}, nil
}

func getEnvParameters(decl *StateDeclaration) map[string]string {
	result := make(map[string]string)
	if env, ok := decl.Parameters["env"].(map[string]interface{}); ok {
		for k, v := range env {
			if vs, ok := v.(string); ok {
				result[k] = vs
			}
		}
	}
	return result
}

// ============================================================================
// Docker Image Module
// ============================================================================

// DockerImageModule manages Docker images.
type DockerImageModule struct {
	*BaseModule
}

// NewDockerImageModule creates a new DockerImageModule.
func NewDockerImageModule() *DockerImageModule {
	return &DockerImageModule{
		BaseModule: NewBaseModule("docker_image", []string{"present", "absent"}),
	}
}

// Check checks if a Docker image matches the desired state.
func (m *DockerImageModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	tag := getStringParameter(decl, "tag", "latest")
	fullName := fmt.Sprintf("%s:%s", name, tag)

	// Check if docker is available
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker is not available: %w", err)
	}

	// Check if image exists
	cmd = exec.CommandContext(ctx, "docker", "image", "inspect", fullName)
	exists := cmd.Run() == nil

	result := &ModuleCheckResult{
		Present:  exists,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.CurrentState = "present"
		result.Metadata["image"] = fullName
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = exists
	case "absent":
		result.Matches = !exists
	}

	return result, nil
}

// Apply applies the desired Docker image state.
func (m *DockerImageModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: "Image already in desired state",
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	tag := getStringParameter(decl, "tag", "latest")
	fullName := fmt.Sprintf("%s:%s", name, tag)

	switch decl.State {
	case "present":
		// Pull image with optional authentication
		authMethod := GetAuthMethodFromDeclaration(decl)
		var output string
		var pullErr error

		if authMethod != "" {
			// Use authenticated pull
			puller := NewImagePuller(nil) //nolint:contextcheck // NewImagePuller constructor doesn't take context
			output, pullErr = puller.PullImage(ctx, fullName, authMethod)
		} else {
			// Standard pull without auth
			cmd := exec.CommandContext(ctx, "docker", "pull", fullName)
			outputBytes, err := cmd.CombinedOutput()
			output = string(outputBytes)
			pullErr = err
		}

		if pullErr != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to pull image: %v - %s", pullErr, output),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Pulled image '%s'", fullName),
		}, nil

	case "absent":
		force := getBoolParameter(decl, "force", false)
		args := []string{"rmi"}
		if force {
			args = append(args, "-f")
		}
		args = append(args, fullName)

		cmd := exec.CommandContext(ctx, "docker", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to remove image: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed image '%s'", fullName),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: false,
		Comment: fmt.Sprintf("Unknown state: %s", decl.State),
	}, nil
}

// Test runs the module in test mode.
func (m *DockerImageModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// ============================================================================
// Docker Network Module
// ============================================================================

// DockerNetworkModule manages Docker networks.
type DockerNetworkModule struct {
	*BaseModule
}

// NewDockerNetworkModule creates a new DockerNetworkModule.
func NewDockerNetworkModule() *DockerNetworkModule {
	return &DockerNetworkModule{
		BaseModule: NewBaseModule("docker_network", []string{"present", "absent"}),
	}
}

// Check checks if a Docker network matches the desired state.
func (m *DockerNetworkModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	// Check if docker is available
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker is not available: %w", err)
	}

	// Check if network exists
	cmd = exec.CommandContext(ctx, "docker", "network", "inspect", name)
	exists := cmd.Run() == nil

	result := &ModuleCheckResult{
		Present:  exists,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = exists
	case "absent":
		result.Matches = !exists
	}

	return result, nil
}

// Apply applies the desired Docker network state.
func (m *DockerNetworkModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: "Network already in desired state",
		}, nil
	}

	name := getStringParameter(decl, "name", "")

	switch decl.State {
	case "present":
		args := []string{"network", "create"}

		driver := getStringParameter(decl, "driver", "bridge")
		args = append(args, "--driver", driver)

		if subnet := getStringParameter(decl, "subnet", ""); subnet != "" {
			args = append(args, "--subnet", subnet)
		}

		if gateway := getStringParameter(decl, "gateway", ""); gateway != "" {
			args = append(args, "--gateway", gateway)
		}

		if ipRange := getStringParameter(decl, "ip_range", ""); ipRange != "" {
			args = append(args, "--ip-range", ipRange)
		}

		args = append(args, name)

		cmd := exec.CommandContext(ctx, "docker", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to create network: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Created network '%s'", name),
		}, nil

	case "absent":
		cmd := exec.CommandContext(ctx, "docker", "network", "rm", name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to remove network: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed network '%s'", name),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: false,
		Comment: fmt.Sprintf("Unknown state: %s", decl.State),
	}, nil
}

// Test runs the module in test mode.
func (m *DockerNetworkModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// ============================================================================
// Docker Volume Module
// ============================================================================

// DockerVolumeModule manages Docker volumes.
type DockerVolumeModule struct {
	*BaseModule
}

// NewDockerVolumeModule creates a new DockerVolumeModule.
func NewDockerVolumeModule() *DockerVolumeModule {
	return &DockerVolumeModule{
		BaseModule: NewBaseModule("docker_volume", []string{"present", "absent"}),
	}
}

// Check checks if a Docker volume matches the desired state.
func (m *DockerVolumeModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	// Check if docker is available
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker is not available: %w", err)
	}

	// Check if volume exists
	cmd = exec.CommandContext(ctx, "docker", "volume", "inspect", name)
	exists := cmd.Run() == nil

	result := &ModuleCheckResult{
		Present:  exists,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = exists
	case "absent":
		result.Matches = !exists
	}

	return result, nil
}

// Apply applies the desired Docker volume state.
func (m *DockerVolumeModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: "Volume already in desired state",
		}, nil
	}

	name := getStringParameter(decl, "name", "")

	switch decl.State {
	case "present":
		args := []string{"volume", "create"}

		if driver := getStringParameter(decl, "driver", ""); driver != "" {
			args = append(args, "--driver", driver)
		}

		// Add driver options
		if opts := getDriverOpts(decl); len(opts) > 0 {
			for k, v := range opts {
				args = append(args, "--opt", fmt.Sprintf("%s=%s", k, v))
			}
		}

		args = append(args, name)

		cmd := exec.CommandContext(ctx, "docker", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to create volume: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Created volume '%s'", name),
		}, nil

	case "absent":
		force := getBoolParameter(decl, "force", false)
		args := []string{"volume", "rm"}
		if force {
			args = append(args, "-f")
		}
		args = append(args, name)

		cmd := exec.CommandContext(ctx, "docker", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to remove volume: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed volume '%s'", name),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: false,
		Comment: fmt.Sprintf("Unknown state: %s", decl.State),
	}, nil
}

// Test runs the module in test mode.
func (m *DockerVolumeModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

func getDriverOpts(decl *StateDeclaration) map[string]string {
	result := make(map[string]string)
	if opts, ok := decl.Parameters["opts"].(map[string]interface{}); ok {
		for k, v := range opts {
			if vs, ok := v.(string); ok {
				result[k] = vs
			}
		}
	}
	return result
}

// ============================================================================
// Podman Container Module
// ============================================================================

// PodmanContainerModule manages Podman containers.
// It uses the same interface as Docker but calls podman commands.
type PodmanContainerModule struct {
	*BaseModule
}

// NewPodmanContainerModule creates a new PodmanContainerModule.
func NewPodmanContainerModule() *PodmanContainerModule {
	return &PodmanContainerModule{
		BaseModule: NewBaseModule("podman_container", []string{"running", "stopped", "absent"}),
	}
}

// Check checks if a Podman container matches the desired state.
func (m *PodmanContainerModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	image := getStringParameter(decl, "image", "")
	if image == "" && decl.State != "absent" {
		return nil, fmt.Errorf("image parameter is required for state %s", decl.State)
	}

	// Check if podman is available
	cmd := exec.CommandContext(ctx, "podman", "version", "--format", "{{.Version}}")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("podman is not available: %w", err)
	}

	// Get container info
	info, exists := m.getContainerInfo(ctx, name)

	result := &ModuleCheckResult{
		Present:  exists,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.CurrentState = info["State"].(string)
		result.Metadata["container_id"] = info["Id"]
		result.Metadata["image"] = info["Image"]
	}

	switch decl.State {
	case "running":
		if exists && result.CurrentState == "running" {
			result.Matches = true
		}
	case "stopped":
		if exists && (result.CurrentState == "exited" || result.CurrentState == "stopped") {
			result.Matches = true
		}
	case "absent":
		if !exists {
			result.Matches = true
		}
	}

	return result, nil
}

func (m *PodmanContainerModule) getContainerInfo(ctx context.Context, name string) (map[string]interface{}, bool) {
	cmd := exec.CommandContext(ctx, "podman", "inspect", "--type", "container", name)
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var containers []map[string]interface{}
	if err := json.Unmarshal(output, &containers); err != nil {
		return nil, false
	}

	if len(containers) == 0 {
		return nil, false
	}

	info := containers[0]
	// Extract state
	if stateMap, ok := info["State"].(map[string]interface{}); ok {
		if status, ok := stateMap["Status"].(string); ok {
			info["State"] = status
		}
	}

	return info, true
}

// Apply applies the desired Podman container state.
func (m *PodmanContainerModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Container '%s' already in desired state", getStringParameter(decl, "name", "")),
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	image := getStringParameter(decl, "image", "")

	switch decl.State {
	case "running":
		return m.ensureRunning(ctx, name, image, decl)
	case "stopped":
		return m.ensureStopped(ctx, name, image, decl)
	case "absent":
		return m.ensureAbsent(ctx, name, decl)
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: false,
		Comment: fmt.Sprintf("Unknown state: %s", decl.State),
	}, nil
}

func (m *PodmanContainerModule) ensureRunning(ctx context.Context, name, image string, decl *StateDeclaration) (*StateResult, error) {
	info, exists := m.getContainerInfo(ctx, name)

	if exists {
		state := info["State"].(string)
		if state == "running" {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: true,
				Changed: false,
				Comment: fmt.Sprintf("Container '%s' is already running", name),
			}, nil
		}

		// Start the container
		cmd := exec.CommandContext(ctx, "podman", "start", name)
		if err := cmd.Run(); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to start container: %v", err),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Started container '%s'", name),
		}, nil
	}

	// Container doesn't exist, create and start it
	args := []string{"run", "-d", "--name", name}

	// Add ports
	if ports := getStringSliceParameter(decl, "ports"); len(ports) > 0 {
		for _, port := range ports {
			args = append(args, "-p", port)
		}
	}

	// Add volumes
	if volumes := getStringSliceParameter(decl, "volumes"); len(volumes) > 0 {
		for _, vol := range volumes {
			args = append(args, "-v", vol)
		}
	}

	// Add environment variables
	if envs := getEnvParameters(decl); len(envs) > 0 {
		for k, v := range envs {
			args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Add network
	if network := getStringParameter(decl, "network", ""); network != "" {
		args = append(args, "--network", network)
	}

	// Add restart policy
	if restart := getStringParameter(decl, "restart", ""); restart != "" {
		args = append(args, "--restart", restart)
	}

	args = append(args, image)

	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Failed to create container: %v - %s", err, string(output)),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Created and started container '%s'", name),
	}, nil
}

func (m *PodmanContainerModule) ensureStopped(ctx context.Context, name, image string, decl *StateDeclaration) (*StateResult, error) {
	info, exists := m.getContainerInfo(ctx, name)

	if !exists {
		// Create container but don't start it
		args := []string{"create", "--name", name, image}
		cmd := exec.CommandContext(ctx, "podman", args...)
		if err := cmd.Run(); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to create container: %v", err),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Created container '%s' (stopped)", name),
		}, nil
	}

	state := info["State"].(string)
	if state == "exited" || state == "stopped" {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Container '%s' is already stopped", name),
		}, nil
	}

	// Stop the container
	cmd := exec.CommandContext(ctx, "podman", "stop", name)
	if err := cmd.Run(); err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Failed to stop container: %v", err),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Stopped container '%s'", name),
	}, nil
}

func (m *PodmanContainerModule) ensureAbsent(ctx context.Context, name string, decl *StateDeclaration) (*StateResult, error) {
	_, exists := m.getContainerInfo(ctx, name)

	if !exists {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Container '%s' does not exist", name),
		}, nil
	}

	force := getBoolParameter(decl, "force", true)
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)

	cmd := exec.CommandContext(ctx, "podman", args...)
	if err := cmd.Run(); err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Failed to remove container: %v", err),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Removed container '%s'", name),
	}, nil
}

// Test runs the module in test mode.
func (m *PodmanContainerModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// ============================================================================
// Podman Image Module
// ============================================================================

// PodmanImageModule manages Podman images.
type PodmanImageModule struct {
	*BaseModule
}

// NewPodmanImageModule creates a new PodmanImageModule.
func NewPodmanImageModule() *PodmanImageModule {
	return &PodmanImageModule{
		BaseModule: NewBaseModule("podman_image", []string{"present", "absent"}),
	}
}

// Check checks if a Podman image matches the desired state.
func (m *PodmanImageModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	tag := getStringParameter(decl, "tag", "latest")
	fullName := fmt.Sprintf("%s:%s", name, tag)

	cmd := exec.CommandContext(ctx, "podman", "version", "--format", "{{.Version}}")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("podman is not available: %w", err)
	}

	cmd = exec.CommandContext(ctx, "podman", "image", "inspect", fullName)
	exists := cmd.Run() == nil

	result := &ModuleCheckResult{
		Present:  exists,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.CurrentState = "present"
		result.Metadata["image"] = fullName
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = exists
	case "absent":
		result.Matches = !exists
	}

	return result, nil
}

// Apply applies the desired Podman image state.
func (m *PodmanImageModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: "Image already in desired state",
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	tag := getStringParameter(decl, "tag", "latest")
	fullName := fmt.Sprintf("%s:%s", name, tag)

	switch decl.State {
	case "present":
		// Pull image with optional authentication
		authMethod := GetAuthMethodFromDeclaration(decl)
		var output string
		var pullErr error

		if authMethod != "" {
			// Use authenticated pull
			puller := NewPodmanPuller(nil) //nolint:contextcheck // NewPodmanPuller constructor doesn't take context
			output, pullErr = puller.PullImage(ctx, fullName, authMethod)
		} else {
			// Standard pull without auth
			cmd := exec.CommandContext(ctx, "podman", "pull", fullName)
			outputBytes, err := cmd.CombinedOutput()
			output = string(outputBytes)
			pullErr = err
		}

		if pullErr != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to pull image: %v - %s", pullErr, output),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Pulled image '%s'", fullName),
		}, nil

	case "absent":
		force := getBoolParameter(decl, "force", false)
		args := []string{"rmi"}
		if force {
			args = append(args, "-f")
		}
		args = append(args, fullName)

		cmd := exec.CommandContext(ctx, "podman", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to remove image: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed image '%s'", fullName),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: false,
		Comment: fmt.Sprintf("Unknown state: %s", decl.State),
	}, nil
}

// Test runs the module in test mode.
func (m *PodmanImageModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// ============================================================================
// Podman Network Module
// ============================================================================

// PodmanNetworkModule manages Podman networks.
type PodmanNetworkModule struct {
	*BaseModule
}

// NewPodmanNetworkModule creates a new PodmanNetworkModule.
func NewPodmanNetworkModule() *PodmanNetworkModule {
	return &PodmanNetworkModule{
		BaseModule: NewBaseModule("podman_network", []string{"present", "absent"}),
	}
}

// Check checks if a Podman network matches the desired state.
func (m *PodmanNetworkModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	cmd := exec.CommandContext(ctx, "podman", "version", "--format", "{{.Version}}")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("podman is not available: %w", err)
	}

	cmd = exec.CommandContext(ctx, "podman", "network", "inspect", name)
	exists := cmd.Run() == nil

	result := &ModuleCheckResult{
		Present:  exists,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = exists
	case "absent":
		result.Matches = !exists
	}

	return result, nil
}

// Apply applies the desired Podman network state.
func (m *PodmanNetworkModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: "Network already in desired state",
		}, nil
	}

	name := getStringParameter(decl, "name", "")

	switch decl.State {
	case "present":
		args := []string{"network", "create"}

		if driver := getStringParameter(decl, "driver", ""); driver != "" {
			args = append(args, "--driver", driver)
		}

		if subnet := getStringParameter(decl, "subnet", ""); subnet != "" {
			args = append(args, "--subnet", subnet)
		}

		if gateway := getStringParameter(decl, "gateway", ""); gateway != "" {
			args = append(args, "--gateway", gateway)
		}

		args = append(args, name)

		cmd := exec.CommandContext(ctx, "podman", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to create network: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Created network '%s'", name),
		}, nil

	case "absent":
		cmd := exec.CommandContext(ctx, "podman", "network", "rm", name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to remove network: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed network '%s'", name),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: false,
		Comment: fmt.Sprintf("Unknown state: %s", decl.State),
	}, nil
}

// Test runs the module in test mode.
func (m *PodmanNetworkModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// ============================================================================
// Podman Volume Module
// ============================================================================

// PodmanVolumeModule manages Podman volumes.
type PodmanVolumeModule struct {
	*BaseModule
}

// NewPodmanVolumeModule creates a new PodmanVolumeModule.
func NewPodmanVolumeModule() *PodmanVolumeModule {
	return &PodmanVolumeModule{
		BaseModule: NewBaseModule("podman_volume", []string{"present", "absent"}),
	}
}

// Check checks if a Podman volume matches the desired state.
func (m *PodmanVolumeModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	cmd := exec.CommandContext(ctx, "podman", "version", "--format", "{{.Version}}")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("podman is not available: %w", err)
	}

	cmd = exec.CommandContext(ctx, "podman", "volume", "inspect", name)
	exists := cmd.Run() == nil

	result := &ModuleCheckResult{
		Present:  exists,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = exists
	case "absent":
		result.Matches = !exists
	}

	return result, nil
}

// Apply applies the desired Podman volume state.
func (m *PodmanVolumeModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: "Volume already in desired state",
		}, nil
	}

	name := getStringParameter(decl, "name", "")

	switch decl.State {
	case "present":
		args := []string{"volume", "create"}

		if driver := getStringParameter(decl, "driver", ""); driver != "" {
			args = append(args, "--driver", driver)
		}

		// Add driver options
		if opts := getDriverOpts(decl); len(opts) > 0 {
			for k, v := range opts {
				args = append(args, "--opt", fmt.Sprintf("%s=%s", k, v))
			}
		}

		args = append(args, name)

		cmd := exec.CommandContext(ctx, "podman", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to create volume: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Created volume '%s'", name),
		}, nil

	case "absent":
		force := getBoolParameter(decl, "force", false)
		args := []string{"volume", "rm"}
		if force {
			args = append(args, "-f")
		}
		args = append(args, name)

		cmd := exec.CommandContext(ctx, "podman", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Comment: fmt.Sprintf("Failed to remove volume: %v - %s", err, string(output)),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed volume '%s'", name),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: false,
		Comment: fmt.Sprintf("Unknown state: %s", decl.State),
	}, nil
}

// Test runs the module in test mode.
func (m *PodmanVolumeModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// ============================================================================
// Container Runtime Detector
// ============================================================================

// ContainerRuntime represents the container runtime type.
type ContainerRuntime string

// ContainerRuntimeDocker and related constants.
const (
	ContainerRuntimeDocker  ContainerRuntime = "docker"
	ContainerRuntimePodman  ContainerRuntime = "podman"
	ContainerRuntimeUnknown ContainerRuntime = "unknown"
)

// DetectContainerRuntime detects which container runtime is available.
func DetectContainerRuntime(ctx context.Context) ContainerRuntime {
	// Check for docker first
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err == nil {
		return ContainerRuntimeDocker
	}

	// Check for podman
	cmd = exec.CommandContext(ctx, "podman", "version", "--format", "{{.Version}}")
	if err := cmd.Run(); err == nil {
		return ContainerRuntimePodman
	}

	return ContainerRuntimeUnknown
}

// GetContainerRuntimeVersion returns the version of the detected runtime.
func GetContainerRuntimeVersion(ctx context.Context, rt ContainerRuntime) (string, error) {
	switch rt {
	case ContainerRuntimeDocker:
		cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil
	case ContainerRuntimePodman:
		cmd := exec.CommandContext(ctx, "podman", "version", "--format", "{{.Version}}")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil
	default:
		return "", fmt.Errorf("unknown container runtime")
	}
}

// ListContainers lists all containers for the given runtime.
func ListContainers(ctx context.Context, rt ContainerRuntime, all bool) ([]map[string]string, error) {
	var cmd *exec.Cmd

	args := []string{"ps", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}"}
	if all {
		args = append([]string{"ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}"}, args[3:]...)
	}

	switch rt {
	case ContainerRuntimeDocker:
		cmd = exec.CommandContext(ctx, "docker", args...)
	case ContainerRuntimePodman:
		cmd = exec.CommandContext(ctx, "podman", args...)
	default:
		return nil, fmt.Errorf("unknown container runtime")
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var containers []map[string]string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) >= 4 {
			containers = append(containers, map[string]string{
				"id":     parts[0],
				"name":   parts[1],
				"image":  parts[2],
				"status": parts[3],
			})
		}
	}

	return containers, nil
}

// Ensure unused imports don't cause errors
var (
	_ = runtime.GOOS
	_ = sort.Strings
)

func init() {
	_ = RegisterModule(NewDockerContainerModule())
	_ = RegisterModule(NewDockerImageModule())
	_ = RegisterModule(NewDockerNetworkModule())
	_ = RegisterModule(NewDockerVolumeModule())
	_ = RegisterModule(NewPodmanContainerModule())
	_ = RegisterModule(NewPodmanImageModule())
	_ = RegisterModule(NewPodmanNetworkModule())
	_ = RegisterModule(NewPodmanVolumeModule())
}
