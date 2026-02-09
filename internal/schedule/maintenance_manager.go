package schedule

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MaintenanceManagerConfig holds configuration for the maintenance window manager.
type MaintenanceManagerConfig struct {
	// MemberID is the cluster member ID.
	MemberID string

	// DefaultNotifyBefore is the default notification lead time.
	DefaultNotifyBefore time.Duration

	// MaxWindowDuration is the maximum allowed window duration.
	MaxWindowDuration time.Duration

	// AllowOverlappingWindows allows windows to overlap.
	AllowOverlappingWindows bool
}

// DefaultMaintenanceManagerConfig returns default configuration.
func DefaultMaintenanceManagerConfig() *MaintenanceManagerConfig {
	return &MaintenanceManagerConfig{
		DefaultNotifyBefore: 15 * time.Minute,
		MaxWindowDuration:   24 * time.Hour,
	}
}

// MaintenanceWindowManager manages maintenance window CRUD operations and lifecycle.
type MaintenanceWindowManager struct {
	config    *MaintenanceManagerConfig
	store     Store
	listeners []MaintenanceEventListener
	mu        sync.RWMutex
	closed    bool
}

// MaintenanceEventListener receives maintenance window events.
type MaintenanceEventListener func(event *MaintenanceEvent)

// MaintenanceEventType represents maintenance event types.
type MaintenanceEventType string

// MaintenanceEventCreated constants define the events.
const (
	MaintenanceEventCreated   MaintenanceEventType = "maintenance.created"
	MaintenanceEventUpdated   MaintenanceEventType = "maintenance.updated"
	MaintenanceEventDeleted   MaintenanceEventType = "maintenance.deleted"
	MaintenanceEventApproved  MaintenanceEventType = "maintenance.approved"
	MaintenanceEventStarting  MaintenanceEventType = "maintenance.starting"
	MaintenanceEventStarted   MaintenanceEventType = "maintenance.started"
	MaintenanceEventExtended  MaintenanceEventType = "maintenance.extended"
	MaintenanceEventEnding    MaintenanceEventType = "maintenance.ending"
	MaintenanceEventEnded     MaintenanceEventType = "maintenance.ended"
	MaintenanceEventCancelled MaintenanceEventType = "maintenance.cancelled"
	MaintenanceEventExpired   MaintenanceEventType = "maintenance.expired"
)

// NewMaintenanceWindowManager creates a new maintenance window manager.
func NewMaintenanceWindowManager(config *MaintenanceManagerConfig, store Store) (*MaintenanceWindowManager, error) {
	if config == nil {
		config = DefaultMaintenanceManagerConfig()
	}
	if store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if config.MemberID == "" {
		return nil, fmt.Errorf("member ID is required")
	}

	return &MaintenanceWindowManager{
		config:    config,
		store:     store,
		listeners: make([]MaintenanceEventListener, 0),
	}, nil
}

// Create creates a new maintenance window.
func (m *MaintenanceWindowManager) Create(ctx context.Context, window *MaintenanceWindow) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	if window == nil {
		return fmt.Errorf("maintenance window is required")
	}

	// Generate ID if not provided
	if window.ID == "" {
		window.ID = uuid.New().String()
	}

	// Validate
	if err := m.validate(window); err != nil {
		return err
	}

	// Set defaults
	now := time.Now().UTC()
	if window.Status == "" {
		if window.RequireApproval {
			window.Status = MaintenanceWindowStatusPendingApproval
		} else {
			window.Status = MaintenanceWindowStatusScheduled
		}
	}
	if window.NotifyBefore == 0 {
		window.NotifyBefore = m.config.DefaultNotifyBefore
	}

	window.CreatedAt = now
	window.UpdatedAt = now

	// Check for conflicts
	if !m.config.AllowOverlappingWindows {
		conflicts, err := m.GetConflicts(ctx, window)
		if err != nil {
			return fmt.Errorf("failed to check conflicts: %w", err)
		}
		for _, conflict := range conflicts {
			if conflict.Severity == ConflictSeverityError {
				return fmt.Errorf("%w: %s", ErrMaintenanceConflict, conflict.Message)
			}
		}
	}

	// Store
	if err := m.store.CreateMaintenanceWindow(ctx, window); err != nil {
		return err
	}

	// Emit event
	m.emitEvent(&MaintenanceEvent{
		Type:       string(MaintenanceEventCreated),
		WindowID:   window.ID,
		WindowName: window.Name,
		Timestamp:  now,
		Actor:      window.CreatedBy,
	})

	return nil
}

// Get retrieves a maintenance window by ID.
func (m *MaintenanceWindowManager) Get(ctx context.Context, id string) (*MaintenanceWindow, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	return m.store.GetMaintenanceWindow(ctx, id)
}

// Update updates an existing maintenance window.
func (m *MaintenanceWindowManager) Update(ctx context.Context, window *MaintenanceWindow) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	if window == nil {
		return fmt.Errorf("maintenance window is required")
	}
	if window.ID == "" {
		return fmt.Errorf("maintenance window ID is required")
	}

	// Check if window exists and is modifiable
	existing, err := m.store.GetMaintenanceWindow(ctx, window.ID)
	if err != nil {
		return err
	}

	// Don't allow updating completed/cancelled windows
	if existing.Status == MaintenanceWindowStatusCompleted ||
		existing.Status == MaintenanceWindowStatusCancelled {
		return fmt.Errorf("cannot update window in status %s", existing.Status)
	}

	// Validate
	if err := m.validate(window); err != nil {
		return err
	}

	now := time.Now().UTC()
	window.UpdatedAt = now

	// Store
	if err := m.store.UpdateMaintenanceWindow(ctx, window); err != nil {
		return err
	}

	// Emit event
	m.emitEvent(&MaintenanceEvent{
		Type:       string(MaintenanceEventUpdated),
		WindowID:   window.ID,
		WindowName: window.Name,
		Timestamp:  now,
		Actor:      window.UpdatedBy,
	})

	return nil
}

// Delete deletes a maintenance window.
func (m *MaintenanceWindowManager) Delete(ctx context.Context, id string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	// Get window first for event emission
	window, err := m.store.GetMaintenanceWindow(ctx, id)
	if err != nil {
		return err
	}

	// Don't allow deleting active windows
	if window.Status == MaintenanceWindowStatusActive {
		return ErrMaintenanceActive
	}

	// Delete
	if err := m.store.DeleteMaintenanceWindow(ctx, id); err != nil {
		return err
	}

	// Emit event
	m.emitEvent(&MaintenanceEvent{
		Type:       string(MaintenanceEventDeleted),
		WindowID:   id,
		WindowName: window.Name,
		Timestamp:  time.Now().UTC(),
	})

	return nil
}

// List lists maintenance windows matching the filter.
func (m *MaintenanceWindowManager) List(ctx context.Context, filter *MaintenanceWindowFilter) ([]*MaintenanceWindow, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	return m.store.ListMaintenanceWindows(ctx, filter)
}

// Approve approves a pending maintenance window.
func (m *MaintenanceWindowManager) Approve(ctx context.Context, id, approvedBy string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	window, err := m.store.GetMaintenanceWindow(ctx, id)
	if err != nil {
		return err
	}

	if window.Status != MaintenanceWindowStatusPendingApproval {
		return fmt.Errorf("window is not pending approval, status: %s", window.Status)
	}

	now := time.Now().UTC()
	window.Status = MaintenanceWindowStatusScheduled
	window.ApprovedBy = approvedBy
	window.ApprovedAt = &now
	window.UpdatedAt = now
	window.UpdatedBy = approvedBy

	if err := m.store.UpdateMaintenanceWindow(ctx, window); err != nil {
		return err
	}

	m.emitEvent(&MaintenanceEvent{
		Type:       string(MaintenanceEventApproved),
		WindowID:   id,
		WindowName: window.Name,
		Timestamp:  now,
		Actor:      approvedBy,
	})

	return nil
}

// Start starts a scheduled maintenance window.
func (m *MaintenanceWindowManager) Start(ctx context.Context, id string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	window, err := m.store.GetMaintenanceWindow(ctx, id)
	if err != nil {
		return err
	}

	if window.Status != MaintenanceWindowStatusScheduled {
		return fmt.Errorf("cannot start window in status %s", window.Status)
	}

	now := time.Now().UTC()
	window.Status = MaintenanceWindowStatusActive
	window.ActualStartTime = &now
	window.UpdatedAt = now

	if err := m.store.UpdateMaintenanceWindow(ctx, window); err != nil {
		return err
	}

	m.emitEvent(&MaintenanceEvent{
		Type:       string(MaintenanceEventStarted),
		WindowID:   id,
		WindowName: window.Name,
		Timestamp:  now,
	})

	return nil
}

// End ends an active maintenance window.
func (m *MaintenanceWindowManager) End(ctx context.Context, id string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	window, err := m.store.GetMaintenanceWindow(ctx, id)
	if err != nil {
		return err
	}

	if window.Status != MaintenanceWindowStatusActive {
		return fmt.Errorf("cannot end window in status %s", window.Status)
	}

	now := time.Now().UTC()
	window.Status = MaintenanceWindowStatusCompleted
	window.ActualEndTime = &now
	window.UpdatedAt = now

	if err := m.store.UpdateMaintenanceWindow(ctx, window); err != nil {
		return err
	}

	m.emitEvent(&MaintenanceEvent{
		Type:       string(MaintenanceEventEnded),
		WindowID:   id,
		WindowName: window.Name,
		Timestamp:  now,
	})

	return nil
}

// Cancel cancels a maintenance window.
func (m *MaintenanceWindowManager) Cancel(ctx context.Context, id, cancelledBy, reason string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	window, err := m.store.GetMaintenanceWindow(ctx, id)
	if err != nil {
		return err
	}

	// Don't allow cancelling completed windows
	if window.Status == MaintenanceWindowStatusCompleted {
		return fmt.Errorf("cannot cancel completed window")
	}

	now := time.Now().UTC()
	window.Status = MaintenanceWindowStatusCancelled
	window.CancelledBy = cancelledBy
	window.CancelledAt = &now
	window.CancellationReason = reason
	window.UpdatedAt = now
	window.UpdatedBy = cancelledBy

	if err := m.store.UpdateMaintenanceWindow(ctx, window); err != nil {
		return err
	}

	m.emitEvent(&MaintenanceEvent{
		Type:       string(MaintenanceEventCancelled),
		WindowID:   id,
		WindowName: window.Name,
		Timestamp:  now,
		Actor:      cancelledBy,
		Data: map[string]interface{}{
			"reason": reason,
		},
	})

	return nil
}

// Extend extends an active maintenance window.
func (m *MaintenanceWindowManager) Extend(ctx context.Context, id string, newEndTime time.Time, extendedBy string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	window, err := m.store.GetMaintenanceWindow(ctx, id)
	if err != nil {
		return err
	}

	if window.Status != MaintenanceWindowStatusActive &&
		window.Status != MaintenanceWindowStatusScheduled {
		return fmt.Errorf("cannot extend window in status %s", window.Status)
	}

	// Validate new end time
	if newEndTime.Before(window.EndTime) {
		return fmt.Errorf("new end time must be after current end time")
	}

	// Check max duration if configured
	if m.config.MaxWindowDuration > 0 {
		newDuration := newEndTime.Sub(window.StartTime)
		if newDuration > m.config.MaxWindowDuration {
			return fmt.Errorf("extended duration %v exceeds maximum %v", newDuration, m.config.MaxWindowDuration)
		}
	}

	oldEndTime := window.EndTime
	now := time.Now().UTC()
	window.EndTime = newEndTime
	window.UpdatedAt = now
	window.UpdatedBy = extendedBy

	if err := m.store.UpdateMaintenanceWindow(ctx, window); err != nil {
		return err
	}

	m.emitEvent(&MaintenanceEvent{
		Type:       string(MaintenanceEventExtended),
		WindowID:   id,
		WindowName: window.Name,
		Timestamp:  now,
		Actor:      extendedBy,
		Data: map[string]interface{}{
			"old_end_time": oldEndTime,
			"new_end_time": newEndTime,
		},
	})

	return nil
}

// IsInMaintenance checks if an agent is currently in a maintenance window.
func (m *MaintenanceWindowManager) IsInMaintenance(ctx context.Context, agentID string) (*MaintenanceWindow, bool, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, false, ErrStoreClosed
	}
	m.mu.RUnlock()

	now := time.Now().UTC()
	filter := &MaintenanceWindowFilter{
		Status:       []MaintenanceWindowStatus{MaintenanceWindowStatusActive},
		IncludesTime: &now,
		AgentID:      agentID,
	}

	windows, err := m.store.ListMaintenanceWindows(ctx, filter)
	if err != nil {
		return nil, false, err
	}

	if len(windows) > 0 {
		return windows[0], true, nil
	}

	return nil, false, nil
}

// GetActiveWindows returns all currently active maintenance windows.
func (m *MaintenanceWindowManager) GetActiveWindows(ctx context.Context) ([]*MaintenanceWindow, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	filter := &MaintenanceWindowFilter{
		Status: []MaintenanceWindowStatus{MaintenanceWindowStatusActive},
	}

	return m.store.ListMaintenanceWindows(ctx, filter)
}

// GetUpcomingWindows returns scheduled windows starting within the specified duration.
func (m *MaintenanceWindowManager) GetUpcomingWindows(ctx context.Context, within time.Duration) ([]*MaintenanceWindow, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	now := time.Now().UTC()
	endTime := now.Add(within)

	filter := &MaintenanceWindowFilter{
		Status:     []MaintenanceWindowStatus{MaintenanceWindowStatusScheduled},
		StartAfter: &now,
		EndBefore:  &endTime,
	}

	return m.store.ListMaintenanceWindows(ctx, filter)
}

// GetConflicts checks for conflicts with other windows.
func (m *MaintenanceWindowManager) GetConflicts(ctx context.Context, window *MaintenanceWindow) ([]*MaintenanceConflict, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	// Get all windows that might overlap
	filter := &MaintenanceWindowFilter{
		Status: []MaintenanceWindowStatus{
			MaintenanceWindowStatusScheduled,
			MaintenanceWindowStatusActive,
		},
		StartAfter: &window.StartTime,
	}

	windows, err := m.store.ListMaintenanceWindows(ctx, filter)
	if err != nil {
		return nil, err
	}

	conflicts := make([]*MaintenanceConflict, 0)

	for _, other := range windows {
		// Skip self
		if other.ID == window.ID {
			continue
		}

		// Check for time overlap
		if window.Overlaps(other) {
			// Check for scope overlap
			affectedAgents := m.findOverlappingAgents(window.Scope, other.Scope)

			severity := ConflictSeverityWarning
			if len(affectedAgents) > 0 {
				severity = ConflictSeverityError
			}

			conflict := &MaintenanceConflict{
				WindowID:        window.ID,
				WindowName:      window.Name,
				ConflictType:    ConflictTypeOverlap,
				ConflictingID:   other.ID,
				ConflictingName: other.Name,
				OverlapStart:    maxTime(window.StartTime, other.StartTime),
				OverlapEnd:      minTime(window.EndTime, other.EndTime),
				AffectedAgents:  affectedAgents,
				Severity:        severity,
				Message: fmt.Sprintf("window overlaps with %q from %s to %s",
					other.Name,
					maxTime(window.StartTime, other.StartTime).Format(time.RFC3339),
					minTime(window.EndTime, other.EndTime).Format(time.RFC3339)),
			}

			conflicts = append(conflicts, conflict)
		}
	}

	return conflicts, nil
}

// GetStats returns maintenance window statistics.
func (m *MaintenanceWindowManager) GetStats(ctx context.Context) (*MaintenanceStats, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	windows, err := m.store.ListMaintenanceWindows(ctx, nil)
	if err != nil {
		return nil, err
	}

	stats := &MaintenanceStats{
		ByType:   make(map[MaintenanceWindowType]int),
		ByStatus: make(map[MaintenanceWindowStatus]int),
	}

	now := time.Now().UTC()
	next24h := now.Add(24 * time.Hour)
	var totalDuration time.Duration
	completedCount := 0

	for _, w := range windows {
		stats.TotalWindows++
		stats.ByType[w.Type]++
		stats.ByStatus[w.Status]++

		switch w.Status {
		case MaintenanceWindowStatusActive:
			stats.ActiveWindows++
		case MaintenanceWindowStatusScheduled:
			stats.ScheduledWindows++
		case MaintenanceWindowStatusCompleted:
			stats.CompletedWindows++
			// Calculate actual duration for completed windows
			if w.ActualStartTime != nil && w.ActualEndTime != nil {
				totalDuration += w.ActualEndTime.Sub(*w.ActualStartTime)
				completedCount++
			}
		case MaintenanceWindowStatusCancelled:
			stats.CancelledWindows++
		default:
			// MaintenanceWindowStatusPendingApproval, MaintenanceWindowStatusExpired counted via ByStatus map
		}

		// Check if upcoming in 24h
		if w.Status == MaintenanceWindowStatusScheduled &&
			w.StartTime.After(now) && w.StartTime.Before(next24h) {
			stats.UpcomingIn24h++
		}
	}

	stats.TotalDuration = totalDuration
	if completedCount > 0 {
		stats.AverageDuration = totalDuration / time.Duration(completedCount)
	}

	// Count affected agents for active windows
	activeWindows, _ := m.GetActiveWindows(ctx)
	for _, w := range activeWindows {
		if w.Scope != nil {
			stats.AffectedAgentsNow += len(w.Scope.AgentIDs)
			if w.Scope.All {
				stats.AffectedAgentsNow = -1 // Indicates all agents
				break
			}
		}
	}

	return stats, nil
}

// AddListener adds an event listener.
func (m *MaintenanceWindowManager) AddListener(listener MaintenanceEventListener) {
	m.mu.Lock()
	m.listeners = append(m.listeners, listener)
	m.mu.Unlock()
}

// Close closes the manager.
func (m *MaintenanceWindowManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true

	return nil
}

// validate validates a maintenance window.
func (m *MaintenanceWindowManager) validate(window *MaintenanceWindow) error {
	if window.Name == "" {
		return fmt.Errorf("name is required")
	}

	if window.StartTime.IsZero() {
		return fmt.Errorf("start time is required")
	}

	if window.EndTime.IsZero() {
		return fmt.Errorf("end time is required")
	}

	if window.EndTime.Before(window.StartTime) {
		return fmt.Errorf("end time must be after start time")
	}

	// Check max duration
	if m.config.MaxWindowDuration > 0 {
		duration := window.EndTime.Sub(window.StartTime)
		if duration > m.config.MaxWindowDuration {
			return fmt.Errorf("duration %v exceeds maximum %v", duration, m.config.MaxWindowDuration)
		}
	}

	// Validate scope
	if window.Scope == nil {
		return fmt.Errorf("scope is required")
	}

	// Scope must have at least one selection criteria
	if !window.Scope.All &&
		len(window.Scope.AgentIDs) == 0 &&
		window.Scope.Glob == "" &&
		len(window.Scope.Tags) == 0 &&
		len(window.Scope.Roles) == 0 {
		return fmt.Errorf("scope must specify agents, glob, tags, roles, or all")
	}

	// Validate timezone if provided
	if window.Timezone != "" {
		if _, err := time.LoadLocation(window.Timezone); err != nil {
			return fmt.Errorf("invalid timezone %q", window.Timezone)
		}
	}

	return nil
}

// findOverlappingAgents finds agents affected by both scopes.
func (m *MaintenanceWindowManager) findOverlappingAgents(scope1, scope2 *MaintenanceScope) []string {
	if scope1 == nil || scope2 == nil {
		return nil
	}

	// If either affects all, they overlap
	if scope1.All || scope2.All {
		return nil // Can't determine specific agents
	}

	// Simple intersection of agent IDs
	agents1 := make(map[string]bool)
	for _, id := range scope1.AgentIDs {
		agents1[id] = true
	}

	var overlapping []string
	for _, id := range scope2.AgentIDs {
		if agents1[id] {
			overlapping = append(overlapping, id)
		}
	}

	return overlapping
}

// emitEvent emits an event to all listeners.
func (m *MaintenanceWindowManager) emitEvent(event *MaintenanceEvent) {
	m.mu.RLock()
	listeners := m.listeners
	m.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// Helper functions

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// AgentInScope checks if an agent ID matches the maintenance scope.
func AgentInScope(agentID string, scope *MaintenanceScope) bool {
	if scope == nil {
		return false
	}

	// Check all
	if scope.All {
		return true
	}

	// Check specific agent IDs
	for _, id := range scope.AgentIDs {
		if id == agentID {
			return true
		}
	}

	// Check glob pattern
	if scope.Glob != "" {
		matched, _ := path.Match(scope.Glob, agentID)
		if matched {
			return true
		}
	}

	// Note: Tag and role matching would require additional agent info
	// which is outside the scope of this check

	return false
}
