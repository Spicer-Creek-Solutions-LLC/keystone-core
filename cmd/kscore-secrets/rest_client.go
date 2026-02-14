package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	apisecrets "github.com/shawnbutts/keystone-core/pkg/api/secrets"
)

// RESTClient is a REST client for the secrets API.
type RESTClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRESTClient creates a new secrets REST client.
func NewRESTClient(baseURL string) *RESTClient {
	return &RESTClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ListBackends lists all configured secret backends.
func (c *RESTClient) ListBackends() (*apisecrets.BackendListResponse, error) {
	var resp apisecrets.BackendListResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/secrets/backends", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetBackend retrieves details for a single backend.
func (c *RESTClient) GetBackend(name string) (*apisecrets.BackendInfoResponse, error) {
	var resp apisecrets.BackendInfoResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/secrets/backends/"+name, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AuditListOpts holds options for listing audit entries.
type AuditListOpts struct {
	Path    string
	Action  string
	AgentID string
	Start   string
	End     string
	Limit   int
	Offset  int
}

// ListAuditEntries lists secret access audit entries with optional filters.
func (c *RESTClient) ListAuditEntries(opts AuditListOpts) (*apisecrets.AuditLogResponse, error) {
	params := url.Values{}
	if opts.Path != "" {
		params.Set("path", opts.Path)
	}
	if opts.Action != "" {
		params.Set("action", opts.Action)
	}
	if opts.AgentID != "" {
		params.Set("agent_id", opts.AgentID)
	}
	if opts.Start != "" {
		params.Set("start", opts.Start)
	}
	if opts.End != "" {
		params.Set("end", opts.End)
	}
	if opts.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", opts.Offset))
	}

	endpoint := c.baseURL + "/api/v1/audit/logs"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var resp apisecrets.AuditLogResponse
	if err := c.doJSON(http.MethodGet, endpoint, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetCacheStats retrieves cache statistics.
func (c *RESTClient) GetCacheStats() (*apisecrets.CacheStatsResponse, error) {
	var resp apisecrets.CacheStatsResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/secrets/cache/stats", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListRotations lists all active rotations.
func (c *RESTClient) ListRotations() (*apisecrets.RotationListResponse, error) {
	var resp apisecrets.RotationListResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/rotations", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetRotation retrieves a single rotation by ID.
func (c *RESTClient) GetRotation(id string) (*apisecrets.RotationResponse, error) {
	var resp apisecrets.RotationResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/rotations/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StartRotation starts a new rotation.
func (c *RESTClient) StartRotation(req *apisecrets.StartRotationRequest) (*apisecrets.RotationResponse, error) {
	var resp apisecrets.RotationResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/rotations", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CancelRotation cancels an in-progress rotation.
func (c *RESTClient) CancelRotation(id string) (*apisecrets.RotationActionResponse, error) {
	var resp apisecrets.RotationActionResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/rotations/"+id+"/cancel", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RollbackRotation rolls back a rotation.
func (c *RESTClient) RollbackRotation(id string) (*apisecrets.RotationActionResponse, error) {
	var resp apisecrets.RotationActionResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/rotations/"+id+"/rollback", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PauseRotation pauses an in-progress rotation.
func (c *RESTClient) PauseRotation(id string) (*apisecrets.RotationActionResponse, error) {
	var resp apisecrets.RotationActionResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/rotations/"+id+"/pause", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ResumeRotation resumes a paused rotation.
func (c *RESTClient) ResumeRotation(id string) (*apisecrets.RotationActionResponse, error) {
	var resp apisecrets.RotationActionResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/rotations/"+id+"/resume", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// TriggerRotation triggers a scheduled rotation immediately.
func (c *RESTClient) TriggerRotation(id string) (*apisecrets.RotationActionResponse, error) {
	var resp apisecrets.RotationActionResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/rotations/"+id+"/trigger", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListRotationPolicies lists all rotation policies.
func (c *RESTClient) ListRotationPolicies() (*apisecrets.RotationPolicyListResponse, error) {
	var resp apisecrets.RotationPolicyListResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/secrets/rotation/policies", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetRotationPolicy retrieves a single rotation policy by ID.
func (c *RESTClient) GetRotationPolicy(id string) (*apisecrets.RotationPolicyResponse, error) {
	var resp apisecrets.RotationPolicyResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/secrets/rotation/policies/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateRotationPolicy creates a new rotation policy.
func (c *RESTClient) CreateRotationPolicy(req *apisecrets.CreateRotationPolicyRequest) (*apisecrets.RotationPolicyResponse, error) {
	var resp apisecrets.RotationPolicyResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/secrets/rotation/policies", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteRotationPolicy deletes a rotation policy.
func (c *RESTClient) DeleteRotationPolicy(id string) (*apisecrets.RotationPolicyActionResponse, error) {
	var resp apisecrets.RotationPolicyActionResponse
	if err := c.doJSON(http.MethodDelete, c.baseURL+"/api/v1/secrets/rotation/policies/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// EnableRotationPolicy enables a rotation policy.
func (c *RESTClient) EnableRotationPolicy(id string) (*apisecrets.RotationPolicyActionResponse, error) {
	var resp apisecrets.RotationPolicyActionResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/secrets/rotation/policies/"+id+"/enable", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DisableRotationPolicy disables a rotation policy.
func (c *RESTClient) DisableRotationPolicy(id string) (*apisecrets.RotationPolicyActionResponse, error) {
	var resp apisecrets.RotationPolicyActionResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/secrets/rotation/policies/"+id+"/disable", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// TransitRewrap rewraps ciphertext with the latest key version.
func (c *RESTClient) TransitRewrap(keyName string, req *apisecrets.TransitRewrapRequest) (*apisecrets.TransitRewrapResponse, error) {
	var resp apisecrets.TransitRewrapResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/transit/rewrap/"+keyName, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ClearCache clears the secret cache.
func (c *RESTClient) ClearCache() (*apisecrets.CacheClearResponse, error) {
	var resp apisecrets.CacheClearResponse
	if err := c.doJSON(http.MethodDelete, c.baseURL+"/api/v1/secrets/cache", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *RESTClient) doJSON(method, endpoint string, reqBody, respBody interface{}) error {
	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, endpoint, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode >= 400 {
		respData, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respData, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("server error (%d): %s", resp.StatusCode, apiErr.Error)
		}
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respData))
	}

	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
