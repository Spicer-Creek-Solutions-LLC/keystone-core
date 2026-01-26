package events

import (
	"context"
	"fmt"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/audit"
)

// SecurityEventCategory categorizes security-relevant events
type SecurityEventCategory string

const (
	// SecurityCategoryAuth represents authentication-related events
	SecurityCategoryAuth SecurityEventCategory = "auth"

	// SecurityCategoryAccess represents access control events
	SecurityCategoryAccess SecurityEventCategory = "access"

	// SecurityCategoryPolicy represents policy enforcement events
	SecurityCategoryPolicy SecurityEventCategory = "policy"

	// SecurityCategoryCommand represents command execution events
	SecurityCategoryCommand SecurityEventCategory = "command"

	// SecurityCategoryState represents state modification events
	SecurityCategoryState SecurityEventCategory = "state"

	// SecurityCategorySystem represents system-level events
	SecurityCategorySystem SecurityEventCategory = "system"

	// SecurityCategoryAgent represents agent lifecycle events
	SecurityCategoryAgent SecurityEventCategory = "agent"
)

// SecurityEventMapping maps event types to their security category and audit action
type SecurityEventMapping struct {
	// Category is the security category
	Category SecurityEventCategory

	// AuditAction is the corresponding audit action
	AuditAction audit.AuditAction

	// MinSeverity is the minimum severity to audit (events below this are ignored)
	MinSeverity Severity

	// AlwaysAudit means audit regardless of severity
	AlwaysAudit bool
}

// defaultSecurityMappings defines the default event-to-audit mappings
var defaultSecurityMappings = map[EventType]*SecurityEventMapping{
	// Authentication events - always audit
	EventTypeUserLogin: {
		Category:    SecurityCategoryAuth,
		AuditAction: audit.ActionAuthAttempt,
		AlwaysAudit: true,
	},

	// Command execution events - audit all
	EventTypeUserCommand: {
		Category:    SecurityCategoryCommand,
		AuditAction: audit.ActionCommandExecuted,
		AlwaysAudit: true,
	},
	EventTypeJobStart: {
		Category:    SecurityCategoryCommand,
		AuditAction: audit.ActionCommandExecuted,
		MinSeverity: SeverityInfo,
	},
	EventTypeJobComplete: {
		Category:    SecurityCategoryCommand,
		AuditAction: audit.ActionCommandExecuted,
		MinSeverity: SeverityInfo,
	},
	EventTypeJobFail: {
		Category:    SecurityCategoryCommand,
		AuditAction: audit.ActionCommandExecuted,
		AlwaysAudit: true,
	},

	// State modification events
	EventTypeStateApplyStart: {
		Category:    SecurityCategoryState,
		AuditAction: audit.ActionStateApplied,
		MinSeverity: SeverityInfo,
	},
	EventTypeStateApplyDone: {
		Category:    SecurityCategoryState,
		AuditAction: audit.ActionStateApplied,
		MinSeverity: SeverityInfo,
	},
	EventTypeStateApplyFail: {
		Category:    SecurityCategoryState,
		AuditAction: audit.ActionStateApplied,
		AlwaysAudit: true,
	},
	EventTypeStateChange: {
		Category:    SecurityCategoryState,
		AuditAction: audit.ActionStateApplied,
		MinSeverity: SeverityInfo,
	},
	EventTypeStateDrift: {
		Category:    SecurityCategoryState,
		AuditAction: audit.ActionStateApplied,
		MinSeverity: SeverityWarning,
	},

	// Policy events - always audit violations
	EventTypePolicyViolation: {
		Category:    SecurityCategoryPolicy,
		AuditAction: audit.ActionPolicyEvaluated,
		AlwaysAudit: true,
	},
	EventTypePolicyPass: {
		Category:    SecurityCategoryPolicy,
		AuditAction: audit.ActionPolicyEvaluated,
		MinSeverity: SeverityInfo,
	},

	// Agent lifecycle events - audit connects/disconnects
	EventTypeAgentConnect: {
		Category:    SecurityCategoryAgent,
		AuditAction: audit.ActionClusterJoined,
		MinSeverity: SeverityInfo,
	},
	EventTypeAgentDisconnect: {
		Category:    SecurityCategoryAgent,
		AuditAction: audit.ActionClusterLeft,
		MinSeverity: SeverityInfo,
	},
	EventTypeAgentError: {
		Category:    SecurityCategoryAgent,
		AuditAction: audit.ActionClusterLeft,
		AlwaysAudit: true,
	},

	// System events - audit errors
	EventTypeSystemStartup: {
		Category:    SecurityCategorySystem,
		AuditAction: audit.ActionMonitorStarted,
		MinSeverity: SeverityInfo,
	},
	EventTypeSystemShutdown: {
		Category:    SecurityCategorySystem,
		AuditAction: audit.ActionMonitorStarted,
		MinSeverity: SeverityInfo,
	},
	EventTypeSystemError: {
		Category:    SecurityCategorySystem,
		AuditAction: audit.ActionMonitorStarted,
		AlwaysAudit: true,
	},

	// User errors - always audit
	EventTypeUserError: {
		Category:    SecurityCategoryAuth,
		AuditAction: audit.ActionAuthAttempt,
		AlwaysAudit: true,
	},
}

// EventAuditBridge bridges the event system to the audit log
type EventAuditBridge struct {
	auditor     *audit.Auditor
	subscriber  EventSubscriber
	mappings    map[EventType]*SecurityEventMapping
	subscriptions []*Subscription

	// Filters
	includeCategories []SecurityEventCategory
	excludeTypes      []EventType
	minSeverity       Severity

	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// EventAuditBridgeConfig configures the event-audit bridge
type EventAuditBridgeConfig struct {
	// Auditor is the audit logger to use
	Auditor *audit.Auditor

	// Subscriber is the event subscriber
	Subscriber EventSubscriber

	// CustomMappings allows overriding default event-to-audit mappings
	CustomMappings map[EventType]*SecurityEventMapping

	// IncludeCategories filters to only these security categories (empty = all)
	IncludeCategories []SecurityEventCategory

	// ExcludeTypes excludes specific event types from auditing
	ExcludeTypes []EventType

	// MinSeverity sets the minimum severity to audit (overrides per-mapping settings)
	MinSeverity Severity
}

// NewEventAuditBridge creates a new event-to-audit bridge
func NewEventAuditBridge(config *EventAuditBridgeConfig) (*EventAuditBridge, error) {
	if config.Auditor == nil {
		return nil, fmt.Errorf("auditor is required")
	}
	if config.Subscriber == nil {
		return nil, fmt.Errorf("subscriber is required")
	}

	// Start with default mappings
	mappings := make(map[EventType]*SecurityEventMapping)
	for k, v := range defaultSecurityMappings {
		mappings[k] = v
	}

	// Apply custom mappings
	for k, v := range config.CustomMappings {
		mappings[k] = v
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &EventAuditBridge{
		auditor:           config.Auditor,
		subscriber:        config.Subscriber,
		mappings:          mappings,
		includeCategories: config.IncludeCategories,
		excludeTypes:      config.ExcludeTypes,
		minSeverity:       config.MinSeverity,
		ctx:               ctx,
		cancel:            cancel,
	}, nil
}

// Start begins listening for events and forwarding to audit log
func (b *EventAuditBridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return fmt.Errorf("bridge already running")
	}

	// Subscribe to all security-relevant event types
	for eventType := range b.mappings {
		if b.isExcluded(eventType) {
			continue
		}

		// Convert event type to NATS subject pattern
		subject := "events." + string(eventType)

		sub, err := b.subscriber.Subscribe(subject, b.handleEvent)
		if err != nil {
			// Clean up any subscriptions we've already made
			b.cleanupSubscriptions()
			return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
		}

		b.subscriptions = append(b.subscriptions, sub)
	}

	// Also subscribe to wildcard patterns for custom event types
	wildcardSub, err := b.subscriber.Subscribe("events.security.*", b.handleEvent)
	if err == nil {
		b.subscriptions = append(b.subscriptions, wildcardSub)
	}

	b.running = true
	return nil
}

// Stop stops the event-audit bridge
func (b *EventAuditBridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return nil
	}

	b.cancel()
	b.cleanupSubscriptions()
	b.running = false

	return nil
}

// cleanupSubscriptions unsubscribes from all subscriptions
func (b *EventAuditBridge) cleanupSubscriptions() {
	for _, sub := range b.subscriptions {
		if sub != nil && sub.unsubscribe != nil {
			sub.unsubscribe()
		}
	}
	b.subscriptions = nil
}

// handleEvent processes an event and forwards it to the audit log
func (b *EventAuditBridge) handleEvent(event *Event) error {
	// Check if this event type is mapped
	mapping, ok := b.mappings[event.Type]
	if !ok {
		// Check for custom security events (events.security.*)
		mapping = b.getSecurityMapping(event)
		if mapping == nil {
			return nil // Not a security event
		}
	}

	// Check category filter
	if !b.isCategoryIncluded(mapping.Category) {
		return nil
	}

	// Check severity threshold
	if !b.shouldAudit(event, mapping) {
		return nil
	}

	// Convert event to audit entry
	entry := b.eventToAuditEntry(event, mapping)

	// Log the audit entry
	return b.auditor.Log(b.ctx, entry)
}

// eventToAuditEntry converts an event to an audit entry
func (b *EventAuditBridge) eventToAuditEntry(event *Event, mapping *SecurityEventMapping) *audit.AuditEntry {
	entry := b.auditor.StartEntry(mapping.AuditAction, string(event.Type))

	// Copy event fields to audit entry
	entry.Timestamp = event.Time
	entry.CorrelationID = event.CorrelationID

	// Map severity to result
	entry.Result = b.severityToResult(event.Severity)

	// Copy event source to target
	entry.Target = event.Source

	// Copy relevant data fields
	entry.Extra = make(map[string]interface{})
	entry.Extra["event_id"] = event.ID
	entry.Extra["event_type"] = string(event.Type)
	entry.Extra["severity"] = string(event.Severity)
	entry.Extra["security_category"] = string(mapping.Category)

	// Copy tags
	if event.Tags != nil {
		for k, v := range event.Tags {
			entry.Extra["tag_"+k] = v
		}
	}

	// Copy select data fields
	if event.Data != nil {
		b.copySecurityRelevantData(event.Data, entry.Extra)
	}

	// Extract error if present
	if errMsg, ok := event.Data["error"].(string); ok {
		entry.Error = errMsg
	}

	// Extract agent count if present
	if count, ok := event.Data["agent_count"].(float64); ok {
		entry.AgentsMatched = int(count)
	}

	return entry
}

// copySecurityRelevantData copies security-relevant fields from event data
func (b *EventAuditBridge) copySecurityRelevantData(data map[string]interface{}, extra map[string]interface{}) {
	// Fields that are security-relevant and safe to copy
	securityFields := []string{
		"user", "username", "user_id",
		"agent_id", "agent_name",
		"command", "action",
		"resource", "resource_type",
		"policy_id", "policy_name",
		"source_ip", "remote_addr",
		"state_id", "state_name",
		"status", "success", "failed",
		"violation", "violation_type",
		"reason",
	}

	for _, field := range securityFields {
		if val, ok := data[field]; ok {
			extra[field] = val
		}
	}
}

// severityToResult maps event severity to audit result
func (b *EventAuditBridge) severityToResult(severity Severity) audit.AuditResult {
	switch severity {
	case SeverityCritical, SeverityError:
		return audit.ResultFailure
	case SeverityWarning:
		return audit.ResultFailure
	default:
		return audit.ResultSuccess
	}
}

// shouldAudit determines if an event should be audited based on severity
func (b *EventAuditBridge) shouldAudit(event *Event, mapping *SecurityEventMapping) bool {
	// Always audit if configured
	if mapping.AlwaysAudit {
		return true
	}

	// Check global minimum severity
	if b.minSeverity != "" && severityRank(event.Severity) < severityRank(b.minSeverity) {
		return false
	}

	// Check mapping minimum severity
	if mapping.MinSeverity != "" && severityRank(event.Severity) < severityRank(mapping.MinSeverity) {
		return false
	}

	return true
}

// severityRank returns a numeric rank for severity comparison
func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 5
	case SeverityError:
		return 4
	case SeverityWarning:
		return 3
	case SeverityInfo:
		return 2
	case SeverityDebug:
		return 1
	default:
		return 0
	}
}

// isExcluded checks if an event type is excluded
func (b *EventAuditBridge) isExcluded(eventType EventType) bool {
	for _, excluded := range b.excludeTypes {
		if excluded == eventType {
			return true
		}
	}
	return false
}

// isCategoryIncluded checks if a category is included (empty = all included)
func (b *EventAuditBridge) isCategoryIncluded(category SecurityEventCategory) bool {
	if len(b.includeCategories) == 0 {
		return true
	}

	for _, included := range b.includeCategories {
		if included == category {
			return true
		}
	}
	return false
}

// getSecurityMapping returns a mapping for custom security events
func (b *EventAuditBridge) getSecurityMapping(event *Event) *SecurityEventMapping {
	// Check if this is a custom security event
	if category, ok := event.Tags["security_category"]; ok {
		return &SecurityEventMapping{
			Category:    SecurityEventCategory(category),
			AuditAction: audit.ActionConfigChanged, // Default action for custom events
			MinSeverity: SeverityInfo,
		}
	}
	return nil
}

// AddMapping adds or updates an event-to-audit mapping
func (b *EventAuditBridge) AddMapping(eventType EventType, mapping *SecurityEventMapping) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mappings[eventType] = mapping
}

// RemoveMapping removes an event-to-audit mapping
func (b *EventAuditBridge) RemoveMapping(eventType EventType) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.mappings, eventType)
}

// GetMappings returns a copy of all event-to-audit mappings
func (b *EventAuditBridge) GetMappings() map[EventType]*SecurityEventMapping {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[EventType]*SecurityEventMapping)
	for k, v := range b.mappings {
		result[k] = v
	}
	return result
}

// Stats returns statistics about the event-audit bridge
type EventAuditBridgeStats struct {
	Running            bool                                  `json:"running"`
	SubscriptionCount  int                                   `json:"subscription_count"`
	MappingCount       int                                   `json:"mapping_count"`
	IncludeCategories  []SecurityEventCategory               `json:"include_categories,omitempty"`
	ExcludeTypes       []EventType                           `json:"exclude_types,omitempty"`
	MinSeverity        Severity                              `json:"min_severity,omitempty"`
	CategoryCounts     map[SecurityEventCategory]int         `json:"category_counts"`
}

// GetStats returns current statistics
func (b *EventAuditBridge) GetStats() *EventAuditBridgeStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := &EventAuditBridgeStats{
		Running:           b.running,
		SubscriptionCount: len(b.subscriptions),
		MappingCount:      len(b.mappings),
		IncludeCategories: b.includeCategories,
		ExcludeTypes:      b.excludeTypes,
		MinSeverity:       b.minSeverity,
		CategoryCounts:    make(map[SecurityEventCategory]int),
	}

	// Count mappings by category
	for _, mapping := range b.mappings {
		stats.CategoryCounts[mapping.Category]++
	}

	return stats
}
