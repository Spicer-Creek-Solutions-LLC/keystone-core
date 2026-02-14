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

	apirunbook "github.com/shawnbutts/keystone-core/pkg/api/runbook"
)

// Client is a REST client for the runbook API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new runbook REST client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ListRunbooks lists available runbooks.
func (c *Client) ListRunbooks() (*apirunbook.SummaryList, error) {
	var resp apirunbook.SummaryList
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/runbooks", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetExecution retrieves a runbook execution by ID.
func (c *Client) GetExecution(id string) (*apirunbook.ExecutionResponse, error) {
	var resp apirunbook.ExecutionResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/runbooks/executions/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListExecutionsOpts holds options for listing executions.
type ListExecutionsOpts struct {
	Runbook string
	State   string
	Since   string
	Limit   int
	Offset  int
}

// ListExecutions lists runbook executions with optional filters.
func (c *Client) ListExecutions(opts ListExecutionsOpts) (*apirunbook.ExecutionListResponse, error) {
	params := url.Values{}
	if opts.Runbook != "" {
		params.Set("runbook", opts.Runbook)
	}
	if opts.State != "" {
		params.Set("state", opts.State)
	}
	if opts.Since != "" {
		params.Set("since", opts.Since)
	}
	if opts.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", opts.Offset))
	}

	endpoint := c.baseURL + "/api/v1/runbooks/executions"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var resp apirunbook.ExecutionListResponse
	if err := c.doJSON(http.MethodGet, endpoint, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ExecuteRunbook executes a runbook by name.
func (c *Client) ExecuteRunbook(name string, req *apirunbook.ExecuteRequest) (*apirunbook.ExecuteResponse, error) {
	var resp apirunbook.ExecuteResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/runbooks/"+name+"/execute", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListAuditOpts holds options for listing audit events.
type ListAuditOpts struct {
	ExecutionID string
	Runbook     string
	Actor       string
	Outcome     string
	Start       string
	End         string
	Limit       int
	Offset      int
}

// ListAuditEvents lists runbook audit events with optional filters.
func (c *Client) ListAuditEvents(opts ListAuditOpts) (*apirunbook.AuditListResponse, error) {
	params := url.Values{}
	if opts.ExecutionID != "" {
		params.Set("execution_id", opts.ExecutionID)
	}
	if opts.Runbook != "" {
		params.Set("runbook", opts.Runbook)
	}
	if opts.Actor != "" {
		params.Set("actor", opts.Actor)
	}
	if opts.Outcome != "" {
		params.Set("outcome", opts.Outcome)
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

	endpoint := c.baseURL + "/api/v1/runbooks/audit"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var resp apirunbook.AuditListResponse
	if err := c.doJSON(http.MethodGet, endpoint, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) doJSON(method, endpoint string, reqBody, respBody interface{}) error {
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
