// Package ciena provides Ciena SAOS device adapters for proxy agents.
package ciena

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

// SAOSAdapter implements VendorAdapter for Ciena SAOS devices.
type SAOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
}

// SAOSConfig contains Ciena SAOS specific configuration.
type SAOSConfig struct {
	*vendors.VendorConfig
}

// DefaultSAOSConfig returns a default SAOS configuration.
func DefaultSAOSConfig() *SAOSConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = ">"
	cfg.ConfigPrompt = ">"
	cfg.DisablePaging = true
	return &SAOSConfig{VendorConfig: cfg}
}

// NewSAOSAdapter creates a new Ciena SAOS adapter.
func NewSAOSAdapter(config *SAOSConfig) *SAOSAdapter {
	if config == nil {
		config = DefaultSAOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &SAOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *SAOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorCienaSAOS
}

// Type implements ProtocolAdapter.Type.
func (a *SAOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *SAOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "ciena_saos",
		Prompts: []string{">", "#"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "system shell set terminal rows infinite")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *SAOSAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false

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
func (a *SAOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *SAOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "software show")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["software"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *SAOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *SAOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *SAOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if section == "" {
		return a.runCommand(ctx, "configuration show")
	}
	return a.runCommand(ctx, fmt.Sprintf("configuration show %s", section))
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *SAOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *SAOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Ciena",
		OSType: "SAOS",
		Raw:    make(map[string]string),
	}

	software, err := a.runCommand(ctx, "software show")
	if err != nil {
		return nil, fmt.Errorf("failed to get software info: %w", err)
	}
	facts.Raw["software show"] = software
	a.parseSoftware(software, facts)

	chassis, err := a.runCommand(ctx, "chassis show")
	if err == nil {
		facts.Raw["chassis show"] = chassis
		a.parseChassis(chassis, facts)
	}

	ports, err := a.runCommand(ctx, "port show")
	if err == nil {
		facts.Raw["port show"] = ports
		facts.Interfaces = a.parsePorts(ports)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *SAOSAdapter) SaveConfig(ctx context.Context) error {
	_, err := a.runCommand(ctx, "configuration save")
	return err
}

func (a *SAOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "error") && strings.Contains(lowerOutput, "command") ||
		strings.Contains(lowerOutput, "unknown command") ||
		strings.Contains(lowerOutput, "invalid input") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *SAOSAdapter) parseSoftware(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "SAOS") {
			if match := regexp.MustCompile(`SAOS\s+([\d.]+[\w.-]*)`).FindStringSubmatch(line); len(match) > 1 {
				if facts.OSVersion == "" {
					facts.OSVersion = match[1]
				}
			}
		}

		if (strings.Contains(line, "Running") && strings.Contains(line, "Package")) && facts.OSVersion == "" {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.OSVersion = strings.TrimSpace(parts[1])
			}
		}
	}
}

func (a *SAOSAdapter) parseChassis(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Type") || strings.HasPrefix(line, "Model") ||
			strings.HasPrefix(line, "Platform") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				model := strings.TrimSpace(parts[1])
				if model != "" && facts.Model == "" {
					facts.Model = model
				}
			}
		}

		if strings.Contains(line, "Serial") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				if facts.SerialNumber == "" {
					facts.SerialNumber = strings.TrimSpace(parts[1])
				}
			}
		}

		if strings.HasPrefix(line, "Name") || strings.HasPrefix(line, "Hostname") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				hostname := strings.TrimSpace(parts[1])
				if hostname != "" && facts.Hostname == "" {
					facts.Hostname = hostname
				}
			}
		}

		if strings.Contains(line, "Up Time") || strings.Contains(line, "Uptime") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Uptime = parseSAOSUptime(strings.TrimSpace(parts[1]))
			}
		}
	}
}

func (a *SAOSAdapter) parsePorts(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "|") && !strings.ContainsAny(line, "0123456789") {
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

		iface := vendors.InterfaceFact{Name: name}

		for _, f := range fields[1:] {
			lower := strings.ToLower(f)
			switch lower {
			case "enabled":
				iface.AdminStatus = "up"
			case "disabled":
				iface.AdminStatus = "down"
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
			}
		}

		if iface.AdminStatus != "" {
			interfaces = append(interfaces, iface)
		}
	}

	return interfaces
}

func parseSAOSUptime(s string) time.Duration {
	var days, hours, minutes, seconds int

	if match := regexp.MustCompile(`(\d+)\s*day`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &days)
	}
	if match := regexp.MustCompile(`(\d+)\s*hour`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &hours)
	}
	if match := regexp.MustCompile(`(\d+)\s*min`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &minutes)
	}
	if match := regexp.MustCompile(`(\d+)\s*sec`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &seconds)
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}

// NewSAOSAdapterFactory creates an adapter factory for Ciena SAOS.
func NewSAOSAdapterFactory(config *SAOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultSAOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewSAOSAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorCienaSAOS, NewSAOSAdapterFactory(nil))
}
