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

// NftablesModule implements Linux nftables rule management
type NftablesModule struct {
	*BaseModule
}

// NewNftablesModule creates a new nftables module
func NewNftablesModule() *NftablesModule {
	return &NftablesModule{
		BaseModule: NewBaseModule("nftables", []string{"present", "absent"}),
	}
}

// NftablesConfig holds nftables-specific configuration
type NftablesConfig struct {
	Family        string // ip, ip6, inet, arp, bridge, netdev
	Table         string // Table name
	Chain         string // Chain name
	ChainType     string // filter, nat, route
	ChainHook     string // input, output, forward, prerouting, postrouting
	ChainPriority int    // Chain priority
	Rule          string // Full rule (if specified, other params ignored)
	Protocol      string // tcp, udp, icmp, etc.
	Source        string // Source IP/CIDR
	Destination   string // Destination IP/CIDR
	InInterface   string // Input interface
	OutInterface  string // Output interface
	SourcePort    string // Source port(s)
	DestPort      string // Destination port(s)
	Counter       bool   // Include counter
	Comment       string // Rule comment
	Action        string // accept, drop, reject, jump, goto, masquerade, etc.
	Position      int    // Rule position (handle for existing rule)
	State         string // Connection state (new, established, related)
	// Advanced
	Limit     string // Rate limit
	LimitOver string // Limit over action
	SnatTo    string // SNAT target
	DnatTo    string // DNAT target
	Mark      string // Packet mark
	Log       bool   // Log packets
	LogPrefix string // Log prefix
}

// Check checks the current state of an nftables rule
func (m *NftablesModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("nftables module is only supported on Linux")
	}

	config, err := m.parseNftablesConfig(decl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse nftables config: %w", err)
	}

	result.Metadata["family"] = config.Family
	result.Metadata["table"] = config.Table
	result.Metadata["chain"] = config.Chain

	// Check table exists
	tableExists, err := m.tableExists(ctx, config)
	if err != nil {
		return nil, err
	}

	if !tableExists {
		result.Present = false
		result.CurrentState = "absent"
		result.Matches = decl.State == "absent"
		if decl.State == "present" {
			result.Diff["table"] = map[string]string{"current": "absent", "desired": "present"}
		}
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Check chain exists
	chainExists, err := m.chainExists(ctx, config)
	if err != nil {
		return nil, err
	}

	if !chainExists {
		result.Present = false
		result.CurrentState = "absent"
		result.Matches = decl.State == "absent"
		if decl.State == "present" {
			result.Diff["chain"] = map[string]string{"current": "absent", "desired": "present"}
		}
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// Check rule exists
	ruleExists, handle, err := m.ruleExists(ctx, config)
	if err != nil {
		return nil, err
	}

	result.Present = ruleExists
	if ruleExists {
		result.CurrentState = "present"
		result.Metadata["handle"] = handle
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

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the nftables configuration
func (m *NftablesModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Error = fmt.Errorf("nftables module is only supported on Linux")
		result.Comment = result.Error.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	config, err := m.parseNftablesConfig(decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	var applyErr error
	switch decl.State {
	case "present":
		applyErr = m.addRule(ctx, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Added nftables rule to %s %s %s", config.Family, config.Table, config.Chain)
		}
	case "absent":
		if handle, ok := checkResult.Metadata["handle"].(int); ok && handle > 0 {
			config.Position = handle
		}
		applyErr = m.deleteRule(ctx, config)
		if applyErr == nil {
			result.Comment = fmt.Sprintf("Removed nftables rule from %s %s %s", config.Family, config.Table, config.Chain)
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

// Test tests if the nftables rule is in the desired state
func (m *NftablesModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // intentional
}

// parseNftablesConfig parses nftables configuration from declaration
func (m *NftablesModule) parseNftablesConfig(decl *StateDeclaration) (*NftablesConfig, error) {
	config := &NftablesConfig{
		Family:        "ip",
		Table:         "filter",
		Chain:         "input",
		ChainType:     "filter",
		ChainHook:     "input",
		ChainPriority: 0,
		Protocol:      "all",
		Action:        "accept",
	}

	config.Family = getStringParameter(decl, "family", "ip")
	config.Table = getStringParameter(decl, "table", "filter")
	config.Chain = getStringParameter(decl, "chain", "input")
	config.ChainType = getStringParameter(decl, "chain_type", "filter")
	config.ChainHook = getStringParameter(decl, "chain_hook", "input")
	config.ChainPriority = getIntParameter(decl, "chain_priority", 0)
	config.Rule = getStringParameter(decl, "rule", "")
	config.Protocol = getStringParameter(decl, "protocol", "all")
	config.Source = getStringParameter(decl, "source", "")
	config.Destination = getStringParameter(decl, "destination", "")
	config.InInterface = getStringParameter(decl, "in_interface", "")
	config.OutInterface = getStringParameter(decl, "out_interface", "")
	config.SourcePort = getStringParameter(decl, "source_port", "")
	config.DestPort = getStringParameter(decl, "dest_port", "")
	config.Counter = getBoolParameter(decl, "counter", false)
	config.Comment = getStringParameter(decl, "comment", "")
	config.Action = getStringParameter(decl, "action", "accept")
	config.Position = getIntParameter(decl, "position", 0)
	config.State = getStringParameter(decl, "state_match", "")
	config.Limit = getStringParameter(decl, "limit", "")
	config.LimitOver = getStringParameter(decl, "limit_over", "")
	config.SnatTo = getStringParameter(decl, "snat_to", "")
	config.DnatTo = getStringParameter(decl, "dnat_to", "")
	config.Mark = getStringParameter(decl, "mark", "")
	config.Log = getBoolParameter(decl, "log", false)
	config.LogPrefix = getStringParameter(decl, "log_prefix", "")

	// Validate family
	validFamilies := map[string]bool{
		"ip": true, "ip6": true, "inet": true, "arp": true, "bridge": true, "netdev": true,
	}
	if !validFamilies[config.Family] {
		return nil, fmt.Errorf("invalid family: %s", config.Family)
	}

	return config, nil //nolint:nilerr // intentional
}

// tableExists checks if a table exists
func (m *NftablesModule) tableExists(ctx context.Context, config *NftablesConfig) (bool, error) {
	cmd := exec.CommandContext(ctx, "nft", "list", "table", config.Family, config.Table)
	err := cmd.Run()
	return err == nil, nil //nolint:nilerr // intentional
}

// chainExists checks if a chain exists
func (m *NftablesModule) chainExists(ctx context.Context, config *NftablesConfig) (bool, error) {
	cmd := exec.CommandContext(ctx, "nft", "list", "chain", config.Family, config.Table, config.Chain)
	err := cmd.Run()
	return err == nil, nil //nolint:nilerr // error means chain doesn't exist, which is a valid state
}

// ruleExists checks if a rule exists and returns its handle
func (m *NftablesModule) ruleExists(ctx context.Context, config *NftablesConfig) (exists bool, handle int, err error) {
	// Get rules with handles
	cmd := exec.CommandContext(ctx, "nft", "-a", "list", "chain", config.Family, config.Table, config.Chain)
	output, err := cmd.Output()
	if err != nil {
		return false, 0, nil //nolint:nilerr // chain not existing returns error, which is a valid state
	}

	// Build a pattern to match our rule
	patterns := m.buildMatchPatterns(config)

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		matched := true
		for _, pattern := range patterns {
			if !strings.Contains(line, pattern) {
				matched = false
				break
			}
		}
		if matched {
			// Extract handle
			if idx := strings.Index(line, "# handle "); idx >= 0 {
				handleStr := strings.TrimSpace(line[idx+9:])
				if handle, err := strconv.Atoi(handleStr); err == nil {
					return true, handle, nil //nolint:nilerr // returning rule existence with handle, no error
				}
			}
			return true, 0, nil //nolint:nilerr // rule found but handle not parsed
		}
	}

	return false, 0, nil //nolint:nilerr // rule not found is a valid state
}

// buildMatchPatterns builds patterns to match a rule
func (m *NftablesModule) buildMatchPatterns(config *NftablesConfig) []string {
	var patterns []string

	if config.Protocol != "" && config.Protocol != "all" {
		patterns = append(patterns, config.Protocol)
	}

	if config.Source != "" {
		patterns = append(patterns, "saddr "+config.Source)
	}

	if config.Destination != "" {
		patterns = append(patterns, "daddr "+config.Destination)
	}

	if config.DestPort != "" {
		patterns = append(patterns, "dport "+config.DestPort)
	}

	if config.Comment != "" {
		patterns = append(patterns, "comment \""+config.Comment+"\"")
	}

	return patterns
}

// buildRule builds the nftables rule statement
func (m *NftablesModule) buildRule(config *NftablesConfig) string {
	if config.Rule != "" {
		return config.Rule
	}

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

	// Interfaces
	if config.InInterface != "" {
		parts = append(parts, "iifname", "\""+config.InInterface+"\"")
	}
	if config.OutInterface != "" {
		parts = append(parts, "oifname", "\""+config.OutInterface+"\"")
	}

	// Ports (need protocol first)
	if config.SourcePort != "" {
		parts = append(parts, "sport", config.SourcePort)
	}
	if config.DestPort != "" {
		parts = append(parts, "dport", config.DestPort)
	}

	// Connection state
	if config.State != "" {
		parts = append(parts, "ct", "state", config.State)
	}

	// Counter
	if config.Counter {
		parts = append(parts, "counter")
	}

	// Rate limit
	if config.Limit != "" {
		parts = append(parts, "limit", "rate", config.Limit)
		if config.LimitOver != "" {
			parts = append(parts, "over", config.LimitOver)
		}
	}

	// Logging
	if config.Log {
		if config.LogPrefix != "" {
			parts = append(parts, "log", "prefix", "\""+config.LogPrefix+"\"")
		} else {
			parts = append(parts, "log")
		}
	}

	// Mark
	if config.Mark != "" {
		parts = append(parts, "meta", "mark", "set", config.Mark)
	}

	// Comment
	if config.Comment != "" {
		parts = append(parts, "comment", "\""+config.Comment+"\"")
	}

	// Action
	switch strings.ToLower(config.Action) {
	case "accept":
		parts = append(parts, "accept")
	case "drop":
		parts = append(parts, "drop")
	case "reject":
		parts = append(parts, "reject")
	case "masquerade":
		parts = append(parts, "masquerade")
	case "snat":
		if config.SnatTo != "" {
			parts = append(parts, "snat", "to", config.SnatTo)
		}
	case "dnat":
		if config.DnatTo != "" {
			parts = append(parts, "dnat", "to", config.DnatTo)
		}
	default:
		parts = append(parts, config.Action)
	}

	return strings.Join(parts, " ")
}

// ensureTableAndChain ensures the table and chain exist
func (m *NftablesModule) ensureTableAndChain(ctx context.Context, config *NftablesConfig) error {
	// Create table if not exists
	cmd := exec.CommandContext(ctx, "nft", "add", "table", config.Family, config.Table)
	cmd.Run() // Ignore error if already exists

	// Create chain if not exists
	chainDef := fmt.Sprintf("{ type %s hook %s priority %d; }",
		config.ChainType, config.ChainHook, config.ChainPriority)
	cmd = exec.CommandContext(ctx, "nft", "add", "chain", config.Family, config.Table, config.Chain, chainDef)
	cmd.Run() // Ignore error if already exists

	return nil
}

// addRule adds an nftables rule
func (m *NftablesModule) addRule(ctx context.Context, config *NftablesConfig) error {
	// Ensure table and chain exist
	if err := m.ensureTableAndChain(ctx, config); err != nil {
		return err
	}

	rule := m.buildRule(config)
	ruleFields := strings.Fields(rule)
	args := make([]string, 0, 5+len(ruleFields))
	args = append(args, "add", "rule", config.Family, config.Table, config.Chain)
	args = append(args, ruleFields...)

	cmd := exec.CommandContext(ctx, "nft", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// deleteRule deletes an nftables rule
func (m *NftablesModule) deleteRule(ctx context.Context, config *NftablesConfig) error {
	// If we have a handle, delete by handle
	if config.Position > 0 {
		args := []string{"delete", "rule", config.Family, config.Table, config.Chain, "handle", strconv.Itoa(config.Position)}
		cmd := exec.CommandContext(ctx, "nft", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("nft failed: %w (output: %s)", err, string(output))
		}
		return nil
	}

	// Otherwise, try to find and delete by matching
	exists, handle, err := m.ruleExists(ctx, config)
	if err != nil || !exists {
		return nil //nolint:nilerr // error or !exists means rule doesn't exist
	}

	if handle > 0 {
		args := []string{"delete", "rule", config.Family, config.Table, config.Chain, "handle", strconv.Itoa(handle)}
		cmd := exec.CommandContext(ctx, "nft", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("nft failed: %w (output: %s)", err, string(output))
		}
	}

	return nil
}

func init() {
	_ = RegisterModule(NewNftablesModule()) //nolint:errcheck // module registration in init
}
