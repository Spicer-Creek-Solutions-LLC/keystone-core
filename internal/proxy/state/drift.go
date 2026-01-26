// Package state provides drift detection for proxied devices.
package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// DriftDetector detects configuration drift on proxied devices.
type DriftDetector struct {
	// proxyAgent is the proxy agent managing devices
	proxyAgent *proxy.Manager

	// baselineStore stores configuration baselines
	baselineStore BaselineStore

	// eventEmitter emits drift events
	eventEmitter EventEmitter

	// config holds drift detection configuration
	config DriftConfig

	// mu protects internal state
	mu sync.RWMutex

	// lastCheck tracks when each device was last checked
	lastCheck map[string]time.Time

	// stopCh is used to stop background checks
	stopCh chan struct{}

	// running indicates if background checks are running
	running bool
}

// DriftConfig configures drift detection.
type DriftConfig struct {
	// CheckInterval is how often to check for drift
	CheckInterval time.Duration

	// Timeout for drift checks
	Timeout time.Duration

	// MaxConcurrent is the maximum concurrent drift checks
	MaxConcurrent int

	// IgnoreFields is a list of fields to ignore during comparison
	IgnoreFields []string

	// StrictMode fails on any drift detected
	StrictMode bool
}

// DefaultDriftConfig returns default drift configuration.
func DefaultDriftConfig() DriftConfig {
	return DriftConfig{
		CheckInterval: 1 * time.Hour,
		Timeout:       5 * time.Minute,
		MaxConcurrent: 5,
		IgnoreFields:  []string{},
		StrictMode:    false,
	}
}

// BaselineStore stores configuration baselines.
type BaselineStore interface {
	// Save stores a baseline for a device
	Save(deviceID string, baseline *ConfigBaseline) error

	// Load retrieves a baseline for a device
	Load(deviceID string) (*ConfigBaseline, error)

	// Delete removes a baseline for a device
	Delete(deviceID string) error

	// List returns all device IDs with baselines
	List() ([]string, error)
}

// ConfigBaseline represents a configuration baseline for a device.
type ConfigBaseline struct {
	// DeviceID is the device identifier
	DeviceID string `json:"device_id"`

	// Timestamp is when the baseline was captured
	Timestamp time.Time `json:"timestamp"`

	// Hash is the hash of the configuration
	Hash string `json:"hash"`

	// Config is the configuration content
	Config string `json:"config"`

	// Metadata contains additional information
	Metadata map[string]string `json:"metadata,omitempty"`

	// Sections are individual configuration sections with their hashes
	Sections map[string]ConfigSection `json:"sections,omitempty"`
}

// ConfigSection represents a section of configuration.
type ConfigSection struct {
	Name    string `json:"name"`
	Hash    string `json:"hash"`
	Content string `json:"content"`
}

// DriftReport is the result of a drift detection check.
type DriftReport struct {
	// DeviceID is the device that was checked
	DeviceID string `json:"device_id"`

	// Timestamp is when the check was performed
	Timestamp time.Time `json:"timestamp"`

	// HasDrift indicates if drift was detected
	HasDrift bool `json:"has_drift"`

	// Severity is the drift severity level
	Severity DriftSeverity `json:"severity"`

	// Diffs contains the detected differences
	Diffs []DriftDiff `json:"diffs,omitempty"`

	// BaselineHash is the expected hash
	BaselineHash string `json:"baseline_hash"`

	// CurrentHash is the actual hash
	CurrentHash string `json:"current_hash"`

	// Error contains any error that occurred
	Error string `json:"error,omitempty"`

	// Duration is how long the check took
	Duration time.Duration `json:"duration"`
}

// DriftSeverity indicates the severity of detected drift.
type DriftSeverity string

const (
	DriftSeverityNone     DriftSeverity = "none"
	DriftSeverityLow      DriftSeverity = "low"
	DriftSeverityMedium   DriftSeverity = "medium"
	DriftSeverityHigh     DriftSeverity = "high"
	DriftSeverityCritical DriftSeverity = "critical"
)

// DriftDiff represents a single difference in configuration.
type DriftDiff struct {
	// Path is the configuration path that differs
	Path string `json:"path"`

	// Section is the configuration section
	Section string `json:"section,omitempty"`

	// Type is the type of difference
	Type DriftDiffType `json:"type"`

	// Expected is the expected value
	Expected string `json:"expected,omitempty"`

	// Actual is the actual value
	Actual string `json:"actual,omitempty"`

	// Severity is the severity of this specific diff
	Severity DriftSeverity `json:"severity"`
}

// DriftDiffType indicates the type of configuration difference.
type DriftDiffType string

const (
	DiffTypeAdded    DriftDiffType = "added"
	DiffTypeRemoved  DriftDiffType = "removed"
	DiffTypeModified DriftDiffType = "modified"
)

// NewDriftDetector creates a new drift detector.
func NewDriftDetector(proxyAgent *proxy.Manager, store BaselineStore, config DriftConfig) (*DriftDetector, error) {
	if proxyAgent == nil {
		return nil, fmt.Errorf("proxy agent is required")
	}

	if store == nil {
		store = NewInMemoryBaselineStore()
	}

	return &DriftDetector{
		proxyAgent:    proxyAgent,
		baselineStore: store,
		config:        config,
		lastCheck:     make(map[string]time.Time),
		stopCh:        make(chan struct{}),
	}, nil
}

// SetEventEmitter sets the event emitter for drift events.
func (d *DriftDetector) SetEventEmitter(emitter EventEmitter) {
	d.eventEmitter = emitter
}

// CaptureBaseline captures the current configuration as a baseline.
func (d *DriftDetector) CaptureBaseline(ctx context.Context, deviceID string) (*ConfigBaseline, error) {
	// Get the device
	device, err := d.proxyAgent.Registry().Get(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("device not found: %w", err)
	}

	// Get executor
	executor := d.proxyAgent.Executor()
	if executor == nil {
		return nil, fmt.Errorf("no executor available")
	}

	// Get configuration
	config, err := d.getDeviceConfig(ctx, device, executor)
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Calculate hash
	hash := d.hashConfig(config)

	baseline := &ConfigBaseline{
		DeviceID:  deviceID,
		Timestamp: time.Now(),
		Hash:      hash,
		Config:    config,
		Metadata: map[string]string{
			"device_type": string(device.Type),
		},
	}

	// Store baseline
	if err := d.baselineStore.Save(deviceID, baseline); err != nil {
		return nil, fmt.Errorf("failed to store baseline: %w", err)
	}

	return baseline, nil
}

// CheckDrift checks for drift on a device against its baseline.
func (d *DriftDetector) CheckDrift(ctx context.Context, deviceID string) (*DriftReport, error) {
	startTime := time.Now()

	report := &DriftReport{
		DeviceID:  deviceID,
		Timestamp: startTime,
	}

	// Load baseline
	baseline, err := d.baselineStore.Load(deviceID)
	if err != nil {
		report.Error = fmt.Sprintf("failed to load baseline: %v", err)
		report.Duration = time.Since(startTime)
		return report, err
	}

	if baseline == nil {
		report.Error = "no baseline found for device"
		report.Duration = time.Since(startTime)
		return report, fmt.Errorf("no baseline found")
	}

	report.BaselineHash = baseline.Hash

	// Get the device
	device, err := d.proxyAgent.Registry().Get(ctx, deviceID)
	if err != nil {
		report.Error = fmt.Sprintf("device not found: %v", err)
		report.Duration = time.Since(startTime)
		return report, err
	}

	// Get executor
	executor := d.proxyAgent.Executor()
	if executor == nil {
		report.Error = "no executor available"
		report.Duration = time.Since(startTime)
		return report, fmt.Errorf("no executor")
	}

	// Get current configuration
	currentConfig, err := d.getDeviceConfig(ctx, device, executor)
	if err != nil {
		report.Error = fmt.Sprintf("failed to get configuration: %v", err)
		report.Duration = time.Since(startTime)
		return report, err
	}

	// Calculate current hash
	currentHash := d.hashConfig(currentConfig)
	report.CurrentHash = currentHash

	// Check for drift
	if currentHash != baseline.Hash {
		report.HasDrift = true
		report.Diffs = d.computeDiffs(baseline.Config, currentConfig)
		report.Severity = d.calculateSeverity(report.Diffs)
	} else {
		report.Severity = DriftSeverityNone
	}

	report.Duration = time.Since(startTime)

	// Update last check time
	d.mu.Lock()
	d.lastCheck[deviceID] = time.Now()
	d.mu.Unlock()

	// Emit event if drift detected
	if report.HasDrift && d.eventEmitter != nil {
		d.eventEmitter.Emit(StateEvent{
			Type:     EventStateDrift,
			DeviceID: deviceID,
			Result:   StateResult(report.Severity),
			Duration: report.Duration,
		})
	}

	return report, nil
}

// CheckAllDrift checks for drift on all devices with baselines.
func (d *DriftDetector) CheckAllDrift(ctx context.Context) (map[string]*DriftReport, error) {
	deviceIDs, err := d.baselineStore.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list baselines: %w", err)
	}

	reports := make(map[string]*DriftReport)
	reportsMu := sync.Mutex{}

	// Use semaphore for concurrency control
	sem := make(chan struct{}, d.config.MaxConcurrent)
	var wg sync.WaitGroup

	for _, deviceID := range deviceIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			// Create timeout context
			checkCtx, cancel := context.WithTimeout(ctx, d.config.Timeout)
			defer cancel()

			report, err := d.CheckDrift(checkCtx, id)
			if err != nil {
				report = &DriftReport{
					DeviceID:  id,
					Timestamp: time.Now(),
					Error:     err.Error(),
				}
			}

			reportsMu.Lock()
			reports[id] = report
			reportsMu.Unlock()
		}(deviceID)
	}

	wg.Wait()
	return reports, nil
}

// StartBackgroundChecks starts periodic drift checking.
func (d *DriftDetector) StartBackgroundChecks(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("background checks already running")
	}
	d.running = true
	d.mu.Unlock()

	go func() {
		ticker := time.NewTicker(d.config.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.stopCh:
				return
			case <-ticker.C:
				d.CheckAllDrift(ctx)
			}
		}
	}()

	return nil
}

// StopBackgroundChecks stops periodic drift checking.
func (d *DriftDetector) StopBackgroundChecks() {
	d.mu.Lock()
	if d.running {
		close(d.stopCh)
		d.running = false
		d.stopCh = make(chan struct{})
	}
	d.mu.Unlock()
}

// getDeviceConfig gets the configuration from a device.
func (d *DriftDetector) getDeviceConfig(ctx context.Context, device *proxy.ProxiedDevice, executor proxy.ProxiedExecutor) (string, error) {
	// Try common configuration retrieval commands
	commands := []string{
		"show running-config",         // Cisco, Arista
		"show configuration",          // VyOS, Juniper
		"show configuration commands", // VyOS alt
	}

	for _, cmd := range commands {
		result, err := executor.Execute(ctx, &proxy.ProxiedExecuteRequest{
			DeviceID: device.ID,
			Command:  cmd,
		})
		if err == nil && result.ExitCode == 0 && len(result.Stdout) > 0 {
			return string(result.Stdout), nil
		}
	}

	return "", fmt.Errorf("unable to retrieve device configuration")
}

// hashConfig calculates a hash of the configuration.
func (d *DriftDetector) hashConfig(config string) string {
	h := sha256.New()
	h.Write([]byte(config))
	return hex.EncodeToString(h.Sum(nil))
}

// computeDiffs computes the differences between two configurations.
func (d *DriftDetector) computeDiffs(baseline, current string) []DriftDiff {
	var diffs []DriftDiff

	// Simple line-by-line comparison
	baselineLines := splitLines(baseline)
	currentLines := splitLines(current)

	baselineSet := make(map[string]bool)
	currentSet := make(map[string]bool)

	for _, line := range baselineLines {
		baselineSet[line] = true
	}

	for _, line := range currentLines {
		currentSet[line] = true
	}

	// Find removed lines
	for line := range baselineSet {
		if !currentSet[line] {
			diffs = append(diffs, DriftDiff{
				Path:     line,
				Type:     DiffTypeRemoved,
				Expected: line,
				Severity: d.classifyLineSeverity(line),
			})
		}
	}

	// Find added lines
	for line := range currentSet {
		if !baselineSet[line] {
			diffs = append(diffs, DriftDiff{
				Path:     line,
				Type:     DiffTypeAdded,
				Actual:   line,
				Severity: d.classifyLineSeverity(line),
			})
		}
	}

	return diffs
}

// calculateSeverity calculates the overall severity from diffs.
func (d *DriftDetector) calculateSeverity(diffs []DriftDiff) DriftSeverity {
	if len(diffs) == 0 {
		return DriftSeverityNone
	}

	maxSeverity := DriftSeverityLow
	severityOrder := map[DriftSeverity]int{
		DriftSeverityNone:     0,
		DriftSeverityLow:      1,
		DriftSeverityMedium:   2,
		DriftSeverityHigh:     3,
		DriftSeverityCritical: 4,
	}

	for _, diff := range diffs {
		if severityOrder[diff.Severity] > severityOrder[maxSeverity] {
			maxSeverity = diff.Severity
		}
	}

	return maxSeverity
}

// classifyLineSeverity classifies the severity of a configuration line change.
func (d *DriftDetector) classifyLineSeverity(line string) DriftSeverity {
	// Critical: security, authentication, encryption changes
	criticalPatterns := []string{"password", "secret", "key", "crypto", "aaa", "tacacs", "radius", "enable"}
	for _, pattern := range criticalPatterns {
		if containsIgnoreCase(line, pattern) {
			return DriftSeverityCritical
		}
	}

	// High: routing, firewall changes
	highPatterns := []string{"ip route", "router", "firewall", "access-list", "acl", "permit", "deny"}
	for _, pattern := range highPatterns {
		if containsIgnoreCase(line, pattern) {
			return DriftSeverityHigh
		}
	}

	// Medium: interface, VLAN changes
	mediumPatterns := []string{"interface", "vlan", "spanning-tree", "trunk"}
	for _, pattern := range mediumPatterns {
		if containsIgnoreCase(line, pattern) {
			return DriftSeverityMedium
		}
	}

	return DriftSeverityLow
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		line := s[start:]
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > 0 && len(substr) > 0 && contains(toLower(s), toLower(substr))))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// =============================================================================
// In-Memory Baseline Store
// =============================================================================

// InMemoryBaselineStore is an in-memory implementation of BaselineStore.
type InMemoryBaselineStore struct {
	baselines map[string]*ConfigBaseline
	mu        sync.RWMutex
}

// NewInMemoryBaselineStore creates a new in-memory baseline store.
func NewInMemoryBaselineStore() *InMemoryBaselineStore {
	return &InMemoryBaselineStore{
		baselines: make(map[string]*ConfigBaseline),
	}
}

// Save stores a baseline.
func (s *InMemoryBaselineStore) Save(deviceID string, baseline *ConfigBaseline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baselines[deviceID] = baseline
	return nil
}

// Load retrieves a baseline.
func (s *InMemoryBaselineStore) Load(deviceID string) (*ConfigBaseline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.baselines[deviceID], nil
}

// Delete removes a baseline.
func (s *InMemoryBaselineStore) Delete(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.baselines, deviceID)
	return nil
}

// List returns all device IDs.
func (s *InMemoryBaselineStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.baselines))
	for id := range s.baselines {
		ids = append(ids, id)
	}
	return ids, nil
}

// =============================================================================
// File-Based Baseline Store
// =============================================================================

// FileBaselineStore stores baselines in files.
type FileBaselineStore struct {
	dir string
	mu  sync.RWMutex
}

// NewFileBaselineStore creates a new file-based baseline store.
func NewFileBaselineStore(dir string) *FileBaselineStore {
	return &FileBaselineStore{dir: dir}
}

// Save stores a baseline to a file.
func (s *FileBaselineStore) Save(deviceID string, baseline *ConfigBaseline) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}

	// Note: In real implementation, use os.WriteFile
	_ = data
	return nil
}

// Load retrieves a baseline from a file.
func (s *FileBaselineStore) Load(deviceID string) (*ConfigBaseline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Note: In real implementation, use os.ReadFile
	return nil, nil
}

// Delete removes a baseline file.
func (s *FileBaselineStore) Delete(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Note: In real implementation, use os.Remove
	return nil
}

// List returns all device IDs with baselines.
func (s *FileBaselineStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Note: In real implementation, use os.ReadDir
	return nil, nil
}
