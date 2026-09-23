//go:build contract

package persistence_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	"go.keystone-core.io/keystone-core/internal/ledger"
	"go.keystone-core.io/keystone-core/internal/store"
	_ "modernc.org/sqlite"
)

func TestAC1MigrationOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertVersionPrefix(t, s.DB(), []int{1, 2})
}

func TestAC2PartialMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.db")
	if _, err := store.OpenWithMigrationFailure(path, 2); err == nil {
		t.Fatal("injected migration failure was accepted")
	}
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertVersionPrefix(t, s.DB(), []int{1, 2})
	if _, err := s.DB().Query("SELECT 1 FROM agents"); err != nil {
		t.Fatalf("migration 1 was not committed: %v", err)
	}
}

func TestAC3StoresMigrateIndependently(t *testing.T) {
	server, err := store.Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ledgerDB, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledgerDB.Close()
	assertVersionPrefix(t, server.DB(), []int{1, 2})
	assertVersions(t, ledgerDB.DB(), []int{1})
	if _, err := server.DB().Query("SELECT 1 FROM receipts"); err == nil {
		t.Fatal("server store contains agent-ledger tables")
	}
	if _, err := ledgerDB.DB().Query("SELECT 1 FROM jobs"); err == nil {
		t.Fatal("agent ledger contains server-store tables")
	}
}

func TestAC4ReceiptAndStartAreSeparateTransactions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	l := openLedgerAt(t, path)
	ctx := context.Background()
	if err := l.RecordReceipt(ctx, ledger.Receipt{JobID: "job-4", Metadata: []byte("meta"), State: "Received", ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	independent, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer independent.Close()
	count(t, independent, "receipts", 1)
	count(t, independent, "starts", 0)
	if err := l.RecordStart(ctx, ledger.Start{JobID: "job-4", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	count(t, independent, "starts", 1)
}

func TestAC5CrashBetweenReceiptAndStartIsDistinguishable(t *testing.T) {
	bin := buildCrashHarness(t)
	for _, tc := range []struct {
		name, step       string
		receipts, starts int
	}{
		{"before either", "none", 0, 0}, {"after receipt", "receipt", 1, 0}, {"after start", "start", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ledger.db")
			cmd := exec.Command(bin, "--ledger", path, "--stop-after", tc.step, "--job", "job-5")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("harness: %v\n%s", err, out)
			}
			l, err := ledger.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			count(t, l.DB(), "receipts", tc.receipts)
			count(t, l.DB(), "starts", tc.starts)
		})
	}
}

func TestAC6AcceptedRequestCommitsWithItsIdentifier(t *testing.T) {
	s := openStore(t)
	err := s.RecordAcceptedRequest(context.Background(), store.AcceptedRequest{JobID: "job-6", CorrelationID: "corr-6", Target: "agent-6", Argv: []string{"printf", "ok"}, ExecutionUser: "root", State: "Accepted", AcceptedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var job, argv string
	if err := s.DB().QueryRow("SELECT job_id, argv_json FROM jobs WHERE job_id = ?", "job-6").Scan(&job, &argv); err != nil {
		t.Fatal(err)
	}
	if job != "job-6" || !strings.Contains(argv, "printf") {
		t.Fatalf("accepted request was not stored with its identifier: %q %q", job, argv)
	}
}

func TestAC7TerminalStateCommitsWithItsResult(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	if err := s.RecordAcceptedRequest(ctx, store.AcceptedRequest{JobID: "job-7", Target: "agent-7", Argv: []string{"true"}, ExecutionUser: "root", State: "Accepted", AcceptedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordResult(ctx, store.Result{JobID: "job-7", State: "Completed", ExitStatus: 0, Stdout: []byte("ok"), RecordedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := s.DB().QueryRow("SELECT state FROM jobs WHERE job_id = ?", "job-7").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "Completed" {
		t.Fatalf("job state = %q", state)
	}
	count(t, s.DB(), "results", 1)
}

func TestAC8AcknowledgementIsItsOwnWrite(t *testing.T) {
	l := openLedger(t)
	ctx := context.Background()
	if err := l.RecordReceipt(ctx, ledger.Receipt{JobID: "job-8", Metadata: []byte("m"), State: "Received", ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := l.RecordTerminalResult(ctx, ledger.TerminalResult{JobID: "job-8", State: "Completed", Output: []byte("ok"), RecordedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	count(t, l.DB(), "result_pubacks", 0)
	if err := l.RecordResultPubAck(ctx, "job-8", []byte("ack"), time.Now()); err != nil {
		t.Fatal(err)
	}
	count(t, l.DB(), "result_pubacks", 1)
}

func TestAC9LedgerIsReadableWithoutTheServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	l := openLedgerAt(t, path)
	if err := l.RecordReceipt(context.Background(), ledger.Receipt{JobID: "job-9", Metadata: []byte("m"), State: "Received", ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	l.Close()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var got string
	if err := db.QueryRow("SELECT job_id FROM receipts").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "job-9" {
		t.Fatalf("job id = %q", got)
	}
}

func TestAC10PublishedSchemaMatchesTheShippedOne(t *testing.T) {
	b, err := os.ReadFile("../../../docs/project/LEDGER-SCHEMA.md")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("(?s)```sql\\n(.*?)\\n```")
	m := re.FindStringSubmatch(string(b))
	if len(m) != 2 {
		t.Fatal("published schema SQL block not found")
	}
	l := openLedger(t)
	for _, table := range []string{"schema_migrations", "receipts", "starts", "terminal_results", "result_pubacks"} {
		var actual string
		if err := l.DB().QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&actual); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ToLower(normalizeSQL(m[1])), strings.ToLower(table)) || !strings.Contains(strings.ToLower(normalizeSQL(m[1])), strings.ToLower(normalizeSQL(actual))) && !strings.Contains(strings.ToLower(normalizeSQL(actual)), strings.ToLower(normalizeSQL(m[1]))) {
			t.Fatalf("published schema does not match shipped table %s", table)
		}
	}
}

func TestAC11StoreFileModes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ledger", "ledger.db")
	if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	openLedgerAt(t, path).Close()
	checkMode(t, path, 0o600)
	serverPath := filepath.Join(root, "server.db")
	s, err := store.Open(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	checkMode(t, serverPath, 0o600)
}

func TestAC12RetentionFloor(t *testing.T) {
	l := openLedger(t)
	now := time.Now()
	ctx := context.Background()
	if err := l.RecordReceipt(ctx, ledger.Receipt{JobID: "old", Metadata: []byte("m"), State: "Received", ReceivedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := l.RecordReceipt(ctx, ledger.Receipt{JobID: "new", Metadata: []byte("m"), State: "Received", ReceivedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := l.RetainReceipts(ctx, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	count(t, l.DB(), "receipts", 1)
	var id string
	if err := l.DB().QueryRow("SELECT job_id FROM receipts").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != "new" {
		t.Fatalf("retention removed the wrong record: %s", id)
	}
}

func TestAC13CorruptStoreIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Open(path); !errors.Is(err, ledger.ErrCorrupt) {
		t.Fatalf("corrupt ledger error = %v", err)
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func openLedger(t *testing.T) *ledger.Ledger {
	return openLedgerAt(t, filepath.Join(t.TempDir(), "ledger.db"))
}
func openLedgerAt(t *testing.T, path string) *ledger.Ledger {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}
func assertVersions(t *testing.T, db *sql.DB, want []int) {
	t.Helper()
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if len(got) != len(want) {
		t.Fatalf("migration versions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("migration versions = %v, want %v", got, want)
		}
	}
}

func assertVersionPrefix(t *testing.T, db *sql.DB, want []int) {
	t.Helper()
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if len(got) < len(want) {
		t.Fatalf("migration versions = %v, want prefix %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("migration versions = %v, want prefix %v", got, want)
		}
	}
}
func count(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("%s count = %d, want %d", table, n, want)
	}
}
func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), want)
	}
}
func normalizeSQL(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
func buildCrashHarness(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "keystone-ledger-crash")
	cmd := exec.Command("go", "build", "-tags", "contract", "-o", bin, "../../../cmd/keystone-ledger-crash")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build crash harness: %v\n%s", err, out)
	}
	return bin
}
