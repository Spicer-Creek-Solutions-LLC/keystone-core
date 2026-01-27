package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// APIHandler executes HTTP API calls.
type APIHandler struct {
	client *http.Client
}

// NewAPIHandler creates a new API handler with default settings.
func NewAPIHandler() *APIHandler {
	return &APIHandler{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewAPIHandlerWithClient creates a new API handler with a custom HTTP client.
func NewAPIHandlerWithClient(client *http.Client) *APIHandler {
	return &APIHandler{
		client: client,
	}
}

// Type returns the step type.
func (h *APIHandler) Type() runbook.StepType {
	return runbook.StepTypeAPI
}

// Validate checks step config.
func (h *APIHandler) Validate(step *runbook.Step) error {
	// URL is required
	url, hasURL := step.Config["url"]
	if !hasURL {
		return errors.New("api step requires 'url' in config")
	}

	if _, ok := url.(string); !ok {
		return errors.New("url must be a string")
	}

	// Validate method if provided
	if method, ok := step.Config["method"].(string); ok {
		method = strings.ToUpper(method)
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
			// Valid methods
		default:
			return fmt.Errorf("invalid HTTP method: %s", method)
		}
	}

	// Validate timeout if provided
	if timeout, ok := step.Config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			return errors.New("invalid timeout format")
		}
	}

	// Validate expected_status if provided
	if expectedStatus, ok := step.Config["expected_status"]; ok {
		switch v := expectedStatus.(type) {
		case int:
			// Valid
		case float64:
			// YAML parses numbers as float64
			if v != float64(int(v)) {
				return errors.New("expected_status must be an integer")
			}
		default:
			return errors.New("expected_status must be an integer")
		}
	}

	return nil
}

// Execute runs the step.
func (h *APIHandler) Execute(ctx context.Context, step *runbook.Step, vars VariableContext) (*runbook.StepResult, error) {
	start := time.Now()

	// Get URL
	url, _ := step.Config["url"].(string)

	// Get method (default to GET)
	method := "GET"
	if m, ok := step.Config["method"].(string); ok {
		method = strings.ToUpper(m)
	}

	// Build request body
	var body io.Reader
	if bodyData, ok := step.Config["body"]; ok {
		switch v := bodyData.(type) {
		case string:
			body = strings.NewReader(v)
		case map[string]interface{}:
			jsonData, err := json.Marshal(v)
			if err != nil {
				return &runbook.StepResult{
					Success:  false,
					Message:  fmt.Sprintf("failed to marshal body: %v", err),
					Duration: time.Since(start),
				}, err
			}
			body = bytes.NewReader(jsonData)
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to create request: %v", err),
			Duration: time.Since(start),
		}, err
	}

	// Set headers
	if headers, ok := step.Config["headers"].(map[string]interface{}); ok {
		for key, val := range headers {
			if strVal, ok := val.(string); ok {
				req.Header.Set(key, strVal)
			}
		}
	}

	// Set default Content-Type for POST/PUT/PATCH if not specified and body exists
	if body != nil && req.Header.Get("Content-Type") == "" {
		if _, ok := step.Config["body"].(map[string]interface{}); ok {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	// Set timeout if specified
	var client *http.Client
	if timeout, ok := step.Config["timeout"].(string); ok {
		if dur, err := time.ParseDuration(timeout); err == nil {
			client = &http.Client{Timeout: dur}
		}
	}
	if client == nil {
		client = h.client
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("request failed: %v", err),
			Duration: time.Since(start),
			Outputs: map[string]interface{}{
				"error": err.Error(),
			},
		}, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to read response: %v", err),
			Duration: time.Since(start),
			Outputs: map[string]interface{}{
				"status_code": resp.StatusCode,
				"error":       err.Error(),
			},
		}, err
	}

	// Build outputs
	outputs := map[string]interface{}{
		"status_code": resp.StatusCode,
		"status":      resp.Status,
		"body":        string(respBody),
		"headers":     headerToMap(resp.Header),
	}

	// Try to parse JSON response
	var jsonBody interface{}
	if err := json.Unmarshal(respBody, &jsonBody); err == nil {
		outputs["json"] = jsonBody
	}

	// Check expected status
	expectedStatus := 0
	if expected, ok := step.Config["expected_status"].(int); ok {
		expectedStatus = expected
	} else if expected, ok := step.Config["expected_status"].(float64); ok {
		expectedStatus = int(expected)
	}

	// Default: 2xx status codes are successful
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if expectedStatus > 0 {
		success = resp.StatusCode == expectedStatus
	}

	result := &runbook.StepResult{
		Success:  success,
		Duration: time.Since(start),
		Outputs:  outputs,
	}

	if success {
		result.Message = fmt.Sprintf("%s %s returned %d", method, url, resp.StatusCode)
	} else {
		result.Message = fmt.Sprintf("%s %s returned unexpected status %d", method, url, resp.StatusCode)
		return result, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result, nil
}

// headerToMap converts http.Header to a simple string map.
func headerToMap(header http.Header) map[string]string {
	result := make(map[string]string)
	for key, values := range header {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}
