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

// FirewallModule implements cross-platform firewall rule management
type FirewallModule struct {
	*BaseModule
}

// NewFirewallModule creates a new firewall module
func NewFirewallModule() *FirewallModule {
	return &FirewallModule{
		BaseModule: NewBaseModule("firewall", []string{"present", "absent"}),
	}
}

// FirewallBackend represents the underlying firewall technology
type FirewallBackend string

const (
	FBUnknown   FirewallBackend = "unknown"
	FBIptables  FirewallBackend = "iptables"
	FBNftables  FirewallBackend = "nftables"
	FBFirewalld FirewallBackend = "firewalld"
	FBPF        FirewallBackend = "pf"
	FBNetsh     FirewallBackend = "netsh"
)

// FirewallAction represents what to do with matching traffic
type FirewallAction string

const (
	FAAccept FirewallAction = "accept"
	FADrop   FirewallAction = "drop"
	FAReject FirewallAction = "reject"
)

// FirewallDirection represents traffic direction
type FirewallDirection string

const (
	FDInput   FirewallDirection = "input"
	FDOutput  FirewallDirection = "output"
	FDForward FirewallDirection = "forward"
)

// FirewallConfig holds firewall rule configuration
type FirewallConfig struct {
	Name        string            // Rule name/identifier
	Protocol    string            // tcp, udp, icmp, all
	Port        int               // Destination port (0 = any)
	PortRange   string            // Port range (e.g., "8000:8100")
	SourcePort  int               // Source port
	Source      string            // Source IP/CIDR
	Destination string            // Destination IP/CIDR
	Interface   string            // Network interface
	Action      FirewallAction    // accept, drop, reject
	Direction   FirewallDirection // input, output, forward
	Zone        string            // Zone (firewalld) or profile (Windows)
	Comment     string            // Rule comment
	Position    int               // Rule position (0 = append)
	State       string            // Connection state (new, established, related)
	// Linux-specific
	Chain string // iptables/nftables chain
	Table string // iptables table (filter, nat, mangle) or nftables table
	// Windows-specific
	Profile   string // domain, private, public, any
	Program   string // Program path for application rules
	LocalPort int    // For Windows rules
}

// Check checks the current state of a firewall rule
func (m *FirewallModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseFirewallConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse firewall config: %w", err)
	}

	// Detect firewall backend
	backend, err := m.detectFirewallBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to detect firewall backend: %w", err)
	}
	result.Metadata["backend"] = string(backend)

	// Check if rule exists
	ruleExists, currentRule, err := m.checkRuleExists(ctx, config, backend)
	if err != nil {
		return nil, fmt.Errorf("failed to check rule: %w", err)
	}

	result.Present = ruleExists
	if ruleExists {
		result.CurrentState = "present"
		result.Metadata["current_rule"] = currentRule
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		if !ruleExists {
			result.Matches = false
			result.Diff["rule"] = map[string]string{"current": "absent", "desired": "present"}
		} else {
			result.Matches = true
		}
	case "absent":
		result.Matches = !ruleExists
		if ruleExists {
			result.Diff["rule"] = map[string]string{"current": "present", "desired": "absent"}
		}
	}

	return result, nil
}

// Apply applies the firewall rule configuration
func (m *FirewallModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	config, err := m.parseFirewallConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Detect firewall backend
	backend, err := m.detectFirewallBackend()
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect firewall backend: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "present":
		applyErr = m.addRule(ctx, config, backend, result)
	case "absent":
		applyErr = m.deleteRule(ctx, config, backend)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Deleted firewall rule: %s", config.Name)
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
	return result, nil
}

// Test tests if the firewall rule is in the desired state
func (m *FirewallModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseFirewallConfig extracts firewall configuration from declaration
func (m *FirewallModule) parseFirewallConfig(decl *StateDeclaration) (*FirewallConfig, error) {
	config := &FirewallConfig{
		Name:      decl.ID,
		Protocol:  "tcp",
		Action:    FAAccept,
		Direction: FDInput,
		Chain:     "INPUT",
		Table:     "filter",
		Profile:   "any",
	}

	// Override name if explicitly set
	if name := getStringParameter(decl, "name", ""); name != "" {
		config.Name = name
	}

	config.Protocol = getStringParameter(decl, "protocol", "tcp")
	config.Port = getIntParameter(decl, "port", 0)
	config.PortRange = getStringParameter(decl, "port_range", "")
	config.SourcePort = getIntParameter(decl, "source_port", 0)
	config.Source = getStringParameter(decl, "source", "")
	config.Destination = getStringParameter(decl, "destination", "")
	config.Interface = getStringParameter(decl, "interface", "")
	config.Zone = getStringParameter(decl, "zone", "")
	config.Comment = getStringParameter(decl, "comment", "")
	config.Position = getIntParameter(decl, "position", 0)
	config.State = getStringParameter(decl, "state_match", "") // renamed to avoid collision
	config.Chain = getStringParameter(decl, "chain", "INPUT")
	config.Table = getStringParameter(decl, "table", "filter")
	config.Profile = getStringParameter(decl, "profile", "any")
	config.Program = getStringParameter(decl, "program", "")
	config.LocalPort = getIntParameter(decl, "local_port", 0)

	// Parse action
	actionStr := getStringParameter(decl, "action", "accept")
	switch strings.ToLower(actionStr) {
	case "accept", "allow":
		config.Action = FAAccept
	case "drop", "block":
		config.Action = FADrop
	case "reject":
		config.Action = FAReject
	default:
		return nil, fmt.Errorf("invalid action: %s (must be accept, drop, or reject)", actionStr)
	}

	// Parse direction
	dirStr := getStringParameter(decl, "direction", "input")
	switch strings.ToLower(dirStr) {
	case "input", "in":
		config.Direction = FDInput
		config.Chain = "INPUT"
	case "output", "out":
		config.Direction = FDOutput
		config.Chain = "OUTPUT"
	case "forward", "fwd":
		config.Direction = FDForward
		config.Chain = "FORWARD"
	default:
		return nil, fmt.Errorf("invalid direction: %s", dirStr)
	}

	// Override chain if explicitly set
	if chain := getStringParameter(decl, "chain", ""); chain != "" {
		config.Chain = chain
	}

	// Validate protocol
	validProtocols := map[string]bool{"tcp": true, "udp": true, "icmp": true, "all": true, "any": true}
	if !validProtocols[strings.ToLower(config.Protocol)] {
		return nil, fmt.Errorf("invalid protocol: %s", config.Protocol)
	}
	if config.Protocol == "any" {
		config.Protocol = "all"
	}

	return config, nil
}

// detectFirewallBackend detects which firewall technology is available
func (m *FirewallModule) detectFirewallBackend() (FirewallBackend, error) {
	switch runtime.GOOS {
	case "linux":
		return m.detectLinuxFirewall()
	case "darwin":
		return FBPF, nil
	case "windows":
		return FBNetsh, nil
	default:
		return FBUnknown, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// detectLinuxFirewall detects the Linux firewall technology
func (m *FirewallModule) detectLinuxFirewall() (FirewallBackend, error) {
	// Check for firewalld first (most user-friendly)
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		// Check if firewalld is running
		cmd := exec.Command("systemctl", "is-active", "firewalld")
		if output, _ := cmd.Output(); strings.TrimSpace(string(output)) == "active" {
			return FBFirewalld, nil
		}
	}

	// Check for nftables
	if _, err := exec.LookPath("nft"); err == nil {
		// Check if nftables has tables defined
		cmd := exec.Command("nft", "list", "tables")
		if output, err := cmd.Output(); err == nil && len(output) > 0 {
			return FBNftables, nil
		}
	}

	// Fall back to iptables
	if _, err := exec.LookPath("iptables"); err == nil {
		return FBIptables, nil
	}

	return FBUnknown, fmt.Errorf("no supported firewall found (checked: firewalld, nftables, iptables)")
}

// checkRuleExists checks if a firewall rule exists
func (m *FirewallModule) checkRuleExists(ctx context.Context, config *FirewallConfig, backend FirewallBackend) (bool, string, error) {
	switch backend {
	case FBIptables:
		return m.checkRuleExistsIptables(ctx, config)
	case FBNftables:
		return m.checkRuleExistsNftables(ctx, config)
	case FBFirewalld:
		return m.checkRuleExistsFirewalld(ctx, config)
	case FBPF:
		return m.checkRuleExistsPF(ctx, config)
	case FBNetsh:
		return m.checkRuleExistsNetsh(ctx, config)
	default:
		return false, "", fmt.Errorf("unsupported firewall backend: %s", backend)
	}
}

// checkRuleExistsIptables checks if an iptables rule exists
func (m *FirewallModule) checkRuleExistsIptables(ctx context.Context, config *FirewallConfig) (bool, string, error) {
	args := []string{"-t", config.Table, "-C", config.Chain}
	args = append(args, m.buildIptablesRuleArgs(config)...)

	cmd := exec.CommandContext(ctx, "iptables", args...)
	err := cmd.Run()
	if err == nil {
		return true, m.buildRuleDescription(config), nil
	}
	// Exit code 1 means rule doesn't exist
	return false, "", nil
}

// checkRuleExistsNftables checks if an nftables rule exists
func (m *FirewallModule) checkRuleExistsNftables(ctx context.Context, config *FirewallConfig) (bool, string, error) {
	// List rules and search for our rule by comment
	args := []string{"list", "chain", "ip", config.Table, strings.ToLower(config.Chain)}
	cmd := exec.CommandContext(ctx, "nft", args...)
	output, err := cmd.Output()
	if err != nil {
		// Chain might not exist
		return false, "", nil
	}

	// Look for rule with matching comment or port/protocol
	outputStr := string(output)
	searchStr := config.Name
	if config.Comment != "" {
		searchStr = config.Comment
	}
	if strings.Contains(outputStr, searchStr) {
		return true, m.buildRuleDescription(config), nil
	}

	// Also search by port if specified
	if config.Port > 0 {
		portStr := fmt.Sprintf("dport %d", config.Port)
		if strings.Contains(outputStr, portStr) {
			return true, m.buildRuleDescription(config), nil
		}
	}

	return false, "", nil
}

// checkRuleExistsFirewalld checks if a firewalld rule exists
func (m *FirewallModule) checkRuleExistsFirewalld(ctx context.Context, config *FirewallConfig) (bool, string, error) {
	zone := config.Zone
	if zone == "" {
		// Get default zone
		cmd := exec.CommandContext(ctx, "firewall-cmd", "--get-default-zone")
		output, err := cmd.Output()
		if err != nil {
			return false, "", err
		}
		zone = strings.TrimSpace(string(output))
	}

	// Check if port is open in zone
	if config.Port > 0 {
		args := []string{"--zone", zone, "--query-port", fmt.Sprintf("%d/%s", config.Port, config.Protocol)}
		cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
		err := cmd.Run()
		if err == nil {
			return true, m.buildRuleDescription(config), nil
		}
	}

	// Check rich rules if we have source/destination
	if config.Source != "" || config.Destination != "" {
		args := []string{"--zone", zone, "--query-rich-rule", m.buildFirewalldRichRule(config)}
		cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
		err := cmd.Run()
		if err == nil {
			return true, m.buildRuleDescription(config), nil
		}
	}

	return false, "", nil
}

// checkRuleExistsPF checks if a pf rule exists (macOS)
func (m *FirewallModule) checkRuleExistsPF(ctx context.Context, config *FirewallConfig) (bool, string, error) {
	// Check if pf is enabled
	cmd := exec.CommandContext(ctx, "pfctl", "-s", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", nil
	}
	if !strings.Contains(string(output), "Status: Enabled") {
		return false, "", nil
	}

	// Get current rules
	cmd = exec.CommandContext(ctx, "pfctl", "-s", "rules")
	output, err = cmd.Output()
	if err != nil {
		return false, "", nil
	}

	// Search for our rule
	rulePattern := m.buildPFRulePattern(config)
	if strings.Contains(string(output), rulePattern) {
		return true, m.buildRuleDescription(config), nil
	}

	return false, "", nil
}

// checkRuleExistsNetsh checks if a Windows Firewall rule exists
func (m *FirewallModule) checkRuleExistsNetsh(ctx context.Context, config *FirewallConfig) (bool, string, error) {
	args := []string{"advfirewall", "firewall", "show", "rule", "name=" + config.Name}
	cmd := exec.CommandContext(ctx, "netsh", args...)
	output, err := cmd.Output()
	if err != nil {
		return false, "", nil
	}

	if strings.Contains(string(output), "Rule Name:") {
		return true, m.buildRuleDescription(config), nil
	}

	return false, "", nil
}

// addRule adds a firewall rule
func (m *FirewallModule) addRule(ctx context.Context, config *FirewallConfig, backend FirewallBackend, result *StateResult) error {
	switch backend {
	case FBIptables:
		return m.addRuleIptables(ctx, config, result)
	case FBNftables:
		return m.addRuleNftables(ctx, config, result)
	case FBFirewalld:
		return m.addRuleFirewalld(ctx, config, result)
	case FBPF:
		return m.addRulePF(ctx, config, result)
	case FBNetsh:
		return m.addRuleNetsh(ctx, config, result)
	default:
		return fmt.Errorf("unsupported firewall backend: %s", backend)
	}
}

// addRuleIptables adds an iptables rule
func (m *FirewallModule) addRuleIptables(ctx context.Context, config *FirewallConfig, result *StateResult) error {
	action := "-A"
	if config.Position > 0 {
		action = "-I"
	}

	args := []string{"-t", config.Table, action, config.Chain}
	if config.Position > 0 {
		args = append(args, strconv.Itoa(config.Position))
	}
	args = append(args, m.buildIptablesRuleArgs(config)...)

	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add iptables rule: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Added iptables rule: %s", m.buildRuleDescription(config))
	return nil
}

// addRuleNftables adds an nftables rule
func (m *FirewallModule) addRuleNftables(ctx context.Context, config *FirewallConfig, result *StateResult) error {
	// Ensure table and chain exist
	tableName := config.Table
	chainName := strings.ToLower(config.Chain)

	// Create table if it doesn't exist
	cmd := exec.CommandContext(ctx, "nft", "add", "table", "ip", tableName)
	cmd.Run() // Ignore error if exists

	// Create chain if it doesn't exist
	chainType := "filter"
	hookName := chainName
	priority := "0"
	cmd = exec.CommandContext(ctx, "nft", "add", "chain", "ip", tableName, chainName,
		"{ type "+chainType+" hook "+hookName+" priority "+priority+"; }")
	cmd.Run() // Ignore error if exists

	// Build the rule
	rule := m.buildNftablesRule(config)

	// Add the rule
	args := []string{"add", "rule", "ip", tableName, chainName}
	args = append(args, strings.Fields(rule)...)
	cmd = exec.CommandContext(ctx, "nft", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add nftables rule: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Added nftables rule: %s", m.buildRuleDescription(config))
	return nil
}

// addRuleFirewalld adds a firewalld rule
func (m *FirewallModule) addRuleFirewalld(ctx context.Context, config *FirewallConfig, result *StateResult) error {
	zone := config.Zone
	if zone == "" {
		// Get default zone
		cmd := exec.CommandContext(ctx, "firewall-cmd", "--get-default-zone")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get default zone: %w", err)
		}
		zone = strings.TrimSpace(string(output))
	}

	var args []string

	// Simple port rule
	if config.Port > 0 && config.Source == "" && config.Destination == "" {
		args = []string{"--zone", zone, "--add-port", fmt.Sprintf("%d/%s", config.Port, config.Protocol), "--permanent"}
	} else {
		// Rich rule for more complex scenarios
		richRule := m.buildFirewalldRichRule(config)
		args = []string{"--zone", zone, "--add-rich-rule", richRule, "--permanent"}
	}

	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add firewalld rule: %w (output: %s)", err, string(output))
	}

	// Reload firewalld
	cmd = exec.CommandContext(ctx, "firewall-cmd", "--reload")
	cmd.Run() // Ignore reload errors

	result.Comment = fmt.Sprintf("Added firewalld rule: %s", m.buildRuleDescription(config))
	return nil
}

// addRulePF adds a pf rule (macOS)
func (m *FirewallModule) addRulePF(ctx context.Context, config *FirewallConfig, result *StateResult) error {
	// Build the pf rule
	rule := m.buildPFRule(config)

	// pf requires writing to the anchor or main rules file
	// For simplicity, we use an anchor named "keystone"
	anchorName := "com.keystone"

	// Create anchor rule in main config if not exists
	// This is typically done once, but we check anyway

	// Add rule to anchor
	// Note: This requires elevated privileges
	cmd := exec.CommandContext(ctx, "pfctl", "-a", anchorName, "-f", "-")
	cmd.Stdin = strings.NewReader(rule + "\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add pf rule: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Added pf rule to anchor %s: %s", anchorName, m.buildRuleDescription(config))
	return nil
}

// addRuleNetsh adds a Windows Firewall rule
func (m *FirewallModule) addRuleNetsh(ctx context.Context, config *FirewallConfig, result *StateResult) error {
	direction := "in"
	if config.Direction == FDOutput {
		direction = "out"
	}

	action := "allow"
	if config.Action == FADrop {
		action = "block"
	}

	args := []string{
		"advfirewall", "firewall", "add", "rule",
		"name=" + config.Name,
		"dir=" + direction,
		"action=" + action,
	}

	// Add protocol
	if config.Protocol != "all" {
		args = append(args, "protocol="+config.Protocol)
	}

	// Add port
	if config.Port > 0 {
		args = append(args, "localport="+strconv.Itoa(config.Port))
	} else if config.PortRange != "" {
		args = append(args, "localport="+config.PortRange)
	}

	// Add remote address (source)
	if config.Source != "" {
		args = append(args, "remoteip="+config.Source)
	}

	// Add local address (destination)
	if config.Destination != "" {
		args = append(args, "localip="+config.Destination)
	}

	// Add profile
	if config.Profile != "" && config.Profile != "any" {
		args = append(args, "profile="+config.Profile)
	}

	// Add program
	if config.Program != "" {
		args = append(args, "program="+config.Program)
	}

	// Add description
	if config.Comment != "" {
		args = append(args, "description="+config.Comment)
	}

	cmd := exec.CommandContext(ctx, "netsh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add Windows Firewall rule: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Added Windows Firewall rule: %s", config.Name)
	return nil
}

// deleteRule deletes a firewall rule
func (m *FirewallModule) deleteRule(ctx context.Context, config *FirewallConfig, backend FirewallBackend) error {
	switch backend {
	case FBIptables:
		return m.deleteRuleIptables(ctx, config)
	case FBNftables:
		return m.deleteRuleNftables(ctx, config)
	case FBFirewalld:
		return m.deleteRuleFirewalld(ctx, config)
	case FBPF:
		return m.deleteRulePF(ctx, config)
	case FBNetsh:
		return m.deleteRuleNetsh(ctx, config)
	default:
		return fmt.Errorf("unsupported firewall backend: %s", backend)
	}
}

// deleteRuleIptables deletes an iptables rule
func (m *FirewallModule) deleteRuleIptables(ctx context.Context, config *FirewallConfig) error {
	args := []string{"-t", config.Table, "-D", config.Chain}
	args = append(args, m.buildIptablesRuleArgs(config)...)

	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete iptables rule: %w (output: %s)", err, string(output))
	}
	return nil
}

// deleteRuleNftables deletes an nftables rule
func (m *FirewallModule) deleteRuleNftables(ctx context.Context, config *FirewallConfig) error {
	// nftables requires rule handle to delete, so we need to find it first
	tableName := config.Table
	chainName := strings.ToLower(config.Chain)

	// Get rules with handles
	args := []string{"-a", "list", "chain", "ip", tableName, chainName}
	cmd := exec.CommandContext(ctx, "nft", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil // Chain doesn't exist, rule is already gone
	}

	// Find the handle for our rule
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Look for matching rule and extract handle
		if config.Port > 0 && strings.Contains(line, fmt.Sprintf("dport %d", config.Port)) {
			// Extract handle
			parts := strings.Split(line, "# handle ")
			if len(parts) == 2 {
				handle := strings.TrimSpace(parts[1])
				// Delete by handle
				cmd = exec.CommandContext(ctx, "nft", "delete", "rule", "ip", tableName, chainName, "handle", handle)
				cmd.Run()
				return nil
			}
		}
	}

	return nil
}

// deleteRuleFirewalld deletes a firewalld rule
func (m *FirewallModule) deleteRuleFirewalld(ctx context.Context, config *FirewallConfig) error {
	zone := config.Zone
	if zone == "" {
		cmd := exec.CommandContext(ctx, "firewall-cmd", "--get-default-zone")
		output, err := cmd.Output()
		if err != nil {
			return err
		}
		zone = strings.TrimSpace(string(output))
	}

	var args []string
	if config.Port > 0 && config.Source == "" && config.Destination == "" {
		args = []string{"--zone", zone, "--remove-port", fmt.Sprintf("%d/%s", config.Port, config.Protocol), "--permanent"}
	} else {
		richRule := m.buildFirewalldRichRule(config)
		args = []string{"--zone", zone, "--remove-rich-rule", richRule, "--permanent"}
	}

	cmd := exec.CommandContext(ctx, "firewall-cmd", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove firewalld rule: %w (output: %s)", err, string(output))
	}

	// Reload
	cmd = exec.CommandContext(ctx, "firewall-cmd", "--reload")
	cmd.Run()

	return nil
}

// deleteRulePF deletes a pf rule
func (m *FirewallModule) deleteRulePF(ctx context.Context, config *FirewallConfig) error {
	// For pf, we would need to remove the rule from the anchor
	// This typically involves rewriting the anchor without the rule
	anchorName := "com.keystone"

	// Get current rules
	cmd := exec.CommandContext(ctx, "pfctl", "-a", anchorName, "-s", "rules")
	output, err := cmd.Output()
	if err != nil {
		return nil // Anchor might not exist
	}

	// Filter out our rule
	ruleToRemove := m.buildPFRule(config)
	var newRules []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != ruleToRemove {
			newRules = append(newRules, line)
		}
	}

	// Reload anchor with remaining rules
	cmd = exec.CommandContext(ctx, "pfctl", "-a", anchorName, "-f", "-")
	cmd.Stdin = strings.NewReader(strings.Join(newRules, "\n"))
	cmd.Run()

	return nil
}

// deleteRuleNetsh deletes a Windows Firewall rule
func (m *FirewallModule) deleteRuleNetsh(ctx context.Context, config *FirewallConfig) error {
	args := []string{"advfirewall", "firewall", "delete", "rule", "name=" + config.Name}

	cmd := exec.CommandContext(ctx, "netsh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete Windows Firewall rule: %w (output: %s)", err, string(output))
	}
	return nil
}

// Helper functions

// buildIptablesRuleArgs builds iptables command arguments for a rule
func (m *FirewallModule) buildIptablesRuleArgs(config *FirewallConfig) []string {
	var args []string

	// Protocol
	if config.Protocol != "" && config.Protocol != "all" {
		args = append(args, "-p", config.Protocol)
	}

	// Source
	if config.Source != "" {
		args = append(args, "-s", config.Source)
	}

	// Destination
	if config.Destination != "" {
		args = append(args, "-d", config.Destination)
	}

	// Interface (input/output based on chain)
	if config.Interface != "" {
		if config.Direction == FDInput {
			args = append(args, "-i", config.Interface)
		} else {
			args = append(args, "-o", config.Interface)
		}
	}

	// Port
	if config.Port > 0 {
		args = append(args, "--dport", strconv.Itoa(config.Port))
	} else if config.PortRange != "" {
		args = append(args, "--dport", config.PortRange)
	}

	// Source port
	if config.SourcePort > 0 {
		args = append(args, "--sport", strconv.Itoa(config.SourcePort))
	}

	// Connection state
	if config.State != "" {
		args = append(args, "-m", "state", "--state", strings.ToUpper(config.State))
	}

	// Comment
	if config.Comment != "" {
		args = append(args, "-m", "comment", "--comment", config.Comment)
	}

	// Action
	switch config.Action {
	case FAAccept:
		args = append(args, "-j", "ACCEPT")
	case FADrop:
		args = append(args, "-j", "DROP")
	case FAReject:
		args = append(args, "-j", "REJECT")
	}

	return args
}

// buildNftablesRule builds an nftables rule string
func (m *FirewallModule) buildNftablesRule(config *FirewallConfig) string {
	var parts []string

	// Protocol
	if config.Protocol != "" && config.Protocol != "all" {
		parts = append(parts, config.Protocol)
	}

	// Source
	if config.Source != "" {
		parts = append(parts, "ip", "saddr", config.Source)
	}

	// Destination
	if config.Destination != "" {
		parts = append(parts, "ip", "daddr", config.Destination)
	}

	// Port
	if config.Port > 0 {
		parts = append(parts, "dport", strconv.Itoa(config.Port))
	}

	// Comment
	if config.Comment != "" {
		parts = append(parts, "comment", "\""+config.Comment+"\"")
	}

	// Action
	switch config.Action {
	case FAAccept:
		parts = append(parts, "accept")
	case FADrop:
		parts = append(parts, "drop")
	case FAReject:
		parts = append(parts, "reject")
	}

	return strings.Join(parts, " ")
}

// buildFirewalldRichRule builds a firewalld rich rule
func (m *FirewallModule) buildFirewalldRichRule(config *FirewallConfig) string {
	var parts []string

	parts = append(parts, "rule")

	// Source
	if config.Source != "" {
		parts = append(parts, fmt.Sprintf("source address=\"%s\"", config.Source))
	}

	// Destination
	if config.Destination != "" {
		parts = append(parts, fmt.Sprintf("destination address=\"%s\"", config.Destination))
	}

	// Port
	if config.Port > 0 {
		parts = append(parts, fmt.Sprintf("port port=\"%d\" protocol=\"%s\"", config.Port, config.Protocol))
	}

	// Action
	switch config.Action {
	case FAAccept:
		parts = append(parts, "accept")
	case FADrop:
		parts = append(parts, "drop")
	case FAReject:
		parts = append(parts, "reject")
	}

	return strings.Join(parts, " ")
}

// buildPFRule builds a pf rule
func (m *FirewallModule) buildPFRule(config *FirewallConfig) string {
	var parts []string

	// Action
	switch config.Action {
	case FAAccept:
		parts = append(parts, "pass")
	case FADrop:
		parts = append(parts, "block")
	case FAReject:
		parts = append(parts, "block return")
	}

	// Direction
	switch config.Direction {
	case FDInput:
		parts = append(parts, "in")
	case FDOutput:
		parts = append(parts, "out")
	}

	// Quick (return immediately if matched)
	parts = append(parts, "quick")

	// Interface
	if config.Interface != "" {
		parts = append(parts, "on", config.Interface)
	}

	// Protocol
	if config.Protocol != "" && config.Protocol != "all" {
		parts = append(parts, "proto", config.Protocol)
	}

	// Source
	if config.Source != "" {
		parts = append(parts, "from", config.Source)
	} else {
		parts = append(parts, "from", "any")
	}

	// Destination
	if config.Destination != "" {
		parts = append(parts, "to", config.Destination)
	} else {
		parts = append(parts, "to", "any")
	}

	// Port
	if config.Port > 0 {
		parts = append(parts, "port", strconv.Itoa(config.Port))
	}

	return strings.Join(parts, " ")
}

// buildPFRulePattern builds a pattern for searching existing pf rules
func (m *FirewallModule) buildPFRulePattern(config *FirewallConfig) string {
	// Build a partial match pattern
	parts := []string{}

	if config.Port > 0 {
		parts = append(parts, fmt.Sprintf("port %d", config.Port))
	}

	if config.Protocol != "" && config.Protocol != "all" {
		parts = append(parts, config.Protocol)
	}

	return strings.Join(parts, " ")
}

// buildRuleDescription builds a human-readable rule description
func (m *FirewallModule) buildRuleDescription(config *FirewallConfig) string {
	parts := []string{string(config.Action)}

	if config.Protocol != "" && config.Protocol != "all" {
		parts = append(parts, config.Protocol)
	}

	if config.Port > 0 {
		parts = append(parts, fmt.Sprintf("port %d", config.Port))
	} else if config.PortRange != "" {
		parts = append(parts, fmt.Sprintf("ports %s", config.PortRange))
	}

	if config.Source != "" {
		parts = append(parts, fmt.Sprintf("from %s", config.Source))
	}

	if config.Destination != "" {
		parts = append(parts, fmt.Sprintf("to %s", config.Destination))
	}

	return strings.Join(parts, " ")
}

func init() {
	RegisterModule(NewFirewallModule())
}
