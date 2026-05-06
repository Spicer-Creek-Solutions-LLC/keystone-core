package state

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrationOptions_BatchSizeDefault(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"zero gets 100", 0, 100},
		{"negative gets 100", -1, 100},
		{"explicit preserved", 250, 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := MigrationOptions{BatchSize: tt.in}
			o.applyDefaults()
			if o.BatchSize != tt.want {
				t.Errorf("BatchSize = %d, want %d", o.BatchSize, tt.want)
			}
		})
	}
}

func TestTransactionLog_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "txlog.jsonl")

	tl, err := OpenTxLog(path)
	if err != nil {
		t.Fatalf("OpenTxLog: %v", err)
	}

	entries := []TxLogEntry{
		{Table: "agents", Op: "insert", ID: "a-1", Status: "ok"},
		{Table: "agents", Op: "insert", ID: "a-2", Status: "skipped"},
		{Table: "agents", Op: "insert", ID: "a-3", Status: "error", Error: "boom"},
	}
	for _, e := range entries {
		if err := tl.Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := tl.Checkpoint("agents", "a-3"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if err := tl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-read by parsing JSONL directly.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var got []TxLogEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e TxLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("decode line %q: %v", scanner.Text(), err)
		}
		got = append(got, e)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("got %d lines, want 4", len(got))
	}
	if got[0].ID != "a-1" || got[0].Status != "ok" {
		t.Errorf("first entry: %+v", got[0])
	}
	if got[2].Error != "boom" {
		t.Errorf("error entry lost message: %+v", got[2])
	}
	if got[3].Op != "checkpoint" || got[3].LastID != "a-3" {
		t.Errorf("checkpoint entry: %+v", got[3])
	}
	for i, e := range got {
		if e.Time.IsZero() {
			t.Errorf("entry %d has zero Time", i)
		}
	}
}

func TestTransactionLog_NilReceiverSafe(t *testing.T) {
	var tl *TransactionLog
	if err := tl.Append(TxLogEntry{Table: "x", Op: "insert"}); err != nil {
		t.Errorf("Append on nil: %v", err)
	}
	if err := tl.Close(); err != nil {
		t.Errorf("Close on nil: %v", err)
	}
}

func TestTransactionLog_OpenError(t *testing.T) {
	// /dev/null/foo is unwritable; OpenTxLog should surface the error.
	_, err := OpenTxLog("/dev/null/cannot-create.jsonl")
	if err == nil {
		t.Fatal("expected error opening unwritable path")
	}
}

func TestComputeRateAndETA(t *testing.T) {
	tests := []struct {
		name      string
		startAgo  time.Duration
		completed int
		total     int
		wantRate  float64 // approximate; we check tolerance
		wantETA   time.Duration
		etaTol    time.Duration
	}{
		{
			name:      "halfway through; rate is completed/elapsed",
			startAgo:  10 * time.Second,
			completed: 50,
			total:     100,
			wantRate:  5,
			wantETA:   10 * time.Second,
			etaTol:    500 * time.Millisecond,
		},
		{
			name:      "no completion yet; zero rate, zero ETA",
			startAgo:  5 * time.Second,
			completed: 0,
			total:     100,
			wantRate:  0,
			wantETA:   0,
		},
		{
			name:      "all done; rate non-zero, ETA zero",
			startAgo:  10 * time.Second,
			completed: 100,
			total:     100,
			wantRate:  10,
			wantETA:   0,
		},
		{
			name:      "unknown total -> zero ETA",
			startAgo:  5 * time.Second,
			completed: 50,
			total:     0,
			wantRate:  10,
			wantETA:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now().Add(-tt.startAgo)
			rate, eta := computeRateAndETA(start, tt.completed, tt.total)
			if math.Abs(rate-tt.wantRate) > 0.5 {
				t.Errorf("rate = %.2f, want approx %.2f", rate, tt.wantRate)
			}
			if abs(eta-tt.wantETA) > tt.etaTol+200*time.Millisecond {
				t.Errorf("eta = %s, want approx %s (tol %s)", eta, tt.wantETA, tt.etaTol)
			}
		})
	}
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func TestProgressReporter_NoCallbackIsNoOp(t *testing.T) {
	r := newProgressReporter(nil)
	r.Start("agents", 100)
	r.Update("agents", 10) // must not panic
}

func TestProgressReporter_EmitsForEachUpdate(t *testing.T) {
	var got []ProgressUpdate
	r := newProgressReporter(func(p ProgressUpdate) { got = append(got, p) })
	r.Start("agents", 100)

	r.Update("agents", 25)
	r.Update("agents", 50)
	r.Update("agents", 100)

	if len(got) != 3 {
		t.Fatalf("got %d updates, want 3", len(got))
	}
	if got[0].Table != "agents" || got[0].RowsCompleted != 25 || got[0].RowsTotal != 100 {
		t.Errorf("first update: %+v", got[0])
	}
	if got[2].RowsCompleted != 100 {
		t.Errorf("last update: %+v", got[2])
	}
}

func TestNewMigrator(t *testing.T) {
	src := newSQLiteStoreForTest(t)
	// Construct a Migrator with a nil dst; we never call Migrate so the
	// nil dst is fine — we're testing constructor wiring only.
	m := NewMigrator(src, nil)
	if m.src != src {
		t.Errorf("Migrator did not retain src")
	}
}
