// Package conflict provides conflict resolution strategies for distributed data.
package conflict

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// Errors returned by the conflict resolver.
var (
	ErrNoResolver    = errors.New("no resolver for conflict type")
	ErrUnresolvable  = errors.New("conflict cannot be resolved automatically")
	ErrMergeConflict = errors.New("merge conflict detected")
)

// Strategy defines how conflicts should be resolved.
type Strategy string

const (
	// StrategyLastWriteWins uses the most recent write.
	StrategyLastWriteWins Strategy = "last_write_wins"
	// StrategyFirstWriteWins uses the first write.
	StrategyFirstWriteWins Strategy = "first_write_wins"
	// StrategyHighestVersion uses the highest version number.
	StrategyHighestVersion Strategy = "highest_version"
	// StrategyMerge attempts to merge changes.
	StrategyMerge Strategy = "merge"
	// StrategyCustom uses a custom resolver.
	StrategyCustom Strategy = "custom"
	// StrategyManual requires manual resolution.
	StrategyManual Strategy = "manual"
)

// Version represents a version vector for conflict detection.
type Version struct {
	// Clock is the logical clock value.
	Clock uint64 `json:"clock"`
	// NodeID is the node that made the change.
	NodeID string `json:"node_id"`
	// Timestamp is when the change occurred.
	Timestamp time.Time `json:"timestamp"`
	// Vector is the version vector for causal ordering.
	Vector map[string]uint64 `json:"vector,omitempty"`
}

// Compare compares two versions.
// Returns -1 if v < other, 0 if concurrent, 1 if v > other.
func (v *Version) Compare(other *Version) int {
	if v.Vector != nil && other.Vector != nil {
		return compareVectors(v.Vector, other.Vector)
	}
	// Fall back to clock comparison
	if v.Clock < other.Clock {
		return -1
	}
	if v.Clock > other.Clock {
		return 1
	}
	// Same clock, use timestamp
	if v.Timestamp.Before(other.Timestamp) {
		return -1
	}
	if v.Timestamp.After(other.Timestamp) {
		return 1
	}
	return 0
}

func compareVectors(a, b map[string]uint64) int {
	aLess := false
	bLess := false

	// Check all keys from a
	for k, av := range a {
		bv := b[k]
		if av < bv {
			aLess = true
		} else if av > bv {
			bLess = true
		}
	}

	// Check keys only in b
	for k, bv := range b {
		if _, exists := a[k]; !exists {
			if bv > 0 {
				aLess = true
			}
		}
	}

	if aLess && !bLess {
		return -1
	}
	if bLess && !aLess {
		return 1
	}
	return 0 // Concurrent
}

// Document represents a versioned document.
type Document struct {
	// ID is the document identifier.
	ID string `json:"id"`
	// Version is the document version.
	Version *Version `json:"version"`
	// Data is the document content.
	Data []byte `json:"data"`
	// Metadata contains additional metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
	// Hash is the content hash.
	Hash string `json:"hash,omitempty"`
}

// ComputeHash computes the content hash.
func (d *Document) ComputeHash() string {
	h := sha256.Sum256(d.Data)
	return string(h[:])
}

// Conflict represents a detected conflict.
type Conflict struct {
	// ID is the conflict identifier.
	ID string `json:"id"`
	// DocumentID is the conflicting document ID.
	DocumentID string `json:"document_id"`
	// Local is the local version.
	Local *Document `json:"local"`
	// Remote is the remote version.
	Remote *Document `json:"remote"`
	// Base is the common ancestor, if known.
	Base *Document `json:"base,omitempty"`
	// DetectedAt is when the conflict was detected.
	DetectedAt time.Time `json:"detected_at"`
	// Resolved indicates if the conflict is resolved.
	Resolved bool `json:"resolved"`
	// Resolution is the resolved document.
	Resolution *Document `json:"resolution,omitempty"`
	// Strategy is the strategy used to resolve.
	Strategy Strategy `json:"strategy,omitempty"`
}

// Resolution represents a conflict resolution.
type Resolution struct {
	// Document is the resolved document.
	Document *Document
	// Strategy is the strategy that was used.
	Strategy Strategy
	// Manual indicates if manual resolution was required.
	Manual bool
	// Metadata contains resolution metadata.
	Metadata map[string]string
}

// Resolver is a function that resolves conflicts.
type Resolver func(conflict *Conflict) (*Resolution, error)

// Config holds resolver configuration.
type Config struct {
	// DefaultStrategy is the default resolution strategy.
	DefaultStrategy Strategy
	// TypeStrategies maps document types to strategies.
	TypeStrategies map[string]Strategy
	// MergeEnabled enables merge resolution.
	MergeEnabled bool
	// KeepHistory keeps conflict history.
	KeepHistory bool
	// MaxHistorySize is the maximum history entries.
	MaxHistorySize int
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		DefaultStrategy: StrategyLastWriteWins,
		TypeStrategies:  make(map[string]Strategy),
		MergeEnabled:    true,
		KeepHistory:     true,
		MaxHistorySize:  1000,
	}
}

// Manager manages conflict detection and resolution.
type Manager struct {
	config    *Config
	resolvers map[Strategy]Resolver
	conflicts map[string]*Conflict
	history   []*Conflict
	listeners []Listener
	mu        sync.RWMutex
}

// Listener receives conflict events.
type Listener func(event *Event)

// Event represents a conflict-related event.
type Event struct {
	Type       string
	Conflict   *Conflict
	Resolution *Resolution
	Timestamp  time.Time
}

// NewManager creates a new conflict manager.
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	m := &Manager{
		config:    config,
		resolvers: make(map[Strategy]Resolver),
		conflicts: make(map[string]*Conflict),
	}

	// Register built-in resolvers
	m.RegisterResolver(StrategyLastWriteWins, lastWriteWinsResolver)
	m.RegisterResolver(StrategyFirstWriteWins, firstWriteWinsResolver)
	m.RegisterResolver(StrategyHighestVersion, highestVersionResolver)
	m.RegisterResolver(StrategyMerge, m.mergeResolver)

	return m
}

// RegisterResolver registers a custom resolver.
func (m *Manager) RegisterResolver(strategy Strategy, resolver Resolver) {
	m.mu.Lock()
	m.resolvers[strategy] = resolver
	m.mu.Unlock()
}

// Detect detects if there's a conflict between local and remote documents.
func (m *Manager) Detect(local, remote *Document) (*Conflict, error) {
	// Same content - no conflict
	if bytes.Equal(local.Data, remote.Data) {
		return nil, nil
	}

	// Check version relationship
	cmp := local.Version.Compare(remote.Version)
	if cmp == -1 {
		// Local is older - no conflict, just update
		return nil, nil
	}
	if cmp == 1 {
		// Local is newer - no conflict, local wins
		return nil, nil
	}

	// Concurrent modifications - conflict!
	conflict := &Conflict{
		ID:         generateID(),
		DocumentID: local.ID,
		Local:      local,
		Remote:     remote,
		DetectedAt: time.Now(),
	}

	m.mu.Lock()
	m.conflicts[conflict.ID] = conflict
	m.mu.Unlock()

	m.emitEvent(&Event{
		Type:      "conflict_detected",
		Conflict:  conflict,
		Timestamp: time.Now(),
	})

	return conflict, nil
}

// Resolve resolves a conflict using the configured strategy.
func (m *Manager) Resolve(conflict *Conflict) (*Resolution, error) {
	// Determine strategy
	strategy := m.config.DefaultStrategy
	if docType, ok := conflict.Local.Metadata["type"]; ok {
		if s, exists := m.config.TypeStrategies[docType]; exists {
			strategy = s
		}
	}

	return m.ResolveWithStrategy(conflict, strategy)
}

// ResolveWithStrategy resolves a conflict with a specific strategy.
func (m *Manager) ResolveWithStrategy(conflict *Conflict, strategy Strategy) (*Resolution, error) {
	m.mu.RLock()
	resolver, exists := m.resolvers[strategy]
	m.mu.RUnlock()

	if !exists {
		if strategy == StrategyManual {
			return nil, ErrUnresolvable
		}
		return nil, ErrNoResolver
	}

	resolution, err := resolver(conflict)
	if err != nil {
		return nil, err
	}

	resolution.Strategy = strategy

	// Mark conflict as resolved
	m.mu.Lock()
	conflict.Resolved = true
	conflict.Resolution = resolution.Document
	conflict.Strategy = strategy

	if m.config.KeepHistory {
		m.history = append(m.history, conflict)
		if len(m.history) > m.config.MaxHistorySize {
			m.history = m.history[1:]
		}
	}

	delete(m.conflicts, conflict.ID)
	m.mu.Unlock()

	m.emitEvent(&Event{
		Type:       "conflict_resolved",
		Conflict:   conflict,
		Resolution: resolution,
		Timestamp:  time.Now(),
	})

	return resolution, nil
}

// ManualResolve manually resolves a conflict with a chosen document.
func (m *Manager) ManualResolve(conflictID, choice string) (*Resolution, error) {
	m.mu.Lock()
	conflict, exists := m.conflicts[conflictID]
	if !exists {
		m.mu.Unlock()
		return nil, errors.New("conflict not found")
	}
	m.mu.Unlock()

	var doc *Document
	switch choice {
	case "local":
		doc = conflict.Local
	case "remote":
		doc = conflict.Remote
	case "base":
		if conflict.Base == nil {
			return nil, errors.New("no base version available")
		}
		doc = conflict.Base
	default:
		return nil, errors.New("invalid choice")
	}

	resolution := &Resolution{
		Document: doc,
		Strategy: StrategyManual,
		Manual:   true,
		Metadata: map[string]string{
			"choice": choice,
		},
	}

	m.mu.Lock()
	conflict.Resolved = true
	conflict.Resolution = doc
	conflict.Strategy = StrategyManual

	if m.config.KeepHistory {
		m.history = append(m.history, conflict)
	}

	delete(m.conflicts, conflictID)
	m.mu.Unlock()

	m.emitEvent(&Event{
		Type:       "conflict_resolved",
		Conflict:   conflict,
		Resolution: resolution,
		Timestamp:  time.Now(),
	})

	return resolution, nil
}

// GetConflict returns a conflict by ID.
func (m *Manager) GetConflict(id string) *Conflict {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conflicts[id]
}

// ListConflicts returns all unresolved conflicts.
func (m *Manager) ListConflicts() []*Conflict {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Conflict, 0, len(m.conflicts))
	for _, c := range m.conflicts {
		result = append(result, c)
	}
	return result
}

// History returns resolved conflicts.
func (m *Manager) History() []*Conflict {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Conflict, len(m.history))
	copy(result, m.history)
	return result
}

// AddListener adds an event listener.
func (m *Manager) AddListener(listener Listener) {
	m.mu.Lock()
	m.listeners = append(m.listeners, listener)
	m.mu.Unlock()
}

func (m *Manager) emitEvent(event *Event) {
	m.mu.RLock()
	listeners := m.listeners
	m.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// Built-in resolvers

func lastWriteWinsResolver(conflict *Conflict) (*Resolution, error) {
	var winner *Document

	if conflict.Local.Version.Timestamp.After(conflict.Remote.Version.Timestamp) {
		winner = conflict.Local
	} else {
		winner = conflict.Remote
	}

	return &Resolution{
		Document: winner,
		Metadata: map[string]string{
			"winner": winner.ID,
		},
	}, nil
}

func firstWriteWinsResolver(conflict *Conflict) (*Resolution, error) {
	var winner *Document

	if conflict.Local.Version.Timestamp.Before(conflict.Remote.Version.Timestamp) {
		winner = conflict.Local
	} else {
		winner = conflict.Remote
	}

	return &Resolution{
		Document: winner,
		Metadata: map[string]string{
			"winner": winner.ID,
		},
	}, nil
}

func highestVersionResolver(conflict *Conflict) (*Resolution, error) {
	var winner *Document

	if conflict.Local.Version.Clock >= conflict.Remote.Version.Clock {
		winner = conflict.Local
	} else {
		winner = conflict.Remote
	}

	return &Resolution{
		Document: winner,
		Metadata: map[string]string{
			"winner": winner.ID,
		},
	}, nil
}

func (m *Manager) mergeResolver(conflict *Conflict) (*Resolution, error) {
	if !m.config.MergeEnabled {
		return nil, ErrMergeConflict
	}

	// Try to merge JSON documents
	var localData, remoteData map[string]interface{}

	if err := json.Unmarshal(conflict.Local.Data, &localData); err != nil {
		// Not JSON, fall back to last-write-wins
		return lastWriteWinsResolver(conflict)
	}

	if err := json.Unmarshal(conflict.Remote.Data, &remoteData); err != nil {
		// Not JSON, fall back to last-write-wins
		return lastWriteWinsResolver(conflict)
	}

	// Merge the maps
	merged := mergeRecursive(localData, remoteData)

	data, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}

	doc := &Document{
		ID: conflict.DocumentID,
		Version: &Version{
			Clock:     max(conflict.Local.Version.Clock, conflict.Remote.Version.Clock) + 1,
			Timestamp: time.Now(),
		},
		Data: data,
		Metadata: map[string]string{
			"merged": "true",
		},
	}

	return &Resolution{
		Document: doc,
		Metadata: map[string]string{
			"merged": "true",
		},
	}, nil
}

func mergeRecursive(local, remote map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all from local
	for k, v := range local {
		result[k] = v
	}

	// Merge from remote
	for k, rv := range remote {
		if lv, exists := result[k]; exists {
			// Both have the key
			switch lvt := lv.(type) {
			case map[string]interface{}:
				if rvt, ok := rv.(map[string]interface{}); ok {
					// Both are maps, merge recursively
					result[k] = mergeRecursive(lvt, rvt)
				} else {
					// Type mismatch, remote wins
					result[k] = rv
				}
			default:
				// Non-map, remote wins if different
				result[k] = rv
			}
		} else {
			// Only in remote
			result[k] = rv
		}
	}

	return result
}


var idCounter uint64
var idMu sync.Mutex

func generateID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return time.Now().Format("20060102150405") + "-" + string(rune('a'+idCounter%26))
}

// ThreeWayMerger performs three-way merges.
type ThreeWayMerger struct{}

// Merge performs a three-way merge.
func (m *ThreeWayMerger) Merge(base, local, remote *Document) (*Document, error) {
	// Simple line-based merge for text content
	baseLines := splitLines(base.Data)
	localLines := splitLines(local.Data)
	remoteLines := splitLines(remote.Data)

	result, conflicts := threeWayMerge(baseLines, localLines, remoteLines)

	if len(conflicts) > 0 {
		return nil, ErrMergeConflict
	}

	return &Document{
		ID: local.ID,
		Version: &Version{
			Clock:     max(local.Version.Clock, remote.Version.Clock) + 1,
			Timestamp: time.Now(),
		},
		Data: joinLines(result),
	}, nil
}

func splitLines(data []byte) []string {
	parts := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	result := make([]string, len(parts))
	for i, v := range parts {
		result[i] = string(v)
	}
	return result
}

func joinLines(lines []string) []byte {
	var buf bytes.Buffer
	for i, line := range lines {
		buf.WriteString(line)
		if i < len(lines)-1 {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}

func threeWayMerge(base, local, remote []string) (result []string, conflicts []int) {

	// Simple merge: if local and remote are same, use that
	// If one matches base, use the other
	// Otherwise, conflict

	maxLen := len(base)
	if len(local) > maxLen {
		maxLen = len(local)
	}
	if len(remote) > maxLen {
		maxLen = len(remote)
	}

	for i := 0; i < maxLen; i++ {
		baseLine := ""
		localLine := ""
		remoteLine := ""

		if i < len(base) {
			baseLine = base[i]
		}
		if i < len(local) {
			localLine = local[i]
		}
		if i < len(remote) {
			remoteLine = remote[i]
		}

		switch {
		case localLine == remoteLine:
			// Both agree
			result = append(result, localLine)
		case localLine == baseLine:
			// Local unchanged, use remote
			result = append(result, remoteLine)
		case remoteLine == baseLine:
			// Remote unchanged, use local
			result = append(result, localLine)
		default:
			// Conflict
			conflicts = append(conflicts, i)
			result = append(result, localLine) // Default to local
		}
	}

	return result, conflicts
}

// CRDTResolver provides CRDT-based conflict resolution.
type CRDTResolver struct {
	// Type is the CRDT type (counter, set, map, etc.).
	Type string
}

// ResolveCounter resolves counter CRDT conflicts.
func (r *CRDTResolver) ResolveCounter(conflict *Conflict) (*Resolution, error) {
	// For counters, take the max value
	var localVal, remoteVal int64

	json.Unmarshal(conflict.Local.Data, &localVal)
	json.Unmarshal(conflict.Remote.Data, &remoteVal)

	maxVal := localVal
	if remoteVal > localVal {
		maxVal = remoteVal
	}

	data, _ := json.Marshal(maxVal)

	return &Resolution{
		Document: &Document{
			ID: conflict.DocumentID,
			Version: &Version{
				Clock:     max(conflict.Local.Version.Clock, conflict.Remote.Version.Clock) + 1,
				Timestamp: time.Now(),
			},
			Data: data,
		},
	}, nil
}

// ResolveSet resolves set CRDT conflicts (union).
func (r *CRDTResolver) ResolveSet(conflict *Conflict) (*Resolution, error) {
	var localSet, remoteSet []interface{}

	json.Unmarshal(conflict.Local.Data, &localSet)
	json.Unmarshal(conflict.Remote.Data, &remoteSet)

	// Union of sets
	seen := make(map[string]bool)
	var result []interface{}

	for _, v := range localSet {
		key, _ := json.Marshal(v)
		if !seen[string(key)] {
			seen[string(key)] = true
			result = append(result, v)
		}
	}

	for _, v := range remoteSet {
		key, _ := json.Marshal(v)
		if !seen[string(key)] {
			seen[string(key)] = true
			result = append(result, v)
		}
	}

	data, _ := json.Marshal(result)

	return &Resolution{
		Document: &Document{
			ID: conflict.DocumentID,
			Version: &Version{
				Clock:     max(conflict.Local.Version.Clock, conflict.Remote.Version.Clock) + 1,
				Timestamp: time.Now(),
			},
			Data: data,
		},
	}, nil
}
