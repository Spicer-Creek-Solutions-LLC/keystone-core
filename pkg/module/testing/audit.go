// SPDX-License-Identifier: Apache-2.0

package moduletest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"go.keystone-core.io/keystone-core/pkg/module/audit"
)

// AuditOptions is the runner's view of the kscore-module
// --audit-level / --audit-output flags.
//
//	Level:  "" | "all"      -> every capability invocation
//	        "failure"|"deny" -> only failed/denied invocations
//	Output: ""               -> discarded
//	        "stdout"|"stderr"
//	        <path>           -> appended as JSON lines
type AuditOptions struct {
	Level  string
	Output string
}

// jsonAuditor writes capability entries as JSON lines. It honours
// the Auditor contract: it never blocks on, nor errors back to,
// the caller (a log write failure is dropped — failure-to-log is a
// bug, not a test failure).
type jsonAuditor struct {
	mu       sync.Mutex
	w        io.Writer
	failOnly bool
}

func (a *jsonAuditor) Emit(_ context.Context, e audit.Entry) {
	if a.failOnly && e.Success {
		return
	}
	rec := struct {
		Timestamp  string            `json:"timestamp"`
		Module     string            `json:"module"`
		Version    string            `json:"version"`
		Capability string            `json:"capability"`
		Operation  string            `json:"operation"`
		Success    bool              `json:"success"`
		DurationMS int64             `json:"duration_ms"`
		Details    map[string]string `json:"details,omitempty"`
	}{
		Timestamp:  e.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		Module:     e.Module,
		Version:    e.Version,
		Capability: e.Capability,
		Operation:  e.Operation,
		Success:    e.Success,
		DurationMS: e.Duration.Milliseconds(),
		Details:    e.Details,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = a.w.Write(append(b, '\n'))
}

// newAuditor builds the capability auditor for a run plus a closer
// (a no-op unless an output file was opened).
func newAuditor(o AuditOptions) (audit.Auditor, func() error, error) {
	var failOnly bool
	switch o.Level {
	case "", "all":
	case "failure", "deny":
		failOnly = true
	default:
		return nil, nil, fmt.Errorf("%w: level %q (want all|failure)", ErrAuditOption, o.Level)
	}

	noClose := func() error { return nil }
	switch o.Output {
	case "":
		return audit.NoopAuditor{}, noClose, nil
	case "stdout":
		return &jsonAuditor{w: os.Stdout, failOnly: failOnly}, noClose, nil
	case "stderr":
		return &jsonAuditor{w: os.Stderr, failOnly: failOnly}, noClose, nil
	default:
		f, err := os.OpenFile(o.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // G304: operator-supplied --audit-output path
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrAuditOption, err)
		}
		return &jsonAuditor{w: f, failOnly: failOnly}, f.Close, nil
	}
}
