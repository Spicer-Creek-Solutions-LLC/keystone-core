package server

import (
	"context"

	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// NATSManager is the narrow surface the Server needs from the NATS
// transport: lifecycle, health, and an envelope publish path used by
// the CommandDispatcher. internal/nats.Manager (Epic 05) is the
// production implementation; NoopNATSManager remains as a test stub
// so pkg/api/server tests do not depend on the embedded server.
//
// PROJECT-DETAILS §4.2 mandates an Envelope around every published
// message, so PublishEnvelope is the only publish path — the
// byte-level Publish was retired in task 5.
type NATSManager interface {
	// Start brings the manager up (embedded server start, or external
	// connect). Must be safe to call once; subsequent calls return nil.
	Start(ctx context.Context) error

	// Shutdown closes connections, drains JetStream, and (for embedded
	// mode) stops the in-process server. Bounded by ctx.
	Shutdown(ctx context.Context) error

	// Health returns nil if the transport is currently usable. Used by
	// /health/ready (task 7) and the 30s status ticker.
	Health(ctx context.Context) error

	// PublishEnvelope marshals env and publishes on subject. The
	// implementation is responsible for subject-prefix and envelope
	// validation; callers do not pre-marshal.
	PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error
}

// NoopNATSManager is a test stub. All operations succeed without
// doing anything; cmd/kscore-server uses internal/nats.Manager in
// production.
type NoopNATSManager struct{}

func (NoopNATSManager) Start(context.Context) error    { return nil }
func (NoopNATSManager) Shutdown(context.Context) error { return nil }
func (NoopNATSManager) Health(context.Context) error   { return nil }
func (NoopNATSManager) PublishEnvelope(context.Context, string, envelope.Envelope) error {
	return nil
}
