package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestEndpointType(t *testing.T) {
	tests := []struct {
		endpointType EndpointType
		expected     string
	}{
		{EndpointTypeNATS, "nats"},
		{EndpointTypeNATSTLS, "nats-tls"},
		{EndpointTypeWebSocket, "nats-ws"},
		{EndpointTypeWebSocketTLS, "nats-wss"},
	}

	for _, tt := range tests {
		t.Run(string(tt.endpointType), func(t *testing.T) {
			if string(tt.endpointType) != tt.expected {
				t.Errorf("EndpointType = %v, want %v", tt.endpointType, tt.expected)
			}
		})
	}
}

func TestEndpointHealthStatus(t *testing.T) {
	tests := []struct {
		status   EndpointHealthStatus
		expected string
	}{
		{EndpointHealthUnknown, "unknown"},
		{EndpointHealthHealthy, "healthy"},
		{EndpointHealthDegraded, "degraded"},
		{EndpointHealthUnhealthy, "unhealthy"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("EndpointHealthStatus = %v, want %v", tt.status, tt.expected)
			}
		})
	}
}

func TestEndpointAdvertisement_GetURL(t *testing.T) {
	tests := []struct {
		name     string
		adv      *EndpointAdvertisement
		expected string
	}{
		{
			name: "basic NATS",
			adv: &EndpointAdvertisement{
				EndpointType: EndpointTypeNATS,
				Host:         "192.168.1.10",
				Port:         4222,
			},
			expected: "nats://192.168.1.10:4222",
		},
		{
			name: "NATS TLS",
			adv: &EndpointAdvertisement{
				EndpointType: EndpointTypeNATSTLS,
				Host:         "192.168.1.10",
				Port:         4222,
			},
			expected: "tls://192.168.1.10:4222",
		},
		{
			name: "WebSocket",
			adv: &EndpointAdvertisement{
				EndpointType: EndpointTypeWebSocket,
				Host:         "192.168.1.10",
				Port:         80,
			},
			expected: "ws://192.168.1.10:80",
		},
		{
			name: "WebSocket TLS",
			adv: &EndpointAdvertisement{
				EndpointType: EndpointTypeWebSocketTLS,
				Host:         "192.168.1.10",
				Port:         443,
			},
			expected: "wss://192.168.1.10:443",
		},
		{
			name: "with public host",
			adv: &EndpointAdvertisement{
				EndpointType: EndpointTypeNATS,
				Host:         "192.168.1.10",
				Port:         4222,
				PublicHost:   "agent.example.com",
			},
			expected: "nats://agent.example.com:4222",
		},
		{
			name: "with public port",
			adv: &EndpointAdvertisement{
				EndpointType: EndpointTypeNATS,
				Host:         "192.168.1.10",
				Port:         4222,
				PublicHost:   "agent.example.com",
				PublicPort:   5222,
			},
			expected: "nats://agent.example.com:5222",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.adv.GetURL()
			if got != tt.expected {
				t.Errorf("GetURL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEndpointAdvertisement_IsExpired(t *testing.T) {
	tests := []struct {
		name    string
		adv     *EndpointAdvertisement
		expired bool
	}{
		{
			name: "not expired",
			adv: &EndpointAdvertisement{
				TTL:       60,
				Timestamp: time.Now(),
			},
			expired: false,
		},
		{
			name: "expired",
			adv: &EndpointAdvertisement{
				TTL:       1,
				Timestamp: time.Now().Add(-5 * time.Second),
			},
			expired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.adv.IsExpired(); got != tt.expired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expired)
			}
		})
	}
}

func TestEndpointAdvertisement_Validate(t *testing.T) {
	tests := []struct {
		name    string
		adv     *EndpointAdvertisement
		wantErr bool
	}{
		{
			name: "valid",
			adv: &EndpointAdvertisement{
				AgentID: "agent-1",
				Host:    "192.168.1.10",
				Port:    4222,
				TTL:     30,
			},
			wantErr: false,
		},
		{
			name: "missing agent_id",
			adv: &EndpointAdvertisement{
				Host: "192.168.1.10",
				Port: 4222,
				TTL:  30,
			},
			wantErr: true,
		},
		{
			name: "missing host",
			adv: &EndpointAdvertisement{
				AgentID: "agent-1",
				Port:    4222,
				TTL:     30,
			},
			wantErr: true,
		},
		{
			name: "invalid port zero",
			adv: &EndpointAdvertisement{
				AgentID: "agent-1",
				Host:    "192.168.1.10",
				Port:    0,
				TTL:     30,
			},
			wantErr: true,
		},
		{
			name: "invalid port negative",
			adv: &EndpointAdvertisement{
				AgentID: "agent-1",
				Host:    "192.168.1.10",
				Port:    -1,
				TTL:     30,
			},
			wantErr: true,
		},
		{
			name: "invalid port too large",
			adv: &EndpointAdvertisement{
				AgentID: "agent-1",
				Host:    "192.168.1.10",
				Port:    70000,
				TTL:     30,
			},
			wantErr: true,
		},
		{
			name: "invalid TTL",
			adv: &EndpointAdvertisement{
				AgentID: "agent-1",
				Host:    "192.168.1.10",
				Port:    4222,
				TTL:     0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.adv.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultEndpointAdvertiserConfig(t *testing.T) {
	cfg := DefaultEndpointAdvertiserConfig("agent-1", 4222)

	if cfg.AgentID != "agent-1" {
		t.Errorf("AgentID = %v, want agent-1", cfg.AgentID)
	}
	if cfg.LocalPort != 4222 {
		t.Errorf("LocalPort = %v, want 4222", cfg.LocalPort)
	}
	if cfg.TTL != 30 {
		t.Errorf("TTL = %v, want 30", cfg.TTL)
	}
	if cfg.AdvertiseInterval != 10*time.Second {
		t.Errorf("AdvertiseInterval = %v, want 10s", cfg.AdvertiseInterval)
	}
	if !cfg.DetectPublicIP {
		t.Error("DetectPublicIP should be true by default")
	}
	if len(cfg.PublicIPServices) == 0 {
		t.Error("PublicIPServices should have default values")
	}
}

func TestNewEndpointAdvertiser(t *testing.T) {
	tests := []struct {
		name    string
		config  *EndpointAdvertiserConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing agent_id",
			config: &EndpointAdvertiserConfig{
				LocalPort: 4222,
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: &EndpointAdvertiserConfig{
				AgentID:   "agent-1",
				LocalPort: 0,
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &EndpointAdvertiserConfig{
				AgentID:   "agent-1",
				LocalPort: 4222,
			},
			wantErr: false,
		},
		{
			name:    "default config",
			config:  DefaultEndpointAdvertiserConfig("agent-1", 4222),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adv, err := NewEndpointAdvertiser(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEndpointAdvertiser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && adv == nil {
				t.Error("expected advertiser to be non-nil")
			}
		})
	}
}

func TestEndpointAdvertiser_Lifecycle(t *testing.T) {
	cfg := DefaultEndpointAdvertiserConfig("agent-1", 4222)
	cfg.DetectPublicIP = false // Skip public IP detection for tests
	cfg.AdvertiseInterval = 100 * time.Millisecond

	adv, err := NewEndpointAdvertiser(cfg)
	if err != nil {
		t.Fatalf("failed to create advertiser: %v", err)
	}

	var advertisements []*EndpointAdvertisement
	var mu sync.Mutex

	adv.SetAdvertiseCallback(func(ad *EndpointAdvertisement) error {
		mu.Lock()
		advertisements = append(advertisements, ad)
		mu.Unlock()
		return nil
	})

	ctx := context.Background()

	// Start
	if err := adv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !adv.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	// Wait for a few advertisements
	time.Sleep(350 * time.Millisecond)

	mu.Lock()
	count := len(advertisements)
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 advertisements, got %d", count)
	}

	// Check last advertisement
	last := adv.GetLastAdvertisement()
	if last == nil {
		t.Error("GetLastAdvertisement() = nil")
	}
	if last != nil {
		if last.AgentID != "agent-1" {
			t.Errorf("AgentID = %v, want agent-1", last.AgentID)
		}
		if last.SequenceNumber < 1 {
			t.Errorf("SequenceNumber = %v, want >= 1", last.SequenceNumber)
		}
	}

	// Stop
	if err := adv.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if adv.IsRunning() {
		t.Error("IsRunning() = true after stop, want false")
	}
}

func TestEndpointAdvertiser_ManualAdvertise(t *testing.T) {
	cfg := DefaultEndpointAdvertiserConfig("agent-1", 4222)
	cfg.DetectPublicIP = false
	cfg.PublicHost = "public.example.com"
	cfg.TLSEnabled = true
	cfg.AuthRequired = true
	cfg.Capabilities = []string{"jetstream", "leafnode"}
	cfg.Metadata = map[string]string{"region": "us-east-1"}

	adv, err := NewEndpointAdvertiser(cfg)
	if err != nil {
		t.Fatalf("failed to create advertiser: %v", err)
	}

	var received *EndpointAdvertisement
	adv.SetAdvertiseCallback(func(ad *EndpointAdvertisement) error {
		received = ad
		return nil
	})

	// Manual advertise
	if err := adv.Advertise(); err != nil {
		t.Fatalf("Advertise() error = %v", err)
	}

	if received == nil {
		t.Fatal("expected to receive advertisement")
	}

	if received.AgentID != "agent-1" {
		t.Errorf("AgentID = %v, want agent-1", received.AgentID)
	}
	if received.PublicHost != "public.example.com" {
		t.Errorf("PublicHost = %v, want public.example.com", received.PublicHost)
	}
	if received.EndpointType != EndpointTypeNATSTLS {
		t.Errorf("EndpointType = %v, want nats-tls", received.EndpointType)
	}
	if !received.TLSEnabled {
		t.Error("TLSEnabled should be true")
	}
	if !received.AuthRequired {
		t.Error("AuthRequired should be true")
	}
	if len(received.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(received.Capabilities))
	}
	if received.Metadata["region"] != "us-east-1" {
		t.Errorf("Metadata[region] = %v, want us-east-1", received.Metadata["region"])
	}
	if received.SequenceNumber != 1 {
		t.Errorf("SequenceNumber = %v, want 1", received.SequenceNumber)
	}
	if received.HealthStatus != EndpointHealthHealthy {
		t.Errorf("HealthStatus = %v, want healthy", received.HealthStatus)
	}
}

func TestEndpointAdvertiser_NoCallback(t *testing.T) {
	cfg := DefaultEndpointAdvertiserConfig("agent-1", 4222)
	cfg.DetectPublicIP = false

	adv, err := NewEndpointAdvertiser(cfg)
	if err != nil {
		t.Fatalf("failed to create advertiser: %v", err)
	}

	// Advertise without callback should return error
	if err := adv.Advertise(); err == nil {
		t.Error("Advertise() should return error when no callback set")
	}
}

func TestEndpointAdvertiser_LocalAddresses(t *testing.T) {
	cfg := DefaultEndpointAdvertiserConfig("agent-1", 4222)
	cfg.DetectPublicIP = false

	adv, err := NewEndpointAdvertiser(cfg)
	if err != nil {
		t.Fatalf("failed to create advertiser: %v", err)
	}

	ctx := context.Background()
	adv.SetAdvertiseCallback(func(ad *EndpointAdvertisement) error { return nil })

	if err := adv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer adv.Stop()

	addrs := adv.GetLocalAddresses()
	// May have local addresses depending on network config
	t.Logf("Local addresses: %v", addrs)
}

func TestEndpointRegistry(t *testing.T) {
	registry := NewEndpointRegistry()

	if registry.Count() != 0 {
		t.Errorf("Count() = %d, want 0", registry.Count())
	}

	// Register an endpoint
	adv1 := &EndpointAdvertisement{
		AgentID:        "agent-1",
		Host:           "192.168.1.10",
		Port:           4222,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 1,
		HealthStatus:   EndpointHealthHealthy,
	}

	if err := registry.Register(adv1); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if registry.Count() != 1 {
		t.Errorf("Count() = %d, want 1", registry.Count())
	}

	// Get the endpoint
	got := registry.Get("agent-1")
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.Host != "192.168.1.10" {
		t.Errorf("Host = %v, want 192.168.1.10", got.Host)
	}

	// Update with higher sequence number
	adv2 := &EndpointAdvertisement{
		AgentID:        "agent-1",
		Host:           "192.168.1.11",
		Port:           4222,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 2,
		HealthStatus:   EndpointHealthHealthy,
	}

	if err := registry.Register(adv2); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got = registry.Get("agent-1")
	if got.Host != "192.168.1.11" {
		t.Errorf("Host = %v, want 192.168.1.11 after update", got.Host)
	}

	// Stale update (lower sequence number) should be ignored
	adv3 := &EndpointAdvertisement{
		AgentID:        "agent-1",
		Host:           "192.168.1.12",
		Port:           4222,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 1, // Lower than current
		HealthStatus:   EndpointHealthHealthy,
	}

	if err := registry.Register(adv3); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got = registry.Get("agent-1")
	if got.Host != "192.168.1.11" {
		t.Errorf("Host = %v, want 192.168.1.11 (stale update should be ignored)", got.Host)
	}
}

func TestEndpointRegistry_GetHealthy(t *testing.T) {
	registry := NewEndpointRegistry()

	// Add healthy endpoint
	registry.Register(&EndpointAdvertisement{
		AgentID:        "agent-1",
		Host:           "192.168.1.10",
		Port:           4222,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 1,
		HealthStatus:   EndpointHealthHealthy,
	})

	// Add unhealthy endpoint
	registry.Register(&EndpointAdvertisement{
		AgentID:        "agent-2",
		Host:           "192.168.1.11",
		Port:           4222,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 1,
		HealthStatus:   EndpointHealthUnhealthy,
	})

	// Add expired endpoint
	registry.Register(&EndpointAdvertisement{
		AgentID:        "agent-3",
		Host:           "192.168.1.12",
		Port:           4222,
		TTL:            1,
		Timestamp:      time.Now().Add(-10 * time.Second),
		SequenceNumber: 1,
		HealthStatus:   EndpointHealthHealthy,
	})

	healthy := registry.GetHealthy()
	if len(healthy) != 1 {
		t.Errorf("GetHealthy() returned %d endpoints, want 1", len(healthy))
	}
	if healthy[0].AgentID != "agent-1" {
		t.Errorf("healthy[0].AgentID = %v, want agent-1", healthy[0].AgentID)
	}
}

func TestEndpointRegistry_CleanExpired(t *testing.T) {
	registry := NewEndpointRegistry()

	// Add non-expired endpoint
	registry.Register(&EndpointAdvertisement{
		AgentID:        "agent-1",
		Host:           "192.168.1.10",
		Port:           4222,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 1,
		HealthStatus:   EndpointHealthHealthy,
	})

	// Add expired endpoint
	registry.Register(&EndpointAdvertisement{
		AgentID:        "agent-2",
		Host:           "192.168.1.11",
		Port:           4222,
		TTL:            1,
		Timestamp:      time.Now().Add(-10 * time.Second),
		SequenceNumber: 1,
		HealthStatus:   EndpointHealthHealthy,
	})

	if registry.Count() != 2 {
		t.Errorf("Count() = %d, want 2 before clean", registry.Count())
	}

	removed := registry.CleanExpired()
	if removed != 1 {
		t.Errorf("CleanExpired() removed %d, want 1", removed)
	}

	if registry.Count() != 1 {
		t.Errorf("Count() = %d, want 1 after clean", registry.Count())
	}
}

func TestEndpointRegistry_ChangeCallback(t *testing.T) {
	registry := NewEndpointRegistry()

	var changes []struct {
		agentID string
		adv     *EndpointAdvertisement
	}

	registry.SetChangeCallback(func(agentID string, adv *EndpointAdvertisement) {
		changes = append(changes, struct {
			agentID string
			adv     *EndpointAdvertisement
		}{agentID, adv})
	})

	// Register
	registry.Register(&EndpointAdvertisement{
		AgentID:        "agent-1",
		Host:           "192.168.1.10",
		Port:           4222,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 1,
		HealthStatus:   EndpointHealthHealthy,
	})

	// Unregister
	registry.Unregister("agent-1")

	if len(changes) != 2 {
		t.Errorf("expected 2 change callbacks, got %d", len(changes))
	}

	if changes[0].agentID != "agent-1" || changes[0].adv == nil {
		t.Errorf("first change should be register")
	}
	if changes[1].agentID != "agent-1" || changes[1].adv != nil {
		t.Errorf("second change should be unregister (nil adv)")
	}
}

func TestEndpointAdvertisement_JSON(t *testing.T) {
	adv := &EndpointAdvertisement{
		AgentID:        "agent-1",
		EndpointType:   EndpointTypeNATSTLS,
		Host:           "192.168.1.10",
		Port:           4222,
		PublicHost:     "public.example.com",
		PublicPort:     5222,
		LocalAddresses: []string{"192.168.1.10", "10.0.0.5"},
		TLSEnabled:     true,
		AuthRequired:   true,
		Capabilities:   []string{"jetstream"},
		Metadata:       map[string]string{"region": "us-east-1"},
		TTL:            30,
		Timestamp:      time.Now().UTC().Truncate(time.Second),
		SequenceNumber: 5,
		HealthStatus:   EndpointHealthHealthy,
	}

	// Marshal
	data, err := json.Marshal(adv)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Unmarshal
	var decoded EndpointAdvertisement
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify
	if decoded.AgentID != adv.AgentID {
		t.Errorf("AgentID = %v, want %v", decoded.AgentID, adv.AgentID)
	}
	if decoded.EndpointType != adv.EndpointType {
		t.Errorf("EndpointType = %v, want %v", decoded.EndpointType, adv.EndpointType)
	}
	if decoded.Host != adv.Host {
		t.Errorf("Host = %v, want %v", decoded.Host, adv.Host)
	}
	if decoded.Port != adv.Port {
		t.Errorf("Port = %v, want %v", decoded.Port, adv.Port)
	}
	if decoded.SequenceNumber != adv.SequenceNumber {
		t.Errorf("SequenceNumber = %v, want %v", decoded.SequenceNumber, adv.SequenceNumber)
	}
}

func TestGetLocalAddresses(t *testing.T) {
	addrs, err := getLocalAddresses()
	if err != nil {
		t.Fatalf("getLocalAddresses() error = %v", err)
	}

	// Should have at least one address on most systems
	t.Logf("Local addresses: %v", addrs)
}
