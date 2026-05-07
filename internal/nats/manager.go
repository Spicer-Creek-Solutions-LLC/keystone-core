package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
)

// embeddedReadyTimeout bounds how long Manager.Start waits for the
// in-process nats-server to accept connections. nats-server's own
// ReadyForConnections defaults are sub-second; 10s leaves slack for
// CI hosts under load without disguising a stuck startup.
const embeddedReadyTimeout = 10 * time.Second

// Manager owns the v1.0 NATS lifecycle. It runs in one of two modes:
//
//   - embedded: starts a nats-server/v2 in-process and dials it with
//     nats.InProcessServer (no loopback socket round-trip).
//   - external: opens a single nats.Connect against the configured URL
//     list, with reconnect handled by nats.go.
//
// It satisfies pkg/api/server.NATSManager. Methods are safe to call
// concurrently; Start and Shutdown are idempotent.
type Manager struct {
	cfg config.NATSConfig
	log *slog.Logger

	mu       sync.Mutex
	started  bool
	stopped  bool
	embedded *natsserver.Server // nil in external mode
	conn     *nats.Conn         // nil before Start, after Shutdown
}

// New constructs a Manager from validated config. It does not open
// connections or start the embedded server — call Start for that.
func New(cfg config.NATSConfig, log *slog.Logger) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("nats: invalid config: %w", err)
	}
	if log == nil {
		log = slog.Default()
	}
	return &Manager{cfg: cfg, log: log}, nil
}

// Start brings the transport up. For embedded mode it boots the
// in-process server (creating the JetStream store dir if needed) and
// connects via InProcessServer. For external mode it opens a single
// connection against the URL list. Subsequent calls return nil.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return errors.New("nats: manager already shut down")
	}
	if m.started {
		return nil
	}

	switch m.cfg.Mode {
	case config.NATSModeEmbedded:
		if err := m.startEmbedded(ctx); err != nil {
			return err
		}
	case config.NATSModeExternal:
		if err := m.startExternal(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("nats: unknown mode %q", string(m.cfg.Mode))
	}

	m.started = true
	return nil
}

func (m *Manager) startEmbedded(ctx context.Context) error {
	if m.cfg.Embedded.EnableJetStream && m.cfg.JetStream.StoreDir != "" {
		if err := os.MkdirAll(m.cfg.JetStream.StoreDir, 0o750); err != nil {
			return fmt.Errorf("nats: jetstream store dir: %w", err)
		}
	}

	opts := &natsserver.Options{
		ServerName: "kscore-embedded",
		Host:       m.cfg.Embedded.Host,
		Port:       m.cfg.Embedded.Port,
		MaxConn:    m.cfg.Embedded.MaxConnections,
		JetStream:  m.cfg.Embedded.EnableJetStream,
		StoreDir:   m.cfg.JetStream.StoreDir,
		// Suppress nats-server's default signal handlers — kscore-server
		// owns the signal pipeline.
		NoSigs: true,
		// Quiet by default; the kscore slog logger surfaces health.
		NoLog: true,
	}
	if m.cfg.Embedded.EnableJetStream {
		opts.JetStreamMaxMemory = m.cfg.Embedded.MaxMemory
		opts.JetStreamMaxStore = m.cfg.JetStream.MaxStorage
	}

	srv, err := natsserver.NewServer(opts)
	if err != nil {
		return fmt.Errorf("nats: new embedded server: %w", err)
	}
	go srv.Start()

	deadline := embeddedReadyTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining > 0 && remaining < deadline {
			deadline = remaining
		}
	}
	if !srv.ReadyForConnections(deadline) {
		srv.Shutdown()
		srv.WaitForShutdown()
		return fmt.Errorf("nats: embedded server not ready within %s", deadline)
	}

	conn, err := nats.Connect(
		"",
		buildClientOptions(m.cfg, nats.InProcessServer(srv))...,
	)
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		return fmt.Errorf("nats: embedded in-process connect: %w", err)
	}

	m.embedded = srv
	m.conn = conn
	m.log.InfoContext(ctx, "nats embedded server started",
		"host", m.cfg.Embedded.Host,
		"port", m.cfg.Embedded.Port,
		"jetstream", m.cfg.Embedded.EnableJetStream,
	)
	return nil
}

func (m *Manager) startExternal() error {
	url := strings.Join(m.cfg.URLs, ",")
	conn, err := nats.Connect(url, buildClientOptions(m.cfg)...)
	if err != nil {
		return fmt.Errorf("nats: connect %q: %w", url, err)
	}
	m.conn = conn
	m.log.Info("nats external connection established",
		"urls", m.cfg.URLs,
	)
	return nil
}

// buildClientOptions assembles the nats.go option list shared by both
// modes. Extra options (e.g., InProcessServer) come first so callers
// can layer mode-specific behavior.
func buildClientOptions(cfg config.NATSConfig, extra ...nats.Option) []nats.Option {
	opts := append([]nats.Option{}, extra...)
	opts = append(opts,
		nats.Name("kscore"),
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(cfg.ReconnectWait),
	)
	if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	}
	if cfg.Credential != "" {
		opts = append(opts, nats.UserCredentials(cfg.Credential))
	}
	return opts
}

// Shutdown closes the client connection and (for embedded mode) stops
// the in-process server. Bounded by ctx — embedded shutdown blocks on
// WaitForShutdown so a long stop cancels through ctx via a watchdog.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started || m.stopped {
		m.stopped = true
		return nil
	}

	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}

	if m.embedded != nil {
		done := make(chan struct{})
		srv := m.embedded
		go func() {
			srv.Shutdown()
			srv.WaitForShutdown()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			m.embedded = nil
			m.stopped = true
			return fmt.Errorf("nats: embedded shutdown: %w", ctx.Err())
		}
		m.embedded = nil
	}

	m.stopped = true
	return nil
}

// Health reports nil when the transport is currently usable. Pre-Start
// or post-Shutdown returns an error so /health/ready flips to 503
// cleanly during boot and shutdown windows.
func (m *Manager) Health(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return errors.New("nats: shut down")
	}
	if !m.started || m.conn == nil {
		return errors.New("nats: not started")
	}
	if !m.conn.IsConnected() {
		return fmt.Errorf("nats: not connected (status=%s)", m.conn.Status())
	}
	// Embedded readiness is verified during Start; once running, we
	// trust the conn-level state. ReadyForConnections(0) returns false
	// even on a healthy server because its internal loop body never
	// executes with d=0.
	return nil
}

// Publish sends data on subject. Subject validation and the cluster
// prefix come from SubjectBuilder in Task 4; this method passes the
// subject through unchanged so the dispatcher controls naming.
func (m *Manager) Publish(_ context.Context, subject string, data []byte) error {
	m.mu.Lock()
	conn := m.conn
	stopped := m.stopped
	started := m.started
	m.mu.Unlock()

	if stopped {
		return errors.New("nats: shut down")
	}
	if !started || conn == nil {
		return errors.New("nats: not started")
	}
	if err := conn.Publish(subject, data); err != nil {
		return fmt.Errorf("nats: publish %q: %w", subject, err)
	}
	return nil
}

// ClientURL returns the URL clients should use to connect. For embedded
// mode it's the local nats-server address; for external mode it's the
// first configured URL. Used by tests; production code should not need
// it directly.
func (m *Manager) ClientURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.embedded != nil {
		return m.embedded.ClientURL()
	}
	if len(m.cfg.URLs) > 0 {
		return m.cfg.URLs[0]
	}
	return ""
}
