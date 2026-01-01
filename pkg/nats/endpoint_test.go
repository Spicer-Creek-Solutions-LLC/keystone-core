package nats

import (
	"os"
	"sort"
	"testing"
	"time"
)

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantErr  bool
		expected *Endpoint
	}{
		{
			name: "simple nats URL",
			url:  "nats://localhost:4222",
			expected: &Endpoint{
				URL:    "nats://localhost:4222",
				Scheme: SchemeNATS,
				Host:   "localhost",
				Port:   4222,
				Weight: 1,
			},
		},
		{
			name: "tls URL",
			url:  "tls://nats.example.com:4222",
			expected: &Endpoint{
				URL:    "tls://nats.example.com:4222",
				Scheme: SchemeTLS,
				Host:   "nats.example.com",
				Port:   4222,
				Weight: 1,
			},
		},
		{
			name: "websocket URL",
			url:  "ws://nats.example.com",
			expected: &Endpoint{
				URL:    "ws://nats.example.com",
				Scheme: SchemeWS,
				Host:   "nats.example.com",
				Port:   80,
				Weight: 1,
			},
		},
		{
			name: "secure websocket URL",
			url:  "wss://nats.example.com",
			expected: &Endpoint{
				URL:    "wss://nats.example.com",
				Scheme: SchemeWSS,
				Host:   "nats.example.com",
				Port:   443,
				Weight: 1,
			},
		},
		{
			name: "URL with credentials",
			url:  "nats://user:pass@localhost:4222",
			expected: &Endpoint{
				URL:      "nats://user:pass@localhost:4222",
				Scheme:   SchemeNATS,
				Host:     "localhost",
				Port:     4222,
				Username: "user",
				Password: "pass",
				Weight:   1,
			},
		},
		{
			name: "URL with token",
			url:  "nats://localhost:4222?token=secrettoken",
			expected: &Endpoint{
				URL:    "nats://localhost:4222?token=secrettoken",
				Scheme: SchemeNATS,
				Host:   "localhost",
				Port:   4222,
				Token:  "secrettoken",
				Weight: 1,
			},
		},
		{
			name: "URL with priority and weight",
			url:  "nats://localhost:4222?priority=5&weight=10",
			expected: &Endpoint{
				URL:      "nats://localhost:4222?priority=5&weight=10",
				Scheme:   SchemeNATS,
				Host:     "localhost",
				Port:     4222,
				Priority: 5,
				Weight:   10,
			},
		},
		{
			name: "URL with tags",
			url:  "nats://localhost:4222?tags=primary,us-east",
			expected: &Endpoint{
				URL:    "nats://localhost:4222?tags=primary,us-east",
				Scheme: SchemeNATS,
				Host:   "localhost",
				Port:   4222,
				Tags:   []string{"primary", "us-east"},
				Weight: 1,
			},
		},
		{
			name: "URL without scheme",
			url:  "localhost:4222",
			expected: &Endpoint{
				URL:    "nats://localhost:4222",
				Scheme: SchemeNATS,
				Host:   "localhost",
				Port:   4222,
				Weight: 1,
			},
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			url:     "http://localhost:4222",
			wantErr: true,
		},
		{
			name:    "invalid port",
			url:     "nats://localhost:invalid",
			wantErr: true,
		},
		{
			name:    "port out of range",
			url:     "nats://localhost:99999",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep, err := ParseEndpoint(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ep.Scheme != tt.expected.Scheme {
				t.Errorf("Scheme = %v, want %v", ep.Scheme, tt.expected.Scheme)
			}
			if ep.Host != tt.expected.Host {
				t.Errorf("Host = %v, want %v", ep.Host, tt.expected.Host)
			}
			if ep.Port != tt.expected.Port {
				t.Errorf("Port = %v, want %v", ep.Port, tt.expected.Port)
			}
			if ep.Username != tt.expected.Username {
				t.Errorf("Username = %v, want %v", ep.Username, tt.expected.Username)
			}
			if ep.Password != tt.expected.Password {
				t.Errorf("Password = %v, want %v", ep.Password, tt.expected.Password)
			}
			if ep.Token != tt.expected.Token {
				t.Errorf("Token = %v, want %v", ep.Token, tt.expected.Token)
			}
			if ep.Priority != tt.expected.Priority {
				t.Errorf("Priority = %v, want %v", ep.Priority, tt.expected.Priority)
			}
			if ep.Weight != tt.expected.Weight {
				t.Errorf("Weight = %v, want %v", ep.Weight, tt.expected.Weight)
			}
		})
	}
}

func TestEndpoint_IsTLS(t *testing.T) {
	tests := []struct {
		scheme Scheme
		want   bool
	}{
		{SchemeNATS, false},
		{SchemeTLS, true},
		{SchemeWS, false},
		{SchemeWSS, true},
	}

	for _, tt := range tests {
		ep := &Endpoint{Scheme: tt.scheme}
		if got := ep.IsTLS(); got != tt.want {
			t.Errorf("IsTLS() for %s = %v, want %v", tt.scheme, got, tt.want)
		}
	}
}

func TestEndpoint_IsWebSocket(t *testing.T) {
	tests := []struct {
		scheme Scheme
		want   bool
	}{
		{SchemeNATS, false},
		{SchemeTLS, false},
		{SchemeWS, true},
		{SchemeWSS, true},
	}

	for _, tt := range tests {
		ep := &Endpoint{Scheme: tt.scheme}
		if got := ep.IsWebSocket(); got != tt.want {
			t.Errorf("IsWebSocket() for %s = %v, want %v", tt.scheme, got, tt.want)
		}
	}
}

func TestEndpoint_ToNATSURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint *Endpoint
		want     string
	}{
		{
			name: "simple endpoint",
			endpoint: &Endpoint{
				Scheme: SchemeNATS,
				Host:   "localhost",
				Port:   4222,
			},
			want: "nats://localhost:4222",
		},
		{
			name: "with username and password",
			endpoint: &Endpoint{
				Scheme:   SchemeNATS,
				Host:     "localhost",
				Port:     4222,
				Username: "user",
				Password: "pass",
			},
			want: "nats://user:pass@localhost:4222",
		},
		{
			name: "tls endpoint",
			endpoint: &Endpoint{
				Scheme: SchemeTLS,
				Host:   "nats.example.com",
				Port:   4222,
			},
			want: "tls://nats.example.com:4222",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.endpoint.ToNATSURL(); got != tt.want {
				t.Errorf("ToNATSURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpointConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *EndpointConfig
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  DefaultEndpointConfig(),
			wantErr: false,
		},
		{
			name: "valid multi-URL config",
			config: &EndpointConfig{
				URLs: []string{
					"nats://nats1:4222",
					"nats://nats2:4222",
					"nats://nats3:4222",
				},
			},
			wantErr: false,
		},
		{
			name:    "empty URLs",
			config:  &EndpointConfig{URLs: []string{}},
			wantErr: true,
		},
		{
			name: "invalid URL",
			config: &EndpointConfig{
				URLs: []string{"http://invalid"},
			},
			wantErr: true,
		},
		{
			name: "invalid primary URL",
			config: &EndpointConfig{
				URLs:    []string{"nats://localhost:4222"},
				Primary: "http://invalid",
			},
			wantErr: true,
		},
		{
			name: "negative connect timeout",
			config: &EndpointConfig{
				URLs:           []string{"nats://localhost:4222"},
				ConnectTimeout: -1 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "cert without key",
			config: &EndpointConfig{
				URLs: []string{"nats://localhost:4222"},
				TLS: EndpointTLSConfig{
					Enabled:  true,
					CertFile: "/path/to/cert",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestEndpointConfig_InterpolateEnv(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_NATS_HOST", "nats.example.com")
	os.Setenv("TEST_NATS_TOKEN", "secrettoken")
	defer os.Unsetenv("TEST_NATS_HOST")
	defer os.Unsetenv("TEST_NATS_TOKEN")

	tests := []struct {
		name     string
		config   *EndpointConfig
		wantURLs []string
	}{
		{
			name: "interpolate ${VAR} syntax",
			config: &EndpointConfig{
				URLs: []string{"nats://${TEST_NATS_HOST}:4222"},
			},
			wantURLs: []string{"nats://nats.example.com:4222"},
		},
		{
			name: "interpolate $VAR syntax",
			config: &EndpointConfig{
				URLs: []string{"nats://$TEST_NATS_HOST:4222"},
			},
			wantURLs: []string{"nats://nats.example.com:4222"},
		},
		{
			name: "interpolate with default value",
			config: &EndpointConfig{
				URLs: []string{"nats://${NONEXISTENT:-localhost}:4222"},
			},
			wantURLs: []string{"nats://localhost:4222"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.InterpolateEnv()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for i, want := range tt.wantURLs {
				if tt.config.URLs[i] != want {
					t.Errorf("URL[%d] = %v, want %v", i, tt.config.URLs[i], want)
				}
			}
		})
	}
}

func TestEndpointConfig_GetEndpoints(t *testing.T) {
	config := &EndpointConfig{
		URLs: []string{
			"nats://nats1:4222",
			"tls://nats2:4222",
			"wss://nats3:443",
		},
		Token: "globaltoken",
	}

	endpoints, err := config.GetEndpoints()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(endpoints))
	}

	// Check priorities are assigned in order
	for i, ep := range endpoints {
		if ep.Priority != i {
			t.Errorf("endpoint[%d].Priority = %d, want %d", i, ep.Priority, i)
		}
		// Global token should be applied
		if ep.Token != "globaltoken" {
			t.Errorf("endpoint[%d].Token = %s, want globaltoken", i, ep.Token)
		}
	}
}

func TestEndpointList_Sort(t *testing.T) {
	list := EndpointList{
		{Host: "c", Priority: 2},
		{Host: "a", Priority: 0},
		{Host: "b", Priority: 1},
	}

	sort.Sort(list)

	if list[0].Host != "a" || list[1].Host != "b" || list[2].Host != "c" {
		t.Error("endpoints not sorted by priority")
	}
}

func TestEndpointList_FilterByScheme(t *testing.T) {
	list := EndpointList{
		{Scheme: SchemeNATS, Host: "a"},
		{Scheme: SchemeTLS, Host: "b"},
		{Scheme: SchemeNATS, Host: "c"},
		{Scheme: SchemeWSS, Host: "d"},
	}

	natsOnly := list.FilterByScheme(SchemeNATS)
	if len(natsOnly) != 2 {
		t.Errorf("expected 2 NATS endpoints, got %d", len(natsOnly))
	}

	tlsOnly := list.FilterByScheme(SchemeTLS)
	if len(tlsOnly) != 1 {
		t.Errorf("expected 1 TLS endpoint, got %d", len(tlsOnly))
	}
}

func TestEndpointList_FilterByTag(t *testing.T) {
	list := EndpointList{
		{Host: "a", Tags: []string{"primary", "us-east"}},
		{Host: "b", Tags: []string{"secondary", "us-west"}},
		{Host: "c", Tags: []string{"primary", "eu-west"}},
	}

	primary := list.FilterByTag("primary")
	if len(primary) != 2 {
		t.Errorf("expected 2 primary endpoints, got %d", len(primary))
	}

	usWest := list.FilterByTag("us-west")
	if len(usWest) != 1 {
		t.Errorf("expected 1 us-west endpoint, got %d", len(usWest))
	}
}

func TestEndpointList_URLs(t *testing.T) {
	list := EndpointList{
		{Scheme: SchemeNATS, Host: "nats1", Port: 4222},
		{Scheme: SchemeTLS, Host: "nats2", Port: 4222},
	}

	urls := list.URLs()
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(urls))
	}

	if urls[0] != "nats://nats1:4222" {
		t.Errorf("URL[0] = %s, want nats://nats1:4222", urls[0])
	}
	if urls[1] != "tls://nats2:4222" {
		t.Errorf("URL[1] = %s, want tls://nats2:4222", urls[1])
	}
}

func TestDefaultEndpointConfig(t *testing.T) {
	config := DefaultEndpointConfig()

	if len(config.URLs) != 1 {
		t.Errorf("expected 1 URL, got %d", len(config.URLs))
	}
	if config.URLs[0] != "nats://localhost:4222" {
		t.Errorf("URL = %s, want nats://localhost:4222", config.URLs[0])
	}
	if config.ConnectTimeout != 5*time.Second {
		t.Errorf("ConnectTimeout = %v, want 5s", config.ConnectTimeout)
	}
	if config.MaxReconnects != 60 {
		t.Errorf("MaxReconnects = %d, want 60", config.MaxReconnects)
	}
}

func TestEndpoint_Address(t *testing.T) {
	ep := &Endpoint{Host: "nats.example.com", Port: 4222}
	if got := ep.Address(); got != "nats.example.com:4222" {
		t.Errorf("Address() = %s, want nats.example.com:4222", got)
	}
}

func TestEndpoint_HasCredentials(t *testing.T) {
	tests := []struct {
		name     string
		endpoint *Endpoint
		want     bool
	}{
		{
			name:     "no credentials",
			endpoint: &Endpoint{},
			want:     false,
		},
		{
			name:     "with username",
			endpoint: &Endpoint{Username: "user"},
			want:     true,
		},
		{
			name:     "with token",
			endpoint: &Endpoint{Token: "token"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.endpoint.HasCredentials(); got != tt.want {
				t.Errorf("HasCredentials() = %v, want %v", got, tt.want)
			}
		})
	}
}
