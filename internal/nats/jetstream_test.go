package nats

import (
	"context"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
)

func TestDefaultStreamDefs_Names(t *testing.T) {
	subjects, err := NewSubjectBuilder("prod-east")
	if err != nil {
		t.Fatalf("NewSubjectBuilder: %v", err)
	}
	defs := DefaultStreamDefs(subjects, config.JetStreamConfig{
		StreamMaxAge:   time.Hour,
		StreamMaxBytes: 1024,
		StreamMaxMsgs:  100,
		StreamReplicas: 1,
	})

	if len(defs) != 2 {
		t.Fatalf("len = %d, want 2", len(defs))
	}
	if defs[0].Name != "KSCORE_COMMANDS_prod-east" {
		t.Errorf("commands stream name = %q", defs[0].Name)
	}
	if defs[1].Name != "KSCORE_EVENTS_prod-east" {
		t.Errorf("events stream name = %q", defs[1].Name)
	}
	// Subject filters scoped to the cluster.
	if got, want := defs[0].Config.Subjects, []string{"kscore.prod-east.agent.*.command"}; !equalStrSlice(got, want) {
		t.Errorf("commands subjects = %v, want %v", got, want)
	}
	if got, want := defs[1].Config.Subjects, []string{"kscore.prod-east.agent.*.events"}; !equalStrSlice(got, want) {
		t.Errorf("events subjects = %v, want %v", got, want)
	}
}

func TestDefaultStreamDefs_AppliesConfigDefaults(t *testing.T) {
	subjects, err := NewSubjectBuilder("default")
	if err != nil {
		t.Fatalf("NewSubjectBuilder: %v", err)
	}
	cfg := config.JetStreamConfig{
		StreamMaxAge:   2 * time.Hour,
		StreamMaxBytes: 4096,
		StreamMaxMsgs:  256,
		StreamReplicas: 3,
	}
	defs := DefaultStreamDefs(subjects, cfg)
	for _, d := range defs {
		if d.Config.MaxAge != cfg.StreamMaxAge {
			t.Errorf("%s MaxAge = %s, want %s", d.Name, d.Config.MaxAge, cfg.StreamMaxAge)
		}
		if d.Config.MaxBytes != cfg.StreamMaxBytes {
			t.Errorf("%s MaxBytes = %d, want %d", d.Name, d.Config.MaxBytes, cfg.StreamMaxBytes)
		}
		if d.Config.MaxMsgs != cfg.StreamMaxMsgs {
			t.Errorf("%s MaxMsgs = %d, want %d", d.Name, d.Config.MaxMsgs, cfg.StreamMaxMsgs)
		}
		if d.Config.Replicas != cfg.StreamReplicas {
			t.Errorf("%s Replicas = %d, want %d", d.Name, d.Config.Replicas, cfg.StreamReplicas)
		}
		if d.Config.Discard != natsclient.DiscardNew {
			t.Errorf("%s Discard = %v, want DiscardNew", d.Name, d.Config.Discard)
		}
		if d.Config.Storage != natsclient.FileStorage {
			t.Errorf("%s Storage = %v, want FileStorage", d.Name, d.Config.Storage)
		}
		if d.Config.Retention != natsclient.LimitsPolicy {
			t.Errorf("%s Retention = %v, want LimitsPolicy", d.Name, d.Config.Retention)
		}
	}
}

func TestManager_StartCreatesJetStreamStreams(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	js := jetStreamCtx(t, m)

	for _, name := range []string{"KSCORE_COMMANDS_test", "KSCORE_EVENTS_test"} {
		info, err := js.StreamInfo(name)
		if err != nil {
			t.Errorf("StreamInfo(%q): %v", name, err)
			continue
		}
		if info.Config.Name != name {
			t.Errorf("info.Name = %q, want %q", info.Config.Name, name)
		}
	}
}

func TestManager_JetStreamDisabledSkipsStreamCreation(t *testing.T) {
	cfg := embeddedConfig(t)
	cfg.JetStream.Enabled = false
	cfg.JetStream.StoreDir = "" // not required when disabled

	m := startManager(t, cfg)
	conn := m.activeConnLocked()
	if conn == nil {
		t.Fatal("activeConnLocked = nil")
	}
	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("JetStream context: %v", err)
	}
	// JetStream isn't enabled on the embedded server, so any
	// StreamInfo call surfaces an error. We just want to confirm
	// our streams weren't auto-created.
	_, err = js.StreamInfo("KSCORE_COMMANDS_test")
	if err == nil {
		t.Error("StreamInfo unexpectedly succeeded with JetStream disabled")
	}
}

func TestManager_EnsureStreamsIdempotent(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	conn := m.activeConnLocked()
	if conn == nil {
		t.Fatal("activeConnLocked = nil")
	}
	// Streams already created during Start. A second ensureStreams
	// must succeed via the UpdateStream path.
	if err := m.ensureStreams(context.Background(), conn); err != nil {
		t.Errorf("second ensureStreams: %v", err)
	}
}

func TestManager_EnsureStreamsAppliesConfigUpdates(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	js := jetStreamCtx(t, m)

	// Bump StreamMaxMsgs and re-ensure.
	m.cfg.JetStream.StreamMaxMsgs = 99
	if err := m.ensureStreams(context.Background(), m.activeConnLocked()); err != nil {
		t.Fatalf("ensureStreams update: %v", err)
	}

	info, err := js.StreamInfo("KSCORE_COMMANDS_test")
	if err != nil {
		t.Fatalf("StreamInfo: %v", err)
	}
	if info.Config.MaxMsgs != 99 {
		t.Errorf("MaxMsgs after update = %d, want 99", info.Config.MaxMsgs)
	}
}

func TestEnsureStreams_NilConnFails(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	err := m.ensureStreams(context.Background(), nil)
	if err == nil {
		t.Fatal("ensureStreams(nil conn): expected error")
	}
	if err.Error() == "" {
		t.Errorf("err message empty: %v", err)
	}
}

// jetStreamCtx returns a JetStream context bound to Manager's active
// connection. Tests use this to inspect server-side state.
func jetStreamCtx(t *testing.T, m *Manager) natsclient.JetStreamContext {
	t.Helper()
	conn := m.activeConnLocked()
	if conn == nil {
		t.Fatal("activeConnLocked = nil")
	}
	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("JetStream context: %v", err)
	}
	return js
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
