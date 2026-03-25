// Package blueprint provides the agent blueprint tracker for tracking
// which blueprints have been applied to which agents.
package blueprint

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrackerConfig configures the blueprint tracker.
type TrackerConfig struct {
	// StorePath is where to store tracking data.
	StorePath string

	// MaxHistoryPerAgent is the maximum number of history entries per agent.
	MaxHistoryPerAgent int

	// PersistOnChange if true, persists to disk on every change.
	PersistOnChange bool
}

// DefaultTrackerConfig returns default tracker configuration.
func DefaultTrackerConfig() *TrackerConfig {
	return &TrackerConfig{
		StorePath:          "/var/lib/keystone-core/blueprint-tracker.json",
		MaxHistoryPerAgent: 100,
		PersistOnChange:    true,
	}
}

// AgentBlueprintState represents the current state of blueprints on an agent.
type AgentBlueprintState struct {
	// AgentID is the agent identifier.
	AgentID string `json:"agent_id"`

	// AppliedBlueprints maps namespace to applied blueprint info.
	AppliedBlueprints map[string]*AppliedBlueprintInfo `json:"applied_blueprints"`

	// LastUpdated is when this state was last updated.
	LastUpdated time.Time `json:"last_updated"`
}

// AppliedBlueprintInfo contains detailed information about an applied blueprint.
type AppliedBlueprintInfo struct {
	// Name is the blueprint name (vendor/name).
	Name string `json:"name"`

	// Version is the resolved version.
	Version string `json:"version"`

	// Namespace is the "as" namespace used.
	Namespace string `json:"namespace"`

	// Parameters are the resolved parameters.
	Parameters map[string]interface{} `json:"parameters,omitempty"`

	// EnabledFeatures lists enabled features.
	EnabledFeatures []string `json:"enabled_features,omitempty"`

	// AppliedAt is when this blueprint was applied.
	AppliedAt time.Time `json:"applied_at"`

	// AppliedBy is who/what triggered the apply (user, automation, etc).
	AppliedBy string `json:"applied_by,omitempty"`

	// StateCount is the number of states in this blueprint.
	StateCount int `json:"state_count"`

	// SuccessfulStates is the number of states that succeeded.
	SuccessfulStates int `json:"successful_states"`

	// FailedStates is the number of states that failed.
	FailedStates int `json:"failed_states"`

	// Status is the current status (applied, failed, removed).
	Status string `json:"status"`

	// LastError is the last error if any.
	LastError string `json:"last_error,omitempty"`

	// Checksum is a hash of the blueprint content for change detection.
	Checksum string `json:"checksum,omitempty"`
}

// HistoryEntry represents a single history entry.
type HistoryEntry struct {
	// Timestamp is when this action occurred.
	Timestamp time.Time `json:"timestamp"`

	// Action is what happened (applied, updated, removed, rolled_back).
	Action string `json:"action"`

	// BlueprintName is the blueprint name.
	BlueprintName string `json:"blueprint_name"`

	// FromVersion is the previous version (for updates/rollbacks).
	FromVersion string `json:"from_version,omitempty"`

	// ToVersion is the new version.
	ToVersion string `json:"to_version,omitempty"`

	// Namespace is the blueprint namespace.
	Namespace string `json:"namespace"`

	// TriggeredBy is who/what triggered this action.
	TriggeredBy string `json:"triggered_by,omitempty"`

	// Success indicates if the action succeeded.
	Success bool `json:"success"`

	// ErrorMessage is the error if it failed.
	ErrorMessage string `json:"error_message,omitempty"`

	// Duration is how long the action took.
	Duration time.Duration `json:"duration"`
}

// AgentHistory contains the history for an agent.
type AgentHistory struct {
	// AgentID is the agent identifier.
	AgentID string `json:"agent_id"`

	// Entries are the history entries, newest first.
	Entries []HistoryEntry `json:"entries"`
}

// TrackerData is the full tracker data structure for persistence.
type TrackerData struct {
	// Version is the data format version.
	Version int `json:"version"`

	// UpdatedAt is when this data was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// Agents maps agent ID to their blueprint state.
	Agents map[string]*AgentBlueprintState `json:"agents"`

	// History maps agent ID to their history.
	History map[string]*AgentHistory `json:"history"`
}

// Tracker tracks blueprint applications across agents.
type Tracker struct {
	config *TrackerConfig
	mu     sync.RWMutex
	data   *TrackerData
}

// NewTracker creates a new blueprint tracker.
func NewTracker(config *TrackerConfig) (*Tracker, error) {
	if config == nil {
		config = DefaultTrackerConfig()
	}

	t := &Tracker{
		config: config,
		data: &TrackerData{
			Version: 1,
			Agents:  make(map[string]*AgentBlueprintState),
			History: make(map[string]*AgentHistory),
		},
	}

	// Try to load existing data
	if err := t.Load(); err != nil && !os.IsNotExist(err) {
		// Log warning but continue - we'll start fresh
		slog.Warn("failed to load tracker data", "error", err)
	}

	return t, nil
}

// RecordApply records that a blueprint was applied to an agent.
func (t *Tracker) RecordApply(agentID string, blueprint *AppliedBlueprintInfo, triggeredBy string, duration time.Duration, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Get or create agent state
	state, ok := t.data.Agents[agentID]
	if !ok {
		state = &AgentBlueprintState{
			AgentID:           agentID,
			AppliedBlueprints: make(map[string]*AppliedBlueprintInfo),
		}
		t.data.Agents[agentID] = state
	}

	// Get previous version for history
	var fromVersion string
	if existing, ok := state.AppliedBlueprints[blueprint.Namespace]; ok {
		fromVersion = existing.Version
	}

	// Update applied blueprint info
	blueprint.AppliedAt = time.Now()
	blueprint.AppliedBy = triggeredBy
	if err != nil {
		blueprint.Status = "failed"
		blueprint.LastError = err.Error()
	} else {
		blueprint.Status = "applied"
		blueprint.LastError = ""
	}
	state.AppliedBlueprints[blueprint.Namespace] = blueprint
	state.LastUpdated = time.Now()

	// Record history
	t.addHistoryEntry(agentID, HistoryEntry{
		Timestamp:     time.Now(),
		Action:        "applied",
		BlueprintName: blueprint.Name,
		FromVersion:   fromVersion,
		ToVersion:     blueprint.Version,
		Namespace:     blueprint.Namespace,
		TriggeredBy:   triggeredBy,
		Success:       err == nil,
		ErrorMessage:  errToString(err),
		Duration:      duration,
	})

	// Persist if configured
	if t.config.PersistOnChange {
		return t.save()
	}

	return nil
}

// RecordRemove records that a blueprint was removed from an agent.
func (t *Tracker) RecordRemove(agentID, namespace, triggeredBy string, duration time.Duration, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.data.Agents[agentID]
	if !ok {
		return nil // Nothing to remove
	}

	existing, ok := state.AppliedBlueprints[namespace]
	if !ok {
		return nil // Blueprint wasn't applied
	}

	// Record history before removing
	t.addHistoryEntry(agentID, HistoryEntry{
		Timestamp:     time.Now(),
		Action:        "removed",
		BlueprintName: existing.Name,
		FromVersion:   existing.Version,
		Namespace:     namespace,
		TriggeredBy:   triggeredBy,
		Success:       err == nil,
		ErrorMessage:  errToString(err),
		Duration:      duration,
	})

	// Remove from applied
	delete(state.AppliedBlueprints, namespace)
	state.LastUpdated = time.Now()

	// Persist if configured
	if t.config.PersistOnChange {
		return t.save()
	}

	return nil
}

// RecordRollback records a rollback operation.
func (t *Tracker) RecordRollback(agentID, namespace, toVersion, triggeredBy string, duration time.Duration, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.data.Agents[agentID]
	if !ok {
		return errors.New("agent not found")
	}

	existing, ok := state.AppliedBlueprints[namespace]
	if !ok {
		return errors.New("blueprint not found")
	}

	fromVersion := existing.Version

	// Update version if successful
	if err == nil {
		existing.Version = toVersion
		existing.AppliedAt = time.Now()
		existing.AppliedBy = triggeredBy
		existing.Status = "applied"
		existing.LastError = ""
	}
	state.LastUpdated = time.Now()

	// Record history
	t.addHistoryEntry(agentID, HistoryEntry{
		Timestamp:     time.Now(),
		Action:        "rolled_back",
		BlueprintName: existing.Name,
		FromVersion:   fromVersion,
		ToVersion:     toVersion,
		Namespace:     namespace,
		TriggeredBy:   triggeredBy,
		Success:       err == nil,
		ErrorMessage:  errToString(err),
		Duration:      duration,
	})

	// Persist if configured
	if t.config.PersistOnChange {
		return t.save()
	}

	return nil
}

// GetAgentState returns the blueprint state for an agent.
func (t *Tracker) GetAgentState(agentID string) *AgentBlueprintState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, ok := t.data.Agents[agentID]
	if !ok {
		return nil
	}

	// Return a copy
	stateCopy := *state
	stateCopy.AppliedBlueprints = make(map[string]*AppliedBlueprintInfo)
	for k, v := range state.AppliedBlueprints {
		infoCopy := *v
		stateCopy.AppliedBlueprints[k] = &infoCopy
	}

	return &stateCopy
}

// GetAppliedBlueprint returns info about a specific applied blueprint.
func (t *Tracker) GetAppliedBlueprint(agentID, namespace string) *AppliedBlueprintInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, ok := t.data.Agents[agentID]
	if !ok {
		return nil
	}

	info, ok := state.AppliedBlueprints[namespace]
	if !ok {
		return nil
	}

	// Return a copy
	infoCopy := *info
	return &infoCopy
}

// GetAgentHistory returns the history for an agent.
func (t *Tracker) GetAgentHistory(agentID string, limit int) []HistoryEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history, ok := t.data.History[agentID]
	if !ok {
		return nil
	}

	if limit <= 0 || limit > len(history.Entries) {
		limit = len(history.Entries)
	}

	// Return copies
	result := make([]HistoryEntry, limit)
	copy(result, history.Entries[:limit])
	return result
}

// GetAllAgents returns all tracked agents.
func (t *Tracker) GetAllAgents() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	agents := make([]string, 0, len(t.data.Agents))
	for agentID := range t.data.Agents {
		agents = append(agents, agentID)
	}
	return agents
}

// GetBlueprintUsage returns which agents are using a specific blueprint.
func (t *Tracker) GetBlueprintUsage(blueprintName string) map[string]*AppliedBlueprintInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*AppliedBlueprintInfo)
	for agentID, state := range t.data.Agents {
		for _, info := range state.AppliedBlueprints {
			if info.Name == blueprintName {
				infoCopy := *info
				result[agentID] = &infoCopy
			}
		}
	}
	return result
}

// FindRollbackTarget finds the previous version of a blueprint on an agent.
func (t *Tracker) FindRollbackTarget(agentID, namespace string) (string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history, ok := t.data.History[agentID]
	if !ok {
		return "", errors.New("no history found for agent")
	}

	// Find the most recent successful apply before the current one
	foundCurrent := false
	for i := range history.Entries {
		entry := &history.Entries[i]
		if entry.Namespace != namespace {
			continue
		}

		if entry.Action == "applied" && entry.Success {
			if foundCurrent && entry.ToVersion != "" {
				return entry.ToVersion, nil
			}
			foundCurrent = true
		}
	}

	return "", errors.New("no previous version found")
}

// Load loads tracker data from disk.
func (t *Tracker) Load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := os.ReadFile(t.config.StorePath)
	if err != nil {
		return err
	}

	var loadedData TrackerData
	if err := json.Unmarshal(data, &loadedData); err != nil {
		return fmt.Errorf("failed to parse tracker data: %w", err)
	}

	t.data = &loadedData
	if t.data.Agents == nil {
		t.data.Agents = make(map[string]*AgentBlueprintState)
	}
	if t.data.History == nil {
		t.data.History = make(map[string]*AgentHistory)
	}

	return nil
}

// Save persists tracker data to disk.
func (t *Tracker) Save() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.save()
}

// save is the internal save method (caller must hold lock).
func (t *Tracker) save() error {
	t.data.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(t.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tracker data: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(t.config.StorePath)
	//nolint:gosec // G301: tracker directory needs to be accessible by service user
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write atomically
	tmpPath := t.config.StorePath + ".tmp"
	//nolint:gosec // G306: tracker data needs to be readable for status queries
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write tracker data: %w", err)
	}

	if err := os.Rename(tmpPath, t.config.StorePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename tracker file: %w", err)
	}

	return nil
}

// ClearAgent clears all tracking data for an agent.
func (t *Tracker) ClearAgent(agentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.data.Agents, agentID)
	delete(t.data.History, agentID)
}

// addHistoryEntry adds a history entry (caller must hold lock).
func (t *Tracker) addHistoryEntry(agentID string, entry HistoryEntry) {
	history, ok := t.data.History[agentID]
	if !ok {
		history = &AgentHistory{
			AgentID: agentID,
		}
		t.data.History[agentID] = history
	}

	// Prepend to entries (newest first)
	history.Entries = append([]HistoryEntry{entry}, history.Entries...)

	// Trim if over limit
	if len(history.Entries) > t.config.MaxHistoryPerAgent {
		history.Entries = history.Entries[:t.config.MaxHistoryPerAgent]
	}
}

// errToString safely converts an error to string.
func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
