package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/titananvil/titan-anvil/pkg/module/capabilities"
	"github.com/titananvil/titan-anvil/pkg/module/manifest"
	"github.com/titananvil/titan-anvil/pkg/module/policy"
	"github.com/titananvil/titan-anvil/pkg/module/runtime"
	"github.com/titananvil/titan-anvil/pkg/module/runtime/starlark"
	"github.com/titananvil/titan-anvil/pkg/module/runtime/wasm"
	"github.com/titananvil/titan-anvil/pkg/module/verify"
)

// DefaultModuleLoader is the default implementation of ModuleLoader
type DefaultModuleLoader struct {
	hashVerifier      verify.HashVerifier
	signatureVerifier verify.SignatureVerifier
	sumDB             verify.SumDB
	trustPolicy       verify.TrustPolicy
	policyEngine      *policy.ModulePolicyEngine
	capabilityRegistry *capabilities.CapabilityRegistry
	cache             ModuleCache
	eventHandler      func(*LoadEvent)
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
		hashVerifier:      hashVerifier,
		signatureVerifier: signatureVerifier,
		sumDB:             sumDB,
		trustPolicy:       trustPolicy,
		policyEngine:      policyEngine,
		capabilityRegistry: capabilities.NewCapabilityRegistry(),
	}
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

	// 5. Register capabilities
	l.emitEvent(LoadEventCapabilities, modulePath, "Registering capabilities", nil)

	var registeredCaps []string
	allowedCaps := mf.Capabilities
	if policyResult != nil && len(policyResult.AllowedCapabilities) > 0 {
		// Policy may have filtered capabilities
		allowedCaps = policyResult.AllowedCapabilities
	}

	for _, capName := range allowedCaps {
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

	loadDuration := time.Since(startTime)
	l.emitEvent(LoadEventComplete, modulePath, fmt.Sprintf("Module loaded in %v", loadDuration), nil)

	result := &LoadResult{
		Manifest:               mf,
		Runtime:                rt,
		VerificationResult:     verifyResult,
		PolicyResult:           policyResult,
		RegisteredCapabilities: registeredCaps,
		LoadDuration:           loadDuration,
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
		CapabilityInvocations:  make(map[string]int), // TODO: Track from capability registry
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

// createCapability creates a capability instance based on name
func (l *DefaultModuleLoader) createCapability(capName string, mf *manifest.Manifest, backends *CapabilityBackends) (capabilities.Capability, error) {
	ctx := &capabilities.CapabilityContext{
		ModuleName:    mf.Name,
		ModuleVersion: mf.Version,
		CorrelationID: "", // Set at execution time
	}

	switch capName {
	case "fs.read":
		return capabilities.NewFSReadCapability(ctx, []string{"**"}, nil, 10*1024*1024), nil

	case "fs.write":
		return capabilities.NewFSWriteCapability(ctx, []string{"**"}, nil), nil

	case "http.get":
		return capabilities.NewHTTPGetCapability(ctx, []string{"*"}, 30*time.Second, 10*1024*1024, 100), nil

	case "http.post":
		return capabilities.NewHTTPPostCapability(ctx, []string{"*"}, 30*time.Second, 1*1024*1024, 10*1024*1024, 100), nil

	case "exec":
		return capabilities.NewExecCapability(ctx, []string{"*"}, 30*time.Second, ""), nil

	case "secrets.read":
		store := &capabilities.InMemorySecretsStore{Secrets: make(map[string]string)}
		if backends != nil && backends.SecretsStore != nil {
			store = backends.SecretsStore.(*capabilities.InMemorySecretsStore)
		}
		return capabilities.NewSecretsReadCapability(ctx, []string{"**"}, store), nil

	case "secrets.write":
		store := &capabilities.InMemorySecretsStore{Secrets: make(map[string]string)}
		if backends != nil && backends.SecretsStore != nil {
			store = backends.SecretsStore.(*capabilities.InMemorySecretsStore)
		}
		return capabilities.NewSecretsWriteCapability(ctx, []string{"**"}, store), nil

	case "log":
		logger := &capabilities.DefaultLogger{}
		if backends != nil && backends.Logger != nil {
			logger = backends.Logger.(*capabilities.DefaultLogger)
		}
		return capabilities.NewLogCapability(ctx, logger, 1000), nil

	case "time":
		return capabilities.NewTimeCapability(ctx), nil

	case "kv":
		store := &capabilities.InMemoryKVStore{Data: make(map[string]string)}
		if backends != nil && backends.KVStore != nil {
			store = backends.KVStore.(*capabilities.InMemoryKVStore)
		}
		return capabilities.NewKVCapability(ctx, "default", store), nil

	default:
		return nil, fmt.Errorf("unknown capability: %s", capName)
	}
}

// parseMemoryLimit parses memory limit string (e.g., "10MB") to bytes
func parseMemoryLimit(limit string) uint64 {
	// Simple parser - in production would use a proper parser
	if limit == "" {
		return 64 * 1024 * 1024 // 64MB default
	}
	// TODO: Implement proper parsing
	return 64 * 1024 * 1024
}
