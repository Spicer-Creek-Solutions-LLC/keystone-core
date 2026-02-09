// Package loader provides module loading, verification, and execution orchestration
package loader

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/capabilities"
	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/policy"
	"github.com/shawnbutts/keystone-core/pkg/module/runtime"
	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

// LoadOptions configures module loading behavior
type LoadOptions struct {
	// SkipVerification bypasses cryptographic verification (DANGEROUS - dev only)
	SkipVerification bool

	// SkipPolicyValidation bypasses policy validation (DANGEROUS - dev only)
	SkipPolicyValidation bool

	// SkipCapabilityPolicyValidation bypasses capability policy validation
	SkipCapabilityPolicyValidation bool

	// TrustLevel for policy evaluation
	TrustLevel policy.TrustLevel

	// Environment for policy evaluation (dev, staging, prod)
	Environment string

	// VerificationOptions for crypto verification
	VerificationOptions *verify.VerificationOptions

	// PolicyContext for policy evaluation
	PolicyContext *policy.ModulePolicyContext

	// CapabilityBackends for pluggable capability implementations
	CapabilityBackends *CapabilityBackends

	// PreviousCapabilities is set when updating a module, to check for lock violations
	PreviousCapabilities []string

	// User who is loading the module (for audit purposes)
	User string
}

// CapabilityBackends provides pluggable implementations for capabilities
type CapabilityBackends struct {
	SecretsStore capabilities.SecretsStore
	Logger       capabilities.Logger
	KVStore      capabilities.KVStore
}

// ExecuteOptions configures module execution
type ExecuteOptions struct {
	// Timeout for module execution
	Timeout time.Duration

	// Context for execution (cancellation, values)
	Context context.Context

	// Input data for module
	Input map[string]interface{}

	// CorrelationID for request tracking
	CorrelationID string
}

// LoadResult contains the result of loading a module
type LoadResult struct {
	// Module manifest
	Manifest *manifest.Manifest

	// Runtime instance
	Runtime runtime.Runtime

	// VerificationResult from crypto verification
	VerificationResult *verify.VerificationResult

	// PolicyResult from policy validation
	PolicyResult *policy.ModulePolicyResult

	// CapabilityPolicyDecisions contains policy decisions for each capability
	CapabilityPolicyDecisions map[string]*capabilities.PolicyDecision

	// RegisteredCapabilities that were granted
	RegisteredCapabilities []string

	// DeniedCapabilities that were blocked by policy
	DeniedCapabilities []string

	// LoadDuration time taken to load
	LoadDuration time.Duration
}

// ExecuteResult contains the result of executing a module
type ExecuteResult struct {
	// Output from module execution
	Output interface{}

	// Error if execution failed
	Error error

	// ExecuteDuration time taken to execute
	ExecuteDuration time.Duration

	// CapabilityInvocations count by capability
	CapabilityInvocations map[string]int
}

// ModuleLoader orchestrates module loading, verification, and execution
type ModuleLoader interface {
	// Load loads a module from a path, verifies it, validates policies, and prepares for execution
	Load(modulePath string, options *LoadOptions) (*LoadResult, error)

	// Execute executes a loaded module
	Execute(result *LoadResult, options *ExecuteOptions) (*ExecuteResult, error)

	// LoadAndExecute is a convenience method that loads and executes in one call
	LoadAndExecute(modulePath string, loadOpts *LoadOptions, execOpts *ExecuteOptions) (*ExecuteResult, error)

	// Unload releases resources associated with a loaded module
	Unload(result *LoadResult) error
}

// ModuleCache provides caching for loaded modules
type ModuleCache interface {
	// Get retrieves a cached module by path and hash
	Get(modulePath string, contentHash string) (*LoadResult, bool)

	// Put stores a loaded module in cache
	Put(modulePath string, contentHash string, result *LoadResult)

	// Invalidate removes a module from cache
	Invalidate(modulePath string)

	// Clear removes all cached modules
	Clear()
}

// LoadEvent represents an event in the module loading process
type LoadEvent struct {
	Type      LoadEventType
	Timestamp time.Time
	ModPath   string
	Message   string
	Error     error
}

// LoadEventType represents different stages of module loading
type LoadEventType string

// LoadEvent constants define the events.
const (
	LoadEventStart                 LoadEventType = "load_start"
	LoadEventManifestParsed        LoadEventType = "manifest_parsed"
	LoadEventVerifying             LoadEventType = "verifying"
	LoadEventVerified              LoadEventType = "verified"
	LoadEventPolicyCheck           LoadEventType = "policy_check"
	LoadEventPolicyApproved        LoadEventType = "policy_approved"
	LoadEventCapabilityPolicyCheck LoadEventType = "capability_policy_check"
	LoadEventCapabilityLockCheck   LoadEventType = "capability_lock_check"
	LoadEventRuntimeInit           LoadEventType = "runtime_init"
	LoadEventCapabilities          LoadEventType = "capabilities_registered"
	LoadEventComplete              LoadEventType = "load_complete"
	LoadEventFailed                LoadEventType = "load_failed"
)
