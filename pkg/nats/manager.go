package nats

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/pkg/config"
)

// Manager handles NATS connections and embedded server
type Manager struct {
	config        *config.NATSConfig
	embeddedMode  bool
	server        *server.Server
	conn          *nats.Conn
	jetStream     nats.JetStreamContext
	mu            sync.RWMutex
	shutdownFuncs []func() error
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewManager creates a new NATS manager
func NewManager(cfg *config.NATSConfig) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config:       cfg,
		embeddedMode: cfg.Mode == config.NATSModeEmbedded || cfg.Mode == config.NATSModeLeaf,
		ctx:          ctx,
		cancel:       cancel,
	}

	return m, nil
}

// Start initializes the NATS connection (and embedded server if needed)
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Start embedded NATS server if in embedded or leaf mode
	if m.embeddedMode {
		if err := m.startEmbeddedServer(); err != nil {
			return fmt.Errorf("failed to start embedded NATS server: %w", err)
		}
	}

	// Connect to NATS
	if err := m.connect(); err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Initialize JetStream if enabled
	if m.config.JetStream.Enabled {
		if err := m.initJetStream(); err != nil {
			return fmt.Errorf("failed to initialize JetStream: %w", err)
		}
	}

	return nil
}

// startEmbeddedServer starts an embedded NATS server
func (m *Manager) startEmbeddedServer() error {
	host := m.config.Embedded.Host
	if host == "" {
		host = "127.0.0.1"
	}
	opts := &server.Options{
		Host:           host,
		Port:           m.config.Embedded.Port,
		MaxConn:        m.config.Embedded.MaxConnections,
		MaxPayload:     1024 * 1024, // 1MB
		WriteDeadline:  10 * time.Second,
		MaxControlLine: 4096,
		DisableShortFirstPing: false,
	}

	// Configure JetStream for embedded mode
	if m.config.Embedded.EnableJetStream {
		storeDir := m.config.Embedded.StoreDir
		if storeDir == "" {
			storeDir = "./data/nats"
		}

		// Ensure store directory exists
		if err := os.MkdirAll(storeDir, 0755); err != nil {
			return fmt.Errorf("failed to create JetStream store directory: %w", err)
		}

		opts.JetStream = true
		opts.StoreDir = storeDir
		if m.config.Embedded.MaxMemory > 0 {
			opts.JetStreamMaxMemory = m.config.Embedded.MaxMemory
		}
		if m.config.JetStream.MaxStorage > 0 {
			opts.JetStreamMaxStore = m.config.JetStream.MaxStorage
		}
	}

	// Configure leaf node mode if enabled
	if m.config.Mode == config.NATSModeLeaf && len(m.config.Embedded.LeafNodeURLs) > 0 {
		remotes := make([]*server.RemoteLeafOpts, 0, len(m.config.Embedded.LeafNodeURLs))
		for _, urlStr := range m.config.Embedded.LeafNodeURLs {
			parsedURL, err := url.Parse(urlStr)
			if err != nil {
				return fmt.Errorf("failed to parse leaf node URL %s: %w", urlStr, err)
			}
			remotes = append(remotes, &server.RemoteLeafOpts{
				URLs: []*url.URL{parsedURL},
			})
		}
		opts.LeafNode = server.LeafNodeOpts{
			Remotes: remotes,
		}
	}

	// Create and start the server
	ns, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %w", err)
	}

	// Configure logger (use NATS built-in logger for now)
	ns.ConfigureLogger()

	// Start the server in a goroutine
	go ns.Start()

	// Wait for the server to be ready
	if !ns.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("embedded NATS server failed to start within timeout")
	}

	m.server = ns
	m.shutdownFuncs = append(m.shutdownFuncs, func() error {
		if m.server != nil {
			m.server.Shutdown()
			m.server.WaitForShutdown()
		}
		return nil
	})

	return nil
}

// connect establishes a connection to NATS
func (m *Manager) connect() error {
	opts := []nats.Option{
		nats.Name("keystone-core"),
		nats.MaxReconnects(m.config.MaxReconnects),
		nats.ReconnectWait(m.config.ReconnectWait),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				fmt.Printf("NATS disconnected: %v\n", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Printf("NATS reconnected to %s\n", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			fmt.Println("NATS connection closed")
		}),
	}

	// Add authentication if configured
	if m.config.Token != "" {
		opts = append(opts, nats.Token(m.config.Token))
	}
	if m.config.Credential != "" {
		opts = append(opts, nats.UserCredentials(m.config.Credential))
	}

	// Determine connection URL
	url := m.config.URL
	if m.embeddedMode && url == "" {
		url = fmt.Sprintf("nats://127.0.0.1:%d", m.config.Embedded.Port)
	}
	if url == "" {
		return fmt.Errorf("NATS URL is required for external mode")
	}

	// Connect to NATS
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS at %s: %w", url, err)
	}

	m.conn = nc
	m.shutdownFuncs = append(m.shutdownFuncs, func() error {
		if m.conn != nil {
			m.conn.Close()
		}
		return nil
	})

	return nil
}

// initJetStream initializes JetStream context
func (m *Manager) initJetStream() error {
	js, err := m.conn.JetStream()
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	m.jetStream = js
	return nil
}

// Conn returns the NATS connection
func (m *Manager) Conn() *nats.Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conn
}

// JetStream returns the JetStream context
func (m *Manager) JetStream() nats.JetStreamContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jetStream
}

// IsEmbedded returns true if running in embedded mode
func (m *Manager) IsEmbedded() bool {
	return m.embeddedMode
}

// Server returns the embedded NATS server (nil if not in embedded mode)
func (m *Manager) Server() *server.Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.server
}

// Health checks the health of the NATS connection
func (m *Manager) Health() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.conn == nil {
		return fmt.Errorf("NATS connection is nil")
	}

	if !m.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not established")
	}

	// Check embedded server health if applicable
	if m.embeddedMode && m.server != nil {
		if !m.server.Running() {
			return fmt.Errorf("embedded NATS server is not running")
		}
	}

	return nil
}

// Shutdown gracefully shuts down the NATS manager
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel context
	if m.cancel != nil {
		m.cancel()
	}

	// Run shutdown functions in reverse order
	for i := len(m.shutdownFuncs) - 1; i >= 0; i-- {
		if err := m.shutdownFuncs[i](); err != nil {
			fmt.Printf("Error during shutdown: %v\n", err)
		}
	}

	return nil
}

// Publish publishes a message to a subject
func (m *Manager) Publish(subject string, data []byte) error {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("NATS connection is not initialized")
	}

	return conn.Publish(subject, data)
}

// PublishRequest publishes a request and waits for a response
func (m *Manager) PublishRequest(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("NATS connection is not initialized")
	}

	return conn.Request(subject, data, timeout)
}

// Subscribe subscribes to a subject
func (m *Manager) Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("NATS connection is not initialized")
	}

	return conn.Subscribe(subject, handler)
}

// QueueSubscribe subscribes to a subject with a queue group
func (m *Manager) QueueSubscribe(subject, queue string, handler nats.MsgHandler) (*nats.Subscription, error) {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("NATS connection is not initialized")
	}

	return conn.QueueSubscribe(subject, queue, handler)
}
