// Package blueprint provides state snapshot capture for rollback support.
package blueprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// SnapshotConfig configures the snapshot system.
type SnapshotConfig struct {
	// StorePath is where to store snapshots.
	StorePath string

	// MaxSnapshotsPerBlueprint is the maximum snapshots to keep per blueprint.
	MaxSnapshotsPerBlueprint int

	// MaxTotalSnapshots is the maximum total snapshots to keep.
	MaxTotalSnapshots int

	// CompressSnapshots enables compression for snapshot data.
	CompressSnapshots bool
}

// DefaultSnapshotConfig returns default snapshot configuration.
func DefaultSnapshotConfig() *SnapshotConfig {
	return &SnapshotConfig{
		StorePath:                "/var/lib/kscore/snapshots",
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
		CompressSnapshots:        false,
	}
}

// Snapshot represents a point-in-time capture of state before blueprint apply.
type Snapshot struct {
	// ID is the unique snapshot identifier.
	ID string `json:"id"`

	// AgentID is the agent this snapshot is for.
	AgentID string `json:"agent_id"`

	// BlueprintName is the blueprint being applied.
	BlueprintName string `json:"blueprint_name"`

	// BlueprintVersion is the version being applied.
	BlueprintVersion string `json:"blueprint_version"`

	// Namespace is the blueprint namespace.
	Namespace string `json:"namespace"`

	// CreatedAt is when the snapshot was created.
	CreatedAt time.Time `json:"created_at"`

	// PreviousVersion is the previously applied version (if any).
	PreviousVersion string `json:"previous_version,omitempty"`

	// StateCapture contains captured state data.
	StateCapture *StateCapture `json:"state_capture"`

	// Checksum is SHA256 of the snapshot data for integrity.
	Checksum string `json:"checksum"`

	// Size is the snapshot size in bytes.
	Size int64 `json:"size"`

	// Metadata contains additional snapshot metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// StateCapture captures the state of resources before modification.
type StateCapture struct {
	// Files captures file states.
	Files []FileCaptureEntry `json:"files,omitempty"`

	// Packages captures package states.
	Packages []PackageCaptureEntry `json:"packages,omitempty"`

	// Services captures service states.
	Services []ServiceCaptureEntry `json:"services,omitempty"`

	// Users captures user states.
	Users []UserCaptureEntry `json:"users,omitempty"`

	// Groups captures group states.
	Groups []GroupCaptureEntry `json:"groups,omitempty"`

	// Custom captures custom state data.
	Custom map[string]interface{} `json:"custom,omitempty"`
}

// FileCaptureEntry captures a file's state.
type FileCaptureEntry struct {
	Path        string      `json:"path"`
	Exists      bool        `json:"exists"`
	Mode        os.FileMode `json:"mode,omitempty"`
	Owner       string      `json:"owner,omitempty"`
	Group       string      `json:"group,omitempty"`
	Size        int64       `json:"size,omitempty"`
	Checksum    string      `json:"checksum,omitempty"`
	ContentHash string      `json:"content_hash,omitempty"`
	IsDir       bool        `json:"is_dir,omitempty"`
	IsSymlink   bool        `json:"is_symlink,omitempty"`
	LinkTarget  string      `json:"link_target,omitempty"`
	ModTime     time.Time   `json:"mod_time,omitempty"`
}

// PackageCaptureEntry captures a package's state.
type PackageCaptureEntry struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
}

// ServiceCaptureEntry captures a service's state.
type ServiceCaptureEntry struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Enabled bool   `json:"enabled"`
}

// UserCaptureEntry captures a user's state.
type UserCaptureEntry struct {
	Name   string   `json:"name"`
	Exists bool     `json:"exists"`
	UID    int      `json:"uid,omitempty"`
	GID    int      `json:"gid,omitempty"`
	Home   string   `json:"home,omitempty"`
	Shell  string   `json:"shell,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

// GroupCaptureEntry captures a group's state.
type GroupCaptureEntry struct {
	Name    string   `json:"name"`
	Exists  bool     `json:"exists"`
	GID     int      `json:"gid,omitempty"`
	Members []string `json:"members,omitempty"`
}

// SnapshotManager manages state snapshots.
type SnapshotManager struct {
	config *SnapshotConfig
	mu     sync.RWMutex
}

// NewSnapshotManager creates a new snapshot manager.
func NewSnapshotManager(config *SnapshotConfig) (*SnapshotManager, error) {
	if config == nil {
		config = DefaultSnapshotConfig()
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(config.StorePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	return &SnapshotManager{
		config: config,
	}, nil
}

// CreateSnapshot creates a new snapshot before blueprint application.
func (m *SnapshotManager) CreateSnapshot(agentID, blueprintName, blueprintVersion, namespace string, capture *StateCapture) (*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate snapshot ID
	id := generateSnapshotID(agentID, blueprintName, namespace)

	snapshot := &Snapshot{
		ID:               id,
		AgentID:          agentID,
		BlueprintName:    blueprintName,
		BlueprintVersion: blueprintVersion,
		Namespace:        namespace,
		CreatedAt:        time.Now(),
		StateCapture:     capture,
		Metadata:         make(map[string]string),
	}

	// Serialize snapshot
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize snapshot: %w", err)
	}

	// Calculate checksum
	hash := sha256.Sum256(data)
	snapshot.Checksum = hex.EncodeToString(hash[:])
	snapshot.Size = int64(len(data))

	// Re-serialize with checksum
	data, err = json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize snapshot: %w", err)
	}

	// Save snapshot
	path := m.snapshotPath(id)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write snapshot: %w", err)
	}

	// Enforce limits
	if err := m.enforceSnapshotLimits(blueprintName, namespace); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: failed to enforce snapshot limits: %v\n", err)
	}

	return snapshot, nil
}

// GetSnapshot retrieves a snapshot by ID.
func (m *SnapshotManager) GetSnapshot(id string) (*Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	path := m.snapshotPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read snapshot: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot: %w", err)
	}

	return &snapshot, nil
}

// GetLatestSnapshot retrieves the latest snapshot for a blueprint/namespace.
func (m *SnapshotManager) GetLatestSnapshot(agentID, blueprintName, namespace string) (*Snapshot, error) {
	snapshots, err := m.ListSnapshots(agentID, blueprintName, namespace)
	if err != nil {
		return nil, err
	}

	if len(snapshots) == 0 {
		return nil, errors.New("no snapshots found")
	}

	return snapshots[0], nil
}

// ListSnapshots lists snapshots matching the criteria.
func (m *SnapshotManager) ListSnapshots(agentID, blueprintName, namespace string) ([]*Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.config.StorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	var snapshots []*Snapshot
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(m.config.StorePath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var snapshot Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}

		// Filter by criteria
		if agentID != "" && snapshot.AgentID != agentID {
			continue
		}
		if blueprintName != "" && snapshot.BlueprintName != blueprintName {
			continue
		}
		if namespace != "" && snapshot.Namespace != namespace {
			continue
		}

		snapshots = append(snapshots, &snapshot)
	}

	// Sort by creation time, newest first
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	return snapshots, nil
}

// DeleteSnapshot deletes a snapshot by ID.
func (m *SnapshotManager) DeleteSnapshot(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.snapshotPath(id)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	return nil
}

// VerifySnapshot verifies snapshot integrity.
func (m *SnapshotManager) VerifySnapshot(id string) error {
	snapshot, err := m.GetSnapshot(id)
	if err != nil {
		return err
	}

	// Temporarily clear checksum and recalculate
	savedChecksum := snapshot.Checksum
	snapshot.Checksum = ""

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to serialize for verification: %w", err)
	}

	hash := sha256.Sum256(data)
	calculated := hex.EncodeToString(hash[:])

	if calculated != savedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", savedChecksum, calculated)
	}

	return nil
}

// CleanupOldSnapshots removes snapshots older than the given duration.
func (m *SnapshotManager) CleanupOldSnapshots(maxAge time.Duration) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.config.StorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list snapshots: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	deleted := 0

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(m.config.StorePath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var snapshot Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}

		if snapshot.CreatedAt.Before(cutoff) {
			if err := os.Remove(path); err == nil {
				deleted++
			}
		}
	}

	return deleted, nil
}

// snapshotPath returns the path for a snapshot file.
func (m *SnapshotManager) snapshotPath(id string) string {
	return filepath.Join(m.config.StorePath, id+".json")
}

// enforceSnapshotLimits enforces per-blueprint and total snapshot limits.
func (m *SnapshotManager) enforceSnapshotLimits(blueprintName, namespace string) error {
	// Get all snapshots for this blueprint/namespace
	snapshots, err := m.listSnapshotsUnsafe("", blueprintName, namespace)
	if err != nil {
		return err
	}

	// Enforce per-blueprint limit
	if len(snapshots) > m.config.MaxSnapshotsPerBlueprint {
		// Delete oldest snapshots
		toDelete := snapshots[m.config.MaxSnapshotsPerBlueprint:]
		for _, s := range toDelete {
			path := m.snapshotPath(s.ID)
			os.Remove(path)
		}
	}

	// Enforce total limit
	allSnapshots, err := m.listSnapshotsUnsafe("", "", "")
	if err != nil {
		return err
	}

	if len(allSnapshots) > m.config.MaxTotalSnapshots {
		// Delete oldest snapshots
		toDelete := allSnapshots[m.config.MaxTotalSnapshots:]
		for _, s := range toDelete {
			path := m.snapshotPath(s.ID)
			os.Remove(path)
		}
	}

	return nil
}

// listSnapshotsUnsafe lists snapshots without locking (caller must hold lock).
func (m *SnapshotManager) listSnapshotsUnsafe(agentID, blueprintName, namespace string) ([]*Snapshot, error) {
	entries, err := os.ReadDir(m.config.StorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var snapshots []*Snapshot
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(m.config.StorePath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var snapshot Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}

		// Filter by criteria
		if agentID != "" && snapshot.AgentID != agentID {
			continue
		}
		if blueprintName != "" && snapshot.BlueprintName != blueprintName {
			continue
		}
		if namespace != "" && snapshot.Namespace != namespace {
			continue
		}

		snapshots = append(snapshots, &snapshot)
	}

	// Sort by creation time, newest first
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	return snapshots, nil
}

// generateSnapshotID generates a unique snapshot ID.
func generateSnapshotID(agentID, blueprintName, namespace string) string {
	timestamp := time.Now().UnixNano()
	data := fmt.Sprintf("%s:%s:%s:%d", agentID, blueprintName, namespace, timestamp)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])[:16]
}

// NewStateCapture creates a new empty state capture.
func NewStateCapture() *StateCapture {
	return &StateCapture{
		Custom: make(map[string]interface{}),
	}
}

// AddFile adds a file capture entry.
func (c *StateCapture) AddFile(entry FileCaptureEntry) {
	c.Files = append(c.Files, entry)
}

// AddPackage adds a package capture entry.
func (c *StateCapture) AddPackage(entry PackageCaptureEntry) {
	c.Packages = append(c.Packages, entry)
}

// AddService adds a service capture entry.
func (c *StateCapture) AddService(entry ServiceCaptureEntry) {
	c.Services = append(c.Services, entry)
}

// AddUser adds a user capture entry.
func (c *StateCapture) AddUser(entry UserCaptureEntry) {
	c.Users = append(c.Users, entry)
}

// AddGroup adds a group capture entry.
func (c *StateCapture) AddGroup(entry GroupCaptureEntry) {
	c.Groups = append(c.Groups, entry)
}

// SetCustom sets custom state data.
func (c *StateCapture) SetCustom(key string, value interface{}) {
	c.Custom[key] = value
}
