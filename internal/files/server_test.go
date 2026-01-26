package files

import (
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	tests := []struct {
		name    string
		config  *ServerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &ServerConfig{
				ClusterID: "test-cluster",
			},
			wantErr: false,
		},
		{
			name:    "missing cluster ID",
			config:  &ServerConfig{},
			wantErr: true,
		},
		{
			name: "config with defaults",
			config: &ServerConfig{
				ClusterID: "test-cluster",
			},
			wantErr: false,
		},
		{
			name: "config with custom values",
			config: &ServerConfig{
				ClusterID:      "test-cluster",
				InstanceID:     "instance-1",
				Workers:        20,
				MaxChunkSize:   2 << 20, // 2MB
				MaxFileSize:    5 << 30, // 5GB
				RequestTimeout: 10 * time.Minute,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewServer(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if s.config.ClusterID != tt.config.ClusterID {
					t.Errorf("ClusterID = %v, want %v", s.config.ClusterID, tt.config.ClusterID)
				}
				// Verify defaults are applied
				if s.config.Workers <= 0 {
					t.Error("Workers should be positive")
				}
				if s.config.MaxChunkSize <= 0 {
					t.Error("MaxChunkSize should be positive")
				}
				if s.config.MaxFileSize <= 0 {
					t.Error("MaxFileSize should be positive")
				}
				if s.config.RequestTimeout <= 0 {
					t.Error("RequestTimeout should be positive")
				}
				if s.config.InstanceID == "" {
					t.Error("InstanceID should not be empty")
				}
			}
		})
	}
}

func TestServerConfigDefaults(t *testing.T) {
	config := &ServerConfig{
		ClusterID: "test-cluster",
	}

	s, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}

	// Verify defaults
	if s.config.Workers != 10 {
		t.Errorf("Default Workers = %v, want 10", s.config.Workers)
	}
	if s.config.MaxChunkSize != DefaultChunkSize {
		t.Errorf("Default MaxChunkSize = %v, want %v", s.config.MaxChunkSize, DefaultChunkSize)
	}
	if s.config.MaxFileSize != DefaultMaxFileSize {
		t.Errorf("Default MaxFileSize = %v, want %v", s.config.MaxFileSize, DefaultMaxFileSize)
	}
	if s.config.RequestTimeout != 5*time.Minute {
		t.Errorf("Default RequestTimeout = %v, want %v", s.config.RequestTimeout, 5*time.Minute)
	}
}

func TestServerMaxChunkSizeCap(t *testing.T) {
	config := &ServerConfig{
		ClusterID:    "test-cluster",
		MaxChunkSize: 100 << 20, // 100MB (over limit)
	}

	s, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}

	// Should be capped at MaxChunkSize
	if s.config.MaxChunkSize != MaxChunkSize {
		t.Errorf("MaxChunkSize = %v, want %v (capped)", s.config.MaxChunkSize, MaxChunkSize)
	}
}

func TestServerMetrics(t *testing.T) {
	config := &ServerConfig{
		ClusterID: "test-cluster",
	}

	s, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}

	// Initial metrics should be zero
	metrics := s.GetMetrics()
	if metrics.RequestsTotal.Load() != 0 {
		t.Errorf("Initial RequestsTotal = %v, want 0", metrics.RequestsTotal.Load())
	}
	if metrics.RequestsSucceeded.Load() != 0 {
		t.Errorf("Initial RequestsSucceeded = %v, want 0", metrics.RequestsSucceeded.Load())
	}
	if metrics.RequestsFailed.Load() != 0 {
		t.Errorf("Initial RequestsFailed = %v, want 0", metrics.RequestsFailed.Load())
	}
	if metrics.BytesTransferred.Load() != 0 {
		t.Errorf("Initial BytesTransferred = %v, want 0", metrics.BytesTransferred.Load())
	}
	if metrics.ActiveTransfers.Load() != 0 {
		t.Errorf("Initial ActiveTransfers = %v, want 0", metrics.ActiveTransfers.Load())
	}
	if metrics.ChunksTransferred.Load() != 0 {
		t.Errorf("Initial ChunksTransferred = %v, want 0", metrics.ChunksTransferred.Load())
	}
}

func TestRateLimitConfig(t *testing.T) {
	config := &ServerConfig{
		ClusterID: "test-cluster",
		RateLimit: &RateLimitConfig{
			PerAgent:            10 << 20, // 10 MB/s
			Global:              100 << 20, // 100 MB/s
			ConcurrentTransfers: 50,
		},
	}

	s, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}

	if s.config.RateLimit == nil {
		t.Fatal("RateLimit should not be nil")
	}
	if s.config.RateLimit.PerAgent != 10<<20 {
		t.Errorf("PerAgent = %v, want %v", s.config.RateLimit.PerAgent, 10<<20)
	}
	if s.config.RateLimit.Global != 100<<20 {
		t.Errorf("Global = %v, want %v", s.config.RateLimit.Global, 100<<20)
	}
	if s.config.RateLimit.ConcurrentTransfers != 50 {
		t.Errorf("ConcurrentTransfers = %v, want 50", s.config.RateLimit.ConcurrentTransfers)
	}
}

func TestServerStartWithoutBackends(t *testing.T) {
	config := &ServerConfig{
		ClusterID: "test-cluster",
	}

	s, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}

	// Start without backends should fail
	err = s.Start(nil)
	if err == nil {
		t.Error("Start() should fail without backends")
	}
}

func TestServerStopWithoutStart(t *testing.T) {
	config := &ServerConfig{
		ClusterID: "test-cluster",
	}

	s, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}

	// Stop without start should not error
	err = s.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}
