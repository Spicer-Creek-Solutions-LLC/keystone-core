package upgrade

import (
	"context"
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

func TestUpgradeStrategy(t *testing.T) {
	strategies := []UpgradeStrategy{
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

func TestUpgradePhase(t *testing.T) {
	phases := []UpgradePhase{
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

func TestUpgradeState(t *testing.T) {
	toVersion, _ := ParseVersion("2.0.0")
	fromVersion, _ := ParseVersion("1.0.0")
	state := &UpgradeState{
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

func TestUpgradeStatePhaseTransition(t *testing.T) {
	state := &UpgradeState{
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

func TestUpgradeStateErrors(t *testing.T) {
	state := &UpgradeState{
		ID:         "upgrade-123",
		Phase:      PhasePending,
		NodeStates: make(map[string]*NodeUpgradeState),
		Errors:     []UpgradeError{},
	}

	// Add error
	state.Errors = append(state.Errors, UpgradeError{
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

func TestNewDefaultUpgradeManager(t *testing.T) {
	manager := NewDefaultUpgradeManager(nil, nil, nil)

	if manager == nil {
		t.Fatal("NewDefaultUpgradeManager() returned nil")
	}
}

func TestUpgradeCheckResult(t *testing.T) {
	currentVersion, _ := ParseVersion("1.0.0")
	targetVersion, _ := ParseVersion("2.0.0")

	check := &UpgradeCheck{
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

func TestUpgradePlan(t *testing.T) {
	plan := &UpgradePlan{
		ID: "plan-123",
		Config: &UpgradeConfig{
			Strategy: StrategyRolling,
		},
		Steps: []UpgradeStep{
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
