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

// IptablesModule implements Linux iptables rule management
type IptablesModule struct {
	*BaseModule
}

// NewIptablesModule creates a new iptables module
func NewIptablesModule() *IptablesModule {
	return &IptablesModule{
		BaseModule: NewBaseModule("iptables", []string{"present", "absent", "flush", "policy"}),
	}
}

// IptablesConfig holds iptables-specific configuration
type IptablesConfig struct {
	Table        string   // filter, nat, mangle, raw, security
	Chain        string   // INPUT, OUTPUT, FORWARD, or custom
	Rule         string   // Full rule specification (alternative to individual params)
	Protocol     string   // tcp, udp, icmp, all
	Source       string   // Source IP/CIDR
	Destination  string   // Destination IP/CIDR
	InInterface  string   // Input interface
	OutInterface string   // Output interface
	Match        string   // Match extension (state, multiport, etc.)
	MatchOptions string   // Options for match extension
	SourcePort   string   // Source port(s)
	DestPort     string   // Destination port(s)
	Jump         string   // Target (ACCEPT, DROP, REJECT, LOG, custom chain)
	GoTo         string   // Goto target (alternative to jump)
	Comment      string   // Rule comment
	Position     int      // Rule position (0 = append)
	State        string   // Connection state for -m state
	Limit        string   // Rate limit (e.g., "5/minute")
	LogPrefix    string   // Log prefix for LOG target
	RejectWith   string   // Reject type (icmp-host-prohibited, etc.)
	ToSource     string   // SNAT target address
	ToDest       string   // DNAT target address
	ToPort       string   // Port redirect
	Extra        []string // Extra arguments
	Wait         int      // Wait for xtables lock (seconds)
	Policy       string   // Chain policy (for policy state)
}

// Check checks the current state of an iptables rule
func (m *IptablesModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("iptables module is only supported on Linux")
	}

	config, err := m.parseIptablesConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse iptables config: %w", err)
	}

	result.Metadata["table"] = config.Table
	result.Metadata["chain"] = config.Chain

	switch decl.State {
	case "present", "absent":
		ruleExists, err := m.checkRuleExists(ctx, config)
		if err != nil {
			return nil, err
		}

		result.Present = ruleExists
		if ruleExists {
			result.CurrentState = "present"
		} else {
			result.CurrentState = "absent"
		}

		if decl.State == "present" {
			result.Matches = ruleExists
			if !ruleExists {
				result.Diff["rule"] = map[string]string{"current": "absent", "desired": "present"}
			}
		} else {
			result.Matches = !ruleExists
			if ruleExists {
				result.Diff["rule"] = map[string]string{"current": "present", "desired": "absent"}
			}
		}

	case "flush":
		// For flush, check if chain has rules
		hasRules, err := m.chainHasRules(ctx, config)
		if err != nil {
			return nil, err
		}
		result.Present = hasRules
		result.CurrentState = "has_rules"
		result.Matches = !hasRules
		if hasRules {
			result.Diff["chain"] = map[string]string{"current": "has_rules", "desired": "flushed"}
		}

	case "policy":
		currentPolicy, err := m.getChainPolicy(ctx, config)
		if err != nil {
			return nil, err
		}
		result.Present = true
		result.CurrentState = currentPolicy
		result.Metadata["current_policy"] = currentPolicy
		result.Matches = strings.EqualFold(currentPolicy, config.Policy)
		if !result.Matches {
			result.Diff["policy"] = map[string]string{"current": currentPolicy, "desired": config.Policy}
		}
	}

	return result, nil
}

// Apply applies the iptables configuration
func (m *IptablesModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Error = fmt.Errorf("iptables module is only supported on Linux")
		result.Comment = result.Error.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	config, err := m.parseIptablesConfig(decl)
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
	switch decl.State {
	case "present":
		applyErr = m.addRule(ctx, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Added iptables rule to %s/%s", config.Table, config.Chain)
		}
	case "absent":
		applyErr = m.deleteRule(ctx, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Removed iptables rule from %s/%s", config.Table, config.Chain)
		}
	case "flush":
		applyErr = m.flushChain(ctx, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Flushed chain %s/%s", config.Table, config.Chain)
		}
	case "policy":
		applyErr = m.setPolicy(ctx, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Set policy for %s/%s to %s", config.Table, config.Chain, config.Policy)
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

// Test tests if the iptables rule is in the desired state
func (m *IptablesModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// parseIptablesConfig parses iptables configuration from declaration
func (m *IptablesModule) parseIptablesConfig(decl *StateDeclaration) (*IptablesConfig, error) {
	config := &IptablesConfig{
		Table:    "filter",
		Chain:    "INPUT",
		Protocol: "all",
		Jump:     "ACCEPT",
		Wait:     1,
	}

	config.Table = getStringParameter(decl, "table", "filter")
	config.Chain = getStringParameter(decl, "chain", "INPUT")
	config.Rule = getStringParameter(decl, "rule", "")
	config.Protocol = getStringParameter(decl, "protocol", "all")
	config.Source = getStringParameter(decl, "source", "")
	config.Destination = getStringParameter(decl, "destination", "")
	config.InInterface = getStringParameter(decl, "in_interface", "")
	config.OutInterface = getStringParameter(decl, "out_interface", "")
	config.Match = getStringParameter(decl, "match", "")
	config.MatchOptions = getStringParameter(decl, "match_options", "")
	config.SourcePort = getStringParameter(decl, "source_port", "")
	config.DestPort = getStringParameter(decl, "dest_port", "")
	config.Jump = getStringParameter(decl, "jump", "ACCEPT")
	config.GoTo = getStringParameter(decl, "goto", "")
	config.Comment = getStringParameter(decl, "comment", "")
	config.Position = getIntParameter(decl, "position", 0)
	config.State = getStringParameter(decl, "state_match", "")
	config.Limit = getStringParameter(decl, "limit", "")
	config.LogPrefix = getStringParameter(decl, "log_prefix", "")
	config.RejectWith = getStringParameter(decl, "reject_with", "")
	config.ToSource = getStringParameter(decl, "to_source", "")
	config.ToDest = getStringParameter(decl, "to_dest", "")
	config.ToPort = getStringParameter(decl, "to_port", "")
	config.Wait = getIntParameter(decl, "wait", 1)
	config.Policy = getStringParameter(decl, "policy", "ACCEPT")

	// Validate table
	validTables := map[string]bool{
		"filter": true, "nat": true, "mangle": true, "raw": true, "security": true,
	}
	if !validTables[config.Table] {
		return nil, fmt.Errorf("invalid table: %s", config.Table)
	}

	// Validate policy for policy state
	if decl.State == "policy" {
		validPolicies := map[string]bool{"ACCEPT": true, "DROP": true, "REJECT": true}
		if !validPolicies[strings.ToUpper(config.Policy)] {
			return nil, fmt.Errorf("invalid policy: %s (must be ACCEPT, DROP, or REJECT)", config.Policy)
		}
		config.Policy = strings.ToUpper(config.Policy)
	}

	return config, nil
}

// buildRuleArgs builds the iptables command arguments
func (m *IptablesModule) buildRuleArgs(config *IptablesConfig) []string {
	var args []string

	// If full rule is specified, parse it
	if config.Rule != "" {
		return strings.Fields(config.Rule)
	}

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

	// Interfaces
	if config.InInterface != "" {
		args = append(args, "-i", config.InInterface)
	}
	if config.OutInterface != "" {
		args = append(args, "-o", config.OutInterface)
	}

	// Match extension
	if config.Match != "" {
		args = append(args, "-m", config.Match)
		if config.MatchOptions != "" {
			args = append(args, strings.Fields(config.MatchOptions)...)
		}
	}

	// Connection state
	if config.State != "" {
		args = append(args, "-m", "state", "--state", strings.ToUpper(config.State))
	}

	// Ports (need protocol tcp or udp)
	if config.SourcePort != "" {
		args = append(args, "--sport", config.SourcePort)
	}
	if config.DestPort != "" {
		args = append(args, "--dport", config.DestPort)
	}

	// Rate limit
	if config.Limit != "" {
		args = append(args, "-m", "limit", "--limit", config.Limit)
	}

	// Comment
	if config.Comment != "" {
		args = append(args, "-m", "comment", "--comment", config.Comment)
	}

	// Jump target
	if config.GoTo != "" {
		args = append(args, "-g", config.GoTo)
	} else if config.Jump != "" {
		args = append(args, "-j", config.Jump)

		// Target-specific options
		if config.Jump == "LOG" && config.LogPrefix != "" {
			args = append(args, "--log-prefix", config.LogPrefix)
		}
		if config.Jump == "REJECT" && config.RejectWith != "" {
			args = append(args, "--reject-with", config.RejectWith)
		}
		if config.Jump == "SNAT" && config.ToSource != "" {
			args = append(args, "--to-source", config.ToSource)
		}
		if config.Jump == "DNAT" && config.ToDest != "" {
			args = append(args, "--to-destination", config.ToDest)
		}
		if config.Jump == "REDIRECT" && config.ToPort != "" {
			args = append(args, "--to-port", config.ToPort)
		}
	}

	return args
}

// checkRuleExists checks if a rule exists
func (m *IptablesModule) checkRuleExists(ctx context.Context, config *IptablesConfig) (bool, error) {
	args := []string{"-w", strconv.Itoa(config.Wait), "-t", config.Table, "-C", config.Chain}
	args = append(args, m.buildRuleArgs(config)...)

	cmd := exec.CommandContext(ctx, "iptables", args...)
	err := cmd.Run()
	return err == nil, nil
}

// chainHasRules checks if a chain has any rules
func (m *IptablesModule) chainHasRules(ctx context.Context, config *IptablesConfig) (bool, error) {
	args := []string{"-w", strconv.Itoa(config.Wait), "-t", config.Table, "-L", config.Chain, "-n", "--line-numbers"}
	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	// Count lines (skip header lines)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	// First two lines are headers
	return len(lines) > 2, nil
}

// getChainPolicy gets the current policy for a chain
func (m *IptablesModule) getChainPolicy(ctx context.Context, config *IptablesConfig) (string, error) {
	args := []string{"-w", strconv.Itoa(config.Wait), "-t", config.Table, "-L", config.Chain, "-n"}
	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse first line: "Chain INPUT (policy ACCEPT)"
	firstLine := strings.Split(string(output), "\n")[0]
	if strings.Contains(firstLine, "policy") {
		start := strings.Index(firstLine, "policy ") + 7
		end := strings.Index(firstLine[start:], ")")
		if end > 0 {
			return firstLine[start : start+end], nil
		}
	}

	return "", fmt.Errorf("could not parse chain policy")
}

// addRule adds an iptables rule
func (m *IptablesModule) addRule(ctx context.Context, config *IptablesConfig) error {
	action := "-A"
	if config.Position > 0 {
		action = "-I"
	}

	args := []string{"-w", strconv.Itoa(config.Wait), "-t", config.Table, action, config.Chain}
	if config.Position > 0 {
		args = append(args, strconv.Itoa(config.Position))
	}
	args = append(args, m.buildRuleArgs(config)...)

	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// deleteRule deletes an iptables rule
func (m *IptablesModule) deleteRule(ctx context.Context, config *IptablesConfig) error {
	args := []string{"-w", strconv.Itoa(config.Wait), "-t", config.Table, "-D", config.Chain}
	args = append(args, m.buildRuleArgs(config)...)

	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// flushChain flushes all rules from a chain
func (m *IptablesModule) flushChain(ctx context.Context, config *IptablesConfig) error {
	args := []string{"-w", strconv.Itoa(config.Wait), "-t", config.Table, "-F", config.Chain}

	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// setPolicy sets the default policy for a chain
func (m *IptablesModule) setPolicy(ctx context.Context, config *IptablesConfig) error {
	args := []string{"-w", strconv.Itoa(config.Wait), "-t", config.Table, "-P", config.Chain, config.Policy}

	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables failed: %w (output: %s)", err, string(output))
	}
	return nil
}

func init() {
	RegisterModule(NewIptablesModule())
}
