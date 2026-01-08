// Package juniper provides Juniper device adapters for proxy agents.
package juniper

import (
	"context"
	"encoding/xml"
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

// JUNOSAdapter implements VendorAdapter for Juniper JUNOS devices.
type JUNOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inConfig   bool
	useXML     bool // JUNOS supports XML output
}

// JUNOSConfig contains Juniper JUNOS specific configuration.
type JUNOSConfig struct {
	*vendors.VendorConfig
	// UseXML enables XML output mode.
	UseXML bool `json:"use_xml,omitempty"`
}

// DefaultJUNOSConfig returns a default JUNOS configuration.
func DefaultJUNOSConfig() *JUNOSConfig {
	config := vendors.DefaultVendorConfig()
	config.EnablePrompt = ">" // JUNOS operational mode
	config.ConfigPrompt = "#" // JUNOS config mode
	return &JUNOSConfig{
		VendorConfig: config,
		UseXML:       false,
	}
}

// NewJUNOSAdapter creates a new Juniper JUNOS adapter.
func NewJUNOSAdapter(config *JUNOSConfig) *JUNOSAdapter {
	if config == nil {
		config = DefaultJUNOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.ConnectionConfig.Timeout = config.Timeout

	return &JUNOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
		useXML:     config.UseXML,
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *JUNOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorJuniperJUNOS
}

// Type implements ProtocolAdapter.Type.
func (a *JUNOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *JUNOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	// Connect using SSH
	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	// Create network device shell
	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:         "juniper",
		Prompts:        []string{">", "#", "(config"},
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
		if _, err := a.runCommand(ctx, "set cli screen-length 0"); err != nil {
			// Non-fatal
		}
		if _, err := a.runCommand(ctx, "set cli screen-width 0"); err != nil {
			// Non-fatal
		}
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *JUNOSAdapter) Disconnect(ctx context.Context) error {
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
func (a *JUNOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *JUNOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show system uptime")
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
func (a *JUNOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *JUNOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *JUNOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show configuration"
	case "set":
		cmd = "show configuration | display set"
	case "interface", "interfaces":
		cmd = "show configuration interfaces"
	case "routing":
		cmd = "show configuration routing-options"
	case "protocols":
		cmd = "show configuration protocols"
	case "security":
		cmd = "show configuration security"
	case "firewall":
		cmd = "show configuration firewall"
	default:
		cmd = fmt.Sprintf("show configuration %s", section)
	}

	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *JUNOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	// Enter config mode if not already
	if !a.inConfig {
		if err := a.enterConfig(ctx); err != nil {
			return err
		}
	}

	// Execute each command
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			// Rollback on error
			a.runCommand(ctx, "rollback 0")
			a.exitConfig(ctx)
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}

	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *JUNOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Juniper",
		OSType: "JUNOS",
		Raw:    make(map[string]string),
	}

	// Try XML output first for more structured data
	if a.useXML {
		if err := a.getFactsXML(ctx, facts); err == nil {
			return facts, nil
		}
	}

	// Get version info
	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version
	a.parseVersionFacts(version, facts)

	// Get hostname
	hostname, err := a.runCommand(ctx, "show configuration system host-name")
	if err == nil {
		facts.Hostname = strings.TrimSpace(hostname)
		facts.Hostname = strings.TrimPrefix(facts.Hostname, "host-name ")
		facts.Hostname = strings.TrimSuffix(facts.Hostname, ";")
	}

	// Get interfaces
	intfs, err := a.runCommand(ctx, "show interfaces terse")
	if err == nil {
		facts.Raw["show interfaces terse"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// getFactsXML gets facts using XML output.
func (a *JUNOSAdapter) getFactsXML(ctx context.Context, facts *vendors.DeviceFacts) error {
	output, err := a.runCommand(ctx, "show version | display xml")
	if err != nil {
		return err
	}

	// Parse XML
	type SoftwareInfo struct {
		Hostname    string `xml:"host-name"`
		Product     string `xml:"product-model"`
		JUNOSVer    string `xml:"junos-version"`
		PackageInfo []struct {
			Name    string `xml:"name"`
			Comment string `xml:"comment"`
		} `xml:"package-information"`
	}

	type VersionInfo struct {
		SoftwareInfo SoftwareInfo `xml:"software-information"`
	}

	var vinfo VersionInfo
	if err := xml.Unmarshal([]byte(output), &vinfo); err != nil {
		return err
	}

	facts.Hostname = vinfo.SoftwareInfo.Hostname
	facts.Model = vinfo.SoftwareInfo.Product
	facts.OSVersion = vinfo.SoftwareInfo.JUNOSVer

	return nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *JUNOSAdapter) SaveConfig(ctx context.Context) error {
	// In JUNOS, config is already saved when committed
	// But we should check if there are uncommitted changes
	if a.inConfig {
		// Commit the configuration
		_, err := a.runCommand(ctx, "commit")
		if err != nil {
			return fmt.Errorf("commit failed: %w", err)
		}
	}
	return nil
}

// Commit commits the configuration.
func (a *JUNOSAdapter) Commit(ctx context.Context) error {
	if !a.inConfig {
		return fmt.Errorf("not in configuration mode")
	}

	output, err := a.runCommand(ctx, "commit")
	if err != nil {
		return err
	}

	// Check for commit errors
	if strings.Contains(output, "error:") || strings.Contains(output, "commit failed") {
		return fmt.Errorf("commit failed: %s", output)
	}

	return nil
}

// CommitConfirm commits with confirmation timeout.
func (a *JUNOSAdapter) CommitConfirm(ctx context.Context, minutes int) error {
	if !a.inConfig {
		return fmt.Errorf("not in configuration mode")
	}

	cmd := fmt.Sprintf("commit confirmed %d", minutes)
	output, err := a.runCommand(ctx, cmd)
	if err != nil {
		return err
	}

	if strings.Contains(output, "error:") {
		return fmt.Errorf("commit confirmed failed: %s", output)
	}

	return nil
}

// Rollback rolls back to a previous configuration.
func (a *JUNOSAdapter) Rollback(ctx context.Context, number int) error {
	if !a.inConfig {
		if err := a.enterConfig(ctx); err != nil {
			return err
		}
	}

	cmd := fmt.Sprintf("rollback %d", number)
	if _, err := a.runCommand(ctx, cmd); err != nil {
		return err
	}

	return a.Commit(ctx)
}

// enterConfig enters configuration mode.
func (a *JUNOSAdapter) enterConfig(ctx context.Context) error {
	if _, err := a.runCommand(ctx, "configure"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}

	a.inConfig = true
	return nil
}

// exitConfig exits configuration mode.
func (a *JUNOSAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}

	if _, err := a.runCommand(ctx, "exit configuration-mode"); err != nil {
		// Try alternative
		a.runCommand(ctx, "exit")
	}

	a.inConfig = false
	return nil
}

// runCommand runs a command and returns the output.
func (a *JUNOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output

	// Check for JUNOS error patterns
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "error:") ||
		strings.Contains(lowerOutput, "syntax error") ||
		strings.Contains(lowerOutput, "unknown command") ||
		strings.Contains(output, "^") { // JUNOS uses ^ to indicate error position
		return output, fmt.Errorf("command error: %s", output)
	}

	return output, nil
}

// parseVersionFacts parses show version output into facts.
func (a *JUNOSAdapter) parseVersionFacts(output string, facts *vendors.DeviceFacts) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse hostname
		if strings.HasPrefix(line, "Hostname:") {
			facts.Hostname = strings.TrimSpace(strings.TrimPrefix(line, "Hostname:"))
		}

		// Parse model
		if strings.HasPrefix(line, "Model:") {
			facts.Model = strings.TrimSpace(strings.TrimPrefix(line, "Model:"))
		}

		// Parse JUNOS version
		if strings.HasPrefix(line, "Junos:") || strings.HasPrefix(line, "JUNOS ") {
			if match := regexp.MustCompile(`(\d+\.\d+[A-Z]?\d*(?:\.\d+)?)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		// Parse serial number
		if strings.Contains(line, "Chassis") && strings.Contains(line, "serial number") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				facts.SerialNumber = parts[len(parts)-1]
			}
		}
	}
}

// parseInterfaces parses show interfaces terse output.
func (a *JUNOSAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Interface") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		intf := vendors.InterfaceFact{
			Name:        fields[0],
			AdminStatus: strings.ToLower(fields[1]),
			OperStatus:  strings.ToLower(fields[2]),
		}

		// Parse IP address if present (column 4+)
		if len(fields) >= 4 && strings.Contains(fields[3], "/") {
			intf.IPAddresses = []string{fields[3]}
		}

		interfaces = append(interfaces, intf)
	}

	return interfaces
}

// GetChassisInfo retrieves chassis hardware information.
func (a *JUNOSAdapter) GetChassisInfo(ctx context.Context) (string, error) {
	return a.runCommand(ctx, "show chassis hardware")
}

// GetAlarms retrieves system alarms.
func (a *JUNOSAdapter) GetAlarms(ctx context.Context) (string, error) {
	return a.runCommand(ctx, "show system alarms")
}

// GetBGPSummary retrieves BGP neighbor summary.
func (a *JUNOSAdapter) GetBGPSummary(ctx context.Context) (string, error) {
	return a.runCommand(ctx, "show bgp summary")
}

// GetRouteTable retrieves the routing table.
func (a *JUNOSAdapter) GetRouteTable(ctx context.Context, table string) (string, error) {
	if table == "" {
		table = "inet.0"
	}
	return a.runCommand(ctx, fmt.Sprintf("show route table %s", table))
}

// parseUptime parses uptime string into duration.
func (a *JUNOSAdapter) parseUptime(line string) time.Duration {
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

// NewJUNOSAdapterFactory creates an adapter factory for Juniper JUNOS.
func NewJUNOSAdapterFactory(config *JUNOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultJUNOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewJUNOSAdapter(config), nil
	}
}

// init registers the Juniper JUNOS adapter with the default registry.
func init() {
	vendors.Register(vendors.VendorJuniperJUNOS, NewJUNOSAdapterFactory(nil))
}
