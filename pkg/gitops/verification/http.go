package verification

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPVerifier performs HTTP health checks
type HTTPVerifier struct {
	client *http.Client
}

// NewHTTPVerifier creates a new HTTP verifier
func NewHTTPVerifier() *HTTPVerifier {
	return &HTTPVerifier{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Type returns the verification type
func (v *HTTPVerifier) Type() VerificationType {
	return VerificationTypeHTTP
}

// Verify performs an HTTP health check
func (v *HTTPVerifier) Verify(step *VerificationStep) (*VerificationResult, error) {
	result := &VerificationResult{
		StepName:  step.Name,
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}

	// Extract configuration
	url, ok := step.Config["url"].(string)
	if !ok || url == "" {
		result.Success = false
		result.Message = "Missing 'url' in config"
		return result, fmt.Errorf("missing url")
	}

	method := "GET"
	if m, ok := step.Config["method"].(string); ok {
		method = strings.ToUpper(m)
	}

	// Optional fields
	expectedStatus := 200
	if s, ok := step.Config["expected_status"].(int); ok {
		expectedStatus = s
	} else if s, ok := step.Config["expected_status"].(float64); ok {
		expectedStatus = int(s)
	}

	var expectedBody string
	if b, ok := step.Config["expected_body"].(string); ok {
		expectedBody = b
	}

	// Create request
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Failed to create request: %v", err)
		return result, err
	}

	// Add headers if specified
	if headers, ok := step.Config["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if vs, ok := v.(string); ok {
				req.Header.Set(k, vs)
			}
		}
	}

	// Execute request
	start := time.Now()
	resp, err := v.client.Do(req)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("HTTP request failed: %v", err)
		result.Data["error"] = err.Error()
		return result, err
	}
	defer resp.Body.Close()

	responseTime := time.Since(start)
	result.Data["response_time_ms"] = responseTime.Milliseconds()
	result.Data["status_code"] = resp.StatusCode

	// Check status code
	if resp.StatusCode != expectedStatus {
		result.Success = false
		result.Message = fmt.Sprintf("Expected status %d, got %d", expectedStatus, resp.StatusCode)
		return result, nil
	}

	// Check body if expected
	if expectedBody != "" {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to read response body: %v", err)
			return result, err
		}

		bodyStr := string(body)
		result.Data["response_body"] = bodyStr

		if !strings.Contains(bodyStr, expectedBody) {
			result.Success = false
			result.Message = fmt.Sprintf("Response body does not contain expected text: %s", expectedBody)
			return result, nil
		}
	}

	// Success
	result.Success = true
	result.Message = fmt.Sprintf("HTTP %s %s returned %d in %dms", method, url, resp.StatusCode, responseTime.Milliseconds())

	return result, nil
}
