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

	apischedule "github.com/shawnbutts/keystone-core/pkg/api/schedule"
)

// Client is a REST client for the schedule API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new schedule REST client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ListSchedules lists schedules with optional filters.
func (c *Client) ListSchedules(schedType, status string, limit int) (*apischedule.ListResponse, error) {
	params := url.Values{}
	if schedType != "" {
		params.Set("type", schedType)
	}
	if status != "" {
		params.Set("status", status)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	u := c.baseURL + "/api/v1/schedules"
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	var resp apischedule.ListResponse
	if err := c.doJSON(http.MethodGet, u, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetSchedule retrieves a schedule by ID.
func (c *Client) GetSchedule(id string) (*apischedule.Response, error) {
	var resp apischedule.Response
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/schedules/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateSchedule creates a new schedule.
func (c *Client) CreateSchedule(req *apischedule.CreateScheduleRequest) (*apischedule.Response, error) {
	var resp apischedule.Response
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/schedules", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateSchedule updates an existing schedule.
func (c *Client) UpdateSchedule(id string, req *apischedule.CreateScheduleRequest) (*apischedule.Response, error) {
	var resp apischedule.Response
	if err := c.doJSON(http.MethodPut, c.baseURL+"/api/v1/schedules/"+id, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteSchedule deletes a schedule.
func (c *Client) DeleteSchedule(id string) error {
	return c.doJSON(http.MethodDelete, c.baseURL+"/api/v1/schedules/"+id, nil, nil)
}

// TriggerSchedule triggers a schedule for immediate execution.
func (c *Client) TriggerSchedule(id, triggeredBy string) (*apischedule.ExecutionResponse, error) {
	req := &apischedule.TriggerRequest{TriggeredBy: triggeredBy}
	var resp apischedule.ExecutionResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/schedules/"+id+"/trigger", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PauseSchedule pauses a schedule.
func (c *Client) PauseSchedule(id string) error {
	return c.doJSON(http.MethodPost, c.baseURL+"/api/v1/schedules/"+id+"/pause", nil, nil)
}

// ResumeSchedule resumes a paused schedule.
func (c *Client) ResumeSchedule(id string) error {
	return c.doJSON(http.MethodPost, c.baseURL+"/api/v1/schedules/"+id+"/resume", nil, nil)
}

// EnableSchedule enables a disabled schedule.
func (c *Client) EnableSchedule(id string) error {
	return c.doJSON(http.MethodPost, c.baseURL+"/api/v1/schedules/"+id+"/enable", nil, nil)
}

// DisableSchedule disables a schedule.
func (c *Client) DisableSchedule(id string) error {
	return c.doJSON(http.MethodPost, c.baseURL+"/api/v1/schedules/"+id+"/disable", nil, nil)
}

// GetHistory retrieves execution history for a schedule.
func (c *Client) GetHistory(scheduleID, status string, limit int) (*apischedule.ExecutionListResponse, error) {
	params := url.Values{}
	if status != "" {
		params.Set("status", status)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	u := c.baseURL + "/api/v1/schedules/" + scheduleID + "/history"
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	var resp apischedule.ExecutionListResponse
	if err := c.doJSON(http.MethodGet, u, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListWindows lists maintenance windows with optional filters.
func (c *Client) ListWindows(status, windowType string, limit int) (*apischedule.WindowListResponse, error) {
	params := url.Values{}
	if status != "" {
		params.Set("status", status)
	}
	if windowType != "" {
		params.Set("type", windowType)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	u := c.baseURL + "/api/v1/maintenance/windows"
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	var resp apischedule.WindowListResponse
	if err := c.doJSON(http.MethodGet, u, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetWindow retrieves a maintenance window by ID.
func (c *Client) GetWindow(id string) (*apischedule.WindowResponse, error) {
	var resp apischedule.WindowResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/maintenance/windows/"+id, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateWindow creates a new maintenance window.
func (c *Client) CreateWindow(req *apischedule.CreateWindowRequest) (*apischedule.WindowResponse, error) {
	var resp apischedule.WindowResponse
	if err := c.doJSON(http.MethodPost, c.baseURL+"/api/v1/maintenance/windows", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteWindow deletes a maintenance window.
func (c *Client) DeleteWindow(id string) error {
	return c.doJSON(http.MethodDelete, c.baseURL+"/api/v1/maintenance/windows/"+id, nil, nil)
}

// StartWindow starts a scheduled maintenance window.
func (c *Client) StartWindow(id string) error {
	return c.doJSON(http.MethodPost, c.baseURL+"/api/v1/maintenance/windows/"+id+"/start", nil, nil)
}

// EndWindow ends an active maintenance window.
func (c *Client) EndWindow(id string) error {
	return c.doJSON(http.MethodPost, c.baseURL+"/api/v1/maintenance/windows/"+id+"/end", nil, nil)
}

// CancelWindow cancels a maintenance window.
func (c *Client) CancelWindow(id, reason string) error {
	req := &apischedule.CancelWindowRequest{Reason: reason}
	return c.doJSON(http.MethodPost, c.baseURL+"/api/v1/maintenance/windows/"+id+"/cancel", req, nil)
}

// ExtendWindow extends a maintenance window.
func (c *Client) ExtendWindow(id string, req *apischedule.ExtendWindowRequest) error {
	return c.doJSON(http.MethodPost, c.baseURL+"/api/v1/maintenance/windows/"+id+"/extend", req, nil)
}

// GetActiveWindows retrieves currently active maintenance windows.
func (c *Client) GetActiveWindows() (*apischedule.WindowListResponse, error) {
	var resp apischedule.WindowListResponse
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/maintenance/windows/active", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetConflicts retrieves conflicts for a maintenance window.
func (c *Client) GetConflicts(id string) ([]apischedule.ConflictResponse, error) {
	var resp struct {
		Conflicts []apischedule.ConflictResponse `json:"conflicts"`
	}
	if err := c.doJSON(http.MethodGet, c.baseURL+"/api/v1/maintenance/windows/"+id+"/conflicts", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Conflicts, nil
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
