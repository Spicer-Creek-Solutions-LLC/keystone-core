package spire

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/identity"
)

// TestDefaultConfig tests default configuration values.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.DialTimeout != 10*time.Second {
		t.Errorf("expected dial timeout 10s, got %v", cfg.DialTimeout)
	}
	if cfg.HealthCheckInterval != 30*time.Second {
		t.Errorf("expected health check interval 30s, got %v", cfg.HealthCheckInterval)
	}
	if cfg.StreamBufferSize != 10 {
		t.Errorf("expected stream buffer size 10, got %d", cfg.StreamBufferSize)
	}
	if cfg.SocketPath == "" {
		t.Error("expected non-empty socket path")
	}
	if cfg.RetryConfig == nil {
		t.Error("expected non-nil retry config")
	}
}

// TestDefaultRetryConfig tests default retry configuration.
func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", cfg.MaxRetries)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected initial delay 100ms, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("expected max delay 30s, got %v", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("expected multiplier 2.0, got %v", cfg.Multiplier)
	}
	if cfg.Jitter != 0.1 {
		t.Errorf("expected jitter 0.1, got %v", cfg.Jitter)
	}
}

// TestDefaultFallbackConfig tests default fallback configuration.
func TestDefaultFallbackConfig(t *testing.T) {
	cfg := DefaultFallbackConfig()

	if cfg.Enabled {
		t.Error("expected fallback disabled by default")
	}
	if cfg.FallbackProvider != "cached" {
		t.Errorf("expected fallback provider 'cached', got %s", cfg.FallbackProvider)
	}
	if cfg.GracePeriod != time.Hour {
		t.Errorf("expected grace period 1h, got %v", cfg.GracePeriod)
	}
	if cfg.ReconnectInterval != time.Minute {
		t.Errorf("expected reconnect interval 1m, got %v", cfg.ReconnectInterval)
	}
}

// TestConfigValidation tests configuration validation.
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "default config is valid",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "empty socket path",
			config: &Config{
				SocketPath:          "",
				Timeout:             time.Second,
				DialTimeout:         time.Second,
				HealthCheckInterval: time.Second,
				StreamBufferSize:    1,
			},
			wantErr: true,
		},
		{
			name: "zero timeout",
			config: &Config{
				SocketPath:          "/tmp/test.sock",
				Timeout:             0,
				DialTimeout:         time.Second,
				HealthCheckInterval: time.Second,
				StreamBufferSize:    1,
			},
			wantErr: true,
		},
		{
			name: "zero dial timeout",
			config: &Config{
				SocketPath:          "/tmp/test.sock",
				Timeout:             time.Second,
				DialTimeout:         0,
				HealthCheckInterval: time.Second,
				StreamBufferSize:    1,
			},
			wantErr: true,
		},
		{
			name: "zero health check interval",
			config: &Config{
				SocketPath:          "/tmp/test.sock",
				Timeout:             time.Second,
				DialTimeout:         time.Second,
				HealthCheckInterval: 0,
				StreamBufferSize:    1,
			},
			wantErr: true,
		},
		{
			name: "zero stream buffer size",
			config: &Config{
				SocketPath:          "/tmp/test.sock",
				Timeout:             time.Second,
				DialTimeout:         time.Second,
				HealthCheckInterval: time.Second,
				StreamBufferSize:    0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRetryConfigValidation tests retry configuration validation.
func TestRetryConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *RetryConfig
		wantErr bool
	}{
		{
			name:    "default config is valid",
			config:  DefaultRetryConfig(),
			wantErr: false,
		},
		{
			name: "negative max retries",
			config: &RetryConfig{
				MaxRetries:   -1,
				InitialDelay: time.Millisecond,
				MaxDelay:     time.Second,
				Multiplier:   2.0,
				Jitter:       0.1,
			},
			wantErr: true,
		},
		{
			name: "zero initial delay",
			config: &RetryConfig{
				MaxRetries:   3,
				InitialDelay: 0,
				MaxDelay:     time.Second,
				Multiplier:   2.0,
				Jitter:       0.1,
			},
			wantErr: true,
		},
		{
			name: "multiplier less than 1",
			config: &RetryConfig{
				MaxRetries:   3,
				InitialDelay: time.Millisecond,
				MaxDelay:     time.Second,
				Multiplier:   0.5,
				Jitter:       0.1,
			},
			wantErr: true,
		},
		{
			name: "jitter greater than 1",
			config: &RetryConfig{
				MaxRetries:   3,
				InitialDelay: time.Millisecond,
				MaxDelay:     time.Second,
				Multiplier:   2.0,
				Jitter:       1.5,
			},
			wantErr: true,
		},
		{
			name: "negative jitter",
			config: &RetryConfig{
				MaxRetries:   3,
				InitialDelay: time.Millisecond,
				MaxDelay:     time.Second,
				Multiplier:   2.0,
				Jitter:       -0.1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestFallbackConfigValidation tests fallback configuration validation.
func TestFallbackConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *FallbackConfig
		wantErr bool
	}{
		{
			name:    "disabled config always valid",
			config:  &FallbackConfig{Enabled: false},
			wantErr: false,
		},
		{
			name:    "default enabled config is valid",
			config:  DefaultFallbackConfig(),
			wantErr: false,
		},
		{
			name: "invalid fallback provider",
			config: &FallbackConfig{
				Enabled:           true,
				FallbackProvider:  "invalid",
				GracePeriod:       time.Hour,
				ReconnectInterval: time.Minute,
			},
			wantErr: true,
		},
		{
			name: "zero grace period",
			config: &FallbackConfig{
				Enabled:           true,
				FallbackProvider:  "cached",
				GracePeriod:       0,
				ReconnectInterval: time.Minute,
			},
			wantErr: true,
		},
		{
			name: "zero reconnect interval",
			config: &FallbackConfig{
				Enabled:           true,
				FallbackProvider:  "cached",
				GracePeriod:       time.Hour,
				ReconnectInterval: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConfigApplyDefaults tests applying default values.
func TestConfigApplyDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()

	if cfg.SocketPath == "" {
		t.Error("expected non-empty socket path after defaults")
	}
	if cfg.Timeout == 0 {
		t.Error("expected non-zero timeout after defaults")
	}
	if cfg.DialTimeout == 0 {
		t.Error("expected non-zero dial timeout after defaults")
	}
	if cfg.HealthCheckInterval == 0 {
		t.Error("expected non-zero health check interval after defaults")
	}
	if cfg.StreamBufferSize == 0 {
		t.Error("expected non-zero stream buffer size after defaults")
	}
	if cfg.RetryConfig == nil {
		t.Error("expected non-nil retry config after defaults")
	}
}

// TestNewClient tests client creation.
func TestNewClient(t *testing.T) {
	t.Run("nil config uses defaults", func(t *testing.T) {
		client, err := NewClient(nil)
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("expected non-nil client")
		}
		if client.State() != ClientStateDisconnected {
			t.Errorf("expected initial state disconnected, got %v", client.State())
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &Config{
			SocketPath:          "/custom/path.sock",
			Timeout:             5 * time.Second,
			DialTimeout:         2 * time.Second,
			HealthCheckInterval: 10 * time.Second,
			StreamBufferSize:    5,
		}
		client, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("expected non-nil client")
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := &Config{
			SocketPath:          "/tmp/test.sock",
			Timeout:             time.Second,
			DialTimeout:         time.Second,
			HealthCheckInterval: time.Second,
			StreamBufferSize:    1,
			RetryConfig: &RetryConfig{
				MaxRetries:   -1, // Invalid: negative retries
				InitialDelay: time.Millisecond,
				MaxDelay:     time.Second,
				Multiplier:   2.0,
				Jitter:       0,
			},
		}
		_, err := NewClient(cfg)
		if err == nil {
			t.Error("expected error for invalid config")
		}
	})
}

// TestClientStateTransitions tests client state transitions.
func TestClientStateTransitions(t *testing.T) {
	client, err := NewClient(nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// Initial state
	if client.State() != ClientStateDisconnected {
		t.Errorf("expected initial state disconnected, got %v", client.State())
	}

	// State change callback
	var stateChanges []struct {
		old, new ClientState
	}
	client.OnStateChange(func(old, new ClientState) {
		stateChanges = append(stateChanges, struct{ old, new ClientState }{old, new})
	})

	// Try to connect (will fail without real SPIRE agent)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = client.Connect(ctx)
	if err == nil {
		t.Error("expected error connecting without SPIRE agent")
	}

	// Should have transitioned: disconnected -> connecting -> disconnected
	if len(stateChanges) < 2 {
		t.Errorf("expected at least 2 state changes, got %d", len(stateChanges))
	}
}

// TestClientStats tests client statistics.
func TestClientStats(t *testing.T) {
	client, err := NewClient(nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	stats := client.Stats()

	if stats.ConnectAttempts != 0 {
		t.Errorf("expected 0 connect attempts, got %d", stats.ConnectAttempts)
	}
	if stats.FetchSVIDCount != 0 {
		t.Errorf("expected 0 fetch SVID count, got %d", stats.FetchSVIDCount)
	}

	// Try to connect (will fail)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = client.Connect(ctx)

	stats = client.Stats()
	if stats.ConnectAttempts != 1 {
		t.Errorf("expected 1 connect attempt, got %d", stats.ConnectAttempts)
	}
	if stats.ConnectFailures != 1 {
		t.Errorf("expected 1 connect failure, got %d", stats.ConnectFailures)
	}
}

// TestClientClose tests client close.
func TestClientClose(t *testing.T) {
	client, err := NewClient(nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if client.State() != ClientStateClosed {
		t.Errorf("expected state closed, got %v", client.State())
	}

	// Double close should be safe
	err = client.Close()
	if err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

// TestParseCertificates tests certificate parsing.
func TestParseCertificates(t *testing.T) {
	// Generate a test certificate
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{{Scheme: "spiffe", Host: "example.org", Path: "/test"}},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	t.Run("parse DER", func(t *testing.T) {
		certs, err := parseCertificates(certDER)
		if err != nil {
			t.Fatalf("parseCertificates() error = %v", err)
		}
		if len(certs) != 1 {
			t.Errorf("expected 1 certificate, got %d", len(certs))
		}
	})

	t.Run("parse PEM", func(t *testing.T) {
		pemData := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDER,
		})
		certs, err := parseCertificates(pemData)
		if err != nil {
			t.Fatalf("parseCertificates() error = %v", err)
		}
		if len(certs) != 1 {
			t.Errorf("expected 1 certificate, got %d", len(certs))
		}
	})

	t.Run("parse multiple PEM", func(t *testing.T) {
		pemData := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDER,
		})
		pemData = append(pemData, pemData...)
		certs, err := parseCertificates(pemData)
		if err != nil {
			t.Fatalf("parseCertificates() error = %v", err)
		}
		if len(certs) != 2 {
			t.Errorf("expected 2 certificates, got %d", len(certs))
		}
	})

	t.Run("empty data", func(t *testing.T) {
		_, err := parseCertificates([]byte{})
		if err == nil {
			t.Error("expected error for empty data")
		}
	})
}

// TestParsePrivateKey tests private key parsing.
func TestParsePrivateKey(t *testing.T) {
	// Generate test keys
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate EC key: %v", err)
	}

	t.Run("parse PKCS8 EC key", func(t *testing.T) {
		pkcs8, err := x509.MarshalPKCS8PrivateKey(ecKey)
		if err != nil {
			t.Fatalf("failed to marshal PKCS8: %v", err)
		}

		key, err := parsePrivateKey(pkcs8)
		if err != nil {
			t.Fatalf("parsePrivateKey() error = %v", err)
		}
		if _, ok := key.(*ecdsa.PrivateKey); !ok {
			t.Errorf("expected ECDSA key, got %T", key)
		}
	})

	t.Run("parse EC key", func(t *testing.T) {
		ecDER, err := x509.MarshalECPrivateKey(ecKey)
		if err != nil {
			t.Fatalf("failed to marshal EC key: %v", err)
		}

		key, err := parsePrivateKey(ecDER)
		if err != nil {
			t.Fatalf("parsePrivateKey() error = %v", err)
		}
		if _, ok := key.(*ecdsa.PrivateKey); !ok {
			t.Errorf("expected ECDSA key, got %T", key)
		}
	})

	t.Run("parse PEM EC key", func(t *testing.T) {
		ecDER, err := x509.MarshalECPrivateKey(ecKey)
		if err != nil {
			t.Fatalf("failed to marshal EC key: %v", err)
		}

		pemData := pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: ecDER,
		})

		key, err := parsePrivateKey(pemData)
		if err != nil {
			t.Fatalf("parsePrivateKey() error = %v", err)
		}
		if _, ok := key.(*ecdsa.PrivateKey); !ok {
			t.Errorf("expected ECDSA key, got %T", key)
		}
	})

	t.Run("invalid data", func(t *testing.T) {
		_, err := parsePrivateKey([]byte("invalid"))
		if err == nil {
			t.Error("expected error for invalid data")
		}
	})
}

// TestNewProvider tests provider creation.
func TestNewProvider(t *testing.T) {
	t.Run("nil config uses defaults", func(t *testing.T) {
		provider, err := NewProvider(nil)
		if err != nil {
			t.Fatalf("NewProvider() error = %v", err)
		}
		if provider == nil {
			t.Fatal("expected non-nil provider")
		}
		if provider.Type() != identity.ProviderTypeSPIRE {
			t.Errorf("expected provider type SPIRE, got %v", provider.Type())
		}
	})

	t.Run("with options", func(t *testing.T) {
		fallbackCfg := DefaultFallbackConfig()
		fallbackCfg.Enabled = true

		provider, err := NewProvider(nil,
			WithFallback(fallbackCfg),
			WithTrustDomain("custom.example.org"),
		)
		if err != nil {
			t.Fatalf("NewProvider() error = %v", err)
		}
		if provider.TrustDomain() != "custom.example.org" {
			t.Errorf("expected trust domain 'custom.example.org', got %s", provider.TrustDomain())
		}
	})
}

// TestProviderInfo tests provider info.
func TestProviderInfo(t *testing.T) {
	provider, err := NewProvider(nil, WithTrustDomain("test.example.org"))
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	info := provider.Info(context.Background())

	if info.Type != identity.ProviderTypeSPIRE {
		t.Errorf("expected type SPIRE, got %v", info.Type)
	}
	if info.TrustDomain != "test.example.org" {
		t.Errorf("expected trust domain 'test.example.org', got %s", info.TrustDomain)
	}
	if len(info.Capabilities) == 0 {
		t.Error("expected non-empty capabilities")
	}
	if info.Metadata["socket_path"] == "" {
		t.Error("expected socket_path in metadata")
	}
}

// TestProviderHealth tests provider health without SPIRE.
func TestProviderHealth(t *testing.T) {
	provider, err := NewProvider(nil)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	// Without starting, health should be unknown
	status := provider.Health(context.Background())
	if status != identity.ProviderStatusUnknown {
		t.Errorf("expected status unknown, got %v", status)
	}
}

// TestCalculateRetryDelay tests retry delay calculation.
func TestCalculateRetryDelay(t *testing.T) {
	client, err := NewClient(&Config{
		SocketPath:          "/tmp/test.sock",
		Timeout:             time.Second,
		DialTimeout:         time.Second,
		HealthCheckInterval: time.Second,
		StreamBufferSize:    1,
		RetryConfig: &RetryConfig{
			MaxRetries:   5,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     10 * time.Second,
			Multiplier:   2.0,
			Jitter:       0, // No jitter for predictable tests
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
	}

	for _, tt := range tests {
		delay := client.calculateRetryDelay(tt.attempt, client.config.RetryConfig)
		if delay != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, delay)
		}
	}
}

// TestCalculateRetryDelayMaxCap tests max delay cap.
func TestCalculateRetryDelayMaxCap(t *testing.T) {
	client, err := NewClient(&Config{
		SocketPath:          "/tmp/test.sock",
		Timeout:             time.Second,
		DialTimeout:         time.Second,
		HealthCheckInterval: time.Second,
		StreamBufferSize:    1,
		RetryConfig: &RetryConfig{
			MaxRetries:   10,
			InitialDelay: time.Second,
			MaxDelay:     5 * time.Second,
			Multiplier:   10.0,
			Jitter:       0,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// After a few attempts, should hit max
	delay := client.calculateRetryDelay(5, client.config.RetryConfig)
	if delay > 5*time.Second {
		t.Errorf("expected delay <= 5s, got %v", delay)
	}
}

// TestValidateKeyPair tests key pair validation.
func TestValidateKeyPair(t *testing.T) {
	// Generate matching key pair
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	t.Run("matching key pair", func(t *testing.T) {
		err := validateKeyPair(cert, key)
		if err != nil {
			t.Errorf("validateKeyPair() error = %v", err)
		}
	})

	t.Run("mismatched key", func(t *testing.T) {
		wrongKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		err := validateKeyPair(cert, wrongKey)
		if err == nil {
			t.Error("expected error for mismatched key")
		}
	})

	t.Run("wrong key type", func(t *testing.T) {
		err := validateKeyPair(cert, "not a key")
		if err == nil {
			t.Error("expected error for wrong key type")
		}
	})
}

// TestX509SVIDData tests X509SVID data types.
func TestX509SVIDData(t *testing.T) {
	data := &X509SVIDData{
		SPIFFEID:    "spiffe://example.org/test",
		X509SVID:    []byte("cert"),
		X509SVIDKey: []byte("key"),
		Bundle:      []byte("bundle"),
		Hint:        "test-hint",
	}

	if data.SPIFFEID != "spiffe://example.org/test" {
		t.Errorf("unexpected SPIFFE ID: %s", data.SPIFFEID)
	}
	if data.Hint != "test-hint" {
		t.Errorf("unexpected hint: %s", data.Hint)
	}
}

// TestJWTSVIDData tests JWTSVID data types.
func TestJWTSVIDData(t *testing.T) {
	now := time.Now()
	data := &JWTSVIDData{
		SPIFFEID:  "spiffe://example.org/test",
		Token:     "eyJ...",
		ExpiresAt: now.Add(time.Hour).Unix(),
		IssuedAt:  now.Unix(),
		Hint:      "jwt-hint",
	}

	if data.Token != "eyJ..." {
		t.Errorf("unexpected token: %s", data.Token)
	}
	if data.ExpiresAt <= data.IssuedAt {
		t.Error("expires at should be after issued at")
	}
}

// TestCreateMockSocket tests creating a mock Unix socket for testing.
func TestCreateMockSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a listener (simulates SPIRE agent socket)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	// Verify socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("socket file does not exist")
	}

	// Create client with this socket
	client, err := NewClient(&Config{
		SocketPath:          socketPath,
		Timeout:             time.Second,
		DialTimeout:         500 * time.Millisecond,
		HealthCheckInterval: time.Second,
		StreamBufferSize:    1,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// Connection should succeed (but gRPC handshake will fail since it's just a raw listener)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Accept connection to prevent connection refused
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			<-ctx.Done()
			conn.Close()
		}
	}()

	// Will fail because it's not a real gRPC server, but demonstrates socket is reachable
	err = client.Connect(ctx)
	// We expect an error here since there's no gRPC server
	if err == nil {
		// If it connected, that's unexpected but okay for test purposes
		client.Close()
	}
}

// Benchmark tests

func BenchmarkConfigValidate(b *testing.B) {
	cfg := DefaultConfig()
	for i := 0; i < b.N; i++ {
		cfg.Validate()
	}
}

func BenchmarkCalculateRetryDelay(b *testing.B) {
	client, _ := NewClient(nil)
	cfg := DefaultRetryConfig()
	for i := 0; i < b.N; i++ {
		client.calculateRetryDelay(3, cfg)
	}
}

func BenchmarkParseCertificates(b *testing.B) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseCertificates(certDER)
	}
}
