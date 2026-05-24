// SPDX-License-Identifier: Apache-2.0

package state

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// TxLogEntry is one append-only audit-trail record.
type TxLogEntry struct {
	Time   time.Time `json:"ts"`
	Table  string    `json:"table"`
	Op     string    `json:"op"`               // "insert" | "checkpoint"
	ID     string    `json:"id,omitempty"`     // record id for inserts
	Status string    `json:"status,omitempty"` // "ok" | "skipped" | "error" | "dryrun"
	Error  string    `json:"error,omitempty"`  // populated when Status="error"
	LastID string    `json:"last_id,omitempty"` // populated when Op="checkpoint"
}

// TransactionLog is an append-only JSONL audit trail of a migration
// run. Each Append writes one line and flushes; Close syncs the file.
//
// The file format also records per-table checkpoints. v1.0 only
// writes them; reading checkpoints to resume a partial run is a future
// enhancement.
//
// Single-goroutine usage from Migrate. Not safe for concurrent use.
type TransactionLog struct {
	f  *os.File
	bw *bufio.Writer
}

// OpenTxLog opens (or creates) a JSONL file at path for append. The
// file is created with mode 0o600 — operational details (record IDs,
// error messages) shouldn't be world-readable.
func OpenTxLog(path string) (*TransactionLog, error) {
	// #nosec G304 -- path is operator-supplied via --txlog flag; this
	// is a deliberate file write, gated by the user.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open txlog %q: %w", path, err)
	}
	return &TransactionLog{f: f, bw: bufio.NewWriter(f)}, nil
}

// Append writes one entry as a JSONL line and flushes the buffer so a
// crash doesn't lose the most recent operations.
func (t *TransactionLog) Append(e TxLogEntry) error {
	if t == nil {
		return nil
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal txlog entry: %w", err)
	}
	if _, err := t.bw.Write(b); err != nil {
		return fmt.Errorf("write txlog: %w", err)
	}
	if err := t.bw.WriteByte('\n'); err != nil {
		return fmt.Errorf("write txlog: %w", err)
	}
	return t.bw.Flush()
}

// Checkpoint writes a checkpoint entry marking that all rows of `table`
// up to (and including) `lastID` have been processed. v1.0 records the
// marker but does not consume it for resume.
func (t *TransactionLog) Checkpoint(table, lastID string) error {
	return t.Append(TxLogEntry{
		Table:  table,
		Op:     "checkpoint",
		LastID: lastID,
	})
}

// Close flushes and closes the underlying file. Safe on nil receiver.
func (t *TransactionLog) Close() error {
	if t == nil || t.f == nil {
		return nil
	}
	if err := t.bw.Flush(); err != nil {
		_ = t.f.Close()
		return err
	}
	return t.f.Close()
}
