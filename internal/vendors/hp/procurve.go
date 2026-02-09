// Package hp provides HP/Aruba device adapters for proxy agents.
package hp

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/ssh"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// ProCurveAdapter implements VendorAdapter for HP ProCurve switches.
type ProCurveAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inEnable   bool
	inConfig   bool
}

// ProCurveConfig contains HP ProCurve specific configuration.
type ProCurveConfig struct {
	*vendors.VendorConfig
}

// DefaultProCurveConfig returns a default ProCurve configuration.
func DefaultProCurveConfig() *ProCurveConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "(config)"
	return &ProCurveConfig{VendorConfig: cfg}
}

// NewProCurveAdapter creates a new HP ProCurve adapter.
func NewProCurveAdapter(config *ProCurveConfig) *ProCurveAdapter {
	if config == nil {
		config = DefaultProCurveConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &ProCurveAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *ProCurveAdapter) Vendor() vendors.VendorType {
	return vendors.VendorHPProCurve
}

// Type implements ProtocolAdapter.Type.
func (a *ProCurveAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *ProCurveAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:         "hp_procurve",
		Prompts:        []string{">", "#", "(config)"},
		EnableCmd:      "enable",
		EnablePassword: a.Config.EnablePassword,
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "no page")
	}

	if a.Config.EnablePassword != "" || a.Config.PrivilegeLevel > 0 {
		if err := a.enterEnable(ctx); err != nil {
			return fmt.Errorf("failed to enter enable mode: %w", err)
		}
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *ProCurveAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
	a.inEnable = false
	a.inConfig = false

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
func (a *ProCurveAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *ProCurveAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show system-information")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["system_info"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *ProCurveAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *ProCurveAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *ProCurveAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show running-config"
	case "startup":
		cmd = "show config"
	case "interface", "interfaces":
		cmd = "show interfaces brief"
	case "vlan", "vlans":
		cmd = "show vlans"
	default:
		cmd = fmt.Sprintf("show running-config %s", section)
	}
	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *ProCurveAdapter) SetConfig(ctx context.Context, commands []string) error {
	if !a.inConfig {
		if err := a.enterConfig(ctx); err != nil {
			return err
		}
		defer func() { _ = a.exitConfig(ctx) }()
	}

	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *ProCurveAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "HP",
		OSType: "ProCurve",
		Raw:    make(map[string]string),
	}

	sysInfo, err := a.runCommand(ctx, "show system-information")
	if err != nil {
		return nil, fmt.Errorf("failed to get system information: %w", err)
	}
	facts.Raw["show system-information"] = sysInfo
	a.parseSystemInfo(sysInfo, facts)

	version, err := a.runCommand(ctx, "show version")
	if err == nil {
		facts.Raw["show version"] = version
		a.parseVersion(version, facts)
	}

	intfs, err := a.runCommand(ctx, "show interfaces brief")
	if err == nil {
		facts.Raw["show interfaces brief"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *ProCurveAdapter) SaveConfig(ctx context.Context) error {
	if a.inConfig {
		if err := a.exitConfig(ctx); err != nil {
			return err
		}
	}
	_, err := a.runCommand(ctx, "write memory")
	return err
}

func (a *ProCurveAdapter) enterEnable(ctx context.Context) error {
	if a.inEnable {
		return nil
	}

	output, err := a.runCommand(ctx, "enable")
	if err != nil {
		return err
	}

	if strings.Contains(strings.ToLower(output), "password") {
		password := a.Config.EnablePassword
		if password == "" {
			if sshCred, ok := a.Credential.(*credentials.SSHPasswordCredential); ok {
				password = sshCred.Password
			}
		}
		if _, err := a.runCommand(ctx, password); err != nil {
			return fmt.Errorf("enable authentication failed: %w", err)
		}
	}

	a.inEnable = true
	return nil
}

func (a *ProCurveAdapter) enterConfig(ctx context.Context) error {
	if !a.inEnable {
		if err := a.enterEnable(ctx); err != nil {
			return err
		}
	}

	if _, err := a.runCommand(ctx, "configure terminal"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}
	a.inConfig = true
	return nil
}

func (a *ProCurveAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}
	if _, err := a.runCommand(ctx, "exit"); err != nil {
		return err
	}
	a.inConfig = false
	return nil
}

func (a *ProCurveAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "invalid input:") ||
		strings.Contains(lowerOutput, "ambiguous command") ||
		strings.Contains(lowerOutput, "not found") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *ProCurveAdapter) parseSystemInfo(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "System Name") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "System Uptime") {
			uptime := parseUptime(line)
			if uptime > 0 {
				facts.Uptime = uptime
			}
		}
		if strings.HasPrefix(line, "Serial Number") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "Memory") && strings.Contains(line, "Total") {
			if match := regexp.MustCompile(`(\d+)`).FindStringSubmatch(line); len(match) > 1 {
				mem, _ := strconv.ParseInt(match[1], 10, 64)
				facts.MemoryTotal = mem * 1024
			}
		}
	}
}

func (a *ProCurveAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Software revision") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.OSVersion = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "ROM Version") || strings.Contains(line, "Boot ROM") {
			if match := regexp.MustCompile(`\b([A-Z]{2}\.\d+\.\d+)`).FindStringSubmatch(line); len(match) > 1 {
				if facts.OSVersion == "" {
					facts.OSVersion = match[1]
				}
			}
		}
	}
}

func (a *ProCurveAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Port") || strings.HasPrefix(line, "----") || strings.HasPrefix(line, " ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		intf := vendors.InterfaceFact{
			Name:        fields[0],
			AdminStatus: strings.ToLower(fields[len(fields)-2]),
			OperStatus:  strings.ToLower(fields[len(fields)-1]),
		}
		interfaces = append(interfaces, intf)
	}
	return interfaces
}

// parseUptime parses uptime from HP-style output.
func parseUptime(line string) time.Duration {
	var total time.Duration

	dayMatch := regexp.MustCompile(`(\d+)\s*d`).FindStringSubmatch(line)
	hourMatch := regexp.MustCompile(`(\d+)\s*h`).FindStringSubmatch(line)
	minuteMatch := regexp.MustCompile(`(\d+)\s*m`).FindStringSubmatch(line)

	if len(dayMatch) > 1 {
		days, _ := strconv.Atoi(dayMatch[1])
		total += time.Duration(days) * 24 * time.Hour
	}
	if len(hourMatch) > 1 {
		hours, _ := strconv.Atoi(hourMatch[1])
		total += time.Duration(hours) * time.Hour
	}
	if len(minuteMatch) > 1 {
		minutes, _ := strconv.Atoi(minuteMatch[1])
		total += time.Duration(minutes) * time.Minute
	}

	return total
}

// NewProCurveAdapterFactory creates an adapter factory for HP ProCurve.
func NewProCurveAdapterFactory(config *ProCurveConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultProCurveConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewProCurveAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorHPProCurve, NewProCurveAdapterFactory(nil))
}
