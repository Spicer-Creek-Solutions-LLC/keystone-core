package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// restClient provides access to the GitOps REST API.
type restClient struct {
	baseURL    string
	httpClient *http.Client
}

func newRESTClient(serverAddr string) *restClient {
	return &restClient{
		baseURL: serverAddr,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// rollbackRequest is the request body for POST /api/v1/gitops/rollback.
type rollbackRequest struct {
	Application      string `json:"application"`
	Namespace        string `json:"namespace,omitempty"`
	Type             string `json:"type"`
	Strategy         string `json:"strategy,omitempty"`
	Revision         string `json:"revision,omitempty"`
	Reason           string `json:"reason"`
	RequestedBy      string `json:"requested_by,omitempty"`
	SkipVerification bool   `json:"skip_verification,omitempty"`
}

// rollbackResponse is the response from the rollback API.
type rollbackResponse struct {
	ID               string    `json:"id"`
	Application      string    `json:"application"`
	Namespace        string    `json:"namespace,omitempty"`
	Type             string    `json:"type"`
	Strategy         string    `json:"strategy"`
	Status           string    `json:"status"`
	PreviousRevision string    `json:"previous_revision,omitempty"`
	CurrentRevision  string    `json:"current_revision,omitempty"`
	Message          string    `json:"message,omitempty"`
	Error            string    `json:"error,omitempty"`
	Duration         string    `json:"duration,omitempty"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time,omitempty"`
}

// apiError is the error response from the API.
type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// TriggerRollback triggers a rollback via the REST API.
func (c *restClient) TriggerRollback(req rollbackRequest) (*rollbackResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/api/v1/gitops/rollback"
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Message != "" {
			return nil, fmt.Errorf("rollback failed (%d): %s", resp.StatusCode, apiErr.Message)
		}
		return nil, fmt.Errorf("rollback failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result rollbackResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}
