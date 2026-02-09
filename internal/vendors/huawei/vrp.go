// Package huawei provides Huawei VRP device adapters for proxy agents.
package huawei

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/ssh"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// VRPAdapter implements VendorAdapter for Huawei VRP devices.
type VRPAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter   *ssh.Adapter
	shell        *ssh.NetworkDeviceShell
	inConfigMode bool
}

// VRPConfig contains Huawei VRP specific configuration.
type VRPConfig struct {
	*vendors.VendorConfig
}

// DefaultVRPConfig returns a default VRP configuration.
func DefaultVRPConfig() *VRPConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = ">"
	cfg.ConfigPrompt = "]"
	cfg.DisablePaging = true
	return &VRPConfig{VendorConfig: cfg}
}

// NewVRPAdapter creates a new Huawei VRP adapter.
func NewVRPAdapter(config *VRPConfig) *VRPAdapter {
	if config == nil {
		config = DefaultVRPConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &VRPAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *VRPAdapter) Vendor() vendors.VendorType {
	return vendors.VendorHuaweiVRP
}

// Type implements ProtocolAdapter.Type.
func (a *VRPAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *VRPAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "huawei_vrp",
		Prompts: []string{">", "]"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "screen-length 0 temporary")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *VRPAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
	a.inConfigMode = false

	if a.shell != nil {
		a.shell.Close()
		a.shell = nil
	}

	if a.sshAdapter != nil {
		return a.sshAdapter.Disconnect(ctx)
	}
	return nil
}

// Execute implements ProtocolAdapter.Execute.
func (a *VRPAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	start := time.Now()
	result := &protocols.ExecuteResult{StartTime: start}

	output, err := a.runCommand(ctx, req.Command)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(start)

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
		return result, err
	}

	result.Stdout = []byte(output)
	result.ExitCode = 0
	return result, nil
}

// HealthCheck implements ProtocolAdapter.HealthCheck.
func (a *VRPAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if !a.Connected || a.shell == nil {
		result.Healthy = false
		result.Status = "not connected"
		return result, nil
	}

	start := time.Now()
	output, err := a.runCommand(ctx, "display version")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["version"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *VRPAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *VRPAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *VRPAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if section == "" {
		return a.runCommand(ctx, "display current-configuration")
	}
	return a.runCommand(ctx, fmt.Sprintf("display current-configuration | section %s", section))
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *VRPAdapter) SetConfig(ctx context.Context, commands []string) error {
	if !a.inConfigMode {
		if _, err := a.runCommand(ctx, "system-view"); err != nil {
			return fmt.Errorf("failed to enter system-view: %w", err)
		}
		a.inConfigMode = true
	}

	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			_, _ = a.runCommand(ctx, "return")
			a.inConfigMode = false
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}

	_, _ = a.runCommand(ctx, "return")
	a.inConfigMode = false
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *VRPAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Huawei",
		OSType: "VRP",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "display version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["display version"] = version
	a.parseVersion(version, facts)

	device, err := a.runCommand(ctx, "display device")
	if err == nil {
		facts.Raw["display device"] = device
		a.parseDevice(device, facts)
	}

	intfs, err := a.runCommand(ctx, "display interface brief")
	if err == nil {
		facts.Raw["display interface brief"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	hostname, err := a.runCommand(ctx, "display current-configuration | include sysname")
	if err == nil {
		a.parseHostname(hostname, facts)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *VRPAdapter) SaveConfig(ctx context.Context) error {
	// Huawei save prompts for confirmation; send Y
	_, _ = a.runCommand(ctx, "save")
	_, err := a.runCommand(ctx, "Y")
	return err
}

func (a *VRPAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "error: unrecognized command") ||
		strings.Contains(lowerOutput, "% wrong parameter") ||
		strings.Contains(lowerOutput, "error: incomplete command") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *VRPAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "VRP") && strings.Contains(line, "Version") {
			if match := regexp.MustCompile(`Version\s+(\d+\.\d+\s*\([^)]+\))`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		if strings.Contains(line, "uptime is") {
			if match := regexp.MustCompile(`uptime is (.+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.Uptime = parseVRPUptime(match[1])
			}
		}

		if strings.Contains(line, "HUAWEI") && facts.Model == "" && !strings.Contains(line, "Copyright") {
			if match := regexp.MustCompile(`HUAWEI\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.Model = match[1]
			}
		}
	}
}

func (a *VRPAdapter) parseDevice(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "ESN:") || strings.Contains(line, "Serial Number") {
			if match := regexp.MustCompile(`:\s*(\S+)\s*$`).FindStringSubmatch(line); len(match) > 1 {
				if facts.SerialNumber == "" {
					facts.SerialNumber = match[1]
				}
			}
		}
	}
}

func (a *VRPAdapter) parseHostname(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "sysname") {
			if parts := strings.Fields(line); len(parts) >= 2 {
				facts.Hostname = parts[len(parts)-1]
			}
		}
	}
}

func (a *VRPAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Interface") || strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "PHY") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		if !strings.ContainsAny(name, "0123456789/") {
			continue
		}

		iface := vendors.InterfaceFact{
			Name: name,
		}

		for _, f := range fields[1:] {
			switch strings.ToLower(f) {
			case "up":
				if iface.AdminStatus == "" {
					iface.AdminStatus = "up"
				} else if iface.OperStatus == "" {
					iface.OperStatus = "up"
				}
			case "down":
				if iface.AdminStatus == "" {
					iface.AdminStatus = "down"
				} else if iface.OperStatus == "" {
					iface.OperStatus = "down"
				}
			case "*down":
				if iface.AdminStatus == "" {
					iface.AdminStatus = "down"
				}
			}
		}

		if iface.AdminStatus != "" {
			interfaces = append(interfaces, iface)
		}
	}

	return interfaces
}

func parseVRPUptime(s string) time.Duration {
	var days, hours, minutes, seconds int

	if match := regexp.MustCompile(`(\d+)\s*day`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &days)
	}
	if match := regexp.MustCompile(`(\d+)\s*hour`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &hours)
	}
	if match := regexp.MustCompile(`(\d+)\s*minute`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &minutes)
	}
	if match := regexp.MustCompile(`(\d+)\s*second`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &seconds)
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}

// NewVRPAdapterFactory creates an adapter factory for Huawei VRP.
func NewVRPAdapterFactory(config *VRPConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultVRPConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewVRPAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorHuaweiVRP, NewVRPAdapterFactory(nil))
}
