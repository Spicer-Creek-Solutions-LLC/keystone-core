// Package trigger provides event-driven triggering of runbooks.
// This file implements ITSM integrations (PagerDuty, Opsgenie).
package trigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// ITSMType represents the type of ITSM integration.
type ITSMType string

const (
	// ITSMTypePagerDuty is PagerDuty integration.
	ITSMTypePagerDuty ITSMType = "pagerduty"

	// ITSMTypeOpsgenie is Opsgenie integration.
	ITSMTypeOpsgenie ITSMType = "opsgenie"

	// ITSMTypeServiceNow is ServiceNow integration.
	ITSMTypeServiceNow ITSMType = "servicenow"
)

// ITSMTrigger represents an ITSM-triggered runbook.
type ITSMTrigger struct {
	// ID is the unique trigger identifier.
	ID string `yaml:"id" json:"id"`

	// Name is a human-readable name.
	Name string `yaml:"name" json:"name"`

	// Description explains what this trigger does.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Type is the ITSM type (pagerduty, opsgenie).
	Type ITSMType `yaml:"type" json:"type"`

	// RunbookRef references the runbook to execute.
	RunbookRef RunbookRef `yaml:"runbook" json:"runbook"`

	// ServiceFilter filters incidents by service.
	ServiceFilter []string `yaml:"serviceFilter,omitempty" json:"service_filter,omitempty"`

	// SeverityFilter filters incidents by severity.
	SeverityFilter []string `yaml:"severityFilter,omitempty" json:"severity_filter,omitempty"`

	// EventTypes are the incident event types to trigger on.
	EventTypes []string `yaml:"eventTypes,omitempty" json:"event_types,omitempty"`

	// InputMappings maps incident data to runbook inputs.
	InputMappings map[string]string `yaml:"inputMappings,omitempty" json:"input_mappings,omitempty"`

	// UpdateIncident enables automatic incident updates.
	UpdateIncident bool `yaml:"updateIncident,omitempty" json:"update_incident,omitempty"`

	// AcknowledgeOnStart acknowledges the incident when runbook starts.
	AcknowledgeOnStart bool `yaml:"acknowledgeOnStart,omitempty" json:"acknowledge_on_start,omitempty"`

	// ResolveOnSuccess resolves the incident when runbook succeeds.
	ResolveOnSuccess bool `yaml:"resolveOnSuccess,omitempty" json:"resolve_on_success,omitempty"`

	// Enabled indicates if the trigger is active.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Tags for categorization.
	Tags map[string]string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// CreatedAt is when the trigger was created.
	CreatedAt time.Time `yaml:"createdAt,omitempty" json:"created_at,omitempty"`

	// UpdatedAt is when the trigger was last updated.
	UpdatedAt time.Time `yaml:"updatedAt,omitempty" json:"updated_at,omitempty"`
}

// Validate validates the ITSM trigger.
func (t *ITSMTrigger) Validate() error {
	if t.ID == "" {
		return &ValidationError{Field: "id", Message: "id is required"}
	}
	if t.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if t.Type == "" {
		return &ValidationError{Field: "type", Message: "type is required"}
	}
	if t.Type != ITSMTypePagerDuty && t.Type != ITSMTypeOpsgenie {
		return &ValidationError{Field: "type", Message: "type must be 'pagerduty' or 'opsgenie'"}
	}
	if t.RunbookRef.Name == "" {
		return &ValidationError{Field: "runbook.name", Message: "runbook name is required"}
	}
	return nil
}

// Incident represents a normalized incident from any ITSM.
type Incident struct {
	// ID is the incident identifier.
	ID string `json:"id"`

	// ExternalID is the ITSM-specific ID.
	ExternalID string `json:"external_id"`

	// Source is the ITSM type.
	Source ITSMType `json:"source"`

	// Title is the incident title.
	Title string `json:"title"`

	// Description is the incident description.
	Description string `json:"description,omitempty"`

	// Status is the incident status.
	Status IncidentStatus `json:"status"`

	// Severity is the incident severity.
	Severity IncidentSeverity `json:"severity"`

	// Service is the affected service.
	Service string `json:"service,omitempty"`

	// ServiceID is the service identifier.
	ServiceID string `json:"service_id,omitempty"`

	// Assignee is the assigned user.
	Assignee string `json:"assignee,omitempty"`

	// URL is the incident URL.
	URL string `json:"url,omitempty"`

	// CreatedAt is when the incident was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the incident was last updated.
	UpdatedAt time.Time `json:"updated_at,omitempty"`

	// RawData contains the original ITSM data.
	RawData map[string]interface{} `json:"raw_data,omitempty"`
}

// IncidentStatus represents incident status.
type IncidentStatus string

// IncidentStatusTriggered constants define the possible statuses.
const (
	IncidentStatusTriggered    IncidentStatus = "triggered"
	IncidentStatusAcknowledged IncidentStatus = "acknowledged"
	IncidentStatusResolved     IncidentStatus = "resolved"
)

// IncidentSeverity represents incident severity.
type IncidentSeverity string

// IncidentSeverity constants define the severity levels.
const (
	IncidentSeverityCritical IncidentSeverity = "critical"
	IncidentSeverityHigh     IncidentSeverity = "high"
	IncidentSeverityMedium   IncidentSeverity = "medium"
	IncidentSeverityLow      IncidentSeverity = "low"
	IncidentSeverityInfo     IncidentSeverity = "info"
)

// IncidentUpdate represents an update to send to an ITSM.
type IncidentUpdate struct {
	IncidentID string            `json:"incident_id"`
	Note       string            `json:"note,omitempty"`
	Status     IncidentStatus    `json:"status,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// ITSMClient is the interface for ITSM integrations.
type ITSMClient interface {
	// Type returns the ITSM type.
	Type() ITSMType

	// ParseWebhook parses a webhook payload into an Incident.
	ParseWebhook(body []byte) (*Incident, error)

	// AcknowledgeIncident acknowledges an incident.
	AcknowledgeIncident(ctx context.Context, incidentID string) error

	// ResolveIncident resolves an incident.
	ResolveIncident(ctx context.Context, incidentID string) error

	// AddNote adds a note to an incident.
	AddNote(ctx context.Context, incidentID, note string) error

	// GetIncident retrieves incident details.
	GetIncident(ctx context.Context, incidentID string) (*Incident, error)
}

// PagerDutyClient implements ITSMClient for PagerDuty.
type PagerDutyClient struct {
	apiKey     string
	email      string // Required for some API calls
	httpClient *http.Client
	baseURL    string
}

// PagerDutyConfig configures the PagerDuty client.
type PagerDutyConfig struct {
	APIKey  string `yaml:"apiKey" json:"api_key"`
	Email   string `yaml:"email" json:"email"`
	BaseURL string `yaml:"baseURL,omitempty" json:"base_url,omitempty"`
}

// NewPagerDutyClient creates a new PagerDuty client.
func NewPagerDutyClient(config *PagerDutyConfig) *PagerDutyClient {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.pagerduty.com"
	}

	return &PagerDutyClient{
		apiKey:     config.APIKey,
		email:      config.Email,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

// Type returns the ITSM type.
func (c *PagerDutyClient) Type() ITSMType {
	return ITSMTypePagerDuty
}

// ParseWebhook parses a PagerDuty webhook payload.
func (c *PagerDutyClient) ParseWebhook(body []byte) (*Incident, error) {
	var payload pagerDutyWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse webhook: %w", err)
	}

	if len(payload.Messages) == 0 {
		return nil, fmt.Errorf("no messages in webhook")
	}

	msg := payload.Messages[0]
	incident := msg.Incident

	return &Incident{
		ID:          uuid.New().String(),
		ExternalID:  incident.ID,
		Source:      ITSMTypePagerDuty,
		Title:       incident.Title,
		Description: incident.Description,
		Status:      mapPagerDutyStatus(incident.Status),
		Severity:    mapPagerDutyUrgency(incident.Urgency),
		Service:     incident.Service.Name,
		ServiceID:   incident.Service.ID,
		URL:         incident.HTMLURL,
		CreatedAt:   incident.CreatedAt,
		RawData: map[string]interface{}{
			"incident": incident,
			"event":    msg.Event,
		},
	}, nil
}

// pagerDutyWebhook represents a PagerDuty webhook payload.
type pagerDutyWebhook struct {
	Messages []pagerDutyMessage `json:"messages"`
}

type pagerDutyMessage struct {
	Event    string            `json:"event"`
	Incident pagerDutyIncident `json:"incident"`
}

type pagerDutyIncident struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Status      string           `json:"status"`
	Urgency     string           `json:"urgency"`
	HTMLURL     string           `json:"html_url"`
	CreatedAt   time.Time        `json:"created_at"`
	Service     pagerDutyService `json:"service"`
}

type pagerDutyService struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func mapPagerDutyStatus(status string) IncidentStatus {
	switch status {
	case "triggered":
		return IncidentStatusTriggered
	case "acknowledged":
		return IncidentStatusAcknowledged
	case "resolved":
		return IncidentStatusResolved
	default:
		return IncidentStatusTriggered
	}
}

func mapPagerDutyUrgency(urgency string) IncidentSeverity {
	switch urgency {
	case "high":
		return IncidentSeverityHigh
	case "low":
		return IncidentSeverityLow
	default:
		return IncidentSeverityMedium
	}
}

// AcknowledgeIncident acknowledges a PagerDuty incident.
func (c *PagerDutyClient) AcknowledgeIncident(ctx context.Context, incidentID string) error {
	return c.updateIncidentStatus(ctx, incidentID, "acknowledged")
}

// ResolveIncident resolves a PagerDuty incident.
func (c *PagerDutyClient) ResolveIncident(ctx context.Context, incidentID string) error {
	return c.updateIncidentStatus(ctx, incidentID, "resolved")
}

func (c *PagerDutyClient) updateIncidentStatus(ctx context.Context, incidentID, status string) error {
	payload := map[string]interface{}{
		"incident": map[string]interface{}{
			"type":   "incident_reference",
			"status": status,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/incidents/%s", c.baseURL, incidentID)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Token token="+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("From", c.email)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PagerDuty API error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// AddNote adds a note to a PagerDuty incident.
func (c *PagerDutyClient) AddNote(ctx context.Context, incidentID, note string) error {
	payload := map[string]interface{}{
		"note": map[string]interface{}{
			"content": note,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/incidents/%s/notes", c.baseURL, incidentID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Token token="+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("From", c.email)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PagerDuty API error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// GetIncident retrieves a PagerDuty incident.
func (c *PagerDutyClient) GetIncident(ctx context.Context, incidentID string) (*Incident, error) {
	url := fmt.Sprintf("%s/incidents/%s", c.baseURL, incidentID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Token token="+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PagerDuty API error: %s - %s", resp.Status, string(respBody))
	}

	var result struct {
		Incident pagerDutyIncident `json:"incident"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &Incident{
		ID:          uuid.New().String(),
		ExternalID:  result.Incident.ID,
		Source:      ITSMTypePagerDuty,
		Title:       result.Incident.Title,
		Description: result.Incident.Description,
		Status:      mapPagerDutyStatus(result.Incident.Status),
		Severity:    mapPagerDutyUrgency(result.Incident.Urgency),
		Service:     result.Incident.Service.Name,
		ServiceID:   result.Incident.Service.ID,
		URL:         result.Incident.HTMLURL,
		CreatedAt:   result.Incident.CreatedAt,
	}, nil
}

// OpsgenieClient implements ITSMClient for Opsgenie.
type OpsgenieClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// OpsgenieConfig configures the Opsgenie client.
type OpsgenieConfig struct {
	APIKey  string `yaml:"apiKey" json:"api_key"`
	BaseURL string `yaml:"baseURL,omitempty" json:"base_url,omitempty"`
}

// NewOpsgenieClient creates a new Opsgenie client.
func NewOpsgenieClient(config *OpsgenieConfig) *OpsgenieClient {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.opsgenie.com/v2"
	}

	return &OpsgenieClient{
		apiKey:     config.APIKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

// Type returns the ITSM type.
func (c *OpsgenieClient) Type() ITSMType {
	return ITSMTypeOpsgenie
}

// ParseWebhook parses an Opsgenie webhook payload.
func (c *OpsgenieClient) ParseWebhook(body []byte) (*Incident, error) {
	var payload opsgenieWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse webhook: %w", err)
	}

	alert := payload.Alert
	return &Incident{
		ID:          uuid.New().String(),
		ExternalID:  alert.AlertID,
		Source:      ITSMTypeOpsgenie,
		Title:       alert.Message,
		Description: alert.Description,
		Status:      mapOpsgenieStatus(payload.Action),
		Severity:    mapOpsgeniePriority(alert.Priority),
		Service:     alert.Source,
		URL:         fmt.Sprintf("https://app.opsgenie.com/alert/detail/%s", alert.AlertID),
		CreatedAt:   alert.CreatedAt,
		RawData: map[string]interface{}{
			"alert":  alert,
			"action": payload.Action,
		},
	}, nil
}

// opsgenieWebhook represents an Opsgenie webhook payload.
type opsgenieWebhook struct {
	Action string        `json:"action"`
	Alert  opsgenieAlert `json:"alert"`
}

type opsgenieAlert struct {
	AlertID     string    `json:"alertId"`
	Message     string    `json:"message"`
	Description string    `json:"description"`
	Priority    string    `json:"priority"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"createdAt"`
}

func mapOpsgenieStatus(action string) IncidentStatus {
	switch action {
	case "Create", "Escalate":
		return IncidentStatusTriggered
	case "Acknowledge":
		return IncidentStatusAcknowledged
	case "Close", "Resolve":
		return IncidentStatusResolved
	default:
		return IncidentStatusTriggered
	}
}

func mapOpsgeniePriority(priority string) IncidentSeverity {
	switch priority {
	case "P1":
		return IncidentSeverityCritical
	case "P2":
		return IncidentSeverityHigh
	case "P3":
		return IncidentSeverityMedium
	case "P4":
		return IncidentSeverityLow
	case "P5":
		return IncidentSeverityInfo
	default:
		return IncidentSeverityMedium
	}
}

// AcknowledgeIncident acknowledges an Opsgenie alert.
func (c *OpsgenieClient) AcknowledgeIncident(ctx context.Context, alertID string) error {
	url := fmt.Sprintf("%s/alerts/%s/acknowledge", c.baseURL, alertID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "GenieKey "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opsgenie API error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// ResolveIncident closes an Opsgenie alert.
func (c *OpsgenieClient) ResolveIncident(ctx context.Context, alertID string) error {
	url := fmt.Sprintf("%s/alerts/%s/close", c.baseURL, alertID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "GenieKey "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opsgenie API error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// AddNote adds a note to an Opsgenie alert.
func (c *OpsgenieClient) AddNote(ctx context.Context, alertID, note string) error {
	payload := map[string]interface{}{
		"note": note,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/alerts/%s/notes", c.baseURL, alertID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "GenieKey "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opsgenie API error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// GetIncident retrieves an Opsgenie alert.
func (c *OpsgenieClient) GetIncident(ctx context.Context, alertID string) (*Incident, error) {
	url := fmt.Sprintf("%s/alerts/%s", c.baseURL, alertID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "GenieKey "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opsgenie API error: %s - %s", resp.Status, string(respBody))
	}

	var result struct {
		Data opsgenieAlert `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &Incident{
		ID:          uuid.New().String(),
		ExternalID:  result.Data.AlertID,
		Source:      ITSMTypeOpsgenie,
		Title:       result.Data.Message,
		Description: result.Data.Description,
		Severity:    mapOpsgeniePriority(result.Data.Priority),
		Service:     result.Data.Source,
		CreatedAt:   result.Data.CreatedAt,
	}, nil
}

// ITSMTriggerManager manages ITSM-based triggers.
type ITSMTriggerManager struct {
	mu       sync.RWMutex
	triggers map[string]*ITSMTrigger

	clients    map[ITSMType]ITSMClient
	repository RunbookRepository
	executor   RunbookExecutor
	publisher  events.EventPublisher

	// Incident tracking
	incidentLinks map[string]string // incident ID -> execution ID
}

// ITSMManagerOption configures an ITSMTriggerManager.
type ITSMManagerOption func(*ITSMTriggerManager)

// WithITSMRepository sets the runbook repository.
func WithITSMRepository(repo RunbookRepository) ITSMManagerOption {
	return func(m *ITSMTriggerManager) {
		m.repository = repo
	}
}

// WithITSMExecutor sets the runbook executor.
func WithITSMExecutor(executor RunbookExecutor) ITSMManagerOption {
	return func(m *ITSMTriggerManager) {
		m.executor = executor
	}
}

// WithITSMPublisher sets the event publisher.
func WithITSMPublisher(publisher events.EventPublisher) ITSMManagerOption {
	return func(m *ITSMTriggerManager) {
		m.publisher = publisher
	}
}

// WithITSMClient adds an ITSM client.
func WithITSMClient(client ITSMClient) ITSMManagerOption {
	return func(m *ITSMTriggerManager) {
		m.clients[client.Type()] = client
	}
}

// NewITSMTriggerManager creates a new ITSM trigger manager.
func NewITSMTriggerManager(opts ...ITSMManagerOption) *ITSMTriggerManager {
	m := &ITSMTriggerManager{
		triggers:      make(map[string]*ITSMTrigger),
		clients:       make(map[ITSMType]ITSMClient),
		incidentLinks: make(map[string]string),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Register adds an ITSM trigger.
func (m *ITSMTriggerManager) Register(trigger *ITSMTrigger) error {
	if err := trigger.Validate(); err != nil {
		return fmt.Errorf("invalid trigger: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.triggers[trigger.ID]; exists {
		return fmt.Errorf("trigger %s already registered", trigger.ID)
	}

	now := time.Now()
	if trigger.CreatedAt.IsZero() {
		trigger.CreatedAt = now
	}
	trigger.UpdatedAt = now

	m.triggers[trigger.ID] = trigger
	return nil
}

// Unregister removes an ITSM trigger.
func (m *ITSMTriggerManager) Unregister(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.triggers[id]; !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	delete(m.triggers, id)
	return nil
}

// Get retrieves an ITSM trigger by ID.
func (m *ITSMTriggerManager) Get(id string) (*ITSMTrigger, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trigger, ok := m.triggers[id]
	return trigger, ok
}

// List returns all ITSM triggers.
func (m *ITSMTriggerManager) List() []*ITSMTrigger {
	m.mu.RLock()
	defer m.mu.RUnlock()

	triggers := make([]*ITSMTrigger, 0, len(m.triggers))
	for _, t := range m.triggers {
		triggers = append(triggers, t)
	}
	return triggers
}

// Enable enables an ITSM trigger.
func (m *ITSMTriggerManager) Enable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	trigger.Enabled = true
	trigger.UpdatedAt = time.Now()
	return nil
}

// Disable disables an ITSM trigger.
func (m *ITSMTriggerManager) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	trigger.Enabled = false
	trigger.UpdatedAt = time.Now()
	return nil
}

// HandleWebhook processes an ITSM webhook.
func (m *ITSMTriggerManager) HandleWebhook(ctx context.Context, itsmType ITSMType, body []byte) error {
	client, ok := m.clients[itsmType]
	if !ok {
		return fmt.Errorf("no client configured for %s", itsmType)
	}

	// Parse incident
	incident, err := client.ParseWebhook(body)
	if err != nil {
		return fmt.Errorf("parse webhook: %w", err)
	}

	// Find matching triggers
	m.mu.RLock()
	var matchingTriggers []*ITSMTrigger
	for _, trigger := range m.triggers {
		if trigger.Enabled && trigger.Type == itsmType && m.matchesTrigger(trigger, incident) {
			matchingTriggers = append(matchingTriggers, trigger)
		}
	}
	m.mu.RUnlock()

	// Execute matching triggers
	for _, trigger := range matchingTriggers {
		if err := m.executeTrigger(ctx, trigger, incident, client); err != nil {
			// Log error but continue with other triggers
			if m.publisher != nil {
				_ = m.publisher.Publish(&events.Event{
					ID:     uuid.New().String(),
					Type:   events.EventType("runbook.itsm.error"),
					Source: "/runbook/itsm/" + trigger.ID,
					Time:   time.Now(),
					Data: map[string]interface{}{
						"trigger_id":  trigger.ID,
						"incident_id": incident.ExternalID,
						"error":       err.Error(),
					},
				})
			}
		}
	}

	return nil
}

// matchesTrigger checks if an incident matches a trigger's filters.
func (m *ITSMTriggerManager) matchesTrigger(trigger *ITSMTrigger, incident *Incident) bool {
	// Check service filter
	if len(trigger.ServiceFilter) > 0 {
		matched := false
		for _, svc := range trigger.ServiceFilter {
			if svc == incident.Service || svc == incident.ServiceID {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check severity filter
	if len(trigger.SeverityFilter) > 0 {
		matched := false
		for _, sev := range trigger.SeverityFilter {
			if IncidentSeverity(sev) == incident.Severity {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// executeTrigger executes a trigger for an incident.
func (m *ITSMTriggerManager) executeTrigger(ctx context.Context, trigger *ITSMTrigger, incident *Incident, client ITSMClient) error {
	// Publish start event
	if m.publisher != nil {
		_ = m.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.itsm.triggered"),
			Source: "/runbook/itsm/" + trigger.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id":     trigger.ID,
				"trigger_name":   trigger.Name,
				"incident_id":    incident.ExternalID,
				"incident_title": incident.Title,
			},
		})
	}

	// Acknowledge incident if configured
	if trigger.AcknowledgeOnStart && incident.Status == IncidentStatusTriggered {
		if err := client.AcknowledgeIncident(ctx, incident.ExternalID); err != nil {
			// Log but continue
			_ = err
		}
	}

	// Get runbook
	if m.repository == nil || m.executor == nil {
		return fmt.Errorf("repository or executor not configured")
	}

	rb, err := m.repository.GetRunbook(trigger.RunbookRef.Name, trigger.RunbookRef.Version)
	if err != nil {
		return fmt.Errorf("get runbook: %w", err)
	}

	// Build inputs
	inputs := m.buildInputs(trigger, incident)

	// Add note about runbook execution
	if trigger.UpdateIncident {
		note := fmt.Sprintf("Keystone runbook '%s' started for this incident.", trigger.RunbookRef.Name)
		_ = client.AddNote(ctx, incident.ExternalID, note)
	}

	// Execute runbook
	exec, err := m.executor.Execute(rb, inputs)
	if err != nil {
		if trigger.UpdateIncident {
			note := fmt.Sprintf("Keystone runbook '%s' failed: %s", trigger.RunbookRef.Name, err.Error())
			_ = client.AddNote(ctx, incident.ExternalID, note)
		}
		return fmt.Errorf("execute runbook: %w", err)
	}

	// Link incident to execution
	m.mu.Lock()
	m.incidentLinks[incident.ExternalID] = exec.ID
	m.mu.Unlock()

	// Handle completion
	if exec.State == runbook.ExecutionStateFailed {
		if trigger.UpdateIncident {
			note := fmt.Sprintf("Keystone runbook '%s' completed with error: %s", trigger.RunbookRef.Name, exec.Error)
			_ = client.AddNote(ctx, incident.ExternalID, note)
		}
		return fmt.Errorf("runbook failed: %s", exec.Error)
	}

	// Update incident on success
	if trigger.UpdateIncident {
		note := fmt.Sprintf("Keystone runbook '%s' completed successfully.", trigger.RunbookRef.Name)
		_ = client.AddNote(ctx, incident.ExternalID, note)
	}

	// Resolve incident if configured
	if trigger.ResolveOnSuccess {
		if err := client.ResolveIncident(ctx, incident.ExternalID); err != nil {
			// Log but don't fail
			_ = err
		}
	}

	// Publish completion event
	if m.publisher != nil {
		_ = m.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.itsm.completed"),
			Source: "/runbook/itsm/" + trigger.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id":   trigger.ID,
				"trigger_name": trigger.Name,
				"incident_id":  incident.ExternalID,
				"execution_id": exec.ID,
				"success":      true,
			},
		})
	}

	return nil
}

// buildInputs builds runbook inputs from incident data.
func (m *ITSMTriggerManager) buildInputs(trigger *ITSMTrigger, incident *Incident) map[string]interface{} {
	inputs := make(map[string]interface{})

	// Add incident metadata
	inputs["__incident_id"] = incident.ExternalID
	inputs["__incident_source"] = string(incident.Source)
	inputs["__trigger_type"] = "itsm"
	inputs["incident_id"] = incident.ExternalID
	inputs["incident_title"] = incident.Title
	inputs["incident_description"] = incident.Description
	inputs["incident_status"] = string(incident.Status)
	inputs["incident_severity"] = string(incident.Severity)
	inputs["incident_service"] = incident.Service
	inputs["incident_url"] = incident.URL

	// Apply input mappings
	for inputName, source := range trigger.InputMappings {
		switch source {
		case "{{ .incident.id }}":
			inputs[inputName] = incident.ExternalID
		case "{{ .incident.title }}":
			inputs[inputName] = incident.Title
		case "{{ .incident.description }}":
			inputs[inputName] = incident.Description
		case "{{ .incident.service }}":
			inputs[inputName] = incident.Service
		case "{{ .incident.severity }}":
			inputs[inputName] = string(incident.Severity)
		default:
			inputs[inputName] = source
		}
	}

	return inputs
}

// GetLinkedExecution returns the execution ID for an incident.
func (m *ITSMTriggerManager) GetLinkedExecution(incidentID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	executionID, ok := m.incidentLinks[incidentID]
	return executionID, ok
}
