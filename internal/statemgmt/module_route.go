package statemgmt

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// RouteModule implements static route management
type RouteModule struct {
	*BaseModule
}

// NewRouteModule creates a new route module
func NewRouteModule() *RouteModule {
	return &RouteModule{
		BaseModule: NewBaseModule("route", []string{"present", "absent"}),
	}
}

// RouteConfig holds route configuration parameters
type RouteConfig struct {
	Destination string // Destination network (CIDR)
	Gateway     string // Gateway IP address
	Interface   string // Network interface
	Metric      int    // Route metric/priority
	Table       string // Routing table (Linux)
}

// Check checks the current state of a route
func (m *RouteModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseRouteConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse route config: %w", err)
	}

	// Check if route exists
	routeExists, currentGateway, currentInterface, err := m.checkRouteExists(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to check route: %w", err)
	}

	result.Present = routeExists
	if routeExists {
		result.CurrentState = "present"
		result.Metadata["gateway"] = currentGateway
		result.Metadata["interface"] = currentInterface
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		if !routeExists {
			result.Matches = false
			result.Diff["route"] = map[string]string{"current": "absent", "desired": "present"}
		} else {
			// Check if gateway matches
			switch {
			case config.Gateway != "" && config.Gateway != currentGateway:
				result.Matches = false
				result.Diff["gateway"] = map[string]string{"current": currentGateway, "desired": config.Gateway}
			case config.Interface != "" && config.Interface != currentInterface:
				result.Matches = false
				result.Diff["interface"] = map[string]string{"current": currentInterface, "desired": config.Interface}
			default:
				result.Matches = true
			}
		}
	case "absent":
		result.Matches = !routeExists
		if routeExists {
			result.Diff["route"] = map[string]string{"current": "present", "desired": "absent"}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the route configuration
func (m *RouteModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	config, err := m.parseRouteConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "present":
		// If route exists but with different config, delete first - best-effort
		if checkResult.Present && len(checkResult.Diff) > 0 {
			_ = m.deleteRoute(ctx, config)
		}
		applyErr = m.addRoute(ctx, config, result)
	case "absent":
		applyErr = m.deleteRoute(ctx, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Deleted route to %s", config.Destination)
		}
	default:
		applyErr = fmt.Errorf("unsupported state: %s", decl.State)
	}

	if applyErr != nil {
		result.Error = applyErr
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test tests if the route is in the desired state
func (m *RouteModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // intentional
}

// parseRouteConfig extracts route configuration from declaration parameters
func (m *RouteModule) parseRouteConfig(decl *StateDeclaration) (*RouteConfig, error) {
	config := &RouteConfig{
		Destination: decl.ID,
	}

	if dest := getStringParameter(decl, "destination", ""); dest != "" {
		config.Destination = dest
	}

	config.Gateway = getStringParameter(decl, "gateway", "")
	config.Interface = getStringParameter(decl, "interface", "")
	config.Metric = getIntParameter(decl, "metric", 0)
	config.Table = getStringParameter(decl, "table", "")

	// Validate destination is a valid network
	if config.Destination != "default" && config.Destination != "0.0.0.0/0" {
		_, _, err := net.ParseCIDR(config.Destination)
		if err != nil {
			// Try parsing as host route
			if net.ParseIP(config.Destination) == nil {
				return nil, fmt.Errorf("invalid destination: %s", config.Destination)
			}
		}
	}

	// Must have either gateway or interface
	if config.Gateway == "" && config.Interface == "" {
		return nil, fmt.Errorf("must specify gateway or interface")
	}

	return config, nil //nolint:nilerr // intentional
}

// checkRouteExists checks if a route exists
func (m *RouteModule) checkRouteExists(ctx context.Context, config *RouteConfig) (exists bool, gateway, iface string, err error) {
	switch runtime.GOOS {
	case "linux":
		return m.checkRouteExistsLinux(ctx, config)
	case "darwin":
		return m.checkRouteExistsDarwin(ctx, config)
	case "windows":
		return m.checkRouteExistsWindows(ctx, config)
	default:
		return false, "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// checkRouteExistsLinux checks route on Linux
func (m *RouteModule) checkRouteExistsLinux(ctx context.Context, config *RouteConfig) (exists bool, gateway, iface string, err error) {
	args := []string{"route", "show"}
	if config.Table != "" {
		args = append(args, "table", config.Table)
	}
	args = append(args, config.Destination)

	cmd := exec.CommandContext(ctx, "ip", args...)
	output, err := cmd.Output()
	if err != nil {
		return false, "", "", nil //nolint:nilerr // error means route doesn't exist
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return false, "", "", nil //nolint:nilerr // empty output means route doesn't exist
	}

	// Parse output: 10.0.0.0/8 via 192.168.1.1 dev eth0
	fields := strings.Fields(outputStr)
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			gateway = fields[i+1]
		}
		if f == "dev" && i+1 < len(fields) {
			iface = fields[i+1]
		}
	}

	return true, gateway, iface, nil //nolint:nilerr // returning parsed route info, no error
}

// checkRouteExistsDarwin checks route on macOS
func (m *RouteModule) checkRouteExistsDarwin(ctx context.Context, config *RouteConfig) (exists bool, gateway, iface string, err error) {
	// Convert destination to format netstat expects
	dest := config.Destination
	if dest == "0.0.0.0/0" {
		dest = "default"
	}

	cmd := exec.CommandContext(ctx, "netstat", "-rn")
	output, err := cmd.Output()
	if err != nil {
		return false, "", "", err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// Match destination
		if fields[0] == dest || fields[0] == config.Destination {
			return true, fields[1], fields[3], nil //nolint:nilerr // returning parsed route info, no error
		}
	}

	return false, "", "", nil //nolint:nilerr // route not found is a valid state
}

// checkRouteExistsWindows checks route on Windows
func (m *RouteModule) checkRouteExistsWindows(ctx context.Context, config *RouteConfig) (exists bool, gateway, iface string, err error) {
	cmd := exec.CommandContext(ctx, "route", "print")
	output, err := cmd.Output()
	if err != nil {
		return false, "", "", err
	}

	// Parse destination
	dest := config.Destination
	mask := "255.255.255.255"
	if strings.Contains(dest, "/") {
		parts := strings.SplitN(dest, "/", 2)
		dest = parts[0]
		mask = cidrToNetmask(parts[1])
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// Match destination and netmask
		if (fields[0] == dest || fields[0] == "0.0.0.0" && dest == "0.0.0.0") &&
			(fields[1] == mask || dest == "0.0.0.0") {
			return true, fields[2], fields[3], nil //nolint:nilerr // returning parsed route info, no error
		}
	}

	return false, "", "", nil //nolint:nilerr // route not found is a valid state
}

// addRoute adds a route
func (m *RouteModule) addRoute(ctx context.Context, config *RouteConfig, result *StateResult) error {
	switch runtime.GOOS {
	case "linux":
		return m.addRouteLinux(ctx, config, result)
	case "darwin":
		return m.addRouteDarwin(ctx, config, result)
	case "windows":
		return m.addRouteWindows(ctx, config, result)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// addRouteLinux adds route on Linux
func (m *RouteModule) addRouteLinux(ctx context.Context, config *RouteConfig, result *StateResult) error {
	args := []string{"route", "add"}
	if config.Table != "" {
		args = append(args, "table", config.Table)
	}
	args = append(args, config.Destination)

	if config.Gateway != "" {
		args = append(args, "via", config.Gateway)
	}
	if config.Interface != "" {
		args = append(args, "dev", config.Interface)
	}
	if config.Metric > 0 {
		args = append(args, "metric", fmt.Sprintf("%d", config.Metric))
	}

	cmd := exec.CommandContext(ctx, "ip", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add route: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Added route to %s via %s", config.Destination, config.Gateway)
	return nil
}

// addRouteDarwin adds route on macOS
func (m *RouteModule) addRouteDarwin(ctx context.Context, config *RouteConfig, result *StateResult) error {
	dest := config.Destination
	if dest == "0.0.0.0/0" {
		dest = "default"
	}

	args := []string{"-n", "add"}

	// Handle CIDR notation
	switch {
	case strings.Contains(dest, "/") && dest != "default":
		parts := strings.SplitN(dest, "/", 2)
		args = append(args, "-net", parts[0], "-netmask", cidrToNetmask(parts[1]))
	case dest == "default":
		args = append(args, "default")
	default:
		args = append(args, "-host", dest)
	}

	if config.Gateway != "" {
		args = append(args, config.Gateway)
	}
	if config.Interface != "" {
		args = append(args, "-interface", config.Interface)
	}

	cmd := exec.CommandContext(ctx, "route", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add route: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Added route to %s via %s", config.Destination, config.Gateway)
	return nil
}

// addRouteWindows adds route on Windows
func (m *RouteModule) addRouteWindows(ctx context.Context, config *RouteConfig, result *StateResult) error {
	dest := config.Destination
	mask := "255.255.255.255"
	if strings.Contains(dest, "/") {
		parts := strings.SplitN(dest, "/", 2)
		dest = parts[0]
		mask = cidrToNetmask(parts[1])
	}
	if dest == "default" {
		dest = "0.0.0.0"
		mask = "0.0.0.0"
	}

	args := []string{"add", dest, "mask", mask, config.Gateway}
	if config.Metric > 0 {
		args = append(args, "metric", fmt.Sprintf("%d", config.Metric))
	}
	// -p for persistent route
	args = append(args, "-p")

	cmd := exec.CommandContext(ctx, "route", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add route: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Added route to %s via %s", config.Destination, config.Gateway)
	return nil
}

// deleteRoute deletes a route
func (m *RouteModule) deleteRoute(ctx context.Context, config *RouteConfig) error {
	switch runtime.GOOS {
	case "linux":
		return m.deleteRouteLinux(ctx, config)
	case "darwin":
		return m.deleteRouteDarwin(ctx, config)
	case "windows":
		return m.deleteRouteWindows(ctx, config)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// deleteRouteLinux deletes route on Linux
func (m *RouteModule) deleteRouteLinux(ctx context.Context, config *RouteConfig) error {
	args := []string{"route", "del"}
	if config.Table != "" {
		args = append(args, "table", config.Table)
	}
	args = append(args, config.Destination)

	cmd := exec.CommandContext(ctx, "ip", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete route: %w (output: %s)", err, string(output))
	}
	return nil
}

// deleteRouteDarwin deletes route on macOS
func (m *RouteModule) deleteRouteDarwin(ctx context.Context, config *RouteConfig) error {
	dest := config.Destination
	if dest == "0.0.0.0/0" {
		dest = "default"
	}

	args := []string{"-n", "delete"}

	switch {
	case strings.Contains(dest, "/") && dest != "default":
		parts := strings.SplitN(dest, "/", 2)
		args = append(args, "-net", parts[0], "-netmask", cidrToNetmask(parts[1]))
	case dest == "default":
		args = append(args, "default")
	default:
		args = append(args, "-host", dest)
	}

	if config.Gateway != "" {
		args = append(args, config.Gateway)
	}

	cmd := exec.CommandContext(ctx, "route", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete route: %w (output: %s)", err, string(output))
	}
	return nil
}

// deleteRouteWindows deletes route on Windows
func (m *RouteModule) deleteRouteWindows(ctx context.Context, config *RouteConfig) error {
	dest := config.Destination
	if dest == "default" {
		dest = "0.0.0.0"
	}
	if strings.Contains(dest, "/") {
		dest = strings.SplitN(dest, "/", 2)[0]
	}

	cmd := exec.CommandContext(ctx, "route", "delete", dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete route: %w (output: %s)", err, string(output))
	}
	return nil
}

func init() {
	_ = RegisterModule(NewRouteModule()) //nolint:errcheck // module registration in init
}
