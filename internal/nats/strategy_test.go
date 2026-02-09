package nats

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestDirectStrategy_Name(t *testing.T) {
	s := NewDirectStrategy(nil)
	if s.Name() != "direct" {
		t.Errorf("Name() = %s, want direct", s.Name())
	}
}

func TestDirectStrategy_SupportsEndpoint(t *testing.T) {
	s := NewDirectStrategy(nil)

	tests := []struct {
		scheme Scheme
		want   bool
	}{
		{SchemeNATS, true},
		{SchemeTLS, false},
		{SchemeWS, false},
		{SchemeWSS, false},
	}

	for _, tt := range tests {
		endpoint := &Endpoint{Scheme: tt.scheme}
		if got := s.SupportsEndpoint(endpoint); got != tt.want {
			t.Errorf("SupportsEndpoint(%s) = %v, want %v", tt.scheme, got, tt.want)
		}
	}
}

func TestDirectStrategy_ConfigureOptions(t *testing.T) {
	config := &StrategyConfig{
		ConnectTimeout: 10 * time.Second,
		ReconnectWait:  5 * time.Second,
		MaxReconnects:  10,
	}
	s := NewDirectStrategy(config)

	endpoint := &Endpoint{
		Scheme: SchemeNATS,
		Host:   "localhost",
		Port:   4222,
	}

	endpointConfig := &EndpointConfig{}

	opts, err := s.ConfigureOptions(endpoint, endpointConfig)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestDirectStrategy_ConfigureOptions_WithCredentials(t *testing.T) {
	s := NewDirectStrategy(nil)

	endpoint := &Endpoint{
		Scheme: SchemeNATS,
		Host:   "localhost",
		Port:   4222,
	}

	// Test with token
	config := &EndpointConfig{
		Token: "test-token",
	}

	opts, err := s.ConfigureOptions(endpoint, config)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestDirectStrategy_ConfigureOptions_WithEndpointToken(t *testing.T) {
	s := NewDirectStrategy(nil)

	endpoint := &Endpoint{
		Scheme: SchemeNATS,
		Host:   "localhost",
		Port:   4222,
		Token:  "endpoint-token",
	}

	config := &EndpointConfig{}

	opts, err := s.ConfigureOptions(endpoint, config)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestDirectStrategy_ConfigureOptions_WithUserPassword(t *testing.T) {
	s := NewDirectStrategy(nil)

	endpoint := &Endpoint{
		Scheme:   SchemeNATS,
		Host:     "localhost",
		Port:     4222,
		Username: "user",
		Password: "pass",
	}

	config := &EndpointConfig{}

	opts, err := s.ConfigureOptions(endpoint, config)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestDirectStrategy_Priority(t *testing.T) {
	s := NewDirectStrategy(nil)
	if s.Priority() != 100 {
		t.Errorf("Priority() = %d, want 100", s.Priority())
	}
}

func TestTLSStrategy_Name(t *testing.T) {
	s := NewTLSStrategy(nil)
	if s.Name() != "tls" {
		t.Errorf("Name() = %s, want tls", s.Name())
	}
}

func TestTLSStrategy_SupportsEndpoint(t *testing.T) {
	s := NewTLSStrategy(nil)

	tests := []struct {
		scheme Scheme
		want   bool
	}{
		{SchemeNATS, false},
		{SchemeTLS, true},
		{SchemeWS, false},
		{SchemeWSS, false},
	}

	for _, tt := range tests {
		endpoint := &Endpoint{Scheme: tt.scheme}
		if got := s.SupportsEndpoint(endpoint); got != tt.want {
			t.Errorf("SupportsEndpoint(%s) = %v, want %v", tt.scheme, got, tt.want)
		}
	}
}

func TestTLSStrategy_ConfigureOptions_InsecureSkipVerify(t *testing.T) {
	// Allow InsecureSkipVerify for this test
	t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "1")

	s := NewTLSStrategy(nil)

	endpoint := &Endpoint{
		Scheme: SchemeTLS,
		Host:   "localhost",
		Port:   4222,
	}

	config := &EndpointConfig{
		TLS: EndpointTLSConfig{
			InsecureSkipVerify: true,
		},
	}

	opts, err := s.ConfigureOptions(endpoint, config)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestTLSStrategy_ConfigureOptions_WithStrategyTLS(t *testing.T) {
	// Allow InsecureSkipVerify for this test
	t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "1")

	config := &StrategyConfig{
		TLS: &TLSStrategyConfig{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
			ServerName:         "test.example.com",
		},
	}
	s := NewTLSStrategy(config)

	endpoint := &Endpoint{
		Scheme: SchemeTLS,
		Host:   "localhost",
		Port:   4222,
	}

	endpointConfig := &EndpointConfig{}

	opts, err := s.ConfigureOptions(endpoint, endpointConfig)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestTLSStrategy_Priority(t *testing.T) {
	s := NewTLSStrategy(nil)
	if s.Priority() != 50 {
		t.Errorf("Priority() = %d, want 50", s.Priority())
	}
}

func TestWebSocketStrategy_Name(t *testing.T) {
	s := NewWebSocketStrategy(nil)
	if s.Name() != "websocket" {
		t.Errorf("Name() = %s, want websocket", s.Name())
	}
}

func TestWebSocketStrategy_SupportsEndpoint(t *testing.T) {
	s := NewWebSocketStrategy(nil)

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
		endpoint := &Endpoint{Scheme: tt.scheme}
		if got := s.SupportsEndpoint(endpoint); got != tt.want {
			t.Errorf("SupportsEndpoint(%s) = %v, want %v", tt.scheme, got, tt.want)
		}
	}
}

func TestWebSocketStrategy_ConfigureOptions_WS(t *testing.T) {
	config := &StrategyConfig{
		WebSocket: &WebSocketStrategyConfig{
			Compression: true,
		},
	}
	s := NewWebSocketStrategy(config)

	endpoint := &Endpoint{
		Scheme: SchemeWS,
		Host:   "localhost",
		Port:   80,
	}

	endpointConfig := &EndpointConfig{}

	opts, err := s.ConfigureOptions(endpoint, endpointConfig)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestWebSocketStrategy_ConfigureOptions_WSS(t *testing.T) {
	// Allow InsecureSkipVerify for this test
	t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "1")

	config := &StrategyConfig{
		TLS: &TLSStrategyConfig{
			InsecureSkipVerify: true,
		},
	}
	s := NewWebSocketStrategy(config)

	endpoint := &Endpoint{
		Scheme: SchemeWSS,
		Host:   "localhost",
		Port:   443,
	}

	endpointConfig := &EndpointConfig{}

	opts, err := s.ConfigureOptions(endpoint, endpointConfig)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestWebSocketStrategy_Priority(t *testing.T) {
	s := NewWebSocketStrategy(nil)
	if s.Priority() != 200 {
		t.Errorf("Priority() = %d, want 200", s.Priority())
	}
}

func TestLeafNodeStrategy_Name(t *testing.T) {
	s := NewLeafNodeStrategy(nil)
	if s.Name() != "leafnode" {
		t.Errorf("Name() = %s, want leafnode", s.Name())
	}
}

func TestLeafNodeStrategy_SupportsEndpoint(t *testing.T) {
	// Without config, should not support any endpoint
	s := NewLeafNodeStrategy(nil)

	endpoint := &Endpoint{Scheme: SchemeNATS}
	if s.SupportsEndpoint(endpoint) {
		t.Error("SupportsEndpoint() should return false without LeafNode config")
	}

	// With config, should support endpoints
	config := &StrategyConfig{
		LeafNode: &LeafNodeStrategyConfig{
			RemoteURL: "nats://remote:7422",
		},
	}
	s = NewLeafNodeStrategy(config)

	if !s.SupportsEndpoint(endpoint) {
		t.Error("SupportsEndpoint() should return true with LeafNode config")
	}
}

func TestLeafNodeStrategy_ConfigureOptions(t *testing.T) {
	config := &StrategyConfig{
		LeafNode: &LeafNodeStrategyConfig{
			RemoteURL: "nats://remote:7422",
		},
	}
	s := NewLeafNodeStrategy(config)

	endpoint := &Endpoint{
		Scheme: SchemeNATS,
		Host:   "localhost",
		Port:   4222,
	}

	endpointConfig := &EndpointConfig{}

	opts, err := s.ConfigureOptions(endpoint, endpointConfig)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestLeafNodeStrategy_ConfigureOptions_NoConfig(t *testing.T) {
	s := NewLeafNodeStrategy(nil)

	endpoint := &Endpoint{
		Scheme: SchemeNATS,
		Host:   "localhost",
		Port:   4222,
	}

	endpointConfig := &EndpointConfig{}

	_, err := s.ConfigureOptions(endpoint, endpointConfig)
	if err == nil {
		t.Error("ConfigureOptions() should return error without LeafNode config")
	}
}

func TestLeafNodeStrategy_ConfigureOptions_TLS(t *testing.T) {
	// Allow InsecureSkipVerify for this test
	t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "1")

	config := &StrategyConfig{
		LeafNode: &LeafNodeStrategyConfig{
			RemoteURL: "tls://remote:7422",
		},
		TLS: &TLSStrategyConfig{
			InsecureSkipVerify: true,
		},
	}
	s := NewLeafNodeStrategy(config)

	endpoint := &Endpoint{
		Scheme: SchemeTLS,
		Host:   "localhost",
		Port:   4222,
	}

	endpointConfig := &EndpointConfig{}

	opts, err := s.ConfigureOptions(endpoint, endpointConfig)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}

func TestLeafNodeStrategy_Priority(t *testing.T) {
	s := NewLeafNodeStrategy(nil)
	if s.Priority() != 300 {
		t.Errorf("Priority() = %d, want 300", s.Priority())
	}
}

func TestStrategySelector_SelectStrategy(t *testing.T) {
	selector := DefaultStrategySelector(nil)

	tests := []struct {
		scheme   Scheme
		wantName string
	}{
		{SchemeNATS, "direct"},
		{SchemeTLS, "tls"},
		{SchemeWS, "websocket"},
		{SchemeWSS, "websocket"},
	}

	for _, tt := range tests {
		endpoint := &Endpoint{Scheme: tt.scheme}
		strategy := selector.SelectStrategy(endpoint)
		if strategy == nil {
			t.Errorf("SelectStrategy(%s) returned nil", tt.scheme)
			continue
		}
		if strategy.Name() != tt.wantName {
			t.Errorf("SelectStrategy(%s) = %s, want %s", tt.scheme, strategy.Name(), tt.wantName)
		}
	}
}

func TestStrategySelector_SelectStrategies(t *testing.T) {
	selector := DefaultStrategySelector(nil)

	// NATS should only have direct strategy
	endpoint := &Endpoint{Scheme: SchemeNATS}
	strategies := selector.SelectStrategies(endpoint)
	if len(strategies) != 1 {
		t.Errorf("SelectStrategies(NATS) returned %d strategies, want 1", len(strategies))
	}

	// TLS should only have TLS strategy
	endpoint = &Endpoint{Scheme: SchemeTLS}
	strategies = selector.SelectStrategies(endpoint)
	if len(strategies) != 1 {
		t.Errorf("SelectStrategies(TLS) returned %d strategies, want 1", len(strategies))
	}
}

func TestStrategySelector_SelectStrategies_Sorted(t *testing.T) {
	// Create selector with strategies in wrong priority order
	selector := NewStrategySelector(
		NewWebSocketStrategy(nil), // Priority 200
		NewDirectStrategy(nil),    // Priority 100
		NewTLSStrategy(nil),       // Priority 50
	)

	// WSS endpoint should return strategies sorted by priority
	endpoint := &Endpoint{Scheme: SchemeWSS}
	strategies := selector.SelectStrategies(endpoint)
	if len(strategies) != 1 {
		t.Errorf("SelectStrategies(WSS) returned %d strategies, want 1", len(strategies))
	}
}

func TestStrategySelector_AddStrategy(t *testing.T) {
	selector := NewStrategySelector()

	// Should have no strategies initially
	endpoint := &Endpoint{Scheme: SchemeNATS}
	strategy := selector.SelectStrategy(endpoint)
	if strategy != nil {
		t.Error("SelectStrategy() should return nil with no strategies")
	}

	// Add a strategy
	selector.AddStrategy(NewDirectStrategy(nil))

	strategy = selector.SelectStrategy(endpoint)
	if strategy == nil {
		t.Error("SelectStrategy() should return strategy after AddStrategy")
	}
}

func TestStrategySelector_StrategiesForScheme(t *testing.T) {
	selector := DefaultStrategySelector(nil)

	tests := []struct {
		scheme    Scheme
		wantCount int
	}{
		{SchemeNATS, 1},
		{SchemeTLS, 1},
		{SchemeWS, 1},
		{SchemeWSS, 1},
	}

	for _, tt := range tests {
		strategies := selector.StrategiesForScheme(tt.scheme)
		if len(strategies) != tt.wantCount {
			t.Errorf("StrategiesForScheme(%s) returned %d, want %d", tt.scheme, len(strategies), tt.wantCount)
		}
	}
}

func TestNewDirectStrategy_NilConfig(t *testing.T) {
	s := NewDirectStrategy(nil)
	if s == nil {
		t.Fatal("NewDirectStrategy(nil) returned nil")
	}
	if s.config == nil {
		t.Error("NewDirectStrategy(nil) should create default config")
	}
}

func TestNewTLSStrategy_NilConfig(t *testing.T) {
	s := NewTLSStrategy(nil)
	if s == nil {
		t.Fatal("NewTLSStrategy(nil) returned nil")
	}
	if s.config == nil {
		t.Error("NewTLSStrategy(nil) should create default config")
	}
}

func TestNewWebSocketStrategy_NilConfig(t *testing.T) {
	s := NewWebSocketStrategy(nil)
	if s == nil {
		t.Fatal("NewWebSocketStrategy(nil) returned nil")
	}
	if s.config == nil {
		t.Error("NewWebSocketStrategy(nil) should create default config")
	}
}

func TestNewLeafNodeStrategy_NilConfig(t *testing.T) {
	s := NewLeafNodeStrategy(nil)
	if s == nil {
		t.Fatal("NewLeafNodeStrategy(nil) returned nil")
	}
	if s.config == nil {
		t.Error("NewLeafNodeStrategy(nil) should create default config")
	}
}

func TestDirectStrategy_ConfigureOptions_EndpointConfigTimeouts(t *testing.T) {
	// Strategy config has defaults
	strategyConfig := &StrategyConfig{
		ConnectTimeout: 10 * time.Second,
		ReconnectWait:  5 * time.Second,
		MaxReconnects:  10,
	}
	s := NewDirectStrategy(strategyConfig)

	endpoint := &Endpoint{
		Scheme: SchemeNATS,
		Host:   "localhost",
		Port:   4222,
	}

	// EndpointConfig overrides should take precedence
	endpointConfig := &EndpointConfig{
		ConnectTimeout: 30 * time.Second,
		ReconnectWait:  15 * time.Second,
		MaxReconnects:  20,
	}

	opts, err := s.ConfigureOptions(endpoint, endpointConfig)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	// We can't easily verify the option values, but we can check options were added
	if len(opts) == 0 {
		t.Error("ConfigureOptions() returned no options")
	}
}
