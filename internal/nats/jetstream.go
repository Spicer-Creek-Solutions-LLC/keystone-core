package nats

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
)

// streamCommandsPrefix and streamEventsPrefix are the static parts of
// the v1.0 JetStream stream names. NATS stream names cannot contain
// dots, so cluster scoping is done with a "_<cluster>" suffix rather
// than the dotted-prefix convention used for subjects.
const (
	streamCommandsPrefix = "KSCORE_COMMANDS"
	streamEventsPrefix   = "KSCORE_EVENTS"
)

// StreamDef is the resolved description of one stream — name plus the
// nats.StreamConfig we send to the server. Built from the
// SubjectBuilder + JetStreamConfig at Manager construction time;
// passed to ensureStreams on Start.
type StreamDef struct {
	Name   string
	Config nats.StreamConfig
}

// DefaultStreamDefs returns the v1.0 stream set:
//
//	KSCORE_COMMANDS_<cluster>  capturing kscore.<cluster>.agent.*.command
//	KSCORE_EVENTS_<cluster>    capturing kscore.<cluster>.events.>
//
// The events-stream subject pattern matches PROJECT-DETAILS §4.9 —
// the project-wide event bus addresses events as
// `kscore.<cluster>.events.<category>.<subtype>`, so the stream
// captures the whole `events.>` subtree under the cluster prefix.
// Epic 11 task 3 (events.JetStreamPublisher) is the canonical
// producer; future subscribers (Epic 11 task 4) durable-consume from
// here.
//
// Both streams share the per-stream limits from JetStreamConfig
// (StreamMaxAge / StreamMaxBytes / StreamMaxMsgs / StreamReplicas).
// Discard policy is hardcoded to DiscardNew per PROJECT-DETAILS §4.2
// — bounded streams reject new messages rather than dropping old
// ones, which is the safer choice for at-least-once durability.
func DefaultStreamDefs(subjects *SubjectBuilder, cfg config.JetStreamConfig) []StreamDef {
	suffix := config.JetStreamSafeName(subjects.Cluster())
	prefix := subjects.Prefix()
	return []StreamDef{
		{
			Name: streamCommandsPrefix + "_" + suffix,
			Config: nats.StreamConfig{
				Name:        streamCommandsPrefix + "_" + suffix,
				Description: "Keystone Core commands stream (Epic 05 task 8)",
				Subjects:    []string{prefix + ".agent.*.command"},
				Retention:   nats.LimitsPolicy,
				MaxAge:      cfg.StreamMaxAge,
				MaxBytes:    cfg.StreamMaxBytes,
				MaxMsgs:     cfg.StreamMaxMsgs,
				Discard:     nats.DiscardNew,
				Storage:     nats.FileStorage,
				Replicas:    cfg.StreamReplicas,
			},
		},
		{
			Name: streamEventsPrefix + "_" + suffix,
			Config: nats.StreamConfig{
				Name:        streamEventsPrefix + "_" + suffix,
				Description: "Keystone Core events stream (Epic 11 task 3) — kscore.<cluster>.events.>",
				Subjects:    []string{prefix + ".events.>"},
				Retention:   nats.LimitsPolicy,
				MaxAge:      cfg.StreamMaxAge,
				MaxBytes:    cfg.StreamMaxBytes,
				MaxMsgs:     cfg.StreamMaxMsgs,
				Discard:     nats.DiscardNew,
				Storage:     nats.FileStorage,
				Replicas:    cfg.StreamReplicas,
			},
		},
	}
}

// ensureStreams creates missing streams and updates existing ones to
// match DefaultStreamDefs. Idempotent — safe to call across restarts.
// Returns the first error encountered; partial creation is left in
// place for operator inspection.
//
// Failure is hard: a misconfigured external NATS without JetStream
// enabled, or a credential without admin permissions, will surface
// here. Operators who don't want auto-creation set
// cfg.JetStream.Enabled = false.
//
// Caller passes the active *nats.Conn rather than letting this
// method look it up so the function is callable from inside Start
// (which already holds m.mu) without re-entering the lock.
func (m *Manager) ensureStreams(ctx context.Context, conn *nats.Conn) error {
	if conn == nil {
		return errors.New("nats: ensureStreams: no active connection")
	}
	js, err := conn.JetStream()
	if err != nil {
		return fmt.Errorf("nats: ensureStreams: jetstream context: %w", err)
	}

	defs := DefaultStreamDefs(m.subjects, m.cfg.JetStream)
	for _, def := range defs {
		if err := upsertStream(js, def); err != nil {
			return fmt.Errorf("nats: ensureStreams: stream %q: %w", def.Name, err)
		}
		m.log.InfoContext(ctx, "nats jetstream ensured",
			"stream", def.Name,
			"subjects", def.Config.Subjects,
		)
	}
	return nil
}

// upsertStream calls AddStream when the stream is missing and
// UpdateStream when it already exists. We probe via StreamInfo
// rather than relying on AddStream's idempotency (it isn't —
// nats.go returns an error when the stream already exists).
func upsertStream(js nats.JetStreamContext, def StreamDef) error {
	_, err := js.StreamInfo(def.Name)
	if err != nil {
		if errors.Is(err, nats.ErrStreamNotFound) {
			if _, addErr := js.AddStream(&def.Config); addErr != nil {
				return fmt.Errorf("add: %w", addErr)
			}
			return nil
		}
		return fmt.Errorf("info: %w", err)
	}
	if _, err := js.UpdateStream(&def.Config); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	return nil
}

// activeConnLocked returns the *nats.Conn currently in use. Caller
// must hold m.mu (read or write). Used by Start to pass the conn
// into ensureStreams without re-locking.
func (m *Manager) activeConnLocked() *nats.Conn {
	if m.conn != nil {
		return m.conn
	}
	if m.connMgr != nil {
		m.connMgr.mu.RLock()
		defer m.connMgr.mu.RUnlock()
		return m.connMgr.conn
	}
	return nil
}
