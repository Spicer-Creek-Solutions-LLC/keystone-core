package blueprint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultTrackerConfig(t *testing.T) {
	config := DefaultTrackerConfig()

	if config == nil {
		t.Fatal("DefaultTrackerConfig() returned nil")
	}

	if config.StorePath == "" {
		t.Error("StorePath should not be empty")
	}

	if config.MaxHistoryPerAgent <= 0 {
		t.Error("MaxHistoryPerAgent should be positive")
	}

	if !config.PersistOnChange {
		t.Error("PersistOnChange should be true by default")
	}
}

func TestNewTracker(t *testing.T) {
	tests := []struct {
		name    string
		config  *TrackerConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "custom config",
			config: &TrackerConfig{
				StorePath:          "/tmp/test-tracker.json",
				MaxHistoryPerAgent: 50,
				PersistOnChange:    false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker, err := NewTracker(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTracker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tracker == nil {
				t.Error("NewTracker() returned nil tracker")
			}
		})
	}
}

func TestTracker_RecordApply(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false, // Disable persistence for test
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	blueprint := &AppliedBlueprintInfo{
		Name:       "myorg/web-stack",
		Version:    "1.0.0",
		Namespace:  "web",
		StateCount: 5,
	}

	// Record successful apply
	err = tracker.RecordApply("agent-1", blueprint, "user@example.com", 5*time.Second, nil)
	if err != nil {
		t.Errorf("RecordApply() error = %v", err)
	}

	// Verify state was recorded
	state := tracker.GetAgentState("agent-1")
	if state == nil {
		t.Fatal("GetAgentState() returned nil")
	}

	if len(state.AppliedBlueprints) != 1 {
		t.Errorf("AppliedBlueprints count = %d, want 1", len(state.AppliedBlueprints))
	}

	info := state.AppliedBlueprints["web"]
	if info == nil {
		t.Fatal("AppliedBlueprints[web] is nil")
	}

	if info.Status != "applied" {
		t.Errorf("Status = %s, want applied", info.Status)
	}

	if info.AppliedBy != "user@example.com" {
		t.Errorf("AppliedBy = %s, want user@example.com", info.AppliedBy)
	}
}

func TestTracker_RecordApply_WithError(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	blueprint := &AppliedBlueprintInfo{
		Name:       "myorg/web-stack",
		Version:    "1.0.0",
		Namespace:  "web",
		StateCount: 5,
	}

	// Record failed apply
	applyErr := errors.New("failed to apply state")
	err = tracker.RecordApply("agent-1", blueprint, "user@example.com", 5*time.Second, applyErr)
	if err != nil {
		t.Errorf("RecordApply() error = %v", err)
	}

	// Verify state was recorded with failure
	state := tracker.GetAgentState("agent-1")
	info := state.AppliedBlueprints["web"]
	if info.Status != "failed" {
		t.Errorf("Status = %s, want failed", info.Status)
	}

	if info.LastError != "failed to apply state" {
		t.Errorf("LastError = %s, want 'failed to apply state'", info.LastError)
	}
}

func TestTracker_RecordRemove(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// First apply a blueprint
	blueprint := &AppliedBlueprintInfo{
		Name:       "myorg/web-stack",
		Version:    "1.0.0",
		Namespace:  "web",
		StateCount: 5,
	}
	tracker.RecordApply("agent-1", blueprint, "user", 5*time.Second, nil)

	// Then remove it
	err = tracker.RecordRemove("agent-1", "web", "user", 2*time.Second, nil)
	if err != nil {
		t.Errorf("RecordRemove() error = %v", err)
	}

	// Verify it was removed
	state := tracker.GetAgentState("agent-1")
	if state == nil {
		t.Fatal("GetAgentState() returned nil")
	}

	if _, ok := state.AppliedBlueprints["web"]; ok {
		t.Error("Blueprint should have been removed")
	}
}

func TestTracker_RecordRemove_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Try to remove from non-existent agent
	err = tracker.RecordRemove("nonexistent-agent", "web", "user", 2*time.Second, nil)
	if err != nil {
		t.Errorf("RecordRemove() for non-existent agent should not error, got %v", err)
	}
}

func TestTracker_RecordRollback(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// First apply a blueprint
	blueprint := &AppliedBlueprintInfo{
		Name:       "myorg/web-stack",
		Version:    "2.0.0",
		Namespace:  "web",
		StateCount: 5,
	}
	tracker.RecordApply("agent-1", blueprint, "user", 5*time.Second, nil)

	// Rollback to previous version
	err = tracker.RecordRollback("agent-1", "web", "1.0.0", "user", 3*time.Second, nil)
	if err != nil {
		t.Errorf("RecordRollback() error = %v", err)
	}

	// Verify version was updated
	state := tracker.GetAgentState("agent-1")
	info := state.AppliedBlueprints["web"]
	if info.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", info.Version)
	}
}

func TestTracker_RecordRollback_AgentNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	err = tracker.RecordRollback("nonexistent", "web", "1.0.0", "user", 3*time.Second, nil)
	if err == nil {
		t.Error("RecordRollback() should return error for non-existent agent")
	}
}

func TestTracker_GetAgentState_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	state := tracker.GetAgentState("nonexistent")
	if state != nil {
		t.Error("GetAgentState() should return nil for non-existent agent")
	}
}

func TestTracker_GetAppliedBlueprint(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Apply a blueprint
	blueprint := &AppliedBlueprintInfo{
		Name:       "myorg/web-stack",
		Version:    "1.0.0",
		Namespace:  "web",
		StateCount: 5,
	}
	tracker.RecordApply("agent-1", blueprint, "user", 5*time.Second, nil)

	// Get specific blueprint
	info := tracker.GetAppliedBlueprint("agent-1", "web")
	if info == nil {
		t.Fatal("GetAppliedBlueprint() returned nil")
	}

	if info.Name != "myorg/web-stack" {
		t.Errorf("Name = %s, want myorg/web-stack", info.Name)
	}
}

func TestTracker_GetAppliedBlueprint_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Non-existent agent
	info := tracker.GetAppliedBlueprint("nonexistent", "web")
	if info != nil {
		t.Error("GetAppliedBlueprint() should return nil for non-existent agent")
	}

	// Apply a blueprint
	blueprint := &AppliedBlueprintInfo{
		Name:       "myorg/web-stack",
		Version:    "1.0.0",
		Namespace:  "web",
		StateCount: 5,
	}
	tracker.RecordApply("agent-1", blueprint, "user", 5*time.Second, nil)

	// Non-existent namespace
	info = tracker.GetAppliedBlueprint("agent-1", "nonexistent")
	if info != nil {
		t.Error("GetAppliedBlueprint() should return nil for non-existent namespace")
	}
}

func TestTracker_GetAgentHistory(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Apply multiple blueprints
	bp1 := &AppliedBlueprintInfo{Name: "bp1", Version: "1.0.0", Namespace: "ns1"}
	bp2 := &AppliedBlueprintInfo{Name: "bp2", Version: "1.0.0", Namespace: "ns2"}
	tracker.RecordApply("agent-1", bp1, "user", 1*time.Second, nil)
	tracker.RecordApply("agent-1", bp2, "user", 1*time.Second, nil)

	// Get history
	history := tracker.GetAgentHistory("agent-1", 10)
	if len(history) != 2 {
		t.Errorf("History length = %d, want 2", len(history))
	}

	// Newest should be first
	if history[0].BlueprintName != "bp2" {
		t.Errorf("First history entry = %s, want bp2", history[0].BlueprintName)
	}
}

func TestTracker_GetAgentHistory_Limit(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Apply multiple blueprints
	for i := 0; i < 5; i++ {
		bp := &AppliedBlueprintInfo{Name: "bp", Version: "1.0.0", Namespace: "ns"}
		tracker.RecordApply("agent-1", bp, "user", 1*time.Second, nil)
	}

	// Get limited history
	history := tracker.GetAgentHistory("agent-1", 3)
	if len(history) != 3 {
		t.Errorf("History length = %d, want 3", len(history))
	}
}

func TestTracker_GetAgentHistory_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	history := tracker.GetAgentHistory("nonexistent", 10)
	if history != nil {
		t.Error("GetAgentHistory() should return nil for non-existent agent")
	}
}

func TestTracker_GetAllAgents(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Initially empty
	agents := tracker.GetAllAgents()
	if len(agents) != 0 {
		t.Errorf("GetAllAgents() initially = %d, want 0", len(agents))
	}

	// Add some blueprints
	bp := &AppliedBlueprintInfo{Name: "bp", Version: "1.0.0", Namespace: "ns"}
	tracker.RecordApply("agent-1", bp, "user", 1*time.Second, nil)
	tracker.RecordApply("agent-2", bp, "user", 1*time.Second, nil)

	agents = tracker.GetAllAgents()
	if len(agents) != 2 {
		t.Errorf("GetAllAgents() = %d, want 2", len(agents))
	}
}

func TestTracker_GetBlueprintUsage(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Apply same blueprint to multiple agents
	bp := &AppliedBlueprintInfo{Name: "myorg/web-stack", Version: "1.0.0", Namespace: "web"}
	tracker.RecordApply("agent-1", bp, "user", 1*time.Second, nil)
	tracker.RecordApply("agent-2", bp, "user", 1*time.Second, nil)

	// Apply different blueprint to one agent
	bp2 := &AppliedBlueprintInfo{Name: "myorg/db-stack", Version: "1.0.0", Namespace: "db"}
	tracker.RecordApply("agent-1", bp2, "user", 1*time.Second, nil)

	// Get usage for web-stack
	usage := tracker.GetBlueprintUsage("myorg/web-stack")
	if len(usage) != 2 {
		t.Errorf("GetBlueprintUsage(web-stack) = %d, want 2", len(usage))
	}

	// Get usage for db-stack
	usage = tracker.GetBlueprintUsage("myorg/db-stack")
	if len(usage) != 1 {
		t.Errorf("GetBlueprintUsage(db-stack) = %d, want 1", len(usage))
	}

	// Get usage for non-existent blueprint
	usage = tracker.GetBlueprintUsage("nonexistent")
	if len(usage) != 0 {
		t.Errorf("GetBlueprintUsage(nonexistent) = %d, want 0", len(usage))
	}
}

func TestTracker_FindRollbackTarget(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Apply multiple versions
	bp1 := &AppliedBlueprintInfo{Name: "bp", Version: "1.0.0", Namespace: "ns"}
	bp2 := &AppliedBlueprintInfo{Name: "bp", Version: "2.0.0", Namespace: "ns"}
	bp3 := &AppliedBlueprintInfo{Name: "bp", Version: "3.0.0", Namespace: "ns"}

	tracker.RecordApply("agent-1", bp1, "user", 1*time.Second, nil)
	tracker.RecordApply("agent-1", bp2, "user", 1*time.Second, nil)
	tracker.RecordApply("agent-1", bp3, "user", 1*time.Second, nil)

	// Find rollback target (should be 2.0.0)
	target, err := tracker.FindRollbackTarget("agent-1", "ns")
	if err != nil {
		t.Fatalf("FindRollbackTarget() error = %v", err)
	}

	if target != "2.0.0" {
		t.Errorf("FindRollbackTarget() = %s, want 2.0.0", target)
	}
}

func TestTracker_FindRollbackTarget_NoHistory(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	_, err = tracker.FindRollbackTarget("nonexistent", "ns")
	if err == nil {
		t.Error("FindRollbackTarget() should return error for non-existent agent")
	}
}

func TestTracker_ClearAgent(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Apply a blueprint
	bp := &AppliedBlueprintInfo{Name: "bp", Version: "1.0.0", Namespace: "ns"}
	tracker.RecordApply("agent-1", bp, "user", 1*time.Second, nil)

	// Clear agent
	tracker.ClearAgent("agent-1")

	// Verify cleared
	state := tracker.GetAgentState("agent-1")
	if state != nil {
		t.Error("GetAgentState() should return nil after ClearAgent()")
	}

	history := tracker.GetAgentHistory("agent-1", 10)
	if history != nil {
		t.Error("GetAgentHistory() should return nil after ClearAgent()")
	}
}

func TestTracker_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "tracker.json")

	// Create tracker and add data
	tracker1, err := NewTracker(&TrackerConfig{
		StorePath:          storePath,
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	bp := &AppliedBlueprintInfo{Name: "myorg/web-stack", Version: "1.0.0", Namespace: "web"}
	tracker1.RecordApply("agent-1", bp, "user", 1*time.Second, nil)

	// Save
	err = tracker1.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Fatal("Save() did not create file")
	}

	// Create new tracker and load
	tracker2, err := NewTracker(&TrackerConfig{
		StorePath:          storePath,
		MaxHistoryPerAgent: 10,
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Verify data was loaded
	state := tracker2.GetAgentState("agent-1")
	if state == nil {
		t.Fatal("GetAgentState() returned nil after load")
	}

	info := state.AppliedBlueprints["web"]
	if info == nil {
		t.Fatal("Blueprint not found after load")
	}

	if info.Name != "myorg/web-stack" {
		t.Errorf("Name = %s, want myorg/web-stack", info.Name)
	}
}

func TestTracker_HistoryTrimming(t *testing.T) {
	tmpDir := t.TempDir()
	tracker, err := NewTracker(&TrackerConfig{
		StorePath:          filepath.Join(tmpDir, "tracker.json"),
		MaxHistoryPerAgent: 3, // Only keep 3 entries
		PersistOnChange:    false,
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Apply more than max history
	for i := 0; i < 5; i++ {
		bp := &AppliedBlueprintInfo{Name: "bp", Version: "1.0.0", Namespace: "ns"}
		tracker.RecordApply("agent-1", bp, "user", 1*time.Second, nil)
	}

	// Get history
	history := tracker.GetAgentHistory("agent-1", 100)
	if len(history) != 3 {
		t.Errorf("History length = %d, want 3 (max)", len(history))
	}
}

func TestErrToString(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{errors.New("test error"), "test error"},
	}

	for _, tt := range tests {
		got := errToString(tt.err)
		if got != tt.want {
			t.Errorf("errToString(%v) = %s, want %s", tt.err, got, tt.want)
		}
	}
}
