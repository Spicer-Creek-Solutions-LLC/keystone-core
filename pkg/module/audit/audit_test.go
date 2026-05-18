package audit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	iaudit "go.keystone-core.io/keystone-core/internal/audit"
	maudit "go.keystone-core.io/keystone-core/pkg/module/audit"
)

type captureSink struct {
	mu      sync.Mutex
	entries []iaudit.AuditEntry
}

func (c *captureSink) Emit(_ context.Context, e iaudit.AuditEntry) {
	c.mu.Lock()
	c.entries = append(c.entries, e)
	c.mu.Unlock()
}

func (c *captureSink) last() iaudit.AuditEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[len(c.entries)-1]
}

func TestStoreBridge_MapsEntry(t *testing.T) {
	cap := &captureSink{}
	b := maudit.NewStoreBridge(cap, nil)

	b.Emit(context.Background(), maudit.Entry{
		Timestamp:  time.Now().UTC(),
		Module:     "acme/widget",
		Version:    "1.2.3",
		Capability: "fs.write",
		Operation:  "write",
		Success:    true,
		Duration:   42 * time.Millisecond,
		Details:    map[string]string{"path": "/etc/x", "module": "SHOULD-NOT-SHADOW"},
	})

	got := cap.last()
	if got.ResourceType != "module" {
		t.Errorf("ResourceType = %q, want module", got.ResourceType)
	}
	if got.Action != "module.fs.write" {
		t.Errorf("Action = %q", got.Action)
	}
	if got.User != "acme/widget@1.2.3" {
		t.Errorf("User = %q", got.User)
	}
	if !got.Allowed || got.Duration != 42*time.Millisecond {
		t.Errorf("Allowed/Duration = %v/%v", got.Allowed, got.Duration)
	}
	if got.Severity != iaudit.SeverityLow {
		t.Errorf("Severity = %v, want Low", got.Severity)
	}
	if got.Metadata["module"] != "acme/widget" || got.Metadata["capability"] != "fs.write" ||
		got.Metadata["operation"] != "write" || got.Metadata["path"] != "/etc/x" {
		t.Errorf("Metadata = %v", got.Metadata)
	}
	if got.ID == "" || got.Timestamp.IsZero() {
		t.Errorf("ID/Timestamp not stamped: %q %v", got.ID, got.Timestamp)
	}
}

func TestStoreBridge_FailureIsMediumAndNotAllowed(t *testing.T) {
	cap := &captureSink{}
	b := maudit.NewStoreBridge(cap, nil)
	b.Emit(context.Background(), maudit.Entry{
		Module: "acme/widget", Version: "1.0.0",
		Capability: "exec", Operation: "denied", Success: false,
	})
	got := cap.last()
	if got.Allowed {
		t.Error("Allowed = true, want false for a failed invocation")
	}
	if got.Severity != iaudit.SeverityMedium {
		t.Errorf("Severity = %v, want Medium", got.Severity)
	}
}

func TestStoreBridge_NilSinkSafe(t *testing.T) {
	b := maudit.NewStoreBridge(nil, nil) // → internal noop
	b.Emit(context.Background(), maudit.Entry{Module: "a/b", Version: "1.0.0", Capability: "kv"})
}

func TestNoopAuditor(t *testing.T) {
	var a maudit.Auditor = maudit.NoopAuditor{}
	a.Emit(context.Background(), maudit.Entry{}) // must not panic
}
