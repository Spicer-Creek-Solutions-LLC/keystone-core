package capabilities

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPGetCapability_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cap         *HTTPGetCapability
		expectError bool
	}{
		{
			name: "valid capability",
			cap: &HTTPGetCapability{
				AllowedDomains: []string{"api.example.com"},
				TimeoutMax:     30 * time.Second,
				MaxRespSize:    1024 * 1024,
			},
			expectError: false,
		},
		{
			name: "no allowed domains",
			cap: &HTTPGetCapability{
				TimeoutMax: 30 * time.Second,
			},
			expectError: true,
		},
		{
			name: "zero timeout",
			cap: &HTTPGetCapability{
				AllowedDomains: []string{"api.example.com"},
				TimeoutMax:     0,
			},
			expectError: true,
		},
		{
			name: "negative max response size",
			cap: &HTTPGetCapability{
				AllowedDomains: []string{"api.example.com"},
				TimeoutMax:     30 * time.Second,
				MaxRespSize:    -1,
			},
			expectError: true,
		},
		{
			name: "invalid rate limit",
			cap: &HTTPGetCapability{
				AllowedDomains: []string{"api.example.com"},
				TimeoutMax:     30 * time.Second,
				RateLimit: &RateLimit{
					Requests: 0,
					Period:   time.Minute,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHTTPGetCapability_CheckDomain(t *testing.T) {
	cap := &HTTPGetCapability{
		AllowedDomains: []string{
			"api.example.com",
			"*.github.com",
		},
		DeniedDomains: []string{
			"blocked.example.com",
		},
		TimeoutMax: 30 * time.Second,
	}

	tests := []struct {
		name        string
		domain      string
		expectError error
	}{
		{
			name:        "allowed domain",
			domain:      "api.example.com",
			expectError: nil,
		},
		{
			name:        "denied domain",
			domain:      "blocked.example.com",
			expectError: ErrDomainDenied,
		},
		{
			name:        "not allowed domain",
			domain:      "malicious.com",
			expectError: ErrDomainNotAllowed,
		},
		{
			name:        "wildcard match",
			domain:      "api.github.com",
			expectError: nil,
		},
		{
			name:        "wildcard root match",
			domain:      "github.com",
			expectError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cap.CheckDomain(tt.domain)
			if tt.expectError != nil {
				if err == nil {
					t.Errorf("expected error %v but got nil", tt.expectError)
				} else if !errors.Is(err, tt.expectError) {
					t.Errorf("expected error %v but got %v", tt.expectError, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHTTPGetCapability_Get(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET request, got %s", r.Method)
		}

		// Check custom header
		if r.Header.Get("X-Custom") != "test" {
			t.Errorf("expected X-Custom header 'test', got %s", r.Header.Get("X-Custom"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	// Extract host from server URL (remove http://)
	serverHost := strings.TrimPrefix(server.URL, "http://")

	cap := &HTTPGetCapability{
		AllowedDomains: []string{serverHost},
		TimeoutMax:     30 * time.Second,
		MaxRespSize:    1024,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test successful GET
	resp, err := cap.Get(ctx, server.URL, map[string]string{
		"X-Custom": "test",
	})

	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if string(resp.Body) != "test response" {
		t.Errorf("expected body 'test response', got %q", string(resp.Body))
	}
}

func TestHTTPGetCapability_GetMaxSize(t *testing.T) {
	// Create test server with large response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write more than max allowed
		w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer server.Close()

	serverHost := strings.TrimPrefix(server.URL, "http://")

	cap := &HTTPGetCapability{
		AllowedDomains: []string{serverHost},
		TimeoutMax:     30 * time.Second,
		MaxRespSize:    50, // Smaller than response
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test GET with size limit
	_, err := cap.Get(ctx, server.URL, nil)
	if !errors.Is(err, ErrMaxSizeExceeded) {
		t.Errorf("expected ErrMaxSizeExceeded, got %v", err)
	}
}

func TestHTTPGetCapability_GetRateLimit(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	serverHost := strings.TrimPrefix(server.URL, "http://")

	cap := &HTTPGetCapability{
		AllowedDomains: []string{serverHost},
		TimeoutMax:     30 * time.Second,
		RateLimit: &RateLimit{
			Requests: 2,
			Period:   time.Minute,
		},
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// First request should succeed
	_, err := cap.Get(ctx, server.URL, nil)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	// Second request should succeed
	_, err = cap.Get(ctx, server.URL, nil)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	// Third request should fail due to rate limit
	_, err = cap.Get(ctx, server.URL, nil)
	if !errors.Is(err, ErrRateLimitExceeded) {
		t.Errorf("expected ErrRateLimitExceeded, got %v", err)
	}
}

func TestHTTPPostCapability_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cap         *HTTPPostCapability
		expectError bool
	}{
		{
			name: "valid capability",
			cap: &HTTPPostCapability{
				AllowedDomains: []string{"api.example.com"},
				TimeoutMax:     30 * time.Second,
				MaxReqSize:     1024,
				MaxRespSize:    1024,
			},
			expectError: false,
		},
		{
			name: "no allowed domains",
			cap: &HTTPPostCapability{
				TimeoutMax: 30 * time.Second,
			},
			expectError: true,
		},
		{
			name: "negative max request size",
			cap: &HTTPPostCapability{
				AllowedDomains: []string{"api.example.com"},
				TimeoutMax:     30 * time.Second,
				MaxReqSize:     -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHTTPPostCapability_Post(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		// Check content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %s", r.Header.Get("Content-Type"))
		}

		// Read body
		body := make([]byte, 100)
		n, _ := r.Body.Read(body)
		body = body[:n]

		if string(body) != `{"test":"data"}` {
			t.Errorf("expected body '{\"test\":\"data\"}', got %q", string(body))
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))
	}))
	defer server.Close()

	serverHost := strings.TrimPrefix(server.URL, "http://")

	cap := &HTTPPostCapability{
		AllowedDomains: []string{serverHost},
		TimeoutMax:     30 * time.Second,
		MaxReqSize:     1024,
		MaxRespSize:    1024,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test successful POST
	resp, err := cap.Post(ctx, server.URL, []byte(`{"test":"data"}`), map[string]string{
		"Content-Type": "application/json",
	})

	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}

	if string(resp.Body) != "created" {
		t.Errorf("expected body 'created', got %q", string(resp.Body))
	}
}

func TestHTTPPostCapability_PostMaxReqSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverHost := strings.TrimPrefix(server.URL, "http://")

	cap := &HTTPPostCapability{
		AllowedDomains: []string{serverHost},
		TimeoutMax:     30 * time.Second,
		MaxReqSize:     10,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test POST with large body
	largeBody := []byte(strings.Repeat("x", 100))
	_, err := cap.Post(ctx, server.URL, largeBody, nil)

	if !errors.Is(err, ErrMaxSizeExceeded) {
		t.Errorf("expected ErrMaxSizeExceeded, got %v", err)
	}
}

func TestRateLimit_Validate(t *testing.T) {
	tests := []struct {
		name        string
		rateLimit   *RateLimit
		expectError bool
	}{
		{
			name: "valid",
			rateLimit: &RateLimit{
				Requests: 10,
				Period:   time.Minute,
			},
			expectError: false,
		},
		{
			name: "zero requests",
			rateLimit: &RateLimit{
				Requests: 0,
				Period:   time.Minute,
			},
			expectError: true,
		},
		{
			name: "zero period",
			rateLimit: &RateLimit{
				Requests: 10,
				Period:   0,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rateLimit.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestMatchesDomain(t *testing.T) {
	tests := []struct {
		pattern  string
		domain   string
		expected bool
	}{
		{
			pattern:  "example.com",
			domain:   "example.com",
			expected: true,
		},
		{
			pattern:  "example.com",
			domain:   "api.example.com",
			expected: false,
		},
		{
			pattern:  "*.example.com",
			domain:   "api.example.com",
			expected: true,
		},
		{
			pattern:  "*.example.com",
			domain:   "example.com",
			expected: true,
		},
		{
			pattern:  "*.example.com",
			domain:   "other.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.domain, func(t *testing.T) {
			result := matchesDomain(tt.pattern, tt.domain)
			if result != tt.expected {
				t.Errorf("matchesDomain(%q, %q) = %v, expected %v", tt.pattern, tt.domain, result, tt.expected)
			}
		})
	}
}
