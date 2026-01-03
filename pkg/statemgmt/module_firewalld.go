package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// FirewalldModule implements Linux firewalld zone and rule management
type FirewalldModule struct {
	*BaseModule
}

// NewFirewalldModule creates a new firewalld module
func NewFirewalldModule() *FirewalldModule {
	return &FirewalldModule{
		BaseModule: NewBaseModule("firewalld", []string{"present", "absent"}),
	}
}

// FirewalldConfig holds firewalld-specific configuration
type FirewalldConfig struct {
	// Zone configuration
	Zone       string // Zone name (public, internal, dmz, etc.)
	Source     string // Source IP/CIDR to add to zone
	Interface  string // Interface to add to zone
	Target     string // Zone default target (default, ACCEPT, DROP, REJECT)

	// Service/Port rules
	Service    string // Service name (ssh, http, https, etc.)
	Port       int    // Port number
	PortRange  string // Port range (e.g., "8000-8100")
	Protocol   string // tcp, udp

	// Rich rules
	RichRule   string // Full rich rule specification
	Family     string // ipv4 or ipv6
	SourceAddr string // Source address for rich rule
	DestAddr   string // Destination address for rich rule
	DestPort   int    // Destination port for rich rule
	Action     string // accept, reject, drop, mark, log
	Limit      string // Rate limit (e.g., "5/m")
	LogPrefix  string // Log prefix
	LogLevel   string // Log level (emerg, alert, crit, err, warning, notice, info, debug)

	// Forward ports
	ToPort     int    // Forward to port
	ToAddr     string // Forward to address

	// Masquerade
	Masquerade bool   // Enable masquerade for zone

	// ICMP blocks
	ICMPBlock  string // ICMP type to block
	ICMPBlockInversion bool // Invert ICMP block

	// Options
	Permanent  bool   // Make changes permanent
	Immediate  bool   // Apply immediately (runtime)
	Timeout    int    // Timeout in seconds (0 = permanent)
}

// Check checks the current state of a firewalld configuration
func (m *FirewalldModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("firewalld module is only supported on Linux")
	}

	// Check if firewalld is running
	running, err := m.isFirewalldRunning(ctx)
	if err != nil || !running {
		return nil, fmt.Errorf("firewalld is not running")
	}

	config, err := m.parseFirewalldConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse firewalld config: %w", err)
	}

	result.Metadata["zone"] = config.Zone

	// Determine what type of resource we're checking
	var exists bool
	switch {
	case config.Service != "":
		exists, err = m.serviceExists(ctx, config)
	case config.Port > 0 || config.PortRange != "":
		exists, err = m.portExists(ctx, config)
	case config.RichRule != "":
		exists, err = m.richRuleExists(ctx, config)
	case config.Source != "":
		exists, err = m.sourceExists(ctx, config)
	case config.Interface != "":
		exists, err = m.interfaceExists(ctx, config)
	case config.Masquerade:
		exists, err = m.masqueradeEnabled(ctx, config)
	case config.ICMPBlock != "":
		exists, err = m.icmpBlockExists(ctx, config)
	case config.ToPort > 0:
		exists, err = m.forwardPortExists(ctx, config)
	default:
		return nil, fmt.Errorf("no valid firewalld resource specified")
	}

	if err != nil {
		return nil, err
	}

	result.Present = exists
	if exists {
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	if decl.State == "present" {
		result.Matches = exists
		if !exists {
			result.Diff["firewalld"] = map[string]string{"current": "absent", "desired": "present"}
		}
	} else {
		result.Matches = !exists
		if exists {
			result.Diff["firewalld"] = map[string]string{"current": "present", "desired": "absent"}
		}
	}

	return result, nil
}

// Apply applies the firewalld configuration
func (m *FirewalldModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	if runtime.GOOS != "linux" {
		result.Error = fmt.Errorf("firewalld module is only supported on Linux")
		result.Comment = result.Error.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	running, err := m.isFirewalldRunning(ctx)
	if err != nil || !running {
		result.Error = fmt.Errorf("firewalld is not running")
		result.Comment = result.Error.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	config, err := m.parseFirewalldConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	var applyErr error
	var comment string

	if decl.State == "present" {
		applyErr, comment = m.addResource(ctx, config)
	} else {
		applyErr, comment = m.removeResource(ctx, config)
	}

	if applyErr != nil {
		result.Error = applyErr
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Comment = comment
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the firewalld configuration is in the desired state
func (m *FirewalldModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseFirewalldConfig parses firewalld configuration from declaration
func (m *FirewalldModule) parseFirewalldConfig(decl *StateDeclaration) (*FirewalldConfig, error) {
	config := &FirewalldConfig{
		Zone:      "public",
		Protocol:  "tcp",
		Family:    "ipv4",
		Action:    "accept",
		Permanent: true,
		Immediate: true,
	}

	config.Zone = getStringParameter(decl, "zone", "public")
	config.Source = getStringParameter(decl, "source", "")
	config.Interface = getStringParameter(decl, "interface", "")
	config.Target = getStringParameter(decl, "target", "")
	config.Service = getStringParameter(decl, "service", "")
	config.Port = getIntParameter(decl, "port", 0)
	config.PortRange = getStringParameter(decl, "port_range", "")
	config.Protocol = getStringParameter(decl, "protocol", "tcp")
	config.RichRule = getStringParameter(decl, "rich_rule", "")
	config.Family = getStringParameter(decl, "family", "ipv4")
	config.SourceAddr = getStringParameter(decl, "source_address", "")
	config.DestAddr = getStringParameter(decl, "destination_address", "")
	config.DestPort = getIntParameter(decl, "destination_port", 0)
	config.Action = getStringParameter(decl, "action", "accept")
	config.Limit = getStringParameter(decl, "limit", "")
	config.LogPrefix = getStringParameter(decl, "log_prefix", "")
	config.LogLevel = getStringParameter(decl, "log_level", "")
	config.ToPort = getIntParameter(decl, "to_port", 0)
	config.ToAddr = getStringParameter(decl, "to_address", "")
	config.Masquerade = getBoolParameter(decl, "masquerade", false)
	config.ICMPBlock = getStringParameter(decl, "icmp_block", "")
	config.ICMPBlockInversion = getBoolParameter(decl, "icmp_block_inversion", false)
	config.Permanent = getBoolParameter(decl, "permanent", true)
	config.Immediate = getBoolParameter(decl, "immediate", true)
	config.Timeout = getIntParameter(decl, "timeout", 0)

	// Validate protocol
	if config.Protocol != "tcp" && config.Protocol != "udp" && config.Protocol != "sctp" && config.Protocol != "dccp" {
		return nil, fmt.Errorf("invalid protocol: %s", config.Protocol)
	}

	return config, nil
}

// Helper functions to check existence

func (m *FirewalldModule) isFirewalldRunning(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "firewall-cmd", "--state")
	output, _ := cmd.Output()
	return strings.TrimSpace(string(output)) == "running", nil
}

func (m *FirewalldModule) serviceExists(ctx context.Context, config *FirewalldConfig) (bool, error) {
	args := []string{"--zone", config.Zone, "--query-service", config.Service}
	if config.Permanent {
		args = append(args, "--permanent")
	}
	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	return cmd.Run() == nil, nil
}

func (m *FirewalldModule) portExists(ctx context.Context, config *FirewalldConfig) (bool, error) {
	portStr := config.PortRange
	if portStr == "" {
		portStr = strconv.Itoa(config.Port)
	}
	portStr = fmt.Sprintf("%s/%s", portStr, config.Protocol)

	args := []string{"--zone", config.Zone, "--query-port", portStr}
	if config.Permanent {
		args = append(args, "--permanent")
	}
	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	return cmd.Run() == nil, nil
}

func (m *FirewalldModule) richRuleExists(ctx context.Context, config *FirewalldConfig) (bool, error) {
	rule := config.RichRule
	if rule == "" {
		rule = m.buildRichRule(config)
	}

	args := []string{"--zone", config.Zone, "--query-rich-rule", rule}
	if config.Permanent {
		args = append(args, "--permanent")
	}
	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	return cmd.Run() == nil, nil
}

func (m *FirewalldModule) sourceExists(ctx context.Context, config *FirewalldConfig) (bool, error) {
	args := []string{"--zone", config.Zone, "--query-source", config.Source}
	if config.Permanent {
		args = append(args, "--permanent")
	}
	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	return cmd.Run() == nil, nil
}

func (m *FirewalldModule) interfaceExists(ctx context.Context, config *FirewalldConfig) (bool, error) {
	args := []string{"--zone", config.Zone, "--query-interface", config.Interface}
	if config.Permanent {
		args = append(args, "--permanent")
	}
	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	return cmd.Run() == nil, nil
}

func (m *FirewalldModule) masqueradeEnabled(ctx context.Context, config *FirewalldConfig) (bool, error) {
	args := []string{"--zone", config.Zone, "--query-masquerade"}
	if config.Permanent {
		args = append(args, "--permanent")
	}
	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	return cmd.Run() == nil, nil
}

func (m *FirewalldModule) icmpBlockExists(ctx context.Context, config *FirewalldConfig) (bool, error) {
	args := []string{"--zone", config.Zone, "--query-icmp-block", config.ICMPBlock}
	if config.Permanent {
		args = append(args, "--permanent")
	}
	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	return cmd.Run() == nil, nil
}

func (m *FirewalldModule) forwardPortExists(ctx context.Context, config *FirewalldConfig) (bool, error) {
	fwdPort := fmt.Sprintf("port=%d:proto=%s:toport=%d", config.Port, config.Protocol, config.ToPort)
	if config.ToAddr != "" {
		fwdPort += ":toaddr=" + config.ToAddr
	}

	args := []string{"--zone", config.Zone, "--query-forward-port", fwdPort}
	if config.Permanent {
		args = append(args, "--permanent")
	}
	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	return cmd.Run() == nil, nil
}

// buildRichRule builds a rich rule from configuration
func (m *FirewalldModule) buildRichRule(config *FirewalldConfig) string {
	var parts []string

	parts = append(parts, "rule")

	if config.Family != "" {
		parts = append(parts, fmt.Sprintf("family=\"%s\"", config.Family))
	}

	if config.SourceAddr != "" {
		parts = append(parts, fmt.Sprintf("source address=\"%s\"", config.SourceAddr))
	}

	if config.DestAddr != "" {
		parts = append(parts, fmt.Sprintf("destination address=\"%s\"", config.DestAddr))
	}

	if config.Service != "" {
		parts = append(parts, fmt.Sprintf("service name=\"%s\"", config.Service))
	} else if config.DestPort > 0 {
		parts = append(parts, fmt.Sprintf("port port=\"%d\" protocol=\"%s\"", config.DestPort, config.Protocol))
	}

	if config.Limit != "" {
		parts = append(parts, fmt.Sprintf("limit value=\"%s\"", config.Limit))
	}

	if config.LogPrefix != "" || config.LogLevel != "" {
		logPart := "log"
		if config.LogPrefix != "" {
			logPart += fmt.Sprintf(" prefix=\"%s\"", config.LogPrefix)
		}
		if config.LogLevel != "" {
			logPart += fmt.Sprintf(" level=\"%s\"", config.LogLevel)
		}
		parts = append(parts, logPart)
	}

	switch config.Action {
	case "accept":
		parts = append(parts, "accept")
	case "reject":
		parts = append(parts, "reject")
	case "drop":
		parts = append(parts, "drop")
	case "mark":
		parts = append(parts, "mark")
	}

	return strings.Join(parts, " ")
}

// addResource adds a firewalld resource
func (m *FirewalldModule) addResource(ctx context.Context, config *FirewalldConfig) (error, string) {
	var args []string
	var comment string

	switch {
	case config.Service != "":
		args = []string{"--zone", config.Zone, "--add-service", config.Service}
		comment = fmt.Sprintf("Added service %s to zone %s", config.Service, config.Zone)
	case config.Port > 0 || config.PortRange != "":
		portStr := config.PortRange
		if portStr == "" {
			portStr = strconv.Itoa(config.Port)
		}
		args = []string{"--zone", config.Zone, "--add-port", fmt.Sprintf("%s/%s", portStr, config.Protocol)}
		comment = fmt.Sprintf("Added port %s/%s to zone %s", portStr, config.Protocol, config.Zone)
	case config.RichRule != "" || config.SourceAddr != "" || config.DestAddr != "":
		rule := config.RichRule
		if rule == "" {
			rule = m.buildRichRule(config)
		}
		args = []string{"--zone", config.Zone, "--add-rich-rule", rule}
		comment = fmt.Sprintf("Added rich rule to zone %s", config.Zone)
	case config.Source != "":
		args = []string{"--zone", config.Zone, "--add-source", config.Source}
		comment = fmt.Sprintf("Added source %s to zone %s", config.Source, config.Zone)
	case config.Interface != "":
		args = []string{"--zone", config.Zone, "--add-interface", config.Interface}
		comment = fmt.Sprintf("Added interface %s to zone %s", config.Interface, config.Zone)
	case config.Masquerade:
		args = []string{"--zone", config.Zone, "--add-masquerade"}
		comment = fmt.Sprintf("Enabled masquerade on zone %s", config.Zone)
	case config.ICMPBlock != "":
		args = []string{"--zone", config.Zone, "--add-icmp-block", config.ICMPBlock}
		comment = fmt.Sprintf("Added ICMP block %s to zone %s", config.ICMPBlock, config.Zone)
	case config.ToPort > 0:
		fwdPort := fmt.Sprintf("port=%d:proto=%s:toport=%d", config.Port, config.Protocol, config.ToPort)
		if config.ToAddr != "" {
			fwdPort += ":toaddr=" + config.ToAddr
		}
		args = []string{"--zone", config.Zone, "--add-forward-port", fwdPort}
		comment = fmt.Sprintf("Added forward port to zone %s", config.Zone)
	default:
		return fmt.Errorf("no valid firewalld resource specified"), ""
	}

	// Add permanent and timeout flags
	if config.Permanent {
		args = append(args, "--permanent")
	}
	if config.Timeout > 0 {
		args = append(args, "--timeout", strconv.Itoa(config.Timeout))
	}

	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("firewall-cmd failed: %w (output: %s)", err, string(output)), ""
	}

	// If permanent and immediate, reload
	if config.Permanent && config.Immediate {
		cmd = exec.CommandContext(ctx, "firewall-cmd", "--reload")
		cmd.Run()
	}

	return nil, comment
}

// removeResource removes a firewalld resource
func (m *FirewalldModule) removeResource(ctx context.Context, config *FirewalldConfig) (error, string) {
	var args []string
	var comment string

	switch {
	case config.Service != "":
		args = []string{"--zone", config.Zone, "--remove-service", config.Service}
		comment = fmt.Sprintf("Removed service %s from zone %s", config.Service, config.Zone)
	case config.Port > 0 || config.PortRange != "":
		portStr := config.PortRange
		if portStr == "" {
			portStr = strconv.Itoa(config.Port)
		}
		args = []string{"--zone", config.Zone, "--remove-port", fmt.Sprintf("%s/%s", portStr, config.Protocol)}
		comment = fmt.Sprintf("Removed port %s/%s from zone %s", portStr, config.Protocol, config.Zone)
	case config.RichRule != "" || config.SourceAddr != "" || config.DestAddr != "":
		rule := config.RichRule
		if rule == "" {
			rule = m.buildRichRule(config)
		}
		args = []string{"--zone", config.Zone, "--remove-rich-rule", rule}
		comment = fmt.Sprintf("Removed rich rule from zone %s", config.Zone)
	case config.Source != "":
		args = []string{"--zone", config.Zone, "--remove-source", config.Source}
		comment = fmt.Sprintf("Removed source %s from zone %s", config.Source, config.Zone)
	case config.Interface != "":
		args = []string{"--zone", config.Zone, "--remove-interface", config.Interface}
		comment = fmt.Sprintf("Removed interface %s from zone %s", config.Interface, config.Zone)
	case config.Masquerade:
		args = []string{"--zone", config.Zone, "--remove-masquerade"}
		comment = fmt.Sprintf("Disabled masquerade on zone %s", config.Zone)
	case config.ICMPBlock != "":
		args = []string{"--zone", config.Zone, "--remove-icmp-block", config.ICMPBlock}
		comment = fmt.Sprintf("Removed ICMP block %s from zone %s", config.ICMPBlock, config.Zone)
	case config.ToPort > 0:
		fwdPort := fmt.Sprintf("port=%d:proto=%s:toport=%d", config.Port, config.Protocol, config.ToPort)
		if config.ToAddr != "" {
			fwdPort += ":toaddr=" + config.ToAddr
		}
		args = []string{"--zone", config.Zone, "--remove-forward-port", fwdPort}
		comment = fmt.Sprintf("Removed forward port from zone %s", config.Zone)
	default:
		return fmt.Errorf("no valid firewalld resource specified"), ""
	}

	if config.Permanent {
		args = append(args, "--permanent")
	}

	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("firewall-cmd failed: %w (output: %s)", err, string(output)), ""
	}

	if config.Permanent && config.Immediate {
		cmd = exec.CommandContext(ctx, "firewall-cmd", "--reload")
		cmd.Run()
	}

	return nil, comment
}

func init() {
	RegisterModule(NewFirewalldModule())
}
