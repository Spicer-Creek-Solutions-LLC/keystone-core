package capabilities

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPolicyEvaluator_EvaluateCapability(t *testing.T) {
	tests := []struct {
		name           string
		policy         *CapabilityPolicy
		moduleName     string
		capName        string
		moduleConfig   *CapabilityPolicyConfig
		wantAllowed    bool
		wantReason     string
	}{
		{
			name: "allow with default policy",
			policy: &CapabilityPolicy{
				SchemaVersion: 1,
				Modules:       make(map[string]*ModulePolicy),
			},
			moduleName:  "test-module",
			capName:     "fs.read",
			wantAllowed: true,
		},
		{
			name: "deny explicitly denied capability",
			policy: &CapabilityPolicy{
				SchemaVersion: 1,
				Defaults: &ModulePolicy{
					DeniedCapabilities: []string{"exec"},
				},
				Modules: make(map[string]*ModulePolicy),
			},
			moduleName:  "test-module",
			capName:     "exec",
			wantAllowed: false,
			wantReason:  "capability \"exec\" is explicitly denied by policy",
		},
		{
			name: "deny with wildcard",
			policy: &CapabilityPolicy{
				SchemaVersion: 1,
				Defaults: &ModulePolicy{
					DeniedCapabilities: []string{"fs.*"},
				},
				Modules: make(map[string]*ModulePolicy),
			},
			moduleName:  "test-module",
			capName:     "fs.write",
			wantAllowed: false,
			wantReason:  "capability \"fs.write\" is explicitly denied by policy",
		},
		{
			name: "deny not in allowed list",
			policy: &CapabilityPolicy{
				SchemaVersion: 1,
				Defaults: &ModulePolicy{
					AllowedCapabilities: []string{"fs.read", "log"},
				},
				Modules: make(map[string]*ModulePolicy),
			},
			moduleName:  "test-module",
			capName:     "exec",
			wantAllowed: false,
			wantReason:  "capability \"exec\" is not in allowed list",
		},
		{
			name: "allow in allowed list",
			policy: &CapabilityPolicy{
				SchemaVersion: 1,
				Defaults: &ModulePolicy{
					AllowedCapabilities: []string{"fs.read", "log"},
				},
				Modules: make(map[string]*ModulePolicy),
			},
			moduleName:  "test-module",
			capName:     "fs.read",
			wantAllowed: true,
		},
		{
			name: "deny by capability mode",
			policy: &CapabilityPolicy{
				SchemaVersion: 1,
				Defaults: &ModulePolicy{
					Capabilities: map[string]*CapabilityPolicyConfig{
						"exec": {Mode: CapabilityModeDeny},
					},
				},
				Modules: make(map[string]*ModulePolicy),
			},
			moduleName:  "test-module",
			capName:     "exec",
			wantAllowed: false,
			wantReason:  "capability \"exec\" is denied by policy mode",
		},
		{
			name: "allow with full trust",
			policy: &CapabilityPolicy{
				SchemaVersion: 1,
				Modules: map[string]*ModulePolicy{
					"trusted-module": {Trust: TrustLevelFull},
				},
			},
			moduleName:  "trusted-module",
			capName:     "exec",
			wantAllowed: true,
			wantReason:  "module has full trust",
		},
		{
			name: "module-specific policy overrides default",
			policy: &CapabilityPolicy{
				SchemaVersion: 1,
				Defaults: &ModulePolicy{
					DeniedCapabilities: []string{"exec"},
				},
				Modules: map[string]*ModulePolicy{
					"special-module": {
						AllowedCapabilities: []string{"exec"},
					},
				},
			},
			moduleName:  "special-module",
			capName:     "exec",
			wantAllowed: false, // Still denied because denied takes precedence
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewPolicyEvaluator(tt.policy, nil)
			decision := eval.EvaluateCapability(tt.moduleName, tt.capName, tt.moduleConfig)

			if decision.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", decision.Allowed, tt.wantAllowed)
			}
			if tt.wantReason != "" && decision.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", decision.Reason, tt.wantReason)
			}
		})
	}
}

func TestPolicyEvaluator_EvaluateAllCapabilities(t *testing.T) {
	policy := &CapabilityPolicy{
		SchemaVersion: 1,
		Defaults: &ModulePolicy{
			DeniedCapabilities: []string{"exec"},
		},
		Modules: make(map[string]*ModulePolicy),
	}

	eval := NewPolicyEvaluator(policy, nil)
	caps := map[string]*CapabilityPolicyConfig{
		"fs.read": nil,
		"exec":    nil,
		"log":     nil,
	}

	results := eval.EvaluateAllCapabilities("test-module", caps)

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	if results["fs.read"].Allowed != true {
		t.Error("fs.read should be allowed")
	}
	if results["exec"].Allowed != false {
		t.Error("exec should be denied")
	}
	if results["log"].Allowed != true {
		t.Error("log should be allowed")
	}
}

func TestPolicyEvaluator_CheckModuleUpdate(t *testing.T) {
	lockStore := NewInMemoryLockStore()
	lock := &CapabilityLock{
		ModuleName:   "locked-module",
		Capabilities: []string{"fs.read", "log"},
		LockedAt:     time.Now(),
	}
	lockStore.SetLock(lock)

	policy := &CapabilityPolicy{
		SchemaVersion: 1,
		Modules: map[string]*ModulePolicy{
			"locked-module": {Lock: true},
		},
	}

	eval := NewPolicyEvaluator(policy, lockStore)

	tests := []struct {
		name        string
		oldCaps     []string
		newCaps     []string
		wantAllowed bool
		wantBlocked []string
	}{
		{
			name:        "no changes allowed",
			oldCaps:     []string{"fs.read", "log"},
			newCaps:     []string{"fs.read", "log"},
			wantAllowed: true,
		},
		{
			name:        "adding new capability blocked",
			oldCaps:     []string{"fs.read", "log"},
			newCaps:     []string{"fs.read", "log", "exec"},
			wantAllowed: false,
			wantBlocked: []string{"exec"},
		},
		{
			name:        "removing capability allowed",
			oldCaps:     []string{"fs.read", "log"},
			newCaps:     []string{"fs.read"},
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.CheckModuleUpdate("locked-module", tt.oldCaps, tt.newCaps)
			if err != nil {
				t.Fatalf("CheckModuleUpdate error: %v", err)
			}

			if result.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.wantAllowed)
			}

			if len(tt.wantBlocked) > 0 {
				if len(result.BlockedCaps) != len(tt.wantBlocked) {
					t.Errorf("BlockedCaps = %v, want %v", result.BlockedCaps, tt.wantBlocked)
				}
			}
		})
	}
}

func TestPolicyEvaluator_MergeConfigs(t *testing.T) {
	policy := &CapabilityPolicy{
		SchemaVersion: 1,
		Defaults: &ModulePolicy{
			Capabilities: map[string]*CapabilityPolicyConfig{
				"fs.read": {
					MaxFileSize: 5 * 1024 * 1024, // 5MB policy limit
				},
			},
		},
		Modules: make(map[string]*ModulePolicy),
	}

	eval := NewPolicyEvaluator(policy, nil)

	moduleConfig := &CapabilityPolicyConfig{
		MaxFileSize: 10 * 1024 * 1024, // 10MB module request
	}

	decision := eval.EvaluateCapability("test-module", "fs.read", moduleConfig)

	if decision.RestrictedConfig == nil {
		t.Fatal("RestrictedConfig should not be nil")
	}

	// Should use the more restrictive (lower) value
	if decision.RestrictedConfig.MaxFileSize != 5*1024*1024 {
		t.Errorf("MaxFileSize = %d, want %d", decision.RestrictedConfig.MaxFileSize, 5*1024*1024)
	}
}

func TestFilePolicyStore(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.yaml")

	store := NewFilePolicyStore(policyPath)

	// Test loading non-existent file returns default
	policy, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if policy.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", policy.SchemaVersion)
	}

	// Test saving and loading
	policy = &CapabilityPolicy{
		SchemaVersion: 1,
		Defaults: &ModulePolicy{
			Trust:              TrustLevelLimited,
			DeniedCapabilities: []string{"exec"},
		},
		Modules: map[string]*ModulePolicy{
			"trusted": {Trust: TrustLevelFull},
		},
	}

	if err := store.Save(policy); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if loaded.Defaults.Trust != TrustLevelLimited {
		t.Errorf("Default Trust = %s, want %s", loaded.Defaults.Trust, TrustLevelLimited)
	}
	if len(loaded.Defaults.DeniedCapabilities) != 1 || loaded.Defaults.DeniedCapabilities[0] != "exec" {
		t.Errorf("DeniedCapabilities = %v, want [exec]", loaded.Defaults.DeniedCapabilities)
	}
	if loaded.Modules["trusted"].Trust != TrustLevelFull {
		t.Errorf("Module Trust = %s, want %s", loaded.Modules["trusted"].Trust, TrustLevelFull)
	}
}

func TestDefaultCapabilityPolicy(t *testing.T) {
	policy := DefaultCapabilityPolicy()

	if policy.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", policy.SchemaVersion)
	}
	if policy.Defaults == nil {
		t.Fatal("Defaults should not be nil")
	}
	if policy.Defaults.Trust != TrustLevelNone {
		t.Errorf("Default Trust = %s, want %s", policy.Defaults.Trust, TrustLevelNone)
	}

	// Check exec is denied by default
	execPolicy := policy.Defaults.Capabilities["exec"]
	if execPolicy == nil || execPolicy.Mode != CapabilityModeDeny {
		t.Error("exec should be denied by default policy")
	}

	// Check fs.write has denied paths
	fsWritePolicy := policy.Defaults.Capabilities["fs.write"]
	if fsWritePolicy == nil || len(fsWritePolicy.DeniedPaths) == 0 {
		t.Error("fs.write should have denied paths")
	}
}

func TestLoadPolicyFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.yaml")

	// Write a policy file
	policyYAML := `schema_version: 1
defaults:
  trust: limited
  denied_capabilities:
    - exec
    - secrets.write
modules:
  internal/trusted-module:
    trust: full
`
	if err := os.WriteFile(policyPath, []byte(policyYAML), 0644); err != nil {
		t.Fatalf("Failed to write policy file: %v", err)
	}

	policy, err := LoadPolicyFromFile(policyPath)
	if err != nil {
		t.Fatalf("LoadPolicyFromFile error: %v", err)
	}

	if policy.Defaults.Trust != TrustLevelLimited {
		t.Errorf("Default Trust = %s, want limited", policy.Defaults.Trust)
	}
	if len(policy.Defaults.DeniedCapabilities) != 2 {
		t.Errorf("DeniedCapabilities count = %d, want 2", len(policy.Defaults.DeniedCapabilities))
	}
	if policy.Modules["internal/trusted-module"].Trust != TrustLevelFull {
		t.Error("internal/trusted-module should have full trust")
	}
}

func TestPolicyEvaluator_CapabilityLockIntegration(t *testing.T) {
	lockStore := NewInMemoryLockStore()

	// Lock a module with specific capabilities
	lock := &CapabilityLock{
		ModuleName:   "locked-module",
		Capabilities: []string{"fs.read", "log"},
		LockedAt:     time.Now(),
		LockedBy:     "admin",
	}
	lockStore.SetLock(lock)

	policy := &CapabilityPolicy{
		SchemaVersion: 1,
		Modules: map[string]*ModulePolicy{
			"locked-module": {Lock: true},
		},
	}

	eval := NewPolicyEvaluator(policy, lockStore)

	// Allowed capability in lock
	decision := eval.EvaluateCapability("locked-module", "fs.read", nil)
	if !decision.Allowed {
		t.Error("fs.read should be allowed (in lock)")
	}

	// New capability not in lock
	decision = eval.EvaluateCapability("locked-module", "exec", nil)
	if decision.Allowed {
		t.Error("exec should be denied (not in lock)")
	}
	if !decision.FromLock {
		t.Error("Decision should be FromLock")
	}
}
