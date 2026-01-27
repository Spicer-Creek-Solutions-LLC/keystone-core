package approval

import (
	"context"
	"sync"
	"time"
)

// ReminderScheduler manages reminder notifications for pending approvals.
type ReminderScheduler struct {
	manager  *Manager
	notifier Notifier
	storage  Storage

	// Configuration
	defaultInterval time.Duration
	checkInterval   time.Duration

	// Tracking last reminder times
	mu            sync.RWMutex
	lastReminders map[string]time.Time

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// ReminderSchedulerConfig contains configuration for the reminder scheduler.
type ReminderSchedulerConfig struct {
	// DefaultInterval is the default reminder interval if not specified per-request.
	DefaultInterval time.Duration

	// CheckInterval is how often to check for pending reminders.
	CheckInterval time.Duration
}

// NewReminderScheduler creates a new reminder scheduler.
func NewReminderScheduler(manager *Manager, notifier Notifier, storage Storage, config ReminderSchedulerConfig) *ReminderScheduler {
	if config.DefaultInterval == 0 {
		config.DefaultInterval = 30 * time.Minute
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = time.Minute
	}

	return &ReminderScheduler{
		manager:         manager,
		notifier:        notifier,
		storage:         storage,
		defaultInterval: config.DefaultInterval,
		checkInterval:   config.CheckInterval,
		lastReminders:   make(map[string]time.Time),
		stopCh:          make(chan struct{}),
	}
}

// Start begins the reminder scheduler background loop.
func (s *ReminderScheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
}

// Stop stops the reminder scheduler.
func (s *ReminderScheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

// run is the main scheduler loop.
func (s *ReminderScheduler) run(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processReminders(ctx)
			s.processEscalations(ctx)
			s.processExpirations(ctx)
		}
	}
}

// processReminders sends reminders for pending approvals.
func (s *ReminderScheduler) processReminders(ctx context.Context) {
	requests, err := s.storage.ListRequests(ctx, ListOptions{
		State:          RequestStatePending,
		IncludePending: true,
	})
	if err != nil {
		return
	}

	now := time.Now()

	for _, req := range requests {
		// Skip if no notify channels in metadata
		channels := s.getNotifyChannels(req)
		if len(channels) == 0 {
			continue
		}

		// Get reminder interval
		interval := s.getReminderInterval(req)
		if interval == 0 {
			continue
		}

		// Check if it's time to send a reminder
		s.mu.RLock()
		lastReminder, hasReminder := s.lastReminders[req.ID]
		s.mu.RUnlock()

		shouldRemind := false
		if !hasReminder {
			// First reminder after creation + interval
			if now.Sub(req.CreatedAt) >= interval {
				shouldRemind = true
			}
		} else {
			// Subsequent reminders
			if now.Sub(lastReminder) >= interval {
				shouldRemind = true
			}
		}

		if shouldRemind {
			if err := s.notifier.NotifyApprovalReminder(ctx, req, channels); err == nil {
				s.mu.Lock()
				s.lastReminders[req.ID] = now
				s.mu.Unlock()
			}
		}
	}

	// Clean up old entries
	s.mu.Lock()
	for id := range s.lastReminders {
		found := false
		for _, req := range requests {
			if req.ID == id {
				found = true
				break
			}
		}
		if !found {
			delete(s.lastReminders, id)
		}
	}
	s.mu.Unlock()
}

// processEscalations handles escalation for stale requests.
func (s *ReminderScheduler) processEscalations(ctx context.Context) {
	requests, err := s.storage.ListRequests(ctx, ListOptions{
		State:          RequestStatePending,
		IncludePending: true,
	})
	if err != nil {
		return
	}

	now := time.Now()

	for _, req := range requests {
		// Check if escalation is configured
		escalateAfter := s.getEscalateAfter(req)
		if escalateAfter == 0 {
			continue
		}

		escalateTo := s.getEscalateTo(req)
		if len(escalateTo) == 0 {
			continue
		}

		// Check if escalation time has passed
		if now.Sub(req.CreatedAt) < escalateAfter {
			continue
		}

		// Check if already escalated (stored in metadata)
		if req.Metadata != nil {
			if _, escalated := req.Metadata["escalated"]; escalated {
				continue
			}
		}

		// Perform escalation: add escalation targets to approvers
		// and send notification to escalation targets
		channels := make([]string, 0, len(escalateTo))
		for _, target := range escalateTo {
			// Add as approver if not already
			isApprover := false
			for _, a := range req.Approvers {
				if a == target {
					isApprover = true
					break
				}
			}
			if !isApprover {
				req.Approvers = append(req.Approvers, target)
			}

			// Add to notification channels
			channels = append(channels, target)
		}

		// Mark as escalated
		if req.Metadata == nil {
			req.Metadata = make(map[string]interface{})
		}
		req.Metadata["escalated"] = true
		req.Metadata["escalated_at"] = now.Format(time.RFC3339)
		req.UpdatedAt = now

		// Save updated request
		if err := s.storage.SaveRequest(ctx, req); err != nil {
			continue
		}

		// Send escalation notification
		_ = s.notifier.NotifyApprovalRequest(ctx, req, channels)
	}
}

// processExpirations marks expired requests and sends notifications.
func (s *ReminderScheduler) processExpirations(ctx context.Context) {
	_, _ = s.manager.CheckExpired(ctx)
}

// getNotifyChannels extracts notification channels from request metadata.
func (s *ReminderScheduler) getNotifyChannels(req *Request) []string {
	if req.Metadata == nil {
		return nil
	}

	channels, ok := req.Metadata["notify_channels"]
	if !ok {
		return nil
	}

	switch v := channels.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, c := range v {
			if s, ok := c.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}

	return nil
}

// getReminderInterval gets the reminder interval for a request.
func (s *ReminderScheduler) getReminderInterval(req *Request) time.Duration {
	if req.Metadata == nil {
		return s.defaultInterval
	}

	interval, ok := req.Metadata["reminder_interval"]
	if !ok {
		return s.defaultInterval
	}

	switch v := interval.(type) {
	case string:
		d, err := time.ParseDuration(v)
		if err != nil {
			return s.defaultInterval
		}
		return d
	case float64:
		return time.Duration(v)
	case int64:
		return time.Duration(v)
	}

	return s.defaultInterval
}

// getEscalateAfter gets the escalation delay for a request.
func (s *ReminderScheduler) getEscalateAfter(req *Request) time.Duration {
	if req.Metadata == nil {
		return 0
	}

	after, ok := req.Metadata["escalate_after"]
	if !ok {
		return 0
	}

	switch v := after.(type) {
	case string:
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0
		}
		return d
	case float64:
		return time.Duration(v)
	case int64:
		return time.Duration(v)
	}

	return 0
}

// getEscalateTo gets the escalation targets for a request.
func (s *ReminderScheduler) getEscalateTo(req *Request) []string {
	if req.Metadata == nil {
		return nil
	}

	targets, ok := req.Metadata["escalate_to"]
	if !ok {
		return nil
	}

	switch v := targets.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, t := range v {
			if s, ok := t.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}

	return nil
}
