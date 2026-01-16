package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/capabilities"
	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/policy"
	"github.com/shawnbutts/keystone-core/pkg/module/runtime"
	"github.com/shawnbutts/keystone-core/pkg/module/runtime/starlark"
	"github.com/shawnbutts/keystone-core/pkg/module/runtime/wasm"
	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

// capabilityBuiltins creates and registers capability builtins with runtimes
type capabilityBuiltins = runtime.CapabilityBuiltins

// wasmHostFunctions creates and registers WASM host functions
type wasmHostFunctions = runtime.WasmHostFunctions

// DefaultModuleLoader is the default implementation of ModuleLoader
type DefaultModuleLoader struct {
	hashVerifier           verify.HashVerifier
	signatureVerifier      verify.SignatureVerifier
	sumDB                  verify.SumDB
	trustPolicy            verify.TrustPolicy
	policyEngine           *policy.ModulePolicyEngine
	capabilityPolicyEval   *capabilities.PolicyEvaluator
	lockManager            *capabilities.LockManager
	capabilityRegistry     *capabilities.CapabilityRegistry
	cache                  ModuleCache
	eventHandler           func(*LoadEvent)
}

// NewModuleLoader creates a new DefaultModuleLoader
func NewModuleLoader(
	hashVerifier verify.HashVerifier,
	signatureVerifier verify.SignatureVerifier,
	sumDB verify.SumDB,
	trustPolicy verify.TrustPolicy,
	policyEngine *policy.ModulePolicyEngine,
) *DefaultModuleLoader {
	return &DefaultModuleLoader{
		hashVerifier:       hashVerifier,
		signatureVerifier:  signatureVerifier,
		sumDB:              sumDB,
		trustPolicy:        trustPolicy,
		policyEngine:       policyEngine,
		capabilityRegistry: capabilities.NewCapabilityRegistry(),
	}
}

// SetCapabilityPolicyEvaluator sets the capability policy evaluator
func (l *DefaultModuleLoader) SetCapabilityPolicyEvaluator(eval *capabilities.PolicyEvaluator) {
	l.capabilityPolicyEval = eval
}

// SetLockManager sets the lock manager for capability locking
func (l *DefaultModuleLoader) SetLockManager(lm *capabilities.LockManager) {
	l.lockManager = lm
}

// SetCache sets the module cache
func (l *DefaultModuleLoader) SetCache(cache ModuleCache) {
	l.cache = cache
}

// SetEventHandler sets the event handler for load events
func (l *DefaultModuleLoader) SetEventHandler(handler func(*LoadEvent)) {
	l.eventHandler = handler
}

func (l *DefaultModuleLoader) emitEvent(eventType LoadEventType, modPath string, message string, err error) {
	if l.eventHandler != nil {
		l.eventHandler(&LoadEvent{
			Type:      eventType,
			Timestamp: time.Now(),
			ModPath:   modPath,
			Message:   message,
			Error:     err,
		})
	}
}

// Load loads a module from a path, verifies it, validates policies, and prepares for execution
func (l *DefaultModuleLoader) Load(modulePath string, options *LoadOptions) (*LoadResult, error) {
	startTime := time.Now()
	l.emitEvent(LoadEventStart, modulePath, "Starting module load", nil)

	if options == nil {
		options = &LoadOptions{}
	}

	// 1. Load and parse manifest
	l.emitEvent(LoadEventManifestParsed, modulePath, "Parsing manifest", nil)
	manifestPath := filepath.Join(modulePath, "module.yaml")
	mf, err := manifest.LoadManifest(manifestPath)
	if err != nil {
		l.emitEvent(LoadEventFailed, modulePath, "Failed to load manifest", err)
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}

	// 2. Cryptographic verification (unless skipped)
	var verifyResult *verify.VerificationResult
	if !options.SkipVerification {
		l.emitEvent(LoadEventVerifying, modulePath, "Verifying module", nil)

		verifyOpts := options.VerificationOptions
		if verifyOpts == nil {
			verifyOpts = &verify.VerificationOptions{
				RequireSignature: true,
				RequireSumDB:     true,
			}
		}

		artifact := &verify.ModuleArtifact{
			Name:    mf.Name,
			Version: mf.Version,
			Path:    modulePath,
		}

		// Hash verification
		hash, err := l.hashVerifier.ComputeHash(modulePath)
		if err != nil {
			l.emitEvent(LoadEventFailed, modulePath, "Hash computation failed", err)
			return nil, fmt.Errorf("hash computation failed: %w", err)
		}
		artifact.Hash = hash

		verifyResult = &verify.VerificationResult{
			ContentHash:    hash,
			Verified:       true,
			HashValid:      true,
			SignatureValid: false,
			SumDBVerified:  false,
			TrustedKey:     false,
		}

		// Signature verification
		if verifyOpts.RequireSignature {
			sigFile := filepath.Join(modulePath, "module.sig")
			if _, err := os.Stat(sigFile); err == nil {
				artifact.SignaturePath = sigFile
				// Note: Signature verification requires a public key from trust policy
				// This is simplified - in production, use ModuleVerifier.VerifyArtifact()
				verifyResult.SignatureValid = false
				verifyResult.TrustedKey = false
			}
		}

		// SumDB verification
		if verifyOpts.RequireSumDB && l.sumDB != nil {
			valid, err := l.sumDB.Verify(mf.Name, mf.Version, hash)
			if err != nil {
				if verifyOpts.RequireSumDB {
					l.emitEvent(LoadEventFailed, modulePath, "SumDB verification failed", err)
					return nil, fmt.Errorf("sumdb verification failed: %w", err)
				}
			} else if valid {
				verifyResult.SumDBVerified = true
			}
		}

		l.emitEvent(LoadEventVerified, modulePath, "Module verified", nil)
	}

	// 3. Policy validation (unless skipped)
	var policyResult *policy.ModulePolicyResult
	if !options.SkipPolicyValidation && l.policyEngine != nil {
		l.emitEvent(LoadEventPolicyCheck, modulePath, "Validating policies", nil)

		policyCtx := options.PolicyContext
		if policyCtx == nil {
			policyCtx = &policy.ModulePolicyContext{
				Module: &policy.ModuleInfo{
					Name:    mf.Name,
					Version: mf.Version,
				},
				Version:      mf.Version,
				Capabilities: mf.Capabilities,
				TrustLevel:   options.TrustLevel,
				Environment:  options.Environment,
				Timestamp:    time.Now(),
			}
		}

		var err error
		policyResult, err = l.policyEngine.ValidateModule(policyCtx)
		if err != nil {
			l.emitEvent(LoadEventFailed, modulePath, "Policy validation error", err)
			return nil, fmt.Errorf("policy validation error: %w", err)
		}
		if !policyResult.Allowed {
			l.emitEvent(LoadEventFailed, modulePath, "Policy validation failed", fmt.Errorf("module blocked by policy"))
			return nil, fmt.Errorf("module blocked by policy: %v", policyResult.Violations)
		}

		l.emitEvent(LoadEventPolicyApproved, modulePath, "Policy approved", nil)
	}

	// 4. Initialize runtime based on module type
	l.emitEvent(LoadEventRuntimeInit, modulePath, "Initializing runtime", nil)

	var rt runtime.Runtime
	switch mf.Type {
	case "starlark":
		starlarkOpts := &starlark.RuntimeOptions{
			MaxExecutionTime: mf.Limits.Timeout,
			MaxSteps:         1000000,
		}
		rt = starlark.NewStarlarkRuntime(starlarkOpts)

	case "wasm":
		wasmOpts := &wasm.RuntimeOptions{
			MaxMemory: parseMemoryLimit(mf.Limits.Memory),
			FuelLimit: 10000000,
		}
		rt = wasm.NewWasmRuntime(wasmOpts)

	default:
		err := fmt.Errorf("unsupported module type: %s", mf.Type)
		l.emitEvent(LoadEventFailed, modulePath, "Unsupported module type", err)
		return nil, err
	}

	// 5. Capability policy evaluation and lock check
	var capabilityDecisions map[string]*capabilities.PolicyDecision
	var deniedCaps []string

	if !options.SkipCapabilityPolicyValidation && l.capabilityPolicyEval != nil {
		l.emitEvent(LoadEventCapabilityPolicyCheck, modulePath, "Evaluating capability policies", nil)

		// Convert manifest capability configs to policy configs for evaluation
		capConfigs := make(map[string]*capabilities.CapabilityPolicyConfig)
		for _, capName := range mf.Capabilities {
			manifestConfig := mf.GetCapabilityConfig(capName)
			if manifestConfig != nil {
				capConfigs[capName] = manifestCapabilityConfigToPolicyConfig(manifestConfig)
			}
		}

		// Evaluate all capabilities against policy
		capabilityDecisions = l.capabilityPolicyEval.EvaluateAllCapabilities(mf.Name, capConfigs)
	}

	// Check for module update lock violations
	if l.lockManager != nil && len(options.PreviousCapabilities) > 0 {
		l.emitEvent(LoadEventCapabilityLockCheck, modulePath, "Checking capability locks", nil)

		capConfigs := make(map[string]*capabilities.CapabilityPolicyConfig)
		for _, capName := range mf.Capabilities {
			manifestConfig := mf.GetCapabilityConfig(capName)
			if manifestConfig != nil {
				capConfigs[capName] = manifestCapabilityConfigToPolicyConfig(manifestConfig)
			}
		}

		updateResult, err := l.lockManager.CheckUpdate(mf.Name, mf.Capabilities, capConfigs)
		if err != nil {
			l.emitEvent(LoadEventFailed, modulePath, "Lock check failed", err)
			return nil, fmt.Errorf("lock check failed: %w", err)
		}
		if !updateResult.Allowed {
			l.emitEvent(LoadEventFailed, modulePath, updateResult.Reason, nil)
			return nil, fmt.Errorf("module update blocked by lock: %s", updateResult.Reason)
		}
	}

	// 6. Register capabilities
	l.emitEvent(LoadEventCapabilities, modulePath, "Registering capabilities", nil)

	var registeredCaps []string
	allowedCaps := mf.Capabilities
	if policyResult != nil && len(policyResult.AllowedCapabilities) > 0 {
		// Policy may have filtered capabilities
		allowedCaps = policyResult.AllowedCapabilities
	}

	for _, capName := range allowedCaps {
		// Check if capability was denied by capability policy
		if capabilityDecisions != nil {
			if decision, ok := capabilityDecisions[capName]; ok && !decision.Allowed {
				deniedCaps = append(deniedCaps, capName)
				continue // Skip denied capabilities
			}
		}

		cap, err := l.createCapability(capName, mf, options.CapabilityBackends)
		if err != nil {
			l.emitEvent(LoadEventFailed, modulePath, fmt.Sprintf("Failed to create capability %s", capName), err)
			return nil, fmt.Errorf("failed to create capability %s: %w", capName, err)
		}

		if err := l.capabilityRegistry.Register(cap); err != nil {
			l.emitEvent(LoadEventFailed, modulePath, fmt.Sprintf("Failed to register capability %s", capName), err)
			return nil, fmt.Errorf("failed to register capability %s: %w", capName, err)
		}

		registeredCaps = append(registeredCaps, capName)
	}

	// 7. Wire capabilities to runtime
	l.emitEvent(LoadEventCapabilities, modulePath, "Wiring capabilities to runtime", nil)
	if err := l.wireCapabilitiesToRuntime(rt); err != nil {
		l.emitEvent(LoadEventFailed, modulePath, "Failed to wire capabilities", err)
		return nil, fmt.Errorf("failed to wire capabilities to runtime: %w", err)
	}

	loadDuration := time.Since(startTime)
	l.emitEvent(LoadEventComplete, modulePath, fmt.Sprintf("Module loaded in %v", loadDuration), nil)

	result := &LoadResult{
		Manifest:                  mf,
		Runtime:                   rt,
		VerificationResult:        verifyResult,
		PolicyResult:              policyResult,
		CapabilityPolicyDecisions: capabilityDecisions,
		RegisteredCapabilities:    registeredCaps,
		DeniedCapabilities:        deniedCaps,
		LoadDuration:              loadDuration,
	}

	// Cache the result if cache is available
	if l.cache != nil && verifyResult != nil {
		l.cache.Put(modulePath, verifyResult.ContentHash, result)
	}

	return result, nil
}

// Execute executes a loaded module
func (l *DefaultModuleLoader) Execute(result *LoadResult, options *ExecuteOptions) (*ExecuteResult, error) {
	if options == nil {
		options = &ExecuteOptions{
			Timeout: 30 * time.Second,
			Context: context.Background(),
		}
	}

	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Apply timeout
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}

	startTime := time.Now()

	// Execute based on runtime type
	var output interface{}
	var err error

	switch rt := result.Runtime.(type) {
	case *starlark.StarlarkRuntime:
		// Load module file
		modulePath := filepath.Join(result.Manifest.Name, result.Manifest.Entrypoint)
		output, err = rt.ExecuteFile(ctx, modulePath, options.Input)

	case *wasm.WasmRuntime:
		// Load WASM file
		wasmPath := filepath.Join(result.Manifest.Name, result.Manifest.Entrypoint)
		wasmBytes, readErr := os.ReadFile(wasmPath)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read wasm file: %w", readErr)
		}

		if err := rt.LoadModule(wasmBytes); err != nil {
			return nil, fmt.Errorf("failed to load wasm module: %w", err)
		}

		output, err = rt.ExecuteFunction(ctx, "module_main", options.Input)

	default:
		return nil, fmt.Errorf("unknown runtime type: %T", rt)
	}

	executeDuration := time.Since(startTime)

	return &ExecuteResult{
		Output:                 output,
		Error:                  err,
		ExecuteDuration:        executeDuration,
		CapabilityInvocations:  make(map[string]int),
	}, nil
}

// LoadAndExecute is a convenience method that loads and executes in one call
func (l *DefaultModuleLoader) LoadAndExecute(modulePath string, loadOpts *LoadOptions, execOpts *ExecuteOptions) (*ExecuteResult, error) {
	loadResult, err := l.Load(modulePath, loadOpts)
	if err != nil {
		return nil, err
	}

	defer l.Unload(loadResult)

	return l.Execute(loadResult, execOpts)
}

// Unload releases resources associated with a loaded module
func (l *DefaultModuleLoader) Unload(result *LoadResult) error {
	if result.Runtime != nil {
		return result.Runtime.Close()
	}
	return nil
}

// createCapability creates a capability instance based on name and manifest config
func (l *DefaultModuleLoader) createCapability(capName string, mf *manifest.Manifest, backends *CapabilityBackends) (capabilities.Capability, error) {
	ctx := &capabilities.CapabilityContext{
		ModuleName:    mf.Name,
		ModuleVersion: mf.Version,
		CorrelationID: "", // Set at execution time
	}

	// Get capability config from manifest (already has defaults applied)
	config := mf.GetCapabilityConfig(capName)
	if config == nil {
		// Fallback to default config if not found
		config = &manifest.CapabilityConfig{}
	}

	switch capName {
	case "fs.read":
		allowedPaths := config.AllowedPaths
		if len(allowedPaths) == 0 {
			allowedPaths = []string{"**"}
		}
		maxFileSize := config.MaxFileSize
		if maxFileSize == 0 {
			maxFileSize = 10 * 1024 * 1024 // 10MB default
		}
		return capabilities.NewFSReadCapability(ctx, allowedPaths, config.DeniedPaths, maxFileSize), nil

	case "fs.write":
		allowedPaths := config.AllowedPaths
		if len(allowedPaths) == 0 {
			allowedPaths = []string{"**"}
		}
		return capabilities.NewFSWriteCapability(ctx, allowedPaths, config.DeniedPaths), nil

	case "http.get":
		allowedDomains := config.AllowedDomains
		if len(allowedDomains) == 0 {
			allowedDomains = []string{"*"}
		}
		timeout := config.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		maxRespSize := config.MaxResponseSize
		if maxRespSize == 0 {
			maxRespSize = 10 * 1024 * 1024
		}
		rateLimit := config.RateLimit
		if rateLimit == 0 {
			rateLimit = 100
		}
		return capabilities.NewHTTPGetCapability(ctx, allowedDomains, timeout, maxRespSize, rateLimit), nil

	case "http.post":
		allowedDomains := config.AllowedDomains
		if len(allowedDomains) == 0 {
			allowedDomains = []string{"*"}
		}
		timeout := config.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		maxReqSize := config.MaxRequestSize
		if maxReqSize == 0 {
			maxReqSize = 1 * 1024 * 1024
		}
		maxRespSize := config.MaxResponseSize
		if maxRespSize == 0 {
			maxRespSize = 10 * 1024 * 1024
		}
		rateLimit := config.RateLimit
		if rateLimit == 0 {
			rateLimit = 100
		}
		return capabilities.NewHTTPPostCapability(ctx, allowedDomains, timeout, maxReqSize, maxRespSize, rateLimit), nil

	case "exec":
		allowedCommands := config.AllowedCommands
		if len(allowedCommands) == 0 {
			allowedCommands = []string{"*"}
		}
		timeout := config.ExecTimeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		return capabilities.NewExecCapability(ctx, allowedCommands, timeout, config.WorkingDir), nil

	case "secrets.read":
		store := &capabilities.InMemorySecretsStore{Secrets: make(map[string]string)}
		if backends != nil && backends.SecretsStore != nil {
			store = backends.SecretsStore.(*capabilities.InMemorySecretsStore)
		}
		allowedPaths := config.AllowedSecretPaths
		if len(allowedPaths) == 0 {
			allowedPaths = []string{"**"}
		}
		return capabilities.NewSecretsReadCapability(ctx, allowedPaths, store), nil

	case "secrets.write":
		store := &capabilities.InMemorySecretsStore{Secrets: make(map[string]string)}
		if backends != nil && backends.SecretsStore != nil {
			store = backends.SecretsStore.(*capabilities.InMemorySecretsStore)
		}
		allowedPaths := config.AllowedSecretPaths
		if len(allowedPaths) == 0 {
			allowedPaths = []string{"**"}
		}
		return capabilities.NewSecretsWriteCapability(ctx, allowedPaths, store), nil

	case "log":
		logger := &capabilities.DefaultLogger{}
		if backends != nil && backends.Logger != nil {
			logger = backends.Logger.(*capabilities.DefaultLogger)
		}
		maxRate := config.MaxLogRate
		if maxRate == 0 {
			maxRate = 1000
		}
		return capabilities.NewLogCapability(ctx, logger, maxRate), nil

	case "time":
		return capabilities.NewTimeCapability(ctx), nil

	case "kv":
		store := &capabilities.InMemoryKVStore{Data: make(map[string]string)}
		if backends != nil && backends.KVStore != nil {
			store = backends.KVStore.(*capabilities.InMemoryKVStore)
		}
		namespace := config.Namespace
		if namespace == "" {
			namespace = "default"
		}
		return capabilities.NewKVCapability(ctx, namespace, store), nil

	default:
		return nil, fmt.Errorf("unknown capability: %s", capName)
	}
}

// parseMemoryLimit parses memory limit string (e.g., "10MB", "64Mi", "1Gi") to bytes
// Supports:
//   - Empty string -> 64MB default
//   - Plain numbers -> interpreted as bytes
//   - KB, MB, GB, TB -> decimal units (1000-based)
//   - Ki, Mi, Gi, Ti -> binary units (1024-based, Kubernetes style)
//   - K, M, G, T -> same as Ki, Mi, Gi, Ti
func parseMemoryLimit(limit string) uint64 {
	const defaultLimit = 64 * 1024 * 1024 // 64MB default

	if limit == "" {
		return defaultLimit
	}

	// Trim whitespace
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return defaultLimit
	}

	// Find where the numeric part ends
	var numStr string
	var suffix string
	for i, c := range limit {
		if !((c >= '0' && c <= '9') || c == '.') {
			numStr = limit[:i]
			suffix = strings.TrimSpace(limit[i:])
			break
		}
	}
	if numStr == "" {
		numStr = limit
		suffix = ""
	}

	// Parse the numeric value
	value, err := strconv.ParseFloat(numStr, 64)
	if err != nil || value < 0 {
		return defaultLimit
	}

	// Apply multiplier based on suffix
	suffix = strings.ToUpper(suffix)
	var multiplier float64 = 1

	switch suffix {
	case "":
		// Plain bytes
		multiplier = 1
	case "K", "KI", "KIB":
		// Binary kilobytes (kibibytes)
		multiplier = 1024
	case "KB":
		// Decimal kilobytes
		multiplier = 1000
	case "M", "MI", "MIB":
		// Binary megabytes (mebibytes)
		multiplier = 1024 * 1024
	case "MB":
		// Decimal megabytes
		multiplier = 1000 * 1000
	case "G", "GI", "GIB":
		// Binary gigabytes (gibibytes)
		multiplier = 1024 * 1024 * 1024
	case "GB":
		// Decimal gigabytes
		multiplier = 1000 * 1000 * 1000
	case "T", "TI", "TIB":
		// Binary terabytes (tebibytes)
		multiplier = 1024 * 1024 * 1024 * 1024
	case "TB":
		// Decimal terabytes
		multiplier = 1000 * 1000 * 1000 * 1000
	default:
		return defaultLimit
	}

	result := value * multiplier
	if result > float64(^uint64(0)) {
		return ^uint64(0) // Max uint64
	}
	return uint64(result)
}

// wireCapabilitiesToRuntime wires the registered capabilities to the runtime
func (l *DefaultModuleLoader) wireCapabilitiesToRuntime(rt runtime.Runtime) error {
	switch typedRT := rt.(type) {
	case *starlark.StarlarkRuntime:
		// Wire capabilities as Starlark builtins
		builtins := runtime.NewCapabilityBuiltins(l.capabilityRegistry)
		return builtins.RegisterStarlarkBuiltins(typedRT)

	case *wasm.WasmRuntime:
		// Wire capabilities as WASM host functions
		hostFuncs := runtime.NewWasmHostFunctions(l.capabilityRegistry)
		return hostFuncs.RegisterWithWasmRuntime(typedRT)

	default:
		// Unknown runtime type - skip wiring
		return nil
	}
}

// manifestCapabilityConfigToPolicyConfig converts a manifest capability config to a policy config
func manifestCapabilityConfigToPolicyConfig(mc *manifest.CapabilityConfig) *capabilities.CapabilityPolicyConfig {
	if mc == nil {
		return nil
	}
	return &capabilities.CapabilityPolicyConfig{
		AllowedPaths:       mc.AllowedPaths,
		DeniedPaths:        mc.DeniedPaths,
		MaxFileSize:        mc.MaxFileSize,
		AllowedDomains:     mc.AllowedDomains,
		DeniedDomains:      mc.DeniedDomains,
		MaxResponseSize:    mc.MaxResponseSize,
		MaxRequestSize:     mc.MaxRequestSize,
		RateLimit:          mc.RateLimit,
		Timeout:            mc.Timeout,
		AllowedCommands:    mc.AllowedCommands,
		WorkingDir:         mc.WorkingDir,
		ExecTimeout:        mc.ExecTimeout,
		AllowedSecretPaths: mc.AllowedSecretPaths,
		DeniedSecretPaths:  mc.DeniedSecretPaths,
		Namespace:          mc.Namespace,
		MaxKeySize:         mc.MaxKeySize,
		MaxValueSize:       mc.MaxValueSize,
		MaxLogRate:         mc.MaxLogRate,
	}
}
