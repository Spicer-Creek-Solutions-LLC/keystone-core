package verification

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPVerifierType(t *testing.T) {
	verifier := NewHTTPVerifier()
	if verifier.Type() != VerificationTypeHTTP {
		t.Errorf("Type() = %v, want %v", verifier.Type(), VerificationTypeHTTP)
	}
}

func TestHTTPVerifierSuccess(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	}))
	defer server.Close()

	verifier := NewHTTPVerifier()
	step := &VerificationStep{
		Name: "http-check",
		Type: VerificationTypeHTTP,
		Config: map[string]interface{}{
			"url":             server.URL,
			"expected_status": 200,
			"expected_body":   "healthy",
		},
	}

	result, err := verifier.Verify(step)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Verification should succeed: %s", result.Message)
	}

	if result.Data["status_code"] != 200 {
		t.Errorf("Status code = %v, want 200", result.Data["status_code"])
	}
}

func TestHTTPVerifierWrongStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	verifier := NewHTTPVerifier()
	step := &VerificationStep{
		Name: "http-check",
		Type: VerificationTypeHTTP,
		Config: map[string]interface{}{
			"url":             server.URL,
			"expected_status": 200,
		},
	}

	result, err := verifier.Verify(step)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if result.Success {
		t.Error("Verification should fail for wrong status code")
	}
}

func TestHTTPVerifierMissingURL(t *testing.T) {
	verifier := NewHTTPVerifier()
	step := &VerificationStep{
		Name:   "http-check",
		Type:   VerificationTypeHTTP,
		Config: map[string]interface{}{},
	}

	result, err := verifier.Verify(step)
	if err == nil {
		t.Error("Expected error for missing URL")
	}

	if result.Success {
		t.Error("Verification should fail for missing URL")
	}
}

func TestHTTPVerifierCustomMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	verifier := NewHTTPVerifier()
	step := &VerificationStep{
		Name: "http-post",
		Type: VerificationTypeHTTP,
		Config: map[string]interface{}{
			"url":    server.URL,
			"method": "POST",
		},
	}

	result, err := verifier.Verify(step)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !result.Success {
		t.Errorf("POST request should succeed: %s", result.Message)
	}
}

func TestHTTPVerifierHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "test-value" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	verifier := NewHTTPVerifier()
	step := &VerificationStep{
		Name: "http-with-headers",
		Type: VerificationTypeHTTP,
		Config: map[string]interface{}{
			"url": server.URL,
			"headers": map[string]interface{}{
				"X-Custom-Header": "test-value",
			},
		},
	}

	result, err := verifier.Verify(step)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Request with headers should succeed: %s", result.Message)
	}
}
