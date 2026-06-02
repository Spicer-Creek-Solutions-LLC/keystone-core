// SPDX-License-Identifier: Apache-2.0

package server

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/health"
	"go.keystone-core.io/keystone-core/internal/metrics"
	"go.keystone-core.io/keystone-core/internal/policy"
	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
	clusterapi "go.keystone-core.io/keystone-core/pkg/api/cluster"
	"go.keystone-core.io/keystone-core/pkg/api/gitops"
	"go.keystone-core.io/keystone-core/pkg/api/webhooks"
)

// Options is the constructor input for New. Required fields: Config,
// Logger, Store, NATSManager, Subjects. Everything else has a default.
type Options struct {
	Config      *config.Config
	Logger      *slog.Logger
	Store       state.Store
	NATSManager NATSManager

	// Subjects is the SubjectBuilder threaded into the
	// CommandDispatcher (Epic 05 task 4). cmd/kscore-server pulls this
	// off internal/nats.Manager via Manager.Subjects(). Tests pass a
	// hand-rolled fake.
	Subjects controlplane.Subjects

	// Signer is the HMAC signer threaded into the CommandDispatcher
	// (Epic 06 task 5). cmd/kscore-server constructs an
	// internal/agent.SecurityEnforcer from cfg.Security and adapts
	// it to controlplane.Signer. Tests pass a fake.
	Signer controlplane.Signer

	// Subscriber is the inbound NATS surface used by the bootstrap
	// handler (Epic 05 task 9). Optional — when nil OR
	// cfg.NATS.Bootstrap.Enabled is false, the handler is not
	// started. cmd/kscore-server passes internal/nats.Manager.
	Subscriber controlplane.Subscriber

	// BootstrapValidator validates agent identity proofs. Optional —
	// only consulted when bootstrap is enabled. Production wiring
	// passes an in-memory PSKValidator constructed from
	// cfg.NATS.Bootstrap.PSKs.
	BootstrapValidator controlplane.BootstrapValidator

	// CredentialIssuer mints "full credentials" once a PSK is
	// consumed. Optional — only consulted when bootstrap is enabled.
	// Production wiring passes an APIKeyIssuer over the API key
	// store.
	CredentialIssuer controlplane.CredentialIssuer

	// AuthInterceptor wires the gRPC auth chain when non-nil. v1.0
	// task 4 leaves this optional; task 6 pins it on by default.
	AuthInterceptor *auth.InterceptorConfig

	// Fencer wires the Epic 13 split-brain fencing guard around the
	// gRPC + REST write paths when non-nil. cmd/kscore-server injects
	// an adapter over the cluster FencingManager when clustering is
	// enabled; nil (the single-node / clustering-disabled path) leaves
	// every request unfenced.
	Fencer Fencer

	// TLSConfig wires gRPC TLS / mTLS when non-nil (Epic 09 task 13).
	// Required when cfg.Server.TLS.Enabled. cmd/kscore-server
	// derives this from the identity provider; tests inject a
	// purpose-built tls.Config.
	TLSConfig *tls.Config

	// Clock is injectable for tests (uptime, status-ticker timing).
	// Defaults to time.Now.
	Clock func() time.Time

	// StatusTickerInterval overrides the 30s default from §4.4 step 21.
	StatusTickerInterval time.Duration

	// SecretsBroker is the Epic 10 task-3 broker — optional, nil when
	// `secrets.enabled: false`. When non-nil, the REST + gRPC
	// secrets surface is wired against it.
	SecretsBroker *secrets.Broker

	// SecretsTransit is the Epic 10 task-7 transit backend —
	// typically the Vault backend instance also referenced via
	// SecretsBroker.Backends. Optional.
	SecretsTransit secrets.TransitBackend

	// SecretsLeases is the Epic 10 task-6 LeaseManager. Optional.
	SecretsLeases *secrets.LeaseManager

	// EventStore is the Epic 11 task-2 SQL-backed event store —
	// optional, nil when `events.enabled: false`. When non-nil, the
	// REST + gRPC EventService surface is wired against it.
	EventStore events.EventStore

	// EventPublisher is the Epic 11 task-3 JetStream publisher.
	// Optional; nil disables the EmitEvent surface (returns
	// Unavailable / 503).
	EventPublisher events.EventPublisher

	// EventSubscriber is the Epic 11 task-4 JetStream subscriber.
	// Optional; nil disables the SubscribeEvents server-streaming
	// RPC.
	EventSubscriber events.EventSubscriber

	// CommandTerminalHook is Epic 12 task 4's audit-emission hook
	// for the command exec sensitive op. The server passes it into
	// CommandDispatcher as OnCommandTerminal. Optional — nil
	// disables audit emission for command terminal states.
	CommandTerminalHook controlplane.TerminalCommandFunc

	// StateAuditor is Epic 12 task 4's audit-emission seam for the
	// state apply sensitive op. The server hands it to the
	// StateGRPCServer where ApplyState wraps the per-request
	// streamObserver with audit.StateApplyObserver. Optional — nil
	// disables audit emission for state apply.
	StateAuditor audit.Auditor

	// Policy* are the Epic 12 task-13 policy REST surface backings
	// (built by cmd/kscore-server startPolicy). Any may be nil; the
	// dependent /api/v1/policies routes then return 503. PolicyAuditLog
	// also backs the gRPC PolicyService; PolicyAuditor receives the
	// per-evaluation audit entry.
	PolicyEngine   *policy.Engine
	PolicyReports  *policy.ReportGenerator
	PolicyAuditLog audit.AuditStore
	PolicyAuditor  audit.Auditor

	// Metrics is the Epic 17 task 2 emitter for HTTP + gRPC request
	// duration. Optional — nil disables emission. When set, the HTTP
	// middleware chain inserts the metrics middleware just outside the
	// auth chain, and the gRPC interceptor chain prepends the metrics
	// interceptors.
	Metrics *Metrics

	// ControlPlaneMetrics is the Epic 17 task 2 emitter for the
	// kscore_agents_total gauge. The server's existing status ticker
	// (PROJECT-DETAILS §4.4 step 21) pushes Counts into it every
	// StatusTickerInterval. Optional; nil disables emission.
	ControlPlaneMetrics *controlplane.Metrics

	// MetricsRegistry is the Epic 17 task 3 Prometheus registry the
	// server exposes at cfg.Metrics.Path (default /metrics). Optional;
	// nil suppresses the handler. Production wiring builds the same
	// Registry that Task 2 emitters register against.
	MetricsRegistry *metrics.Registry

	// JetStreamPinger probes JetStream for /health/ready. Epic 17 task
	// 6 adds JetStream as a v1.0 health-checked component alongside
	// NATS + DB. Optional — when nil, JetStream is omitted from the
	// component map. Production wiring passes an adapter over
	// natsmgr.Manager.JetStream().
	JetStreamPinger health.JetStreamPinger

	// HealthCheckers is the operator-supplied list of custom Checkers
	// the server registers alongside NATS / DB / JetStream. Each one
	// shows up as its own component key in /health/ready and
	// /health/status. Optional.
	HealthCheckers []health.Checker

	// WebhookProviders backs /api/v1/webhooks/* — outbound webhook
	// subscription CRUD + test delivery. Epic 19 task 2c boot-wiring;
	// when zero the routes return 503 (the package's documented
	// not-yet-wired posture).
	WebhookProviders webhooks.Providers

	// GitOpsProviders backs /api/v1/gitops/* — rollback CRUD + FSM
	// transitions + verification history. Epic 19 task 2c boot-wiring;
	// when zero the routes return 503.
	GitOpsProviders gitops.Providers

	// ClusterProviders backs /api/v1/cluster/* — topology + leader +
	// backup. Wired when clustering is constructed at boot (Epic 13);
	// the zero value leaves every cluster route at 503.
	ClusterProviders clusterapi.ClusterProviders
}

// validate returns an error if required fields are missing or
// internally inconsistent. Defaults are applied separately by New.
func (o *Options) validate() error {
	if o.Config == nil {
		return errors.New("server: Options.Config is required")
	}
	if o.Logger == nil {
		return errors.New("server: Options.Logger is required")
	}
	if o.Store == nil {
		return errors.New("server: Options.Store is required")
	}
	if o.NATSManager == nil {
		return errors.New("server: Options.NATSManager is required (use NoopNATSManager{} for tests, internal/nats.Manager in production)")
	}
	if o.Subjects == nil {
		return errors.New("server: Options.Subjects is required (Manager.Subjects() in production; a hand-rolled fake in tests)")
	}
	if o.Signer == nil {
		return errors.New("server: Options.Signer is required (Epic 06 task 5; constructs from cfg.Security in production)")
	}
	if o.StatusTickerInterval < 0 {
		return errors.New("server: StatusTickerInterval must be non-negative")
	}
	if o.Config.NATS.Bootstrap.Enabled {
		if o.Subscriber == nil {
			return errors.New("server: Options.Subscriber is required when NATS bootstrap is enabled")
		}
		if o.BootstrapValidator == nil {
			return errors.New("server: Options.BootstrapValidator is required when NATS bootstrap is enabled")
		}
		if o.CredentialIssuer == nil {
			return errors.New("server: Options.CredentialIssuer is required when NATS bootstrap is enabled")
		}
	}
	return nil
}
