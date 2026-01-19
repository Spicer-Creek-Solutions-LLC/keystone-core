// Package audit provides identity audit log querying and analysis.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// Errors returned by the audit query system.
var (
	ErrEventNotFound   = errors.New("event not found")
	ErrInvalidQuery    = errors.New("invalid query")
	ErrStoreClosed     = errors.New("store closed")
)

// EventType represents the type of identity event.
type EventType string

const (
	EventTypeLogin          EventType = "login"
	EventTypeLogout         EventType = "logout"
	EventTypeLoginFailed    EventType = "login_failed"
	EventTypePasswordChange EventType = "password_change"
	EventTypePasswordReset  EventType = "password_reset"
	EventTypeMFAEnabled     EventType = "mfa_enabled"
	EventTypeMFADisabled    EventType = "mfa_disabled"
	EventTypeMFAChallenge   EventType = "mfa_challenge"
	EventTypeTokenIssued    EventType = "token_issued"
	EventTypeTokenRevoked   EventType = "token_revoked"
	EventTypeTokenRefreshed EventType = "token_refreshed"
	EventTypeRoleAssigned   EventType = "role_assigned"
	EventTypeRoleRevoked    EventType = "role_revoked"
	EventTypePermGranted    EventType = "permission_granted"
	EventTypePermRevoked    EventType = "permission_revoked"
	EventTypeUserCreated    EventType = "user_created"
	EventTypeUserDeleted    EventType = "user_deleted"
	EventTypeUserUpdated    EventType = "user_updated"
	EventTypeGroupCreated   EventType = "group_created"
	EventTypeGroupDeleted   EventType = "group_deleted"
	EventTypeGroupMemberAdd EventType = "group_member_add"
	EventTypeGroupMemberRem EventType = "group_member_remove"
	EventTypeAPIKeyCreated  EventType = "api_key_created"
	EventTypeAPIKeyRevoked  EventType = "api_key_revoked"
	EventTypeSessionCreated EventType = "session_created"
	EventTypeSessionExpired EventType = "session_expired"
)

// Event represents an identity audit event.
type Event struct {
	// ID is the unique event identifier.
	ID string `json:"id"`
	// Type is the event type.
	Type EventType `json:"type"`
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Actor is who performed the action.
	Actor *Actor `json:"actor"`
	// Target is what was affected.
	Target *Target `json:"target,omitempty"`
	// Source contains origin information.
	Source *Source `json:"source"`
	// Result is the outcome of the action.
	Result Result `json:"result"`
	// Details contains additional event data.
	Details map[string]interface{} `json:"details,omitempty"`
	// Metadata contains system metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Actor represents who performed an action.
type Actor struct {
	// ID is the actor's unique identifier.
	ID string `json:"id"`
	// Type is the actor type (user, service, system).
	Type string `json:"type"`
	// Name is the actor's display name.
	Name string `json:"name,omitempty"`
	// Email is the actor's email.
	Email string `json:"email,omitempty"`
	// Roles are the actor's current roles.
	Roles []string `json:"roles,omitempty"`
}

// Target represents what was affected by an action.
type Target struct {
	// ID is the target's unique identifier.
	ID string `json:"id"`
	// Type is the target type (user, group, role, etc.).
	Type string `json:"type"`
	// Name is the target's display name.
	Name string `json:"name,omitempty"`
}

// Source contains origin information.
type Source struct {
	// IP is the source IP address.
	IP string `json:"ip"`
	// UserAgent is the client user agent.
	UserAgent string `json:"user_agent,omitempty"`
	// Location is the geographic location.
	Location string `json:"location,omitempty"`
	// SessionID is the session identifier.
	SessionID string `json:"session_id,omitempty"`
	// RequestID is the request identifier.
	RequestID string `json:"request_id,omitempty"`
}

// Result represents the outcome of an action.
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
	ResultDenied  Result = "denied"
	ResultError   Result = "error"
)

// Query represents an audit log query.
type Query struct {
	// EventTypes filters by event types.
	EventTypes []EventType
	// ActorIDs filters by actor IDs.
	ActorIDs []string
	// ActorTypes filters by actor types.
	ActorTypes []string
	// TargetIDs filters by target IDs.
	TargetIDs []string
	// TargetTypes filters by target types.
	TargetTypes []string
	// Results filters by result types.
	Results []Result
	// SourceIPs filters by source IPs.
	SourceIPs []string
	// StartTime is the start of the time range.
	StartTime time.Time
	// EndTime is the end of the time range.
	EndTime time.Time
	// TextSearch searches in event details.
	TextSearch string
	// Limit is the maximum number of results.
	Limit int
	// Offset is the number of results to skip.
	Offset int
	// OrderBy is the field to order by.
	OrderBy string
	// OrderDesc orders descending if true.
	OrderDesc bool
}

// QueryResult contains query results.
type QueryResult struct {
	// Events are the matching events.
	Events []*Event
	// Total is the total matching count (before pagination).
	Total int
	// HasMore indicates if there are more results.
	HasMore bool
}

// Store is the interface for audit event storage.
type Store interface {
	// Store stores an event.
	Store(ctx context.Context, event *Event) error
	// Get retrieves an event by ID.
	Get(ctx context.Context, id string) (*Event, error)
	// Query queries events.
	Query(ctx context.Context, query *Query) (*QueryResult, error)
	// Delete deletes an event.
	Delete(ctx context.Context, id string) error
	// DeleteBefore deletes events before a timestamp.
	DeleteBefore(ctx context.Context, before time.Time) (int, error)
	// Close closes the store.
	Close() error
}

// MemoryStore is an in-memory audit store for testing.
type MemoryStore struct {
	events map[string]*Event
	mu     sync.RWMutex
	closed bool
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		events: make(map[string]*Event),
	}
}

// Store stores an event.
func (s *MemoryStore) Store(ctx context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	s.events[event.ID] = event
	return nil
}

// Get retrieves an event by ID.
func (s *MemoryStore) Get(ctx context.Context, id string) (*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	event, exists := s.events[id]
	if !exists {
		return nil, ErrEventNotFound
	}

	return event, nil
}

// Query queries events.
func (s *MemoryStore) Query(ctx context.Context, query *Query) (*QueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var matches []*Event

	for _, event := range s.events {
		if s.matchesQuery(event, query) {
			matches = append(matches, event)
		}
	}

	// Sort
	s.sortEvents(matches, query.OrderBy, query.OrderDesc)

	total := len(matches)

	// Apply pagination
	if query.Offset > 0 {
		if query.Offset >= len(matches) {
			matches = nil
		} else {
			matches = matches[query.Offset:]
		}
	}

	hasMore := false
	if query.Limit > 0 && len(matches) > query.Limit {
		matches = matches[:query.Limit]
		hasMore = true
	}

	return &QueryResult{
		Events:  matches,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *MemoryStore) matchesQuery(event *Event, query *Query) bool {
	// Event type filter
	if len(query.EventTypes) > 0 {
		match := false
		for _, t := range query.EventTypes {
			if event.Type == t {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Actor ID filter
	if len(query.ActorIDs) > 0 && event.Actor != nil {
		match := false
		for _, id := range query.ActorIDs {
			if event.Actor.ID == id {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Actor type filter
	if len(query.ActorTypes) > 0 && event.Actor != nil {
		match := false
		for _, t := range query.ActorTypes {
			if event.Actor.Type == t {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Target ID filter
	if len(query.TargetIDs) > 0 {
		if event.Target == nil {
			return false
		}
		match := false
		for _, id := range query.TargetIDs {
			if event.Target.ID == id {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Target type filter
	if len(query.TargetTypes) > 0 {
		if event.Target == nil {
			return false
		}
		match := false
		for _, t := range query.TargetTypes {
			if event.Target.Type == t {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Result filter
	if len(query.Results) > 0 {
		match := false
		for _, r := range query.Results {
			if event.Result == r {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Source IP filter
	if len(query.SourceIPs) > 0 && event.Source != nil {
		match := false
		for _, ip := range query.SourceIPs {
			if event.Source.IP == ip {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Time range filter
	if !query.StartTime.IsZero() && event.Timestamp.Before(query.StartTime) {
		return false
	}
	if !query.EndTime.IsZero() && event.Timestamp.After(query.EndTime) {
		return false
	}

	// Text search
	if query.TextSearch != "" {
		if !s.matchesTextSearch(event, query.TextSearch) {
			return false
		}
	}

	return true
}

func (s *MemoryStore) matchesTextSearch(event *Event, search string) bool {
	search = strings.ToLower(search)

	// Check event type
	if strings.Contains(strings.ToLower(string(event.Type)), search) {
		return true
	}

	// Check actor
	if event.Actor != nil {
		if strings.Contains(strings.ToLower(event.Actor.ID), search) ||
			strings.Contains(strings.ToLower(event.Actor.Name), search) ||
			strings.Contains(strings.ToLower(event.Actor.Email), search) {
			return true
		}
	}

	// Check target
	if event.Target != nil {
		if strings.Contains(strings.ToLower(event.Target.ID), search) ||
			strings.Contains(strings.ToLower(event.Target.Name), search) {
			return true
		}
	}

	// Check source
	if event.Source != nil {
		if strings.Contains(strings.ToLower(event.Source.IP), search) ||
			strings.Contains(strings.ToLower(event.Source.SessionID), search) ||
			strings.Contains(strings.ToLower(event.Source.RequestID), search) ||
			strings.Contains(strings.ToLower(event.Source.UserAgent), search) {
			return true
		}
	}

	// Check details
	if event.Details != nil {
		data, _ := json.Marshal(event.Details)
		if strings.Contains(strings.ToLower(string(data)), search) {
			return true
		}
	}

	return false
}

func (s *MemoryStore) sortEvents(events []*Event, orderBy string, desc bool) {
	if orderBy == "" {
		orderBy = "timestamp"
	}

	sort.Slice(events, func(i, j int) bool {
		var less bool

		switch orderBy {
		case "timestamp":
			less = events[i].Timestamp.Before(events[j].Timestamp)
		case "type":
			less = events[i].Type < events[j].Type
		case "actor":
			if events[i].Actor != nil && events[j].Actor != nil {
				less = events[i].Actor.ID < events[j].Actor.ID
			}
		case "result":
			less = events[i].Result < events[j].Result
		default:
			less = events[i].Timestamp.Before(events[j].Timestamp)
		}

		if desc {
			return !less
		}
		return less
	})
}

// Delete deletes an event.
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	delete(s.events, id)
	return nil
}

// DeleteBefore deletes events before a timestamp.
func (s *MemoryStore) DeleteBefore(ctx context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	count := 0
	for id, event := range s.events {
		if event.Timestamp.Before(before) {
			delete(s.events, id)
			count++
		}
	}

	return count, nil
}

// Close closes the store.
func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	return nil
}

// Count returns the number of events.
func (s *MemoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// Analyzer provides audit log analysis capabilities.
type Analyzer struct {
	store Store
}

// NewAnalyzer creates a new audit analyzer.
func NewAnalyzer(store Store) *Analyzer {
	return &Analyzer{store: store}
}

// LoginActivity returns login activity for a user.
func (a *Analyzer) LoginActivity(ctx context.Context, userID string, since time.Time) (*LoginActivityReport, error) {
	query := &Query{
		ActorIDs:   []string{userID},
		EventTypes: []EventType{EventTypeLogin, EventTypeLoginFailed, EventTypeLogout},
		StartTime:  since,
		OrderBy:    "timestamp",
		OrderDesc:  true,
	}

	result, err := a.store.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	report := &LoginActivityReport{
		UserID: userID,
		Since:  since,
	}

	for _, event := range result.Events {
		switch event.Type {
		case EventTypeLogin:
			report.SuccessfulLogins++
			if report.LastLogin.IsZero() || event.Timestamp.After(report.LastLogin) {
				report.LastLogin = event.Timestamp
			}
		case EventTypeLoginFailed:
			report.FailedLogins++
			if report.LastFailedLogin.IsZero() || event.Timestamp.After(report.LastFailedLogin) {
				report.LastFailedLogin = event.Timestamp
			}
		case EventTypeLogout:
			report.Logouts++
		}

		if event.Source != nil {
			report.UniqueIPs = appendUnique(report.UniqueIPs, event.Source.IP)
		}
	}

	return report, nil
}

// LoginActivityReport contains login activity analysis.
type LoginActivityReport struct {
	UserID           string
	Since            time.Time
	SuccessfulLogins int
	FailedLogins     int
	Logouts          int
	LastLogin        time.Time
	LastFailedLogin  time.Time
	UniqueIPs        []string
}

// PermissionChanges returns permission change history for a target.
func (a *Analyzer) PermissionChanges(ctx context.Context, targetID string, since time.Time) ([]*Event, error) {
	query := &Query{
		TargetIDs: []string{targetID},
		EventTypes: []EventType{
			EventTypeRoleAssigned,
			EventTypeRoleRevoked,
			EventTypePermGranted,
			EventTypePermRevoked,
		},
		StartTime: since,
		OrderBy:   "timestamp",
		OrderDesc: true,
	}

	result, err := a.store.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	return result.Events, nil
}

// SuspiciousActivity detects suspicious activity patterns.
func (a *Analyzer) SuspiciousActivity(ctx context.Context, since time.Time) ([]*SuspiciousEvent, error) {
	// Query failed logins
	query := &Query{
		EventTypes: []EventType{EventTypeLoginFailed},
		StartTime:  since,
	}

	result, err := a.store.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	// Group by actor
	actorFailures := make(map[string][]*Event)
	for _, event := range result.Events {
		if event.Actor != nil {
			actorFailures[event.Actor.ID] = append(actorFailures[event.Actor.ID], event)
		}
	}

	var suspicious []*SuspiciousEvent

	// Check for brute force attempts (5+ failures in time window)
	for actorID, failures := range actorFailures {
		if len(failures) >= 5 {
			suspicious = append(suspicious, &SuspiciousEvent{
				Type:        "brute_force_attempt",
				ActorID:     actorID,
				Description: "Multiple failed login attempts detected",
				Events:      failures,
				Severity:    "high",
			})
		}
	}

	// Group by IP for distributed attacks
	ipFailures := make(map[string][]*Event)
	for _, event := range result.Events {
		if event.Source != nil && event.Source.IP != "" {
			ipFailures[event.Source.IP] = append(ipFailures[event.Source.IP], event)
		}
	}

	for ip, failures := range ipFailures {
		if len(failures) >= 10 {
			suspicious = append(suspicious, &SuspiciousEvent{
				Type:        "distributed_attack",
				SourceIP:    ip,
				Description: "Multiple failed logins from same IP",
				Events:      failures,
				Severity:    "high",
			})
		}
	}

	return suspicious, nil
}

// SuspiciousEvent represents a suspicious activity pattern.
type SuspiciousEvent struct {
	Type        string
	ActorID     string
	SourceIP    string
	Description string
	Events      []*Event
	Severity    string
}

// SessionReport generates a session activity report.
func (a *Analyzer) SessionReport(ctx context.Context, sessionID string) (*SessionActivityReport, error) {
	query := &Query{
		TextSearch: sessionID,
		OrderBy:    "timestamp",
	}

	result, err := a.store.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	report := &SessionActivityReport{
		SessionID:   sessionID,
		TotalEvents: len(result.Events),
		Events:      result.Events,
	}

	for _, event := range result.Events {
		if report.StartTime.IsZero() || event.Timestamp.Before(report.StartTime) {
			report.StartTime = event.Timestamp
		}
		if event.Timestamp.After(report.EndTime) {
			report.EndTime = event.Timestamp
		}

		switch event.Type {
		case EventTypeSessionCreated:
			report.SessionCreated = true
		case EventTypeSessionExpired:
			report.SessionExpired = true
		}
	}

	if !report.StartTime.IsZero() && !report.EndTime.IsZero() {
		report.Duration = report.EndTime.Sub(report.StartTime)
	}

	return report, nil
}

// SessionActivityReport contains session activity analysis.
type SessionActivityReport struct {
	SessionID      string
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	TotalEvents    int
	SessionCreated bool
	SessionExpired bool
	Events         []*Event
}

// Summary generates a summary of audit events.
func (a *Analyzer) Summary(ctx context.Context, since time.Time) (*AuditSummary, error) {
	query := &Query{
		StartTime: since,
	}

	result, err := a.store.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	summary := &AuditSummary{
		Since:       since,
		TotalEvents: result.Total,
		ByType:      make(map[EventType]int),
		ByResult:    make(map[Result]int),
		ByActorType: make(map[string]int),
	}

	for _, event := range result.Events {
		summary.ByType[event.Type]++
		summary.ByResult[event.Result]++

		if event.Actor != nil {
			summary.ByActorType[event.Actor.Type]++
			summary.UniqueActors = appendUnique(summary.UniqueActors, event.Actor.ID)
		}
	}

	return summary, nil
}

// AuditSummary contains a summary of audit events.
type AuditSummary struct {
	Since        time.Time
	TotalEvents  int
	ByType       map[EventType]int
	ByResult     map[Result]int
	ByActorType  map[string]int
	UniqueActors []string
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
