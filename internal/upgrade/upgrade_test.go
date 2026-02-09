package upgrade

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Version Tests
// =============================================================================

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Version
		wantErr bool
	}{
		{
			name:  "simple version",
			input: "1.2.3",
			want:  Version{Major: 1, Minor: 2, Patch: 3},
		},
		{
			name:  "version with v prefix",
			input: "v1.2.3",
			want:  Version{Major: 1, Minor: 2, Patch: 3},
		},
		{
			name:  "version with prerelease",
			input: "1.2.3-alpha.1",
			want:  Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha.1"},
		},
		{
			name:  "version with build metadata",
			input: "1.2.3+build.123",
			want:  Version{Major: 1, Minor: 2, Patch: 3, Build: "build.123"},
		},
		{
			name:  "version with prerelease and build",
			input: "1.2.3-beta.2+build.456",
			want:  Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta.2", Build: "build.456"},
		},
		{
			name:    "invalid version",
			input:   "not-a-version",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "partial version",
			input:   "1.2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Major != tt.want.Major || got.Minor != tt.want.Minor || got.Patch != tt.want.Patch {
					t.Errorf("ParseVersion() = %v, want %v", got, tt.want)
				}
				if got.Prerelease != tt.want.Prerelease {
					t.Errorf("ParseVersion() prerelease = %v, want %v", got.Prerelease, tt.want.Prerelease)
				}
				if got.Build != tt.want.Build {
					t.Errorf("ParseVersion() build = %v, want %v", got.Build, tt.want.Build)
				}
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name string
		v1   Version
		v2   Version
		want int
	}{
		{
			name: "equal versions",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: 0,
		},
		{
			name: "v1 major greater",
			v1:   Version{Major: 2, Minor: 0, Patch: 0},
			v2:   Version{Major: 1, Minor: 9, Patch: 9},
			want: 1,
		},
		{
			name: "v1 major less",
			v1:   Version{Major: 1, Minor: 9, Patch: 9},
			v2:   Version{Major: 2, Minor: 0, Patch: 0},
			want: -1,
		},
		{
			name: "v1 minor greater",
			v1:   Version{Major: 1, Minor: 3, Patch: 0},
			v2:   Version{Major: 1, Minor: 2, Patch: 9},
			want: 1,
		},
		{
			name: "v1 minor less",
			v1:   Version{Major: 1, Minor: 2, Patch: 9},
			v2:   Version{Major: 1, Minor: 3, Patch: 0},
			want: -1,
		},
		{
			name: "v1 patch greater",
			v1:   Version{Major: 1, Minor: 2, Patch: 4},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: 1,
		},
		{
			name: "v1 patch less",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 1, Minor: 2, Patch: 4},
			want: -1,
		},
		{
			name: "prerelease less than release",
			v1:   Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha"},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: -1,
		},
		{
			name: "release greater than prerelease",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha"},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v1.Compare(tt.v2)
			if got != tt.want {
				t.Errorf("Version.Compare() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersionCompatibility(t *testing.T) {
	tests := []struct {
		name string
		v1   Version
		v2   Version
		want bool
	}{
		{
			name: "same major compatible",
			v1:   Version{Major: 1, Minor: 0, Patch: 0},
			v2:   Version{Major: 1, Minor: 2, Patch: 3},
			want: true,
		},
		{
			name: "different major incompatible",
			v1:   Version{Major: 1, Minor: 2, Patch: 3},
			v2:   Version{Major: 2, Minor: 0, Patch: 0},
			want: false,
		},
		{
			name: "zero major same minor compatible",
			v1:   Version{Major: 0, Minor: 3, Patch: 1},
			v2:   Version{Major: 0, Minor: 3, Patch: 5},
			want: true,
		},
		{
			name: "zero major different minor incompatible",
			v1:   Version{Major: 0, Minor: 3, Patch: 1},
			v2:   Version{Major: 0, Minor: 4, Patch: 0},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v1.IsCompatibleWith(tt.v2); got != tt.want {
				t.Errorf("Version.IsCompatibleWith() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		name string
		v    Version
		want string
	}{
		{
			name: "simple version",
			v:    Version{Major: 1, Minor: 2, Patch: 3},
			want: "1.2.3",
		},
		{
			name: "version with prerelease",
			v:    Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha"},
			want: "1.2.3-alpha",
		},
		{
			name: "version with build",
			v:    Version{Major: 1, Minor: 2, Patch: 3, Build: "build.123"},
			want: "1.2.3+build.123",
		},
		{
			name: "version with prerelease and build",
			v:    Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta", Build: "456"},
			want: "1.2.3-beta+456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.String()
			if got != tt.want {
				t.Errorf("Version.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Version Range Tests
// =============================================================================

func TestVersionRangeContains(t *testing.T) {
	tests := []struct {
		name string
		vr   VersionRange
		v    Version
		want bool
	}{
		{
			name: "version within range",
			vr: VersionRange{
				Min:        &Version{Major: 1, Minor: 0, Patch: 0},
				Max:        &Version{Major: 2, Minor: 0, Patch: 0},
				IncludeMin: true,
				IncludeMax: true,
			},
			v:    Version{Major: 1, Minor: 5, Patch: 0},
			want: true,
		},
		{
			name: "version at min boundary with include",
			vr: VersionRange{
				Min:        &Version{Major: 1, Minor: 0, Patch: 0},
				Max:        &Version{Major: 2, Minor: 0, Patch: 0},
				IncludeMin: true,
				IncludeMax: true,
			},
			v:    Version{Major: 1, Minor: 0, Patch: 0},
			want: true,
		},
		{
			name: "version at min boundary without include",
			vr: VersionRange{
				Min:        &Version{Major: 1, Minor: 0, Patch: 0},
				Max:        &Version{Major: 2, Minor: 0, Patch: 0},
				IncludeMin: false,
				IncludeMax: true,
			},
			v:    Version{Major: 1, Minor: 0, Patch: 0},
			want: false,
		},
		{
			name: "version at max boundary with include",
			vr: VersionRange{
				Min:        &Version{Major: 1, Minor: 0, Patch: 0},
				Max:        &Version{Major: 2, Minor: 0, Patch: 0},
				IncludeMin: true,
				IncludeMax: true,
			},
			v:    Version{Major: 2, Minor: 0, Patch: 0},
			want: true,
		},
		{
			name: "version below range",
			vr: VersionRange{
				Min:        &Version{Major: 1, Minor: 0, Patch: 0},
				Max:        &Version{Major: 2, Minor: 0, Patch: 0},
				IncludeMin: true,
				IncludeMax: true,
			},
			v:    Version{Major: 0, Minor: 9, Patch: 9},
			want: false,
		},
		{
			name: "version above range",
			vr: VersionRange{
				Min:        &Version{Major: 1, Minor: 0, Patch: 0},
				Max:        &Version{Major: 2, Minor: 0, Patch: 0},
				IncludeMin: true,
				IncludeMax: true,
			},
			v:    Version{Major: 2, Minor: 0, Patch: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.vr.Contains(tt.v)
			if got != tt.want {
				t.Errorf("VersionRange.Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Version Checker Tests
// =============================================================================

func TestVersionChecker(t *testing.T) {
	checker := NewVersionChecker(nil)

	// Add a compatibility matrix
	matrix := &CompatibilityMatrix{
		Component: ComponentServer,
		Entries: []CompatibilityEntry{
			{
				Version:    Version{Major: 1, Minor: 5, Patch: 0},
				MinUpgrade: &Version{Major: 1, Minor: 0, Patch: 0},
				MaxUpgrade: &Version{Major: 2, Minor: 0, Patch: 0},
			},
			{
				Version:    Version{Major: 1, Minor: 8, Patch: 0},
				MinUpgrade: &Version{Major: 1, Minor: 0, Patch: 0},
				MaxUpgrade: &Version{Major: 2, Minor: 0, Patch: 0},
			},
		},
	}
	checker.LoadMatrix(ComponentServer, matrix)

	t.Run("compatible upgrade", func(t *testing.T) {
		from := Version{Major: 1, Minor: 5, Patch: 0}
		to := Version{Major: 1, Minor: 8, Patch: 0}

		result, err := checker.CheckCompatibility(ComponentServer, from, to)
		if err != nil {
			t.Fatalf("CheckCompatibility() error = %v", err)
		}
		if !result.Compatible {
			t.Errorf("CheckCompatibility() compatible = false, want true")
		}
	})

	t.Run("no matrix for component", func(t *testing.T) {
		from := Version{Major: 1, Minor: 0, Patch: 0}
		to := Version{Major: 1, Minor: 1, Patch: 0}

		result, err := checker.CheckCompatibility(ComponentAgent, from, to)
		if err != nil {
			t.Fatalf("CheckCompatibility() error = %v", err)
		}
		// Should return compatible when no matrix exists (no restrictions)
		if !result.Compatible {
			t.Errorf("CheckCompatibility() compatible = false, want true (no restrictions)")
		}
	})

	t.Run("no matrix incompatible", func(t *testing.T) {
		from := Version{Major: 1, Minor: 0, Patch: 0}
		to := Version{Major: 2, Minor: 0, Patch: 0}

		result, err := checker.CheckCompatibility(ComponentAgent, from, to)
		if err != nil {
			t.Fatalf("CheckCompatibility() error = %v", err)
		}
		if result.Compatible {
			t.Error("CheckCompatibility() compatible = true, want false for incompatible majors")
		}
		if len(result.Blockers) == 0 {
			t.Error("CheckCompatibility() blockers = none, want blockers for incompatible majors")
		}
	})
}

// =============================================================================
// Upgrade Types Tests
// =============================================================================

func TestStrategy(t *testing.T) {
	strategies := []Strategy{
		StrategyRolling,
		StrategyBlueGreen,
		StrategyCanary,
		StrategyInPlace,
	}

	expected := []string{"rolling", "blue-green", "canary", "in-place"}

	for i, s := range strategies {
		if string(s) != expected[i] {
			t.Errorf("Strategy %d = %v, want %v", i, s, expected[i])
		}
	}
}

func TestPhase(t *testing.T) {
	phases := []Phase{
		PhaseIdle,
		PhasePending,
		PhaseValidating,
		PhasePreparing,
		PhaseUpgrading,
		PhaseVerifying,
		PhaseCompleted,
		PhaseFailed,
		PhaseRollingBack,
		PhaseRolledBack,
	}

	expected := []string{
		"idle", "pending", "validating", "preparing",
		"upgrading", "verifying", "completed", "failed",
		"rolling_back", "rolled_back",
	}

	for i, p := range phases {
		if string(p) != expected[i] {
			t.Errorf("Phase %d = %v, want %v", i, p, expected[i])
		}
	}
}

func TestComponentType(t *testing.T) {
	components := []ComponentType{
		ComponentServer,
		ComponentAgent,
		ComponentNATS,
		ComponentDatabase,
		ComponentEtcd,
	}

	expected := []string{
		"server", "agent", "nats",
		"database", "etcd",
	}

	for i, c := range components {
		if string(c) != expected[i] {
			t.Errorf("Component %d = %v, want %v", i, c, expected[i])
		}
	}
}

func TestHealthStatus(t *testing.T) {
	statuses := []HealthStatus{
		HealthUnknown,
		HealthHealthy,
		HealthDegraded,
		HealthUnhealthy,
	}

	expected := []string{"unknown", "healthy", "degraded", "unhealthy"}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("HealthStatus %d = %v, want %v", i, s, expected[i])
		}
	}
}

// =============================================================================
// Config Tests
// =============================================================================

func TestDefaultRollingConfig(t *testing.T) {
	cfg := DefaultRollingConfig()

	if cfg == nil {
		t.Fatal("DefaultRollingConfig() returned nil")
	}
	if cfg.MaxUnavailable != 1 {
		t.Errorf("MaxUnavailable = %d, want 1", cfg.MaxUnavailable)
	}
	if cfg.DrainTimeout != 2*time.Minute {
		t.Errorf("DrainTimeout = %v, want %v", cfg.DrainTimeout, 2*time.Minute)
	}
	if cfg.NodeDelay != 30*time.Second {
		t.Errorf("NodeDelay = %v, want %v", cfg.NodeDelay, 30*time.Second)
	}
	if cfg.Order != "leader_last" {
		t.Errorf("Order = %v, want leader_last", cfg.Order)
	}
}

func TestDefaultCanaryConfig(t *testing.T) {
	cfg := DefaultCanaryConfig()

	if cfg == nil {
		t.Fatal("DefaultCanaryConfig() returned nil")
	}
	if cfg.InitialPercentage != 5 {
		t.Errorf("InitialPercentage = %d, want 5", cfg.InitialPercentage)
	}
	if cfg.Increment != 10 {
		t.Errorf("Increment = %d, want 10", cfg.Increment)
	}
	if cfg.SuccessThreshold != 3 {
		t.Errorf("SuccessThreshold = %d, want 3", cfg.SuccessThreshold)
	}
	if cfg.Interval != 5*time.Minute {
		t.Errorf("Interval = %v, want %v", cfg.Interval, 5*time.Minute)
	}
	if cfg.QueryTimeout != 15*time.Second {
		t.Errorf("QueryTimeout = %v, want %v", cfg.QueryTimeout, 15*time.Second)
	}
}

func TestDefaultHealthCheckConfig(t *testing.T) {
	cfg := DefaultHealthCheckConfig()

	if cfg == nil {
		t.Fatal("DefaultHealthCheckConfig() returned nil")
	}
	if cfg.Interval != 10*time.Second {
		t.Errorf("Interval = %v, want %v", cfg.Interval, 10*time.Second)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 5*time.Second)
	}
	if cfg.SuccessThreshold != 3 {
		t.Errorf("SuccessThreshold = %d, want 3", cfg.SuccessThreshold)
	}
	if cfg.FailureThreshold != 2 {
		t.Errorf("FailureThreshold = %d, want 2", cfg.FailureThreshold)
	}
}

func TestDefaultRollbackConfig(t *testing.T) {
	cfg := DefaultRollbackConfig()

	if cfg == nil {
		t.Fatal("DefaultRollbackConfig() returned nil")
	}
	if !cfg.Automatic {
		t.Error("Automatic should be true")
	}
	if cfg.OnFailureCount != 3 {
		t.Errorf("OnFailureCount = %d, want 3", cfg.OnFailureCount)
	}
	if !cfg.KeepPreviousVersion {
		t.Error("KeepPreviousVersion should be true")
	}
	if cfg.Timeout != 10*time.Minute {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 10*time.Minute)
	}
}

func TestDefaultAgentBatchConfig(t *testing.T) {
	cfg := DefaultAgentBatchConfig()

	if cfg == nil {
		t.Fatal("DefaultAgentBatchConfig() returned nil")
	}
	if cfg.BatchSize != 10 {
		t.Errorf("BatchSize = %d, want 10", cfg.BatchSize)
	}
	if cfg.BatchDelay != 30*time.Second {
		t.Errorf("BatchDelay = %v, want %v", cfg.BatchDelay, 30*time.Second)
	}
	if cfg.MaxFailures != 2 {
		t.Errorf("MaxFailures = %d, want 2", cfg.MaxFailures)
	}
}

// =============================================================================
// Upgrade State Tests
// =============================================================================

func TestState(t *testing.T) {
	toVersion, _ := ParseVersion("2.0.0")
	fromVersion, _ := ParseVersion("1.0.0")
	state := &State{
		ID:          "upgrade-123",
		Phase:       PhasePending,
		Status:      StatusPending,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		StartTime:   time.Now(),
		NodeStates:  make(map[string]*NodeUpgradeState),
	}

	if state.ID != "upgrade-123" {
		t.Errorf("ID = %v, want upgrade-123", state.ID)
	}
	if state.ToVersion.String() != "2.0.0" {
		t.Errorf("ToVersion = %v, want 2.0.0", state.ToVersion.String())
	}
	if state.Phase != PhasePending {
		t.Errorf("Phase = %v, want %v", state.Phase, PhasePending)
	}
	if state.Progress != 0 {
		t.Errorf("Progress = %d, want 0", state.Progress)
	}
	if state.NodeStates == nil {
		t.Error("NodeStates should be initialized")
	}
}

func TestStatePhaseTransition(t *testing.T) {
	state := &State{
		ID:         "upgrade-123",
		Phase:      PhasePending,
		NodeStates: make(map[string]*NodeUpgradeState),
	}

	// Simulate phase transition
	state.Phase = PhaseValidating

	if state.Phase != PhaseValidating {
		t.Errorf("Phase = %v, want %v", state.Phase, PhaseValidating)
	}
}

func TestStateErrors(t *testing.T) {
	state := &State{
		ID:         "upgrade-123",
		Phase:      PhasePending,
		NodeStates: make(map[string]*NodeUpgradeState),
		Errors:     []Error{},
	}

	// Add error
	state.Errors = append(state.Errors, Error{
		Message: "test error",
		NodeID:  "test-node",
		Time:    time.Now(),
	})

	if len(state.Errors) != 1 {
		t.Fatalf("Errors length = %d, want 1", len(state.Errors))
	}
	if state.Errors[0].Message != "test error" {
		t.Errorf("Error message = %v, want 'test error'", state.Errors[0].Message)
	}
	if state.Errors[0].NodeID != "test-node" {
		t.Errorf("Error nodeID = %v, want 'test-node'", state.Errors[0].NodeID)
	}
}

// =============================================================================
// Agent Upgrader Tests
// =============================================================================

func TestNewAgentUpgrader(t *testing.T) {
	upgrader := NewAgentUpgrader(nil, nil, nil)

	if upgrader == nil {
		t.Fatal("NewAgentUpgrader() returned nil")
	}
	if upgrader.config == nil {
		t.Error("config should have default value")
	}
	if upgrader.inProgress == nil {
		t.Error("inProgress map should be initialized")
	}
}

func TestAgentUpgradeProgressPercentComplete(t *testing.T) {
	tests := []struct {
		name     string
		progress AgentUpgradeProgress
		want     int
	}{
		{
			name: "no progress",
			progress: AgentUpgradeProgress{
				CurrentBatch: 0,
				TotalBatches: 5,
				Completed:    0,
				Failed:       0,
				InProgress:   []string{},
			},
			want: 0,
		},
		{
			name: "some progress",
			progress: AgentUpgradeProgress{
				CurrentBatch: 2,
				TotalBatches: 5,
				Completed:    10,
				Failed:       0,
				InProgress:   []string{"agent-1", "agent-2"},
			},
			want: 66, // 10 / (10 + 0 + 2 + 3) = 10/15 ≈ 66%
		},
		{
			name: "completed",
			progress: AgentUpgradeProgress{
				CurrentBatch: 5,
				TotalBatches: 5,
				Completed:    50,
				Failed:       0,
				InProgress:   []string{},
			},
			want: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.progress.PercentComplete()
			if got != tt.want {
				t.Errorf("PercentComplete() = %d, want %d", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Rolling Strategy Tests
// =============================================================================

func TestNewRollingStrategy(t *testing.T) {
	config := DefaultRollingConfig()
	strategy := NewRollingStrategy(nil, nil, config)

	if strategy == nil {
		t.Fatal("NewRollingStrategy() returned nil")
	}
	if strategy.config != config {
		t.Error("config not set correctly")
	}
}

func TestRollingStats(t *testing.T) {
	stats := &RollingStats{
		CurrentBatch:   2,
		CompletedNodes: 5,
		FailedNodes:    1,
		HealthyNodes:   4,
	}

	if stats.CurrentBatch != 2 {
		t.Errorf("CurrentBatch = %d, want 2", stats.CurrentBatch)
	}
	if stats.CompletedNodes != 5 {
		t.Errorf("CompletedNodes = %d, want 5", stats.CompletedNodes)
	}
	if stats.FailedNodes != 1 {
		t.Errorf("FailedNodes = %d, want 1", stats.FailedNodes)
	}
}

// =============================================================================
// Canary Strategy Tests
// =============================================================================

func TestNewCanaryStrategy(t *testing.T) {
	config := DefaultCanaryConfig()
	strategy := NewCanaryStrategy(nil, nil, config)

	if strategy == nil {
		t.Fatal("NewCanaryStrategy() returned nil")
	}
	if strategy.config != config {
		t.Error("config not set correctly")
	}
}

func TestCanaryStrategy_UpgradeNode_ContextCancel(t *testing.T) {
	nm := newMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentServer, Version: Version{Major: 1, Minor: 0, Patch: 0}},
	}
	nm.healthMap["node-1"] = HealthUnhealthy

	strategy := NewCanaryStrategy(nm, nil, DefaultCanaryConfig())
	state := &State{
		Config: &Config{
			TargetVersion: "1.1.0",
		},
		NodeStates: make(map[string]*NodeUpgradeState),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := strategy.upgradeNode(ctx, state, nm.nodes[0])
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestCanaryStats(t *testing.T) {
	stats := &CanaryStats{
		CurrentPercentage: 25,
		SuccessfulChecks:  5,
		FailedChecks:      1,
		Metrics: map[string]float64{
			"error_rate":    0.01,
			"response_time": 150.0,
		},
	}

	if stats.CurrentPercentage != 25 {
		t.Errorf("CurrentPercentage = %d, want 25", stats.CurrentPercentage)
	}
	if stats.SuccessfulChecks != 5 {
		t.Errorf("SuccessfulChecks = %d, want 5", stats.SuccessfulChecks)
	}
	if stats.FailedChecks != 1 {
		t.Errorf("FailedChecks = %d, want 1", stats.FailedChecks)
	}
}

// =============================================================================
// Rollback Manager Tests
// =============================================================================

func TestNewRollbackManager(t *testing.T) {
	config := DefaultRollbackConfig()
	manager := NewRollbackManager(nil, nil, nil, config)

	if manager == nil {
		t.Fatal("NewRollbackManager() returned nil")
	}
	if manager.config != config {
		t.Error("config not set correctly")
	}
}

func TestRollbackDecision(t *testing.T) {
	decision := &RollbackDecision{
		ShouldRollback: true,
		Confidence:     0.85,
		Reasons: []string{
			"High failure rate",
			"Multiple node failures",
		},
	}

	if !decision.ShouldRollback {
		t.Error("ShouldRollback should be true")
	}
	if decision.Confidence != 0.85 {
		t.Errorf("Confidence = %f, want 0.85", decision.Confidence)
	}
	if len(decision.Reasons) != 2 {
		t.Errorf("Reasons length = %d, want 2", len(decision.Reasons))
	}
}

func TestRollbackOperation(t *testing.T) {
	op := &RollbackOperation{
		ID:              "rollback-123",
		UpgradeID:       "upgrade-456",
		Reason:          "Too many failures",
		Automatic:       true,
		Status:          StatusInProgress,
		NodesRolledBack: 5,
		NodesFailed:     1,
	}

	if op.ID != "rollback-123" {
		t.Errorf("ID = %v, want rollback-123", op.ID)
	}
	if op.UpgradeID != "upgrade-456" {
		t.Errorf("UpgradeID = %v, want upgrade-456", op.UpgradeID)
	}
	if !op.Automatic {
		t.Error("Automatic should be true")
	}
	if op.Status != StatusInProgress {
		t.Errorf("Status = %v, want %v", op.Status, StatusInProgress)
	}
}

// =============================================================================
// Upgrade Manager Tests
// =============================================================================

func TestNewDefaultManager(t *testing.T) {
	manager := NewDefaultManager(nil, nil, nil)

	if manager == nil {
		t.Fatal("NewDefaultManager() returned nil")
	}
}

func TestCheckResult(t *testing.T) {
	currentVersion, _ := ParseVersion("1.0.0")
	targetVersion, _ := ParseVersion("2.0.0")

	check := &Check{
		Compatible:     true,
		CurrentVersion: currentVersion,
		TargetVersion:  targetVersion,
		Warnings: []string{
			"Breaking change in API",
		},
	}

	if !check.Compatible {
		t.Error("Compatible should be true")
	}
	if check.CurrentVersion.String() != "1.0.0" {
		t.Errorf("CurrentVersion = %v, want 1.0.0", check.CurrentVersion.String())
	}
	if len(check.Warnings) != 1 {
		t.Errorf("Warnings length = %d, want 1", len(check.Warnings))
	}
}

func TestPlan(t *testing.T) {
	plan := &Plan{
		ID: "plan-123",
		Config: &Config{
			Strategy: StrategyRolling,
		},
		Steps: []Step{
			{
				Order:     1,
				Name:      "upgrade-servers",
				Component: ComponentServer,
				Nodes:     []string{"server-1", "server-2"},
			},
			{
				Order:     2,
				Name:      "upgrade-agents",
				Component: ComponentAgent,
				Nodes:     []string{"agent-1", "agent-2"},
			},
		},
		TotalNodes:        4,
		EstimatedDuration: 30 * time.Minute,
	}

	if plan.ID != "plan-123" {
		t.Errorf("ID = %v, want plan-123", plan.ID)
	}
	if plan.Config.Strategy != StrategyRolling {
		t.Errorf("Strategy = %v, want %v", plan.Config.Strategy, StrategyRolling)
	}
	if len(plan.Steps) != 2 {
		t.Errorf("Steps length = %d, want 2", len(plan.Steps))
	}
}

// =============================================================================
// Integration Tests (with mock components)
// =============================================================================

type mockNodeManager struct {
	mu            sync.Mutex
	nodes         []NodeInfo
	healthMap     map[string]HealthStatus
	drainCalled   map[string]bool
	versionMap    map[string]Version
	upgradedNodes map[string]string
}

func newMockNodeManager() *mockNodeManager {
	return &mockNodeManager{
		nodes:         []NodeInfo{},
		healthMap:     make(map[string]HealthStatus),
		drainCalled:   make(map[string]bool),
		versionMap:    make(map[string]Version),
		upgradedNodes: make(map[string]string),
	}
}

func (m *mockNodeManager) GetNodes(ctx context.Context, component ComponentType) ([]NodeInfo, error) {
	var result []NodeInfo
	for _, n := range m.nodes {
		if n.Component == component {
			result = append(result, n)
		}
	}
	return result, nil
}

func (m *mockNodeManager) DrainNode(ctx context.Context, nodeID string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drainCalled[nodeID] = true
	return nil
}

func (m *mockNodeManager) UncordonNode(ctx context.Context, nodeID string) error {
	return nil
}

func (m *mockNodeManager) UpgradeNode(ctx context.Context, nodeID, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upgradedNodes[nodeID] = version
	// Update the version
	v, _ := ParseVersion(version)
	m.versionMap[nodeID] = v
	return nil
}

func (m *mockNodeManager) GetNodeHealth(ctx context.Context, nodeID string) (HealthStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if status, ok := m.healthMap[nodeID]; ok {
		return status, nil
	}
	return HealthHealthy, nil
}

func (m *mockNodeManager) GetNodeVersion(ctx context.Context, nodeID string) (Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.versionMap[nodeID]; ok {
		return v, nil
	}
	// Return the initial version
	return Version{Major: 1, Minor: 0, Patch: 0}, nil
}

func (m *mockNodeManager) RollbackNode(ctx context.Context, nodeID, version string) error {
	return nil
}

func TestAgentUpgraderWithMock(t *testing.T) {
	nm := newMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "agent-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}},
		{ID: "agent-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}},
		{ID: "agent-3", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}},
	}
	nm.healthMap["agent-1"] = HealthHealthy
	nm.healthMap["agent-2"] = HealthHealthy
	nm.healthMap["agent-3"] = HealthHealthy

	config := &AgentBatchConfig{
		BatchSize:   2,
		BatchDelay:  0, // No delay for testing
		MaxFailures: 1,
	}

	upgrader := NewAgentUpgrader(nm, nil, config)

	ctx := context.Background()
	err := upgrader.UpgradeAgents(ctx, "1.1.0", nil)

	if err != nil {
		t.Errorf("UpgradeAgents() error = %v", err)
	}

	progress := upgrader.GetProgress()
	if progress.Completed != 3 {
		t.Errorf("Completed = %d, want 3", progress.Completed)
	}
}

func TestAgentUpgraderVersionReport(t *testing.T) {
	nm := newMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "agent-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}, Health: HealthHealthy},
		{ID: "agent-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}, Health: HealthHealthy},
		{ID: "agent-3", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}, Health: HealthDegraded},
	}

	upgrader := NewAgentUpgrader(nm, nil, nil)

	ctx := context.Background()
	report, err := upgrader.GetAgentVersionReport(ctx, "1.1.0")

	if err != nil {
		t.Fatalf("GetAgentVersionReport() error = %v", err)
	}

	if report.TotalAgents != 3 {
		t.Errorf("TotalAgents = %d, want 3", report.TotalAgents)
	}
	if report.HealthyAgents != 2 {
		t.Errorf("HealthyAgents = %d, want 2", report.HealthyAgents)
	}
	if report.UnhealthyAgents != 1 {
		t.Errorf("UnhealthyAgents = %d, want 1", report.UnhealthyAgents)
	}
	if len(report.OutdatedAgents) != 2 {
		t.Errorf("OutdatedAgents length = %d, want 2", len(report.OutdatedAgents))
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkParseVersion(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseVersion("1.2.3-alpha.1+build.456")
	}
}

func BenchmarkVersionCompare(b *testing.B) {
	v1 := Version{Major: 1, Minor: 2, Patch: 3}
	v2 := Version{Major: 1, Minor: 2, Patch: 4}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v1.Compare(v2)
	}
}

func BenchmarkVersionString(b *testing.B) {
	v := Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha", Build: "build.123"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.String()
	}
}

// =============================================================================
// Rollback Failure Tests
// =============================================================================

// failingMockNodeManager extends mockNodeManager with configurable failure behaviors
type failingMockNodeManager struct {
	mockNodeManager
	upgradeFailNodes  map[string]bool // Nodes that fail during upgrade
	rollbackFailNodes map[string]bool // Nodes that fail during rollback (downgrade)
	healthFailNodes   map[string]bool // Nodes that never become healthy
	versionMismatch   map[string]bool // Nodes that report wrong version after upgrade
	drainFailNodes    map[string]bool // Nodes that fail to drain
	upgradeCalls      []string        // Track order of upgrade calls
	rollbackCalls     []string        // Track order of rollback calls
	healthCheckCount  map[string]int  // Count health checks per node
	healthCheckDelay  time.Duration   // Delay before health check returns
}

func newFailingMockNodeManager() *failingMockNodeManager {
	return &failingMockNodeManager{
		mockNodeManager: mockNodeManager{
			nodes:         []NodeInfo{},
			healthMap:     make(map[string]HealthStatus),
			drainCalled:   make(map[string]bool),
			versionMap:    make(map[string]Version),
			upgradedNodes: make(map[string]string),
		},
		upgradeFailNodes:  make(map[string]bool),
		rollbackFailNodes: make(map[string]bool),
		healthFailNodes:   make(map[string]bool),
		versionMismatch:   make(map[string]bool),
		drainFailNodes:    make(map[string]bool),
		upgradeCalls:      make([]string, 0),
		rollbackCalls:     make([]string, 0),
		healthCheckCount:  make(map[string]int),
	}
}

func (m *failingMockNodeManager) UpgradeNode(ctx context.Context, nodeID, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	targetVersion, _ := ParseVersion(version)
	currentVersion := m.versionMap[nodeID]

	// Detect rollback (downgrade) - target version is older than current
	isRollback := targetVersion.Compare(currentVersion) < 0

	if isRollback {
		m.rollbackCalls = append(m.rollbackCalls, nodeID)
		if m.rollbackFailNodes[nodeID] {
			return fmt.Errorf("rollback failed for node %s", nodeID)
		}
	} else {
		m.upgradeCalls = append(m.upgradeCalls, nodeID)
		if m.upgradeFailNodes[nodeID] {
			return fmt.Errorf("upgrade failed for node %s", nodeID)
		}
	}

	m.upgradedNodes[nodeID] = version

	// Simulate version mismatch
	if m.versionMismatch[nodeID] {
		// Store a different version than requested
		m.versionMap[nodeID] = Version{Major: 99, Minor: 99, Patch: 99}
	} else {
		m.versionMap[nodeID] = targetVersion
	}

	return nil
}

func (m *failingMockNodeManager) DrainNode(ctx context.Context, nodeID string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.drainCalled[nodeID] = true

	if m.drainFailNodes[nodeID] {
		return fmt.Errorf("drain failed for node %s", nodeID)
	}

	return nil
}

func (m *failingMockNodeManager) GetNodeHealth(ctx context.Context, nodeID string) (HealthStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.healthCheckCount[nodeID]++

	// Simulate delay
	if m.healthCheckDelay > 0 {
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			m.mu.Lock()
			return HealthUnknown, ctx.Err()
		case <-time.After(m.healthCheckDelay):
		}
		m.mu.Lock()
	}

	if m.healthFailNodes[nodeID] {
		return HealthUnhealthy, nil
	}

	if status, ok := m.healthMap[nodeID]; ok {
		return status, nil
	}

	return HealthHealthy, nil
}

func TestRollbackOnUpgradeFailure(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}},
		{ID: "node-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}},
		{ID: "node-3", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}},
	}
	// node-1 was upgraded to 1.1.0 successfully, so initialize its version map
	// to reflect the post-upgrade state for rollback detection
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-1"] = HealthHealthy
	nm.healthMap["node-2"] = HealthHealthy
	nm.healthMap["node-3"] = HealthHealthy

	// node-2 will fail during upgrade
	nm.upgradeFailNodes["node-2"] = true

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.OnFailureCount = 1 // Trigger rollback after 1 failure
	rollbackConfig.Automatic = true

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	// Create an upgrade state simulating a failed upgrade
	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}

	upgradeState := &State{
		ID:          "upgrade-test-1",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Phase:       PhaseFailed,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   time.Now().Add(-2 * time.Minute),
				EndTime:     timePtr(time.Now().Add(-1 * time.Minute)),
			},
		},
	}

	// Evaluate if rollback is needed
	decision := rollbackMgr.EvaluateRollbackNeed(context.Background(), upgradeState)

	// Should not rollback since node-1 succeeded (no failures in state)
	// But let's test the rollback execution
	ctx := context.Background()
	op, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "test rollback", true)

	if err != nil {
		t.Fatalf("RollbackUpgrade() error = %v", err)
	}

	if op.Status != StatusCompleted {
		t.Errorf("Rollback status = %v, want %v", op.Status, StatusCompleted)
	}

	if op.NodesRolledBack != 1 {
		t.Errorf("NodesRolledBack = %d, want 1", op.NodesRolledBack)
	}

	// Check that decision struct is valid
	if decision == nil {
		t.Fatal("EvaluateRollbackNeed returned nil")
	}
}

func TestRollbackPartialSuccess(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-3", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
	}
	// Initialize version map for rollback detection (current version is 1.1.0)
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-2"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-3"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-1"] = HealthHealthy
	nm.healthMap["node-2"] = HealthHealthy
	nm.healthMap["node-3"] = HealthHealthy

	// node-2 will fail during rollback (downgrade)
	nm.rollbackFailNodes["node-2"] = true

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.Automatic = true // Continue on failure

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	upgradeState := &State{
		ID:          "upgrade-test-2",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-3 * time.Minute),
				EndTime:     timePtr(now.Add(-2 * time.Minute)),
			},
			"node-2": {
				NodeID:      "node-2",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-2 * time.Minute),
				EndTime:     timePtr(now.Add(-1 * time.Minute)),
			},
			"node-3": {
				NodeID:      "node-3",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-1 * time.Minute),
				EndTime:     timePtr(now),
			},
		},
	}

	ctx := context.Background()
	op, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "partial rollback test", true)

	// For automatic rollbacks, should complete even with partial failures
	if err != nil {
		t.Fatalf("RollbackUpgrade() unexpected error = %v", err)
	}

	if op.Status != StatusCompleted {
		t.Errorf("Rollback status = %v, want %v", op.Status, StatusCompleted)
	}

	// Should have rolled back 2 nodes (node-1 and node-3), failed 1 (node-2)
	if op.NodesRolledBack != 2 {
		t.Errorf("NodesRolledBack = %d, want 2", op.NodesRolledBack)
	}

	if op.NodesFailed != 1 {
		t.Errorf("NodesFailed = %d, want 1", op.NodesFailed)
	}
}

func TestManualRollbackAbortOnFailure(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-3", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
	}
	// Initialize version map for rollback detection (current version is 1.1.0)
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-2"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-3"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-1"] = HealthHealthy
	nm.healthMap["node-2"] = HealthHealthy
	nm.healthMap["node-3"] = HealthHealthy

	// node-2 will fail during rollback
	nm.rollbackFailNodes["node-2"] = true

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.Automatic = false // Manual: abort on first failure

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	upgradeState := &State{
		ID:          "upgrade-test-3",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-3 * time.Minute),
				EndTime:     timePtr(now.Add(-2 * time.Minute)),
			},
			"node-2": {
				NodeID:      "node-2",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-2 * time.Minute),
				EndTime:     timePtr(now.Add(-1 * time.Minute)),
			},
			"node-3": {
				NodeID:      "node-3",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-1 * time.Minute),
				EndTime:     timePtr(now),
			},
		},
	}

	ctx := context.Background()
	op, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "manual rollback test", false)

	// For manual rollbacks, should fail on first node failure
	if err == nil {
		t.Fatal("RollbackUpgrade() expected error for manual rollback with failure")
	}

	if op.Status != StatusFailed {
		t.Errorf("Rollback status = %v, want %v", op.Status, StatusFailed)
	}

	// Should have failed (stopped early)
	if op.NodesFailed != 1 {
		t.Errorf("NodesFailed = %d, want 1", op.NodesFailed)
	}
}

func TestRollbackHealthCheckTimeout(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
	}
	// Initialize version map for rollback detection
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-2"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-2"] = HealthHealthy

	// node-1 never becomes healthy after rollback, node-2 succeeds
	nm.healthFailNodes["node-1"] = true

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.Automatic = true
	rollbackConfig.Timeout = 5 * time.Second // Short timeout for test

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	upgradeState := &State{
		ID:          "upgrade-test-4",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-2 * time.Minute),
				EndTime:     timePtr(now.Add(-1 * time.Minute)),
			},
			"node-2": {
				NodeID:      "node-2",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-1 * time.Minute),
				EndTime:     timePtr(now),
			},
		},
	}

	ctx := context.Background()
	op, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "health timeout test", true)

	// Should complete (one node succeeded) with node-1 marked as failed
	if err != nil {
		t.Fatalf("RollbackUpgrade() unexpected error = %v", err)
	}

	if op.NodesFailed != 1 {
		t.Errorf("NodesFailed = %d, want 1 (health check failed)", op.NodesFailed)
	}

	if op.NodesRolledBack != 1 {
		t.Errorf("NodesRolledBack = %d, want 1", op.NodesRolledBack)
	}

	nodeState := op.NodeStates["node-1"]
	if nodeState == nil {
		t.Fatal("Expected node state for node-1")
	}

	if nodeState.Status != StatusFailed {
		t.Errorf("Node status = %v, want %v", nodeState.Status, StatusFailed)
	}

	if nodeState.Error != "node did not become healthy after rollback" {
		t.Errorf("Node error = %q, want health check failure message", nodeState.Error)
	}
}

func TestRollbackVersionMismatch(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
	}
	// Initialize version map for rollback detection
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-2"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-1"] = HealthHealthy
	nm.healthMap["node-2"] = HealthHealthy

	// Version will be wrong after rollback for node-1 only
	nm.versionMismatch["node-1"] = true

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.Automatic = true

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	upgradeState := &State{
		ID:          "upgrade-test-5",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-2 * time.Minute),
				EndTime:     timePtr(now.Add(-1 * time.Minute)),
			},
			"node-2": {
				NodeID:      "node-2",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-1 * time.Minute),
				EndTime:     timePtr(now),
			},
		},
	}

	ctx := context.Background()
	op, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "version mismatch test", true)

	// Should complete (one node succeeded) with node-1 marked as failed due to version mismatch
	if err != nil {
		t.Fatalf("RollbackUpgrade() unexpected error = %v", err)
	}

	if op.NodesFailed != 1 {
		t.Errorf("NodesFailed = %d, want 1 (version mismatch)", op.NodesFailed)
	}

	if op.NodesRolledBack != 1 {
		t.Errorf("NodesRolledBack = %d, want 1", op.NodesRolledBack)
	}

	nodeState := op.NodeStates["node-1"]
	if nodeState == nil {
		t.Fatal("Expected node state for node-1")
	}

	if nodeState.Status != StatusFailed {
		t.Errorf("Node status = %v, want %v", nodeState.Status, StatusFailed)
	}
}

func TestRollbackReverseOrder(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-3", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
	}
	// Initialize version map for rollback detection (current version is 1.1.0)
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-2"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-3"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-1"] = HealthHealthy
	nm.healthMap["node-2"] = HealthHealthy
	nm.healthMap["node-3"] = HealthHealthy

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.Automatic = true

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	// Simulate upgrade order: node-1, node-2, node-3
	upgradeState := &State{
		ID:          "upgrade-test-6",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-30 * time.Minute),
				EndTime:     timePtr(now.Add(-20 * time.Minute)), // Completed first
			},
			"node-2": {
				NodeID:      "node-2",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-20 * time.Minute),
				EndTime:     timePtr(now.Add(-10 * time.Minute)), // Completed second
			},
			"node-3": {
				NodeID:      "node-3",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-10 * time.Minute),
				EndTime:     timePtr(now), // Completed last
			},
		},
	}

	ctx := context.Background()
	_, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "reverse order test", true)

	if err != nil {
		t.Fatalf("RollbackUpgrade() error = %v", err)
	}

	// Check that nodes were rolled back in reverse order: node-3, node-2, node-1
	expectedOrder := []string{"node-3", "node-2", "node-1"}
	if len(nm.rollbackCalls) != 3 {
		t.Fatalf("Expected 3 rollback calls, got %d", len(nm.rollbackCalls))
	}

	for i, nodeID := range expectedOrder {
		if nm.rollbackCalls[i] != nodeID {
			t.Errorf("Rollback order[%d] = %s, want %s", i, nm.rollbackCalls[i], nodeID)
		}
	}
}

func TestRollbackSkipsNonCompletedNodes(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 0, Patch: 0}}, // Never upgraded
		{ID: "node-3", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
	}
	// Initialize version map for rollback detection
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-2"] = Version{Major: 1, Minor: 0, Patch: 0} // Still on old version
	nm.versionMap["node-3"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-1"] = HealthHealthy
	nm.healthMap["node-2"] = HealthHealthy
	nm.healthMap["node-3"] = HealthHealthy

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.Automatic = true

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	upgradeState := &State{
		ID:          "upgrade-test-7",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted, // Will be rolled back
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-2 * time.Minute),
				EndTime:     timePtr(now.Add(-1 * time.Minute)),
			},
			"node-2": {
				NodeID:      "node-2",
				Component:   ComponentAgent,
				Status:      StatusFailed, // Should be skipped
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-1 * time.Minute),
				EndTime:     timePtr(now),
			},
			"node-3": {
				NodeID:      "node-3",
				Component:   ComponentAgent,
				Status:      StatusCompleted, // Will be rolled back
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now,
				EndTime:     timePtr(now),
			},
		},
	}

	ctx := context.Background()
	op, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "skip non-completed test", true)

	if err != nil {
		t.Fatalf("RollbackUpgrade() error = %v", err)
	}

	// Only node-1 and node-3 should be rolled back (node-2 was not completed)
	if op.NodesRolledBack != 2 {
		t.Errorf("NodesRolledBack = %d, want 2", op.NodesRolledBack)
	}

	// node-2 should not have been called
	for _, nodeID := range nm.rollbackCalls {
		if nodeID == "node-2" {
			t.Error("node-2 should not have been rolled back (was not completed)")
		}
	}
}

func TestRollbackCancellation(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
		{ID: "node-2", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
	}
	// Initialize version map for rollback detection
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.versionMap["node-2"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-1"] = HealthHealthy
	nm.healthMap["node-2"] = HealthHealthy

	// Add delay to health check so we can cancel mid-operation
	nm.healthCheckDelay = 100 * time.Millisecond

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.Automatic = true

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	upgradeState := &State{
		ID:          "upgrade-test-8",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-2 * time.Minute),
				EndTime:     timePtr(now.Add(-1 * time.Minute)),
			},
			"node-2": {
				NodeID:      "node-2",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-1 * time.Minute),
				EndTime:     timePtr(now),
			},
		},
	}

	// Create a context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	op, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "cancellation test", true)

	// Should fail due to context cancellation
	if err == nil {
		t.Fatal("RollbackUpgrade() expected error for cancelled context")
	}

	if op.Status != StatusFailed {
		t.Errorf("Rollback status = %v, want %v", op.Status, StatusFailed)
	}
}

func TestEvaluateRollbackNeed(t *testing.T) {
	nm := newFailingMockNodeManager()

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.OnFailureCount = 2 // Rollback if 2+ nodes fail
	rollbackConfig.Automatic = true

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	t.Run("no rollback needed", func(t *testing.T) {
		upgradeState := &State{
			ID:          "upgrade-eval-1",
			FromVersion: fromVersion,
			ToVersion:   toVersion,
			NodeStates: map[string]*NodeUpgradeState{
				"node-1": {
					NodeID: "node-1",
					Status: StatusCompleted,
				},
				"node-2": {
					NodeID: "node-2",
					Status: StatusCompleted,
				},
			},
		}

		decision := rollbackMgr.EvaluateRollbackNeed(context.Background(), upgradeState)

		if decision.ShouldRollback {
			t.Error("Should not recommend rollback when all nodes succeeded")
		}
	})

	t.Run("rollback needed - threshold met", func(t *testing.T) {
		upgradeState := &State{
			ID:          "upgrade-eval-2",
			FromVersion: fromVersion,
			ToVersion:   toVersion,
			NodeStates: map[string]*NodeUpgradeState{
				"node-1": {
					NodeID:    "node-1",
					Status:    StatusFailed,
					StartTime: now,
				},
				"node-2": {
					NodeID:    "node-2",
					Status:    StatusFailed,
					StartTime: now,
				},
				"node-3": {
					NodeID:    "node-3",
					Status:    StatusCompleted,
					StartTime: now,
				},
			},
		}

		decision := rollbackMgr.EvaluateRollbackNeed(context.Background(), upgradeState)

		if !decision.ShouldRollback {
			t.Error("Should recommend rollback when failure threshold met")
		}

		if len(decision.Reasons) == 0 {
			t.Error("Decision should have reasons")
		}

		if decision.Confidence < 0.5 {
			t.Errorf("Confidence = %f, expected >= 0.5", decision.Confidence)
		}
	})

	t.Run("below threshold", func(t *testing.T) {
		upgradeState := &State{
			ID:          "upgrade-eval-3",
			FromVersion: fromVersion,
			ToVersion:   toVersion,
			NodeStates: map[string]*NodeUpgradeState{
				"node-1": {
					NodeID: "node-1",
					Status: StatusFailed, // Only 1 failure, threshold is 2
				},
				"node-2": {
					NodeID: "node-2",
					Status: StatusCompleted,
				},
				"node-3": {
					NodeID: "node-3",
					Status: StatusCompleted,
				},
			},
		}

		decision := rollbackMgr.EvaluateRollbackNeed(context.Background(), upgradeState)

		// Should have reasons but not recommend rollback (below threshold)
		if len(decision.Reasons) == 0 {
			t.Error("Decision should have reasons about the failure")
		}
	})
}

func TestRollbackDrainFailureContinues(t *testing.T) {
	nm := newFailingMockNodeManager()
	nm.nodes = []NodeInfo{
		{ID: "node-1", Component: ComponentAgent, Version: Version{Major: 1, Minor: 1, Patch: 0}},
	}
	// Initialize version map for rollback detection
	nm.versionMap["node-1"] = Version{Major: 1, Minor: 1, Patch: 0}
	nm.healthMap["node-1"] = HealthHealthy

	// Drain will fail, but rollback should continue
	nm.drainFailNodes["node-1"] = true

	rollbackConfig := DefaultRollbackConfig()
	rollbackConfig.Automatic = true

	rollbackMgr := NewRollbackManager(nm, nil, nil, rollbackConfig)

	fromVersion := Version{Major: 1, Minor: 0, Patch: 0}
	toVersion := Version{Major: 1, Minor: 1, Patch: 0}
	now := time.Now()

	upgradeState := &State{
		ID:          "upgrade-test-drain",
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		NodeStates: map[string]*NodeUpgradeState{
			"node-1": {
				NodeID:      "node-1",
				Component:   ComponentAgent,
				Status:      StatusCompleted,
				FromVersion: fromVersion,
				ToVersion:   toVersion,
				StartTime:   now.Add(-1 * time.Minute),
				EndTime:     timePtr(now),
			},
		},
	}

	ctx := context.Background()
	op, err := rollbackMgr.RollbackUpgrade(ctx, upgradeState, "drain failure test", true)

	// Should succeed - drain failure is best effort
	if err != nil {
		t.Fatalf("RollbackUpgrade() unexpected error = %v", err)
	}

	if op.Status != StatusCompleted {
		t.Errorf("Rollback status = %v, want %v", op.Status, StatusCompleted)
	}

	if op.NodesRolledBack != 1 {
		t.Errorf("NodesRolledBack = %d, want 1", op.NodesRolledBack)
	}

	// Verify drain was still called
	if !nm.drainCalled["node-1"] {
		t.Error("Drain should have been called even if it fails")
	}
}

// Helper function
func timePtr(t time.Time) *time.Time {
	return &t
}
