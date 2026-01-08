// Package cisco provides Cisco device adapters for proxy agents.
package cisco

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/protocols/ssh"
	"github.com/shawnbutts/keystone-core/pkg/proxy"
	"github.com/shawnbutts/keystone-core/pkg/vendors"
)

// NXOSAdapter implements VendorAdapter for Cisco NX-OS devices.
type NXOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inConfig   bool
	useJSON    bool // NX-OS supports JSON output
}

// NXOSConfig contains Cisco NX-OS specific configuration.
type NXOSConfig struct {
	*vendors.VendorConfig
	// UseJSON enables JSON output mode.
	UseJSON bool `json:"use_json,omitempty"`
}

// DefaultNXOSConfig returns a default NX-OS configuration.
func DefaultNXOSConfig() *NXOSConfig {
	return &NXOSConfig{
		VendorConfig: vendors.DefaultVendorConfig(),
		UseJSON:      true,
	}
}

// NewNXOSAdapter creates a new Cisco NX-OS adapter.
func NewNXOSAdapter(config *NXOSConfig) *NXOSAdapter {
	if config == nil {
		config = DefaultNXOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.ConnectionConfig.Timeout = config.Timeout

	return &NXOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
		useJSON:    config.UseJSON,
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *NXOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorCiscoNXOS
}

// Type implements ProtocolAdapter.Type.
func (a *NXOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *NXOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	// Connect using SSH
	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	// Create network device shell
	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:         "cisco_nxos",
		Prompts:        []string{"#", "(config", "(config-"},
		EnableCmd:      "",
		EnablePassword: a.Config.EnablePassword,
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	// Disable paging
	if a.Config.DisablePaging {
		if _, err := a.runCommand(ctx, "terminal length 0"); err != nil {
			// Non-fatal
		}
		if _, err := a.runCommand(ctx, "terminal width 511"); err != nil {
			// Non-fatal
		}
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *NXOSAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
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
func (a *NXOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	start := time.Now()
	result := &protocols.ExecuteResult{
		StartTime: start,
	}

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
func (a *NXOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show version | include uptime")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["uptime"] = strings.TrimSpace(output)

	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *NXOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *NXOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *NXOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show running-config"
	case "startup":
		cmd = "show startup-config"
	case "interface", "interfaces":
		cmd = "show running-config interface"
	case "vlan":
		cmd = "show running-config vlan"
	case "routing":
		cmd = "show running-config | section router"
	default:
		cmd = fmt.Sprintf("show running-config | section %s", section)
	}

	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *NXOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	// Enter config mode if not already
	if !a.inConfig {
		if err := a.enterConfig(ctx); err != nil {
			return err
		}
		defer a.exitConfig(ctx)
	}

	// Execute each command
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}

	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *NXOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Cisco",
		OSType: "NX-OS",
		Raw:    make(map[string]string),
	}

	// Try JSON output first
	if a.useJSON {
		if err := a.getFactsJSON(ctx, facts); err == nil {
			return facts, nil
		}
		// Fall back to text parsing
	}

	// Get version info
	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version
	a.parseVersionFacts(version, facts)

	// Get hostname
	hostname, err := a.runCommand(ctx, "show hostname")
	if err == nil {
		facts.Hostname = strings.TrimSpace(hostname)
	}

	// Get interfaces
	intfs, err := a.runCommand(ctx, "show ip interface brief")
	if err == nil {
		facts.Raw["show ip interface brief"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// getFactsJSON gets facts using JSON output.
func (a *NXOSAdapter) getFactsJSON(ctx context.Context, facts *vendors.DeviceFacts) error {
	output, err := a.runCommand(ctx, "show version | json")
	if err != nil {
		return err
	}

	var versionData map[string]interface{}
	if err := json.Unmarshal([]byte(output), &versionData); err != nil {
		return err
	}

	// Extract fields from JSON
	if hostname, ok := versionData["host_name"].(string); ok {
		facts.Hostname = hostname
	}
	if version, ok := versionData["nxos_ver_str"].(string); ok {
		facts.OSVersion = version
	}
	if chassis, ok := versionData["chassis_id"].(string); ok {
		facts.Model = chassis
	}
	if serial, ok := versionData["proc_board_id"].(string); ok {
		facts.SerialNumber = serial
	}
	if uptime, ok := versionData["kern_uptm_secs"].(float64); ok {
		facts.Uptime = time.Duration(uptime) * time.Second
	}
	if memory, ok := versionData["memory"].(float64); ok {
		facts.MemoryTotal = int64(memory)
	}

	return nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *NXOSAdapter) SaveConfig(ctx context.Context) error {
	// Exit config mode if in it
	if a.inConfig {
		if err := a.exitConfig(ctx); err != nil {
			return err
		}
	}

	_, err := a.runCommand(ctx, "copy running-config startup-config")
	return err
}

// enterConfig enters configuration mode.
func (a *NXOSAdapter) enterConfig(ctx context.Context) error {
	if _, err := a.runCommand(ctx, "configure terminal"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}

	a.inConfig = true
	return nil
}

// exitConfig exits configuration mode.
func (a *NXOSAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}

	if _, err := a.runCommand(ctx, "end"); err != nil {
		return err
	}

	a.inConfig = false
	return nil
}

// runCommand runs a command and returns the output.
func (a *NXOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "% invalid") ||
		strings.Contains(lowerOutput, "% ambiguous") ||
		strings.Contains(lowerOutput, "% incomplete") ||
		strings.Contains(lowerOutput, "error:") {
		return output, fmt.Errorf("command error: %s", output)
	}

	return output, nil
}

// parseVersionFacts parses show version output into facts.
func (a *NXOSAdapter) parseVersionFacts(output string, facts *vendors.DeviceFacts) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse NX-OS version
		if strings.Contains(line, "NXOS:") || strings.Contains(line, "system:") {
			if match := regexp.MustCompile(`version\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		// Parse software version line
		if strings.Contains(line, "Software") && !strings.Contains(line, "Copyright") {
			if match := regexp.MustCompile(`Version\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		// Parse chassis/model
		if strings.Contains(line, "cisco") && (strings.Contains(line, "Chassis") || strings.Contains(line, "Switch")) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				facts.Model = parts[1]
			}
		}

		// Parse serial number
		if strings.Contains(line, "Processor Board ID") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				facts.SerialNumber = parts[3]
			}
		}

		// Parse uptime
		if strings.Contains(line, "uptime is") || strings.Contains(line, "Kernel uptime") {
			uptime := a.parseUptime(line)
			if uptime > 0 {
				facts.Uptime = uptime
			}
		}
	}
}

// parseUptime parses uptime string into duration.
func (a *NXOSAdapter) parseUptime(line string) time.Duration {
	var total time.Duration

	dayMatch := regexp.MustCompile(`(\d+)\s+day`).FindStringSubmatch(line)
	hourMatch := regexp.MustCompile(`(\d+)\s+hour`).FindStringSubmatch(line)
	minuteMatch := regexp.MustCompile(`(\d+)\s+minute`).FindStringSubmatch(line)
	secondMatch := regexp.MustCompile(`(\d+)\s+second`).FindStringSubmatch(line)

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
	if len(secondMatch) > 1 {
		seconds, _ := strconv.Atoi(secondMatch[1])
		total += time.Duration(seconds) * time.Second
	}

	return total
}

// parseInterfaces parses show ip interface brief output.
func (a *NXOSAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "IP Interface") || strings.HasPrefix(line, "Interface") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		intf := vendors.InterfaceFact{
			Name:       fields[0],
			OperStatus: strings.ToLower(fields[2]),
		}

		// Parse IP address if present
		if fields[1] != "--" && fields[1] != "unassigned" {
			intf.IPAddresses = []string{fields[1]}
		}

		// Admin status is typically in column 3 or 4 for NX-OS
		if len(fields) >= 4 {
			intf.AdminStatus = strings.ToLower(fields[3])
		}

		interfaces = append(interfaces, intf)
	}

	return interfaces
}

// GetVPC retrieves vPC status (NX-OS specific).
func (a *NXOSAdapter) GetVPC(ctx context.Context) (string, error) {
	return a.runCommand(ctx, "show vpc brief")
}

// GetBGPSummary retrieves BGP neighbor summary.
func (a *NXOSAdapter) GetBGPSummary(ctx context.Context) (string, error) {
	return a.runCommand(ctx, "show bgp all summary")
}

// GetOSPFNeighbors retrieves OSPF neighbor information.
func (a *NXOSAdapter) GetOSPFNeighbors(ctx context.Context) (string, error) {
	return a.runCommand(ctx, "show ip ospf neighbors")
}

// NewNXOSAdapterFactory creates an adapter factory for Cisco NX-OS.
func NewNXOSAdapterFactory(config *NXOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultNXOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewNXOSAdapter(config), nil
	}
}

// init registers the Cisco NX-OS adapter with the default registry.
func init() {
	vendors.Register(vendors.VendorCiscoNXOS, NewNXOSAdapterFactory(nil))
}
