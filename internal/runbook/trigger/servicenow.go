package trigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// ServiceNow record types.
const (
	SNTableIncident      = "incident"
	SNTableChangeRequest = "change_request"
	SNTableCMDBCI        = "cmdb_ci"
	SNTableApproval      = "sysapproval_approver"
)

// ServiceNow states for change requests.
const (
	SNChangeStateNew         = -5
	SNChangeStateAssess      = -4
	SNChangeStateAuthorize   = -3
	SNChangeStateScheduled   = -2
	SNChangeStateImplement   = -1
	SNChangeStateReview      = 0
	SNChangeStateClosed      = 3
	SNChangeStateCancelled   = 4
	SNChangeStateInProgress  = 2
)

// ServiceNow approval states.
const (
	SNApprovalNotRequested = "not requested"
	SNApprovalRequested    = "requested"
	SNApprovalApproved     = "approved"
	SNApprovalRejected     = "rejected"
)

// ServiceNowClient implements ITSM integration with ServiceNow.
type ServiceNowClient struct {
	instanceURL string
	username    string
	password    string
	httpClient  *http.Client
	clientID    string // For OAuth
	clientSecret string // For OAuth
	accessToken string
	tokenExpiry time.Time
	mu          sync.Mutex
}

// ServiceNowConfig holds configuration for ServiceNow client.
type ServiceNowConfig struct {
	// InstanceURL is the ServiceNow instance URL (e.g., https://company.service-now.com)
	InstanceURL string `yaml:"instance_url" json:"instance_url"`

	// Username for basic authentication
	Username string `yaml:"username,omitempty" json:"username,omitempty"`

	// Password for basic authentication
	Password string `yaml:"password,omitempty" json:"password,omitempty"`

	// ClientID for OAuth authentication
	ClientID string `yaml:"client_id,omitempty" json:"client_id,omitempty"`

	// ClientSecret for OAuth authentication
	ClientSecret string `yaml:"client_secret,omitempty" json:"client_secret,omitempty"`

	// HTTPTimeout for API calls
	HTTPTimeout time.Duration `yaml:"http_timeout,omitempty" json:"http_timeout,omitempty"`
}

// NewServiceNowClient creates a new ServiceNow client.
func NewServiceNowClient(config ServiceNowConfig) (*ServiceNowClient, error) {
	if config.InstanceURL == "" {
		return nil, fmt.Errorf("instance URL is required")
	}

	// Remove trailing slash
	instanceURL := strings.TrimSuffix(config.InstanceURL, "/")

	timeout := config.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &ServiceNowClient{
		instanceURL:  instanceURL,
		username:     config.Username,
		password:     config.Password,
		clientID:     config.ClientID,
		clientSecret: config.ClientSecret,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	// Validate authentication is configured
	hasBasic := config.Username != "" && config.Password != ""
	hasOAuth := config.ClientID != "" && config.ClientSecret != ""

	if !hasBasic && !hasOAuth {
		return nil, fmt.Errorf("either basic auth (username/password) or OAuth (client_id/client_secret) is required")
	}

	return client, nil
}

// Type returns the ITSM type.
func (c *ServiceNowClient) Type() ITSMType {
	return ITSMTypeServiceNow
}

// doRequest performs an authenticated HTTP request.
func (c *ServiceNowClient) doRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Response, error) {
	url := c.instanceURL + endpoint

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	// Add authentication
	if c.clientID != "" && c.clientSecret != "" {
		token, err := c.getOAuthToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("get OAuth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}

	return c.httpClient.Do(req)
}

// getOAuthToken retrieves or refreshes the OAuth token.
func (c *ServiceNowClient) getOAuthToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid (with 5 min buffer)
	if c.accessToken != "" && time.Now().Add(5*time.Minute).Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	// Request new token
	tokenURL := c.instanceURL + "/oauth_token.do"
	data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s",
		c.clientID, c.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return c.accessToken, nil
}

// ChangeRequest represents a ServiceNow change request.
type ChangeRequest struct {
	SysID            string    `json:"sys_id,omitempty"`
	Number           string    `json:"number,omitempty"`
	ShortDescription string    `json:"short_description,omitempty"`
	Description      string    `json:"description,omitempty"`
	State            int       `json:"state,omitempty"`
	Category         string    `json:"category,omitempty"`
	Priority         int       `json:"priority,omitempty"`
	Risk             int       `json:"risk,omitempty"`
	Impact           int       `json:"impact,omitempty"`
	AssignedTo       string    `json:"assigned_to,omitempty"`
	AssignmentGroup  string    `json:"assignment_group,omitempty"`
	RequestedBy      string    `json:"requested_by,omitempty"`
	StartDate        string    `json:"start_date,omitempty"`
	EndDate          string    `json:"end_date,omitempty"`
	WorkNotes        string    `json:"work_notes,omitempty"`
	CloseCode        string    `json:"close_code,omitempty"`
	CloseNotes       string    `json:"close_notes,omitempty"`
	CIRef            string    `json:"cmdb_ci,omitempty"`
	Type             string    `json:"type,omitempty"` // standard, normal, emergency
	JustificationReason string `json:"justification,omitempty"`
	ImplementationPlan  string `json:"implementation_plan,omitempty"`
	BackoutPlan         string `json:"backout_plan,omitempty"`
	TestPlan            string `json:"test_plan,omitempty"`
}

// CreateChangeRequest creates a new change request in ServiceNow.
func (c *ServiceNowClient) CreateChangeRequest(ctx context.Context, cr *ChangeRequest) (*ChangeRequest, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/now/table/change_request", cr)
	if err != nil {
		return nil, fmt.Errorf("create change request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create change request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result ChangeRequest `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result.Result, nil
}

// GetChangeRequest retrieves a change request by sys_id or number.
func (c *ServiceNowClient) GetChangeRequest(ctx context.Context, idOrNumber string) (*ChangeRequest, error) {
	// Check if this is a sys_id (32 hex chars) or a change number
	endpoint := fmt.Sprintf("/api/now/table/change_request/%s", idOrNumber)
	if strings.HasPrefix(idOrNumber, "CHG") {
		endpoint = fmt.Sprintf("/api/now/table/change_request?sysparm_query=number=%s&sysparm_limit=1", idOrNumber)
	}

	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("get change request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("change request not found: %s", idOrNumber)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get change request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle both single record and query results
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Try single record response first
	var singleResult struct {
		Result ChangeRequest `json:"result"`
	}
	if err := json.Unmarshal(body, &singleResult); err == nil && singleResult.Result.SysID != "" {
		return &singleResult.Result, nil
	}

	// Try array response
	var arrayResult struct {
		Result []ChangeRequest `json:"result"`
	}
	if err := json.Unmarshal(body, &arrayResult); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(arrayResult.Result) == 0 {
		return nil, fmt.Errorf("change request not found: %s", idOrNumber)
	}

	return &arrayResult.Result[0], nil
}

// UpdateChangeRequest updates an existing change request.
func (c *ServiceNowClient) UpdateChangeRequest(ctx context.Context, sysID string, updates *ChangeRequest) (*ChangeRequest, error) {
	resp, err := c.doRequest(ctx, http.MethodPatch, "/api/now/table/change_request/"+sysID, updates)
	if err != nil {
		return nil, fmt.Errorf("update change request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update change request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result ChangeRequest `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result.Result, nil
}

// AddWorkNote adds a work note to a change request.
func (c *ServiceNowClient) AddWorkNote(ctx context.Context, sysID, note string) error {
	updates := &ChangeRequest{
		WorkNotes: note,
	}
	_, err := c.UpdateChangeRequest(ctx, sysID, updates)
	return err
}

// CloseChangeRequest closes a change request with the given code and notes.
func (c *ServiceNowClient) CloseChangeRequest(ctx context.Context, sysID string, closeCode string, closeNotes string) error {
	updates := &ChangeRequest{
		State:      SNChangeStateClosed,
		CloseCode:  closeCode,
		CloseNotes: closeNotes,
	}
	_, err := c.UpdateChangeRequest(ctx, sysID, updates)
	return err
}

// SetChangeState sets the state of a change request.
func (c *ServiceNowClient) SetChangeState(ctx context.Context, sysID string, state int) error {
	updates := &ChangeRequest{
		State: state,
	}
	_, err := c.UpdateChangeRequest(ctx, sysID, updates)
	return err
}

// CMDBConfigurationItem represents a CMDB CI record.
type CMDBConfigurationItem struct {
	SysID           string `json:"sys_id,omitempty"`
	Name            string `json:"name,omitempty"`
	SysClassName    string `json:"sys_class_name,omitempty"`
	OperationalStatus int  `json:"operational_status,omitempty"`
	InstallStatus   int    `json:"install_status,omitempty"`
	Environment     string `json:"environment,omitempty"`
	Comments        string `json:"comments,omitempty"`
	ShortDescription string `json:"short_description,omitempty"`
	Attributes      map[string]interface{} `json:"attributes,omitempty"`
}

// GetCMDBCI retrieves a configuration item by sys_id or name.
func (c *ServiceNowClient) GetCMDBCI(ctx context.Context, idOrName string) (*CMDBConfigurationItem, error) {
	// Try as sys_id first
	endpoint := fmt.Sprintf("/api/now/table/cmdb_ci/%s", idOrName)

	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("get CMDB CI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Try by name
		endpoint = fmt.Sprintf("/api/now/table/cmdb_ci?sysparm_query=name=%s&sysparm_limit=1", idOrName)
		resp, err = c.doRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("get CMDB CI by name: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("get CMDB CI failed with status %d: %s", resp.StatusCode, string(body))
		}

		var arrayResult struct {
			Result []CMDBConfigurationItem `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&arrayResult); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		if len(arrayResult.Result) == 0 {
			return nil, fmt.Errorf("CMDB CI not found: %s", idOrName)
		}

		return &arrayResult.Result[0], nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get CMDB CI failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result CMDBConfigurationItem `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result.Result, nil
}

// UpdateCMDBCI updates a configuration item.
func (c *ServiceNowClient) UpdateCMDBCI(ctx context.Context, sysID string, updates *CMDBConfigurationItem) (*CMDBConfigurationItem, error) {
	resp, err := c.doRequest(ctx, http.MethodPatch, "/api/now/table/cmdb_ci/"+sysID, updates)
	if err != nil {
		return nil, fmt.Errorf("update CMDB CI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update CMDB CI failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result CMDBConfigurationItem `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result.Result, nil
}

// ApprovalRecord represents a ServiceNow approval record.
type ApprovalRecord struct {
	SysID         string `json:"sys_id,omitempty"`
	State         string `json:"state,omitempty"`
	Approver      string `json:"approver,omitempty"`
	DocumentID    string `json:"document_id,omitempty"`
	DocumentTable string `json:"source_table,omitempty"`
	Comments      string `json:"comments,omitempty"`
	ApprovedAt    string `json:"sys_updated_on,omitempty"`
}

// GetApprovals retrieves approval records for a change request.
func (c *ServiceNowClient) GetApprovals(ctx context.Context, changeRequestSysID string) ([]ApprovalRecord, error) {
	endpoint := fmt.Sprintf("/api/now/table/sysapproval_approver?sysparm_query=document_id=%s", changeRequestSysID)

	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("get approvals: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get approvals failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result []ApprovalRecord `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Result, nil
}

// IsApproved checks if all approvals for a change request are approved.
func (c *ServiceNowClient) IsApproved(ctx context.Context, changeRequestSysID string) (bool, error) {
	approvals, err := c.GetApprovals(ctx, changeRequestSysID)
	if err != nil {
		return false, err
	}

	if len(approvals) == 0 {
		// No approvals required
		return true, nil
	}

	for _, approval := range approvals {
		if approval.State != SNApprovalApproved {
			return false, nil
		}
	}

	return true, nil
}

// WaitForApproval waits for a change request to be approved.
func (c *ServiceNowClient) WaitForApproval(ctx context.Context, changeRequestSysID string, pollInterval time.Duration) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			approved, err := c.IsApproved(ctx, changeRequestSysID)
			if err != nil {
				return err
			}
			if approved {
				return nil
			}

			// Check for rejection
			approvals, err := c.GetApprovals(ctx, changeRequestSysID)
			if err != nil {
				return err
			}

			for _, approval := range approvals {
				if approval.State == SNApprovalRejected {
					return fmt.Errorf("change request rejected by approver: %s", approval.Approver)
				}
			}
		}
	}
}

// ParseWebhook parses a ServiceNow webhook payload (incident or change).
func (c *ServiceNowClient) ParseWebhook(body []byte) (*Incident, error) {
	// ServiceNow webhooks can be customized, but typically send JSON
	var payload struct {
		SysID            string `json:"sys_id"`
		Number           string `json:"number"`
		ShortDescription string `json:"short_description"`
		Description      string `json:"description"`
		State            string `json:"state"`
		Priority         string `json:"priority"`
		Urgency          string `json:"urgency"`
		Impact           string `json:"impact"`
		AssignmentGroup  string `json:"assignment_group"`
		AssignedTo       string `json:"assigned_to"`
		CallerID         string `json:"caller_id"`
		Category         string `json:"category"`
		Subcategory      string `json:"subcategory"`
		CMDBCI           string `json:"cmdb_ci"`
		OpenedAt         string `json:"opened_at"`
		ClosedAt         string `json:"closed_at"`
		ResolvedAt       string `json:"resolved_at"`
		Action           string `json:"action"` // insert, update, delete
		TableName        string `json:"table_name"` // incident, change_request
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal ServiceNow webhook: %w", err)
	}

	incident := &Incident{
		ID:          payload.SysID,
		ExternalID:  payload.Number,
		Title:       payload.ShortDescription,
		Description: payload.Description,
		Status:      mapServiceNowState(payload.State),
		Severity:    mapServiceNowPriority(payload.Priority),
		Service:     payload.AssignmentGroup,
		Source:      ITSMTypeServiceNow,
		URL:         fmt.Sprintf("%s/nav_to.do?uri=%s.do?sys_id=%s", c.instanceURL, payload.TableName, payload.SysID),
		RawData: map[string]interface{}{
			"number":      payload.Number,
			"urgency":     payload.Urgency,
			"impact":      payload.Impact,
			"category":    payload.Category,
			"subcategory": payload.Subcategory,
			"cmdb_ci":     payload.CMDBCI,
			"caller_id":   payload.CallerID,
			"action":      payload.Action,
			"table_name":  payload.TableName,
		},
	}

	if payload.OpenedAt != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", payload.OpenedAt); err == nil {
			incident.CreatedAt = t
		}
	}

	return incident, nil
}

// AcknowledgeIncident acknowledges an incident (sets state to In Progress).
func (c *ServiceNowClient) AcknowledgeIncident(ctx context.Context, incidentID string) error {
	updates := map[string]interface{}{
		"state": 2, // In Progress
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, "/api/now/table/incident/"+incidentID, updates)
	if err != nil {
		return fmt.Errorf("acknowledge incident: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("acknowledge incident failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ResolveIncident resolves an incident.
func (c *ServiceNowClient) ResolveIncident(ctx context.Context, incidentID string) error {
	updates := map[string]interface{}{
		"state":             6, // Resolved
		"close_code":        "Solved (Permanently)",
		"close_notes":       "Resolved by Keystone runbook automation",
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, "/api/now/table/incident/"+incidentID, updates)
	if err != nil {
		return fmt.Errorf("resolve incident: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resolve incident failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// AddNote adds a work note to an incident.
func (c *ServiceNowClient) AddNote(ctx context.Context, incidentID, note string) error {
	updates := map[string]interface{}{
		"work_notes": note,
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, "/api/now/table/incident/"+incidentID, updates)
	if err != nil {
		return fmt.Errorf("add note: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add note failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetIncident retrieves an incident by sys_id.
func (c *ServiceNowClient) GetIncident(ctx context.Context, incidentID string) (*Incident, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/now/table/incident/"+incidentID, nil)
	if err != nil {
		return nil, fmt.Errorf("get incident: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get incident failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result struct {
			SysID            string `json:"sys_id"`
			Number           string `json:"number"`
			ShortDescription string `json:"short_description"`
			Description      string `json:"description"`
			State            string `json:"state"`
			Priority         string `json:"priority"`
			AssignmentGroup  string `json:"assignment_group"`
			OpenedAt         string `json:"opened_at"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	incident := &Incident{
		ID:          result.Result.SysID,
		ExternalID:  result.Result.Number,
		Title:       result.Result.ShortDescription,
		Description: result.Result.Description,
		Status:      mapServiceNowState(result.Result.State),
		Severity:    mapServiceNowPriority(result.Result.Priority),
		Service:     result.Result.AssignmentGroup,
		Source:      ITSMTypeServiceNow,
		URL:         fmt.Sprintf("%s/nav_to.do?uri=incident.do?sys_id=%s", c.instanceURL, result.Result.SysID),
	}

	if result.Result.OpenedAt != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", result.Result.OpenedAt); err == nil {
			incident.CreatedAt = t
		}
	}

	return incident, nil
}

func mapServiceNowState(state string) IncidentStatus {
	switch state {
	case "1": // New
		return IncidentStatusTriggered
	case "2": // In Progress
		return IncidentStatusAcknowledged
	case "6": // Resolved
		return IncidentStatusResolved
	case "7": // Closed
		return IncidentStatusResolved
	case "8": // Cancelled
		return IncidentStatusResolved
	default:
		return IncidentStatusTriggered
	}
}

func mapServiceNowPriority(priority string) IncidentSeverity {
	switch priority {
	case "1": // Critical
		return IncidentSeverityCritical
	case "2": // High
		return IncidentSeverityHigh
	case "3": // Moderate
		return IncidentSeverityMedium
	case "4": // Low
		return IncidentSeverityLow
	case "5": // Planning
		return IncidentSeverityInfo
	default:
		return IncidentSeverityMedium
	}
}

// contains checks if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ServiceNowTrigger represents a ServiceNow-triggered runbook.
type ServiceNowTrigger struct {
	// ID is the unique trigger identifier.
	ID string `yaml:"id" json:"id"`

	// Name is a human-readable name.
	Name string `yaml:"name" json:"name"`

	// Description explains what this trigger does.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// RunbookRef references the runbook to execute.
	RunbookRef RunbookRef `yaml:"runbook" json:"runbook"`

	// TableFilter limits to specific ServiceNow tables.
	TableFilter []string `yaml:"table_filter,omitempty" json:"table_filter,omitempty"`

	// AssignmentGroupFilter limits to specific assignment groups.
	AssignmentGroupFilter []string `yaml:"assignment_group_filter,omitempty" json:"assignment_group_filter,omitempty"`

	// CategoryFilter limits to specific categories.
	CategoryFilter []string `yaml:"category_filter,omitempty" json:"category_filter,omitempty"`

	// PriorityFilter limits to specific priorities.
	PriorityFilter []string `yaml:"priority_filter,omitempty" json:"priority_filter,omitempty"`

	// ActionFilter limits to specific actions (insert, update, delete).
	ActionFilter []string `yaml:"action_filter,omitempty" json:"action_filter,omitempty"`

	// CreateChangeRequest creates a change request before execution.
	CreateChangeRequest bool `yaml:"create_change_request,omitempty" json:"create_change_request,omitempty"`

	// WaitForApproval waits for change request approval before executing.
	WaitForApproval bool `yaml:"wait_for_approval,omitempty" json:"wait_for_approval,omitempty"`

	// ApprovalTimeout is the max time to wait for approval.
	ApprovalTimeout time.Duration `yaml:"approval_timeout,omitempty" json:"approval_timeout,omitempty"`

	// AcknowledgeOnStart acknowledges the incident when runbook starts.
	AcknowledgeOnStart bool `yaml:"acknowledge_on_start,omitempty" json:"acknowledge_on_start,omitempty"`

	// ResolveOnSuccess resolves the incident on successful completion.
	ResolveOnSuccess bool `yaml:"resolve_on_success,omitempty" json:"resolve_on_success,omitempty"`

	// UpdateCMDB indicates whether to update CMDB on completion.
	UpdateCMDB bool `yaml:"update_cmdb,omitempty" json:"update_cmdb,omitempty"`

	// ChangeRequestTemplate is the template for change request creation.
	ChangeRequestTemplate *ChangeRequest `yaml:"change_request_template,omitempty" json:"change_request_template,omitempty"`

	// InputMappings map webhook fields to runbook inputs.
	InputMappings map[string]string `yaml:"input_mappings,omitempty" json:"input_mappings,omitempty"`

	// Enabled indicates if the trigger is active.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Tags for categorization.
	Tags map[string]string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// CreatedAt is when the trigger was created.
	CreatedAt time.Time `yaml:"createdAt,omitempty" json:"created_at,omitempty"`

	// UpdatedAt is when the trigger was last updated.
	UpdatedAt time.Time `yaml:"updatedAt,omitempty" json:"updated_at,omitempty"`
}

// Validate validates the ServiceNow trigger.
func (t *ServiceNowTrigger) Validate() error {
	if t.ID == "" {
		return &ValidationError{Field: "id", Message: "id is required"}
	}
	if t.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if t.RunbookRef.Name == "" {
		return &ValidationError{Field: "runbook.name", Message: "runbook name is required"}
	}
	if t.WaitForApproval && !t.CreateChangeRequest {
		return &ValidationError{Field: "wait_for_approval", Message: "wait_for_approval requires create_change_request to be true"}
	}
	return nil
}

// ServiceNowTriggerManager manages ServiceNow triggers.
type ServiceNowTriggerManager struct {
	mu       sync.RWMutex
	triggers map[string]*ServiceNowTrigger

	client     *ServiceNowClient
	repository RunbookRepository
	executor   RunbookExecutor
	publisher  events.EventPublisher

	executions map[string]*ServiceNowExecution
}

// ServiceNowExecution tracks a ServiceNow-triggered execution.
type ServiceNowExecution struct {
	ID               string    `json:"id"`
	TriggerID        string    `json:"trigger_id"`
	IncidentID       string    `json:"incident_id,omitempty"`
	ChangeRequestID  string    `json:"change_request_id,omitempty"`
	ExecutionID      string    `json:"execution_id,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	Status           string    `json:"status"`
	Error            string    `json:"error,omitempty"`
}

// NewServiceNowTriggerManager creates a new ServiceNow trigger manager.
func NewServiceNowTriggerManager(
	client *ServiceNowClient,
	repo RunbookRepository,
	executor RunbookExecutor,
	publisher events.EventPublisher,
) *ServiceNowTriggerManager {
	return &ServiceNowTriggerManager{
		triggers:   make(map[string]*ServiceNowTrigger),
		client:     client,
		repository: repo,
		executor:   executor,
		publisher:  publisher,
		executions: make(map[string]*ServiceNowExecution),
	}
}

// Register adds a ServiceNow trigger.
func (m *ServiceNowTriggerManager) Register(trigger *ServiceNowTrigger) error {
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

// Unregister removes a trigger.
func (m *ServiceNowTriggerManager) Unregister(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.triggers[id]; !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	delete(m.triggers, id)
	return nil
}

// Get retrieves a trigger by ID.
func (m *ServiceNowTriggerManager) Get(id string) (*ServiceNowTrigger, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trigger, ok := m.triggers[id]
	return trigger, ok
}

// List returns all triggers.
func (m *ServiceNowTriggerManager) List() []*ServiceNowTrigger {
	m.mu.RLock()
	defer m.mu.RUnlock()

	triggers := make([]*ServiceNowTrigger, 0, len(m.triggers))
	for _, t := range m.triggers {
		triggers = append(triggers, t)
	}
	return triggers
}

// Enable enables a trigger.
func (m *ServiceNowTriggerManager) Enable(id string) error {
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

// Disable disables a trigger.
func (m *ServiceNowTriggerManager) Disable(id string) error {
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

// HandleWebhook processes a ServiceNow webhook.
func (m *ServiceNowTriggerManager) HandleWebhook(ctx context.Context, body []byte) (*ServiceNowExecution, error) {
	if m.client == nil {
		return nil, fmt.Errorf("ServiceNow client not configured")
	}

	// Parse webhook
	incident, err := m.client.ParseWebhook(body)
	if err != nil {
		return nil, fmt.Errorf("parse webhook: %w", err)
	}

	// Find matching triggers
	m.mu.RLock()
	var matchingTriggers []*ServiceNowTrigger
	for _, trigger := range m.triggers {
		if !trigger.Enabled {
			continue
		}

		if m.matchesTrigger(trigger, incident, body) {
			matchingTriggers = append(matchingTriggers, trigger)
		}
	}
	m.mu.RUnlock()

	if len(matchingTriggers) == 0 {
		return nil, nil // No matching triggers
	}

	// Execute first matching trigger
	trigger := matchingTriggers[0]
	return m.executeTrigger(ctx, trigger, incident)
}

func (m *ServiceNowTriggerManager) matchesTrigger(trigger *ServiceNowTrigger, incident *Incident, body []byte) bool {
	rawData := incident.RawData
	if rawData == nil {
		rawData = make(map[string]interface{})
	}

	// Table filter
	if len(trigger.TableFilter) > 0 {
		tableName, _ := rawData["table_name"].(string)
		if !contains(trigger.TableFilter, tableName) {
			return false
		}
	}

	// Assignment group filter
	if len(trigger.AssignmentGroupFilter) > 0 {
		if !contains(trigger.AssignmentGroupFilter, incident.Service) {
			return false
		}
	}

	// Category filter
	if len(trigger.CategoryFilter) > 0 {
		category, _ := rawData["category"].(string)
		if !contains(trigger.CategoryFilter, category) {
			return false
		}
	}

	// Priority filter
	if len(trigger.PriorityFilter) > 0 {
		if !contains(trigger.PriorityFilter, string(incident.Severity)) {
			return false
		}
	}

	// Action filter
	if len(trigger.ActionFilter) > 0 {
		action, _ := rawData["action"].(string)
		if !contains(trigger.ActionFilter, action) {
			return false
		}
	}

	return true
}

func (m *ServiceNowTriggerManager) executeTrigger(ctx context.Context, trigger *ServiceNowTrigger, incident *Incident) (*ServiceNowExecution, error) {
	exec := &ServiceNowExecution{
		ID:         uuid.New().String(),
		TriggerID:  trigger.ID,
		IncidentID: incident.ID,
		StartedAt:  time.Now(),
		Status:     "running",
	}

	m.mu.Lock()
	m.executions[exec.ID] = exec
	m.mu.Unlock()

	// Publish start event
	if m.publisher != nil {
		_ = m.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.servicenow.started"),
			Source: "/runbook/servicenow/" + trigger.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id":  trigger.ID,
				"incident_id": incident.ID,
				"severity":    incident.Severity,
			},
		})
	}

	// Create change request if configured
	var changeRequest *ChangeRequest
	if trigger.CreateChangeRequest {
		template := trigger.ChangeRequestTemplate
		if template == nil {
			template = &ChangeRequest{}
		}

		cr := &ChangeRequest{
			ShortDescription:   fmt.Sprintf("Automated remediation: %s", incident.Title),
			Description:        fmt.Sprintf("Automated change triggered by incident %s\n\n%s", incident.ExternalID, incident.Description),
			Type:               "emergency",
			Category:           template.Category,
			Priority:           template.Priority,
			Risk:               template.Risk,
			Impact:             template.Impact,
			AssignedTo:         template.AssignedTo,
			AssignmentGroup:    template.AssignmentGroup,
			ImplementationPlan: template.ImplementationPlan,
			BackoutPlan:        template.BackoutPlan,
			TestPlan:           template.TestPlan,
		}

		var err error
		changeRequest, err = m.client.CreateChangeRequest(ctx, cr)
		if err != nil {
			m.recordError(exec, err)
			return exec, fmt.Errorf("create change request: %w", err)
		}

		exec.ChangeRequestID = changeRequest.SysID

		// Add work note linking to incident
		_ = m.client.AddWorkNote(ctx, changeRequest.SysID,
			fmt.Sprintf("Change created by Keystone runbook automation for incident %s", incident.ExternalID))
	}

	// Wait for approval if configured
	if trigger.WaitForApproval && changeRequest != nil {
		approvalTimeout := trigger.ApprovalTimeout
		if approvalTimeout == 0 {
			approvalTimeout = 1 * time.Hour
		}

		approvalCtx, cancel := context.WithTimeout(ctx, approvalTimeout)
		defer cancel()

		if err := m.client.WaitForApproval(approvalCtx, changeRequest.SysID, 30*time.Second); err != nil {
			m.recordError(exec, err)
			return exec, fmt.Errorf("wait for approval: %w", err)
		}
	}

	// Acknowledge incident if configured
	if trigger.AcknowledgeOnStart {
		if err := m.client.AcknowledgeIncident(ctx, incident.ID); err != nil {
			// Log but continue
			_ = err
		}
	}

	// Set change request to implement state
	if changeRequest != nil {
		_ = m.client.SetChangeState(ctx, changeRequest.SysID, SNChangeStateImplement)
	}

	// Get runbook
	rb, err := m.repository.GetRunbook(trigger.RunbookRef.Name, trigger.RunbookRef.Version)
	if err != nil {
		m.recordError(exec, err)
		return exec, fmt.Errorf("get runbook: %w", err)
	}

	// Build inputs
	inputs := buildIncidentInputs(incident)

	// Add custom mappings
	for k, v := range trigger.InputMappings {
		inputs[k] = resolveIncidentMapping(v, incident)
	}

	// Add ServiceNow-specific inputs
	if changeRequest != nil {
		inputs["__change_request_id"] = changeRequest.SysID
		inputs["__change_request_number"] = changeRequest.Number
	}

	// Execute runbook
	rbExec, err := m.executor.Execute(rb, inputs)

	// Record completion
	now := time.Now()
	exec.CompletedAt = &now

	if err != nil {
		m.recordError(exec, err)

		// Close change request as unsuccessful
		if changeRequest != nil {
			_ = m.client.CloseChangeRequest(ctx, changeRequest.SysID, "unsuccessful", err.Error())
		}

		return exec, err
	}

	if rbExec.State == runbook.ExecutionStateFailed {
		err = fmt.Errorf("runbook execution failed: %s", rbExec.Error)
		m.recordError(exec, err)

		// Close change request as unsuccessful
		if changeRequest != nil {
			_ = m.client.CloseChangeRequest(ctx, changeRequest.SysID, "unsuccessful", rbExec.Error)
		}

		return exec, err
	}

	exec.ExecutionID = rbExec.ID
	exec.Status = "completed"

	// Resolve incident if configured
	if trigger.ResolveOnSuccess {
		if err := m.client.ResolveIncident(ctx, incident.ID); err != nil {
			// Log but don't fail
			_ = err
		}
	}

	// Close change request as successful
	if changeRequest != nil {
		_ = m.client.CloseChangeRequest(ctx, changeRequest.SysID, "successful", "Runbook completed successfully")
	}

	// Add work notes
	_ = m.client.AddNote(ctx, incident.ID,
		fmt.Sprintf("Runbook %s executed successfully. Execution ID: %s", trigger.RunbookRef.Name, rbExec.ID))

	// Publish completion event
	if m.publisher != nil {
		_ = m.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.servicenow.completed"),
			Source: "/runbook/servicenow/" + trigger.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id":   trigger.ID,
				"incident_id":  incident.ID,
				"execution_id": rbExec.ID,
				"success":      true,
			},
		})
	}

	return exec, nil
}

func (m *ServiceNowTriggerManager) recordError(exec *ServiceNowExecution, err error) {
	exec.Status = "failed"
	exec.Error = err.Error()
	now := time.Now()
	exec.CompletedAt = &now

	// Publish failure event
	if m.publisher != nil {
		_ = m.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.servicenow.failed"),
			Source: "/runbook/servicenow/" + exec.TriggerID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"trigger_id":  exec.TriggerID,
				"incident_id": exec.IncidentID,
				"error":       err.Error(),
			},
		})
	}
}

// buildIncidentInputs creates runbook inputs from an incident.
func buildIncidentInputs(incident *Incident) map[string]interface{} {
	inputs := make(map[string]interface{})

	// Add incident metadata
	inputs["__incident_id"] = incident.ID
	inputs["__incident_source"] = string(incident.Source)
	inputs["__trigger_type"] = "servicenow"
	inputs["incident_id"] = incident.ID
	inputs["incident_external_id"] = incident.ExternalID
	inputs["incident_title"] = incident.Title
	inputs["incident_description"] = incident.Description
	inputs["incident_status"] = string(incident.Status)
	inputs["incident_severity"] = string(incident.Severity)
	inputs["incident_service"] = incident.Service
	inputs["incident_url"] = incident.URL

	return inputs
}

func resolveIncidentMapping(mapping string, incident *Incident) interface{} {
	// Simple field resolution for ServiceNow incident mappings
	switch mapping {
	case "incident.id":
		return incident.ID
	case "incident.number", "incident.external_id":
		return incident.ExternalID
	case "incident.title":
		return incident.Title
	case "incident.description":
		return incident.Description
	case "incident.severity":
		return string(incident.Severity)
	case "incident.status":
		return string(incident.Status)
	case "incident.service":
		return incident.Service
	default:
		// Check for raw data fields
		if strings.HasPrefix(mapping, "incident.raw_data.") {
			field := strings.TrimPrefix(mapping, "incident.raw_data.")
			if incident.RawData != nil {
				if val, exists := incident.RawData[field]; exists {
					return val
				}
			}
		}
		return mapping
	}
}

// GetExecution retrieves an execution by ID.
func (m *ServiceNowTriggerManager) GetExecution(id string) (*ServiceNowExecution, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exec, ok := m.executions[id]
	return exec, ok
}

// GetClient returns the ServiceNow client.
func (m *ServiceNowTriggerManager) GetClient() *ServiceNowClient {
	return m.client
}
