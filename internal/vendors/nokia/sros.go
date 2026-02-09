// Package nokia provides Nokia SR OS device adapters for proxy agents.
package nokia

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

// SROSAdapter implements VendorAdapter for Nokia SR OS devices.
type SROSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter   *ssh.Adapter
	shell        *ssh.NetworkDeviceShell
	inConfigMode bool
}

// SROSConfig contains Nokia SR OS specific configuration.
type SROSConfig struct {
	*vendors.VendorConfig
}

// DefaultSROSConfig returns a default SR OS configuration.
func DefaultSROSConfig() *SROSConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "#"
	cfg.DisablePaging = true
	return &SROSConfig{VendorConfig: cfg}
}

// NewSROSAdapter creates a new Nokia SR OS adapter.
func NewSROSAdapter(config *SROSConfig) *SROSAdapter {
	if config == nil {
		config = DefaultSROSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &SROSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *SROSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorNokiaSROS
}

// Type implements ProtocolAdapter.Type.
func (a *SROSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *SROSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "nokia_sros",
		Prompts: []string{"#", ">", "$"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "environment no more")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *SROSAdapter) Disconnect(ctx context.Context) error {
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
func (a *SROSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *SROSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show version")
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
func (a *SROSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *SROSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *SROSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if section == "" {
		return a.runCommand(ctx, "admin display-config")
	}
	return a.runCommand(ctx, fmt.Sprintf("admin display-config %s", section))
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *SROSAdapter) SetConfig(ctx context.Context, commands []string) error {
	if !a.inConfigMode {
		if _, err := a.runCommand(ctx, "configure"); err != nil {
			return fmt.Errorf("failed to enter configure mode: %w", err)
		}
		a.inConfigMode = true
	}

	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			_, _ = a.runCommand(ctx, "exit all")
			a.inConfigMode = false
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}

	_, _ = a.runCommand(ctx, "exit all")
	a.inConfigMode = false
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *SROSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Nokia",
		OSType: "SR OS",
		Raw:    make(map[string]string),
	}

	sysInfo, err := a.runCommand(ctx, "show system information")
	if err != nil {
		return nil, fmt.Errorf("failed to get system information: %w", err)
	}
	facts.Raw["show system information"] = sysInfo
	a.parseSystemInfo(sysInfo, facts)

	version, err := a.runCommand(ctx, "show version")
	if err == nil {
		facts.Raw["show version"] = version
		a.parseVersion(version, facts)
	}

	chassis, err := a.runCommand(ctx, "show chassis")
	if err == nil {
		facts.Raw["show chassis"] = chassis
		a.parseChassis(chassis, facts)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *SROSAdapter) SaveConfig(ctx context.Context) error {
	_, err := a.runCommand(ctx, "admin save")
	return err
}

func (a *SROSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "error:") ||
		strings.Contains(lowerOutput, "minor:") && strings.Contains(lowerOutput, "invalid") ||
		strings.Contains(lowerOutput, "command not found") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *SROSAdapter) parseSystemInfo(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "System Name") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "System Up Time") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Uptime = parseSROSUptime(strings.TrimSpace(parts[1]))
			}
		}
	}
}

func (a *SROSAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "TiMOS") {
			if match := regexp.MustCompile(`TiMOS-[A-Z]-(\d+\.\d+\.\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}
	}
}

func (a *SROSAdapter) parseChassis(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Type") && !strings.Contains(line, "Card") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "Serial number") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}
	}
}

func parseSROSUptime(s string) time.Duration {
	var days, hours, minutes, seconds int

	if match := regexp.MustCompile(`(\d+)\s*d`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &days)
	}
	if match := regexp.MustCompile(`(\d+)\s*h`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &hours)
	}
	if match := regexp.MustCompile(`(\d+)\s*m`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &minutes)
	}
	if match := regexp.MustCompile(`(\d+)\s*s`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &seconds)
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}

// NewSROSAdapterFactory creates an adapter factory for Nokia SR OS.
func NewSROSAdapterFactory(config *SROSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultSROSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewSROSAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorNokiaSROS, NewSROSAdapterFactory(nil))
}
