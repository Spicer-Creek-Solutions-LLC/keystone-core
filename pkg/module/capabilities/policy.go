// Package capabilities provides capability policy management for module security
package capabilities

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// CapabilityMode defines how a capability should be handled
type CapabilityMode string

const (
	// CapabilityModeAllow allows the capability with optional restrictions
	CapabilityModeAllow CapabilityMode = "allow"
	// CapabilityModeDeny completely blocks the capability
	CapabilityModeDeny CapabilityMode = "deny"
	// CapabilityModeRestrict allows with operator-defined restrictions
	CapabilityModeRestrict CapabilityMode = "restrict"
)

// TrustLevel defines trust levels for modules
type TrustLevel string

const (
	// TrustLevelNone applies all restrictions (default)
	TrustLevelNone TrustLevel = "none"
	// TrustLevelLimited applies default restrictions
	TrustLevelLimited TrustLevel = "limited"
	// TrustLevelFull trusts module's declared capabilities
	TrustLevelFull TrustLevel = "full"
)

// CapabilityPolicyConfig represents a capability restriction in policy
type CapabilityPolicyConfig struct {
	// Mode determines how the capability is handled (allow, deny, restrict)
	Mode CapabilityMode `yaml:"mode,omitempty"`

	// Filesystem restrictions
	AllowedPaths []string `yaml:"allowed_paths,omitempty"`
	DeniedPaths  []string `yaml:"denied_paths,omitempty"`
	MaxFileSize  int64    `yaml:"max_file_size,omitempty"`

	// HTTP restrictions
	AllowedDomains  []string      `yaml:"allowed_domains,omitempty"`
	DeniedDomains   []string      `yaml:"denied_domains,omitempty"`
	MaxResponseSize int64         `yaml:"max_response_size,omitempty"`
	MaxRequestSize  int64         `yaml:"max_request_size,omitempty"`
	RateLimit       int           `yaml:"rate_limit,omitempty"`
	Timeout         time.Duration `yaml:"timeout,omitempty"`

	// Exec restrictions
	AllowedCommands []string      `yaml:"allowed_commands,omitempty"`
	DeniedCommands  []string      `yaml:"denied_commands,omitempty"`
	WorkingDir      string        `yaml:"working_dir,omitempty"`
	ExecTimeout     time.Duration `yaml:"exec_timeout,omitempty"`

	// Secrets restrictions
	AllowedSecretPaths []string `yaml:"allowed_secret_paths,omitempty"`
	DeniedSecretPaths  []string `yaml:"denied_secret_paths,omitempty"`

	// KV restrictions
	Namespace    string `yaml:"namespace,omitempty"`
	MaxKeySize   int    `yaml:"max_key_size,omitempty"`
	MaxValueSize int    `yaml:"max_value_size,omitempty"`

	// Log restrictions
	MaxLogRate int `yaml:"max_log_rate,omitempty"`
}

// ModulePolicy defines policy for a specific module
type ModulePolicy struct {
	// Trust level for the module
	Trust TrustLevel `yaml:"trust,omitempty"`

	// Lock capabilities - prevent module updates from adding new capabilities
	Lock bool `yaml:"lock,omitempty"`

	// Capability-specific policies (capability name -> policy)
	Capabilities map[string]*CapabilityPolicyConfig `yaml:"capabilities,omitempty"`

	// AllowedCapabilities lists capabilities this module is allowed to use
	// If set, only these capabilities can be granted (whitelist)
	AllowedCapabilities []string `yaml:"allowed_capabilities,omitempty"`

	// DeniedCapabilities lists capabilities this module is explicitly denied
	// Takes precedence over allowed (blacklist)
	DeniedCapabilities []string `yaml:"denied_capabilities,omitempty"`
}

// CapabilityPolicy represents the complete capability policy configuration
type CapabilityPolicy struct {
	// SchemaVersion for policy format versioning
	SchemaVersion int `yaml:"schema_version"`

	// Defaults applied to all modules
	Defaults *ModulePolicy `yaml:"defaults,omitempty"`

	// Per-module policies (module name -> policy)
	Modules map[string]*ModulePolicy `yaml:"modules,omitempty"`
}

// PolicyDecision represents the result of policy evaluation
type PolicyDecision struct {
	// Allowed indicates if the capability is allowed
	Allowed bool

	// Reason explains why the decision was made
	Reason string

	// RestrictedConfig contains any restrictions to apply
	RestrictedConfig *CapabilityPolicyConfig

	// FromLock indicates if the decision was influenced by a capability lock
	FromLock bool

	// PolicySource indicates where the policy came from (default, module-specific)
	PolicySource string
}

// PolicyStore provides storage for capability policies
type PolicyStore interface {
	// Load loads the policy from storage
	Load() (*CapabilityPolicy, error)

	// Save saves the policy to storage
	Save(policy *CapabilityPolicy) error

	// Watch returns a channel that signals when policy changes
	Watch() <-chan struct{}
}

// PolicyEvaluator evaluates capability policies for modules
type PolicyEvaluator struct {
	policy    *CapabilityPolicy
	lockStore LockStore
	mu        sync.RWMutex
}

// NewPolicyEvaluator creates a new policy evaluator
func NewPolicyEvaluator(policy *CapabilityPolicy, lockStore LockStore) *PolicyEvaluator {
	if policy == nil {
		policy = &CapabilityPolicy{
			SchemaVersion: 1,
			Modules:       make(map[string]*ModulePolicy),
		}
	}
	return &PolicyEvaluator{
		policy:    policy,
		lockStore: lockStore,
	}
}

// UpdatePolicy updates the policy
func (pe *PolicyEvaluator) UpdatePolicy(policy *CapabilityPolicy) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.policy = policy
}

// GetPolicy returns the current policy
func (pe *PolicyEvaluator) GetPolicy() *CapabilityPolicy {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.policy
}

// EvaluateCapability evaluates whether a capability should be granted to a module
func (pe *PolicyEvaluator) EvaluateCapability(moduleName, capabilityName string, moduleConfig *CapabilityPolicyConfig) *PolicyDecision {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	decision := &PolicyDecision{
		Allowed:      true,
		PolicySource: "default",
	}

	// Get effective policy for module
	modulePolicy := pe.getModulePolicy(moduleName)

	// Check if capability is explicitly denied
	if pe.isCapabilityDenied(modulePolicy, capabilityName) {
		decision.Allowed = false
		decision.Reason = fmt.Sprintf("capability %q is explicitly denied by policy", capabilityName)
		return decision
	}

	// Check allowed list (if set, acts as whitelist)
	if len(modulePolicy.AllowedCapabilities) > 0 {
		if !pe.isInList(capabilityName, modulePolicy.AllowedCapabilities) {
			decision.Allowed = false
			decision.Reason = fmt.Sprintf("capability %q is not in allowed list", capabilityName)
			return decision
		}
	}

	// Check capability lock
	if modulePolicy.Lock && pe.lockStore != nil {
		lock, err := pe.lockStore.GetLock(moduleName)
		if err == nil && lock != nil {
			if !lock.HasCapability(capabilityName) {
				decision.Allowed = false
				decision.Reason = fmt.Sprintf("capability %q not in locked capabilities (module is locked)", capabilityName)
				decision.FromLock = true
				return decision
			}
		}
	}

	// Get capability-specific policy
	capPolicy := pe.getCapabilityPolicy(modulePolicy, capabilityName)
	if capPolicy != nil {
		// Check if explicitly denied via mode
		if capPolicy.Mode == CapabilityModeDeny {
			decision.Allowed = false
			decision.Reason = fmt.Sprintf("capability %q is denied by policy mode", capabilityName)
			return decision
		}

		// Merge restrictions
		decision.RestrictedConfig = pe.mergeConfigs(moduleConfig, capPolicy)
		decision.PolicySource = "module-policy"
	} else if moduleConfig != nil {
		decision.RestrictedConfig = moduleConfig
	}

	// Apply trust level
	if modulePolicy.Trust == TrustLevelFull {
		decision.Reason = "module has full trust"
		decision.PolicySource = "full-trust"
		// For full trust, use module's config without additional restrictions
		if moduleConfig != nil {
			decision.RestrictedConfig = moduleConfig
		}
	} else {
		decision.Reason = "capability allowed with restrictions"
	}

	return decision
}

// EvaluateAllCapabilities evaluates all capabilities for a module
func (pe *PolicyEvaluator) EvaluateAllCapabilities(moduleName string, capabilities map[string]*CapabilityPolicyConfig) map[string]*PolicyDecision {
	results := make(map[string]*PolicyDecision)
	for capName, capConfig := range capabilities {
		results[capName] = pe.EvaluateCapability(moduleName, capName, capConfig)
	}
	return results
}

// CheckModuleUpdate checks if a module update is allowed based on capability changes
func (pe *PolicyEvaluator) CheckModuleUpdate(moduleName string, oldCaps, newCaps []string) (*UpdateCheckResult, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	result := &UpdateCheckResult{
		Allowed:     true,
		AddedCaps:   []string{},
		RemovedCaps: []string{},
		BlockedCaps: []string{},
	}

	// Find added and removed capabilities
	oldSet := make(map[string]bool)
	for _, cap := range oldCaps {
		oldSet[cap] = true
	}

	newSet := make(map[string]bool)
	for _, cap := range newCaps {
		newSet[cap] = true
		if !oldSet[cap] {
			result.AddedCaps = append(result.AddedCaps, cap)
		}
	}

	for _, cap := range oldCaps {
		if !newSet[cap] {
			result.RemovedCaps = append(result.RemovedCaps, cap)
		}
	}

	// Check if module is locked
	modulePolicy := pe.getModulePolicy(moduleName)
	if modulePolicy.Lock && pe.lockStore != nil {
		lock, err := pe.lockStore.GetLock(moduleName)
		if err == nil && lock != nil {
			// Block any new capabilities
			for _, cap := range result.AddedCaps {
				if !lock.HasCapability(cap) {
					result.BlockedCaps = append(result.BlockedCaps, cap)
					result.Allowed = false
				}
			}
			if !result.Allowed {
				result.Reason = "module is locked and update adds new capabilities"
			}
		}
	}

	return result, nil
}

// UpdateCheckResult contains the result of checking a module update
type UpdateCheckResult struct {
	Allowed     bool
	Reason      string
	AddedCaps   []string
	RemovedCaps []string
	BlockedCaps []string
}

// getModulePolicy returns the effective policy for a module
func (pe *PolicyEvaluator) getModulePolicy(moduleName string) *ModulePolicy {
	// Start with defaults
	result := &ModulePolicy{
		Trust:        TrustLevelNone,
		Capabilities: make(map[string]*CapabilityPolicyConfig),
	}

	// Apply defaults
	if pe.policy.Defaults != nil {
		result = pe.mergeModulePolicies(result, pe.policy.Defaults)
	}

	// Apply module-specific policy
	if modulePolicy, ok := pe.policy.Modules[moduleName]; ok {
		result = pe.mergeModulePolicies(result, modulePolicy)
	}

	return result
}

// mergeModulePolicies merges two module policies, with override taking precedence
func (pe *PolicyEvaluator) mergeModulePolicies(base, override *ModulePolicy) *ModulePolicy {
	result := &ModulePolicy{
		Trust:               base.Trust,
		Lock:                base.Lock,
		Capabilities:        make(map[string]*CapabilityPolicyConfig),
		AllowedCapabilities: append([]string{}, base.AllowedCapabilities...),
		DeniedCapabilities:  append([]string{}, base.DeniedCapabilities...),
	}

	// Copy base capabilities
	for k, v := range base.Capabilities {
		result.Capabilities[k] = v
	}

	// Apply overrides
	if override.Trust != "" {
		result.Trust = override.Trust
	}
	if override.Lock {
		result.Lock = true
	}
	if len(override.AllowedCapabilities) > 0 {
		result.AllowedCapabilities = override.AllowedCapabilities
	}
	if len(override.DeniedCapabilities) > 0 {
		result.DeniedCapabilities = append(result.DeniedCapabilities, override.DeniedCapabilities...)
	}

	// Merge capability configs
	for k, v := range override.Capabilities {
		if existing, ok := result.Capabilities[k]; ok {
			result.Capabilities[k] = pe.mergeCapabilityConfigs(existing, v)
		} else {
			result.Capabilities[k] = v
		}
	}

	return result
}

// getCapabilityPolicy returns the policy for a specific capability
func (pe *PolicyEvaluator) getCapabilityPolicy(modulePolicy *ModulePolicy, capName string) *CapabilityPolicyConfig {
	if modulePolicy == nil || modulePolicy.Capabilities == nil {
		return nil
	}
	return modulePolicy.Capabilities[capName]
}

// isCapabilityDenied checks if a capability is in the denied list
func (pe *PolicyEvaluator) isCapabilityDenied(modulePolicy *ModulePolicy, capName string) bool {
	return pe.isInList(capName, modulePolicy.DeniedCapabilities)
}

// isInList checks if a capability is in a list (supports wildcards)
func (pe *PolicyEvaluator) isInList(capName string, list []string) bool {
	for _, item := range list {
		if item == capName {
			return true
		}
		// Support wildcard matching (e.g., "fs.*" matches "fs.read", "fs.write")
		if strings.HasSuffix(item, ".*") {
			prefix := strings.TrimSuffix(item, ".*")
			if strings.HasPrefix(capName, prefix+".") {
				return true
			}
		}
	}
	return false
}

// mergeConfigs merges module config with policy config, policy takes precedence for restrictions
func (pe *PolicyEvaluator) mergeConfigs(moduleConfig, policyConfig *CapabilityPolicyConfig) *CapabilityPolicyConfig {
	if moduleConfig == nil {
		return policyConfig
	}
	if policyConfig == nil {
		return moduleConfig
	}

	result := &CapabilityPolicyConfig{
		Mode: policyConfig.Mode,
	}

	// For paths, use intersection (more restrictive)
	result.AllowedPaths = pe.intersectPaths(moduleConfig.AllowedPaths, policyConfig.AllowedPaths)
	result.DeniedPaths = pe.unionPaths(moduleConfig.DeniedPaths, policyConfig.DeniedPaths)

	// For domains, use intersection
	result.AllowedDomains = pe.intersectPaths(moduleConfig.AllowedDomains, policyConfig.AllowedDomains)
	result.DeniedDomains = pe.unionPaths(moduleConfig.DeniedDomains, policyConfig.DeniedDomains)

	// For commands, use intersection
	result.AllowedCommands = pe.intersectPaths(moduleConfig.AllowedCommands, policyConfig.AllowedCommands)
	result.DeniedCommands = pe.unionPaths(moduleConfig.DeniedCommands, policyConfig.DeniedCommands)

	// For secrets paths
	result.AllowedSecretPaths = pe.intersectPaths(moduleConfig.AllowedSecretPaths, policyConfig.AllowedSecretPaths)
	result.DeniedSecretPaths = pe.unionPaths(moduleConfig.DeniedSecretPaths, policyConfig.DeniedSecretPaths)

	// For numeric limits, use the more restrictive (lower) value
	result.MaxFileSize = pe.minNonZero(moduleConfig.MaxFileSize, policyConfig.MaxFileSize)
	result.MaxResponseSize = pe.minNonZero(moduleConfig.MaxResponseSize, policyConfig.MaxResponseSize)
	result.MaxRequestSize = pe.minNonZero(moduleConfig.MaxRequestSize, policyConfig.MaxRequestSize)
	result.RateLimit = pe.minNonZeroInt(moduleConfig.RateLimit, policyConfig.RateLimit)
	result.MaxLogRate = pe.minNonZeroInt(moduleConfig.MaxLogRate, policyConfig.MaxLogRate)
	result.MaxKeySize = pe.minNonZeroInt(moduleConfig.MaxKeySize, policyConfig.MaxKeySize)
	result.MaxValueSize = pe.minNonZeroInt(moduleConfig.MaxValueSize, policyConfig.MaxValueSize)

	// For timeouts, use the shorter one
	result.Timeout = pe.minNonZeroDuration(moduleConfig.Timeout, policyConfig.Timeout)
	result.ExecTimeout = pe.minNonZeroDuration(moduleConfig.ExecTimeout, policyConfig.ExecTimeout)

	// For working dir, policy overrides if set
	if policyConfig.WorkingDir != "" {
		result.WorkingDir = policyConfig.WorkingDir
	} else {
		result.WorkingDir = moduleConfig.WorkingDir
	}

	// For namespace, policy overrides if set
	if policyConfig.Namespace != "" {
		result.Namespace = policyConfig.Namespace
	} else {
		result.Namespace = moduleConfig.Namespace
	}

	return result
}

// intersectPaths returns paths that are allowed by both lists
// If either list is empty/nil, uses the other
func (pe *PolicyEvaluator) intersectPaths(a, b []string) []string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	// For simplicity, if policy has specific paths, use those
	// A more sophisticated implementation would do pattern intersection
	return b
}

// unionPaths combines denied paths from both lists
func (pe *PolicyEvaluator) unionPaths(a, b []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, p := range a {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	for _, p := range b {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}

// mergeCapabilityConfigs merges two capability configs
func (pe *PolicyEvaluator) mergeCapabilityConfigs(base, override *CapabilityPolicyConfig) *CapabilityPolicyConfig {
	return pe.mergeConfigs(base, override)
}

func (pe *PolicyEvaluator) minNonZero(a, b int64) int64 {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func (pe *PolicyEvaluator) minNonZeroInt(a, b int) int {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func (pe *PolicyEvaluator) minNonZeroDuration(a, b time.Duration) time.Duration {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// FilePolicyStore implements PolicyStore using a YAML file
type FilePolicyStore struct {
	path    string
	mu      sync.RWMutex
	watchCh chan struct{}
}

// NewFilePolicyStore creates a new file-based policy store
func NewFilePolicyStore(path string) *FilePolicyStore {
	return &FilePolicyStore{
		path:    path,
		watchCh: make(chan struct{}, 1),
	}
}

// Load loads the policy from the file
func (fps *FilePolicyStore) Load() (*CapabilityPolicy, error) {
	fps.mu.RLock()
	defer fps.mu.RUnlock()

	data, err := os.ReadFile(fps.path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default empty policy
			return &CapabilityPolicy{
				SchemaVersion: 1,
				Modules:       make(map[string]*ModulePolicy),
			}, nil
		}
		return nil, fmt.Errorf("failed to read policy file: %w", err)
	}

	var policy CapabilityPolicy
	if err := yaml.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("failed to parse policy file: %w", err)
	}

	if policy.Modules == nil {
		policy.Modules = make(map[string]*ModulePolicy)
	}

	return &policy, nil
}

// Save saves the policy to the file
func (fps *FilePolicyStore) Save(policy *CapabilityPolicy) error {
	fps.mu.Lock()
	defer fps.mu.Unlock()

	data, err := yaml.Marshal(policy)
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(fps.path)
	//nolint:gosec // G301: policy directory needs to be accessible by service user
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create policy directory: %w", err)
	}

	//nolint:gosec // G306: policy files need to be readable by policy engine
	if err := os.WriteFile(fps.path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write policy file: %w", err)
	}

	// Notify watchers
	select {
	case fps.watchCh <- struct{}{}:
	default:
	}

	return nil
}

// Watch returns a channel that signals when policy changes
func (fps *FilePolicyStore) Watch() <-chan struct{} {
	return fps.watchCh
}

// LoadPolicyFromFile loads a capability policy from a YAML file
func LoadPolicyFromFile(path string) (*CapabilityPolicy, error) {
	store := NewFilePolicyStore(path)
	return store.Load()
}

// DefaultCapabilityPolicy returns a secure default policy
func DefaultCapabilityPolicy() *CapabilityPolicy {
	return &CapabilityPolicy{
		SchemaVersion: 1,
		Defaults: &ModulePolicy{
			Trust: TrustLevelNone,
			Capabilities: map[string]*CapabilityPolicyConfig{
				"fs.write": {
					DeniedPaths: []string{
						"/etc/**",
						"/root/**",
						"/sys/**",
						"/proc/**",
						"/dev/**",
						"/boot/**",
						"/usr/**",
						"/bin/**",
						"/sbin/**",
					},
				},
				"exec": {
					Mode: CapabilityModeDeny, // Deny exec by default
				},
				"http.post": {
					RateLimit: 100, // 100 req/min default
				},
			},
		},
		Modules: make(map[string]*ModulePolicy),
	}
}
