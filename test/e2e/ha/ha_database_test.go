// SPDX-License-Identifier: Apache-2.0

//go:build integration

package ha

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq" // postgres driver for the admin kill-connection conn

	"go.keystone-core.io/keystone-core/internal/state"
)

// TestHA_DatabaseFailoverReconnectZeroLoss: the DatabaseFailover
// acceptance line. The clustering stack is etcd-backed; Postgres
// failover is a state.Store-layer concern. Gated on
// KSCORE_TEST_POSTGRES_DSN (skips locally; runs in CI's integration
// job against the sidecar Postgres).
//
// Black-box: write a record, forcibly terminate every server-side
// connection the store holds (simulating a Postgres restart /
// failover), then assert the store transparently reconnects and the
// pre-failure write survived (zero data loss).
func TestHA_DatabaseFailoverReconnectZeroLoss(t *testing.T) {
	dsn := os.Getenv("KSCORE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KSCORE_TEST_POSTGRES_DSN not set; DatabaseFailover needs a live Postgres")
	}

	store, err := state.NewStore(&state.Config{
		Backend:    state.BackendPostgreSQL,
		PostgreSQL: state.PostgreSQLConfig{DSN: dsn},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	const agentID = "ha-dbfailover-agent"
	rec := &state.AgentRecord{
		ID:           agentID,
		Hostname:     "ha-host",
		OS:           "linux",
		Architecture: "amd64",
		Status:       state.AgentStatusConnected,
		RegisteredAt: time.Now().UTC(),
	}
	if err := store.CreateAgent(ctx, rec); err != nil {
		// A leftover row from a prior run is acceptable — the
		// assertion is about survival across the disconnect.
		t.Logf("CreateAgent (pre-existing row tolerated): %v", err)
	}
	if _, err := store.GetAgent(ctx, agentID); err != nil {
		t.Fatalf("baseline GetAgent: %v", err)
	}

	// Simulate the failover: an out-of-band admin connection
	// terminates every other backend on this database, dropping the
	// store's pooled connections underneath it.
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("admin open: %v", err)
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.ExecContext(ctx,
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
         WHERE pid <> pg_backend_pid() AND datname = current_database()`); err != nil {
		t.Fatalf("terminate backends: %v", err)
	}

	// database/sql must transparently re-establish a connection on
	// the next use. Ping may fail once on the killed conn; it must
	// recover well within the budget.
	waitFor(t, settleBudget, "store reconnects after connection loss", func() bool {
		return store.Ping(ctx) == nil
	})

	// Zero data loss: the pre-failure write is still readable.
	got, err := store.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgent after reconnect: %v", err)
	}
	if got.ID != agentID {
		t.Fatalf("recovered agent ID = %q, want %q", got.ID, agentID)
	}
}
