package server

import "context"

// NATSManager is the narrow surface the Server needs from the NATS
// transport: lifecycle, health, and a publish path used by the
// CommandDispatcher. internal/nats.Manager (Epic 05 task 1) is the
// production implementation; NoopNATSManager remains as a test stub
// so pkg/api/server tests do not depend on the embedded server.
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

	// Publish is the CommandDispatcher's outbound path. Wraps
	// nats.Conn.Publish in the real impl.
	Publish(ctx context.Context, subject string, data []byte) error
}

// NoopNATSManager is a test stub. All operations succeed without
// doing anything; cmd/kscore-server uses internal/nats.Manager in
// production.
type NoopNATSManager struct{}

func (NoopNATSManager) Start(context.Context) error             { return nil }
func (NoopNATSManager) Shutdown(context.Context) error          { return nil }
func (NoopNATSManager) Health(context.Context) error            { return nil }
func (NoopNATSManager) Publish(context.Context, string, []byte) error {
	return nil
}
