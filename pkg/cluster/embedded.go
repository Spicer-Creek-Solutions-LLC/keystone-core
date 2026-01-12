package cluster

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.etcd.io/etcd/server/v3/embed"
	"go.uber.org/zap"

	"go.etcd.io/etcd/client/pkg/v3/transport"
)

// EmbeddedEtcdServer manages an embedded etcd server instance.
type EmbeddedEtcdServer struct {
	config   *EtcdConfig
	server   *embed.Etcd
	mu       sync.RWMutex
	running  bool
	dataDir  string
	memberID string
}

// NewEmbeddedEtcdServer creates a new embedded etcd server.
func NewEmbeddedEtcdServer(config *EtcdConfig, memberID string) (*EmbeddedEtcdServer, error) {
	if config == nil {
		return nil, fmt.Errorf("etcd config is required")
	}
	if config.Mode != EtcdModeEmbedded {
		return nil, fmt.Errorf("etcd mode must be 'embedded' for embedded server")
	}
	if config.Embedded == nil {
		return nil, fmt.Errorf("embedded etcd configuration is required")
	}

	return &EmbeddedEtcdServer{
		config:   config,
		memberID: memberID,
	}, nil
}

// Start starts the embedded etcd server.
func (s *EmbeddedEtcdServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil // Already running
	}

	cfg := s.config.Embedded

	// Create data directory if it doesn't exist
	dataDir := cfg.DataDir
	if !filepath.IsAbs(dataDir) {
		// Make relative paths absolute from current working directory
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		dataDir = filepath.Join(wd, dataDir)
	}
	s.dataDir = dataDir

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Build etcd configuration
	embedCfg := embed.NewConfig()

	// Member name - use memberID or generate one
	memberName := s.memberID
	if memberName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			memberName = "default"
		} else {
			memberName = hostname
		}
	}
	embedCfg.Name = memberName

	// Data directory
	embedCfg.Dir = dataDir

	// Determine if TLS is enabled (for URL scheme)
	useTLS := cfg.IsTLSEnabled()

	// Client URLs - use configurable listen/advertise addresses with IPv6 support
	clientListenURL, err := url.Parse(cfg.GetClientListenURL(useTLS))
	if err != nil {
		return fmt.Errorf("failed to parse client listen URL: %w", err)
	}
	clientAdvertiseURL, err := url.Parse(cfg.GetClientAdvertiseURL(useTLS))
	if err != nil {
		return fmt.Errorf("failed to parse client advertise URL: %w", err)
	}
	embedCfg.ListenClientUrls = []url.URL{*clientListenURL}
	embedCfg.AdvertiseClientUrls = []url.URL{*clientAdvertiseURL}

	// Peer URLs - use configurable listen/advertise addresses with IPv6 support
	peerListenURL, err := url.Parse(cfg.GetPeerListenURL(useTLS))
	if err != nil {
		return fmt.Errorf("failed to parse peer listen URL: %w", err)
	}
	peerAdvertiseURL, err := url.Parse(cfg.GetPeerAdvertiseURL(useTLS))
	if err != nil {
		return fmt.Errorf("failed to parse peer advertise URL: %w", err)
	}
	embedCfg.ListenPeerUrls = []url.URL{*peerListenURL}
	embedCfg.AdvertisePeerUrls = []url.URL{*peerAdvertiseURL}

	// Cluster configuration
	if cfg.InitialCluster == "" {
		// Single-node cluster - use advertise URL
		embedCfg.InitialCluster = fmt.Sprintf("%s=%s", memberName, cfg.GetPeerAdvertiseURL(useTLS))
	} else {
		embedCfg.InitialCluster = cfg.InitialCluster
	}
	embedCfg.InitialClusterToken = cfg.InitialClusterToken
	embedCfg.ClusterState = cfg.InitialClusterState

	// Storage configuration
	embedCfg.QuotaBackendBytes = cfg.QuotaBackendBytes
	embedCfg.MaxSnapFiles = cfg.MaxSnapFiles
	embedCfg.MaxWalFiles = cfg.MaxWALFiles

	// Auto compaction
	embedCfg.AutoCompactionMode = cfg.AutoCompactionMode
	embedCfg.AutoCompactionRetention = cfg.AutoCompactionPeriod.String()

	// Logging - use a quiet logger for embedded mode
	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = "warn"
	}
	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = parseZapLevel(logLevel)
	zapCfg.OutputPaths = []string{"stderr"}
	zapCfg.ErrorOutputPaths = []string{"stderr"}
	logger, err := zapCfg.Build()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	embedCfg.ZapLoggerBuilder = embed.NewZapLoggerBuilder(logger)

	// Disable strict reconfiguration check for single-node bootstrap
	embedCfg.StrictReconfigCheck = false

	// Configure TLS if enabled
	if cfg.TLS != nil && cfg.TLS.Enabled {
		// Configure client TLS (for client-to-server connections)
		if cfg.TLS.AutoTLS {
			embedCfg.ClientAutoTLS = true
		} else {
			clientTLSInfo := transport.TLSInfo{
				CertFile:       cfg.TLS.ClientCertFile,
				KeyFile:        cfg.TLS.ClientKeyFile,
				ClientCertAuth: cfg.TLS.ClientCertAuth,
			}
			if cfg.TLS.ClientCAFile != "" {
				clientTLSInfo.TrustedCAFile = cfg.TLS.ClientCAFile
			}
			embedCfg.ClientTLSInfo = clientTLSInfo
		}

		// Configure peer TLS (for peer-to-peer connections)
		if cfg.TLS.PeerAutoTLS {
			embedCfg.PeerAutoTLS = true
		} else {
			peerTLSInfo := transport.TLSInfo{
				CertFile:       cfg.TLS.GetPeerCertFile(),
				KeyFile:        cfg.TLS.GetPeerKeyFile(),
				ClientCertAuth: cfg.TLS.PeerCertAuth,
			}
			if cfg.TLS.GetPeerCAFile() != "" {
				peerTLSInfo.TrustedCAFile = cfg.TLS.GetPeerCAFile()
			}
			embedCfg.PeerTLSInfo = peerTLSInfo
		}
	}

	// Start the embedded server
	server, err := embed.StartEtcd(embedCfg)
	if err != nil {
		return fmt.Errorf("failed to start embedded etcd: %w", err)
	}

	// Wait for server to be ready
	select {
	case <-server.Server.ReadyNotify():
		// Server is ready
	case <-time.After(60 * time.Second):
		server.Close()
		return fmt.Errorf("embedded etcd server took too long to start")
	case <-ctx.Done():
		server.Close()
		return ctx.Err()
	}

	s.server = server
	s.running = true

	return nil
}

// Stop stops the embedded etcd server.
func (s *EmbeddedEtcdServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.server == nil {
		return nil
	}

	s.server.Close()
	s.server = nil
	s.running = false

	return nil
}

// IsRunning returns true if the server is running.
func (s *EmbeddedEtcdServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// ClientEndpoint returns the client endpoint for connecting to this server.
// Returns a properly formatted endpoint with IPv6 bracket support.
func (s *EmbeddedEtcdServer) ClientEndpoint() string {
	return s.config.Embedded.GetClientEndpoint()
}

// DataDir returns the data directory path.
func (s *EmbeddedEtcdServer) DataDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dataDir
}

// parseZapLevel converts a string log level to zapcore.Level.
func parseZapLevel(level string) zap.AtomicLevel {
	switch level {
	case "debug":
		return zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn", "warning":
		return zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		return zap.NewAtomicLevelAt(zap.ErrorLevel)
	case "fatal":
		return zap.NewAtomicLevelAt(zap.FatalLevel)
	default:
		return zap.NewAtomicLevelAt(zap.WarnLevel)
	}
}
