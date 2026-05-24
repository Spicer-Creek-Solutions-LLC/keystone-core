// SPDX-License-Identifier: Apache-2.0

package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/pkg/envelope"
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
//   - external: delegates to ConnectionManager (Task 2) which handles
//     multi-endpoint failover and per-endpoint state tracking.
//
// It satisfies pkg/api/server.NATSManager. Methods are safe to call
// concurrently; Start and Shutdown are idempotent.
type Manager struct {
	cfg      config.NATSConfig
	log      *slog.Logger
	subjects *SubjectBuilder
	dedup    *Dedup // nil when cfg.NATS.Dedup.Enabled is false

	mu       sync.Mutex
	started  bool
	stopped  bool
	embedded *natsserver.Server // nil in external mode
	conn     *nats.Conn         // embedded mode only
	connMgr  *ConnectionManager // external mode only
}

// New constructs a Manager from validated config. It does not open
// connections or start the embedded server — call Start for that.
func New(cfg config.NATSConfig, log *slog.Logger) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("nats: invalid config: %w", err)
	}
	subjects, err := NewSubjectBuilder(cfg.ClusterName)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		cfg:      cfg,
		log:      log,
		subjects: subjects,
		dedup:    NewDedup(cfg.Dedup, log),
	}, nil
}

// Subjects returns the SubjectBuilder that owns the v1.0 subject
// hierarchy. Higher-level callers (CommandDispatcher, future
// bootstrap handler) must use this for construction so the cluster
// prefix is never typo'd inline.
func (m *Manager) Subjects() *SubjectBuilder { return m.subjects }

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
		if err := m.startExternal(ctx); err != nil {
			return err
		}
	default:
		return fmt.Errorf("nats: unknown mode %q", string(m.cfg.Mode))
	}

	if m.cfg.JetStream.Enabled {
		if err := m.ensureStreams(ctx, m.activeConnLocked()); err != nil {
			return err
		}
	}

	m.dedup.Start()
	m.started = true
	return nil
}

func (m *Manager) startEmbedded(ctx context.Context) error {
	jsEnabled := m.cfg.JetStream.Enabled
	if jsEnabled && m.cfg.JetStream.StoreDir != "" {
		if err := os.MkdirAll(m.cfg.JetStream.StoreDir, 0o750); err != nil {
			return fmt.Errorf("nats: jetstream store dir: %w", err)
		}
	}

	opts := &natsserver.Options{
		ServerName: "kscore-embedded",
		Host:       m.cfg.Embedded.Host,
		Port:       m.cfg.Embedded.Port,
		MaxConn:    m.cfg.Embedded.MaxConnections,
		JetStream:  jsEnabled,
		StoreDir:   m.cfg.JetStream.StoreDir,
		// Suppress nats-server's default signal handlers — kscore-server
		// owns the signal pipeline.
		NoSigs: true,
		// Quiet by default; the kscore slog logger surfaces health.
		NoLog: true,
	}
	if jsEnabled {
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
		"jetstream", jsEnabled,
	)
	return nil
}

func (m *Manager) startExternal(ctx context.Context) error {
	cm, err := NewConnectionManager(m.cfg, m.log)
	if err != nil {
		return fmt.Errorf("nats: connection manager: %w", err)
	}
	if err := cm.Start(ctx); err != nil {
		return err
	}
	m.connMgr = cm
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

	m.dedup.Stop()

	if m.connMgr != nil {
		if err := m.connMgr.Shutdown(ctx); err != nil {
			m.connMgr = nil
			m.stopped = true
			return err
		}
		m.connMgr = nil
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
// cleanly during boot and shutdown windows. External mode delegates
// to ConnectionManager.Health; embedded mode checks the in-process
// conn directly.
func (m *Manager) Health(ctx context.Context) error {
	m.mu.Lock()
	stopped := m.stopped
	started := m.started
	cm := m.connMgr
	conn := m.conn
	m.mu.Unlock()

	if stopped {
		return errors.New("nats: shut down")
	}
	if !started {
		return errors.New("nats: not started")
	}
	if cm != nil {
		return cm.Health(ctx)
	}
	if conn == nil {
		return errors.New("nats: not started")
	}
	if !conn.IsConnected() {
		return fmt.Errorf("nats: not connected (status=%s)", conn.Status())
	}
	// Embedded readiness is verified during Start; once running, we
	// trust the conn-level state. ReadyForConnections(0) returns false
	// even on a healthy server because its internal loop body never
	// executes with d=0.
	return nil
}

// PublishEnvelope marshals env and publishes on subject. The
// SubjectBuilder interceptor (Task 4) rejects non-prefixed subjects;
// envelope.Marshal validates required fields (MessageID, Priority,
// ClusterPrefix). PROJECT-DETAILS §4.2 mandates an Envelope around
// every published payload — this is the only publish path Manager
// exposes (the byte-level Publish was retired in Task 5).
//
// PublishEnvelope additionally requires env.ClusterPrefix to match
// the manager's configured prefix; a mismatch indicates an envelope
// constructed against a different cluster's SubjectBuilder, which
// would publish to the right subject but carry the wrong routing
// metadata — a footgun worth catching at the boundary.
//
// Dedup (Task 6, when enabled): IsDuplicate is checked before the
// publish; on hit we return envelope.ErrDuplicate without touching
// NATS. On a successful publish, Record stamps the (subject,
// MessageID) pair into the cache so a retry within the window is
// suppressed. We record after publish so a network failure doesn't
// leave a phantom entry that suppresses the legitimate retry.
func (m *Manager) PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error {
	if err := m.subjects.Validate(subject); err != nil {
		return err
	}
	if env.ClusterPrefix != m.subjects.Prefix() {
		return fmt.Errorf("nats: envelope cluster_prefix %q does not match manager prefix %q",
			env.ClusterPrefix, m.subjects.Prefix())
	}
	if m.dedup != nil && m.dedup.IsDuplicate(subject, env.MessageID) {
		return envelope.ErrDuplicate
	}
	data, err := env.Marshal()
	if err != nil {
		return err
	}
	if err := m.publishBytes(ctx, subject, data); err != nil {
		return err
	}
	if m.dedup != nil {
		m.dedup.Record(subject, env.MessageID)
	}
	return nil
}

// publishBytes is the post-validation publish path shared by
// PublishEnvelope (and, in later tasks, the bootstrap registration
// handler). External mode routes through ConnectionManager so per-
// endpoint state tracks publish outcomes; embedded mode publishes
// directly on the in-process conn.
func (m *Manager) publishBytes(ctx context.Context, subject string, data []byte) error {
	m.mu.Lock()
	stopped := m.stopped
	started := m.started
	cm := m.connMgr
	conn := m.conn
	m.mu.Unlock()

	if stopped {
		return errors.New("nats: shut down")
	}
	if !started {
		return errors.New("nats: not started")
	}
	if cm != nil {
		return cm.publishBytes(ctx, subject, data)
	}
	if conn == nil {
		return errors.New("nats: not started")
	}
	if err := conn.Publish(subject, data); err != nil {
		return fmt.Errorf("nats: publish %q: %w", subject, err)
	}
	return nil
}

// JetStream returns a JetStream context against the manager's
// active connection. Used by cmd/kscore-server/events.go (Epic 11
// task 6) to construct the events runtime; the JetStream stream
// itself is created by ensureStreams during Start.
//
// Returns an error if the manager isn't started or JetStream isn't
// enabled in the config.
func (m *Manager) JetStream() (nats.JetStreamContext, error) {
	m.mu.Lock()
	conn := m.activeConnLocked()
	m.mu.Unlock()
	if conn == nil {
		return nil, errors.New("nats: JetStream: manager not started")
	}
	js, err := conn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("nats: JetStream: %w", err)
	}
	return js, nil
}

// ClientURL returns the URL clients should use to connect. For
// embedded mode it's the local nats-server address. For external mode
// it's the active endpoint when connected, or the highest-priority
// endpoint URL otherwise. Used by tests; production code should not
// need it directly.
func (m *Manager) ClientURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.embedded != nil {
		return m.embedded.ClientURL()
	}
	if m.connMgr != nil {
		if active, ok := m.connMgr.ActiveEndpoint(); ok {
			return active
		}
		eps := m.connMgr.Endpoints()
		if len(eps) > 0 {
			return eps[0].URL
		}
	}
	if len(m.cfg.URLs) > 0 {
		return m.cfg.URLs[0]
	}
	return ""
}

// EndpointSnapshots returns the per-endpoint snapshot in external
// mode, or nil for embedded mode. Used by /api/status (Task 11
// wiring) and by tests.
func (m *Manager) EndpointSnapshots() []EndpointSnapshot {
	m.mu.Lock()
	cm := m.connMgr
	m.mu.Unlock()
	if cm == nil {
		return nil
	}
	return cm.Snapshot()
}
