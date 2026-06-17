// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/dbutil"
)

// TestSQLiteStore_AgentCertMetadata round-trips the cert columns through
// Create -> Get and Update -> Get.
func TestSQLiteStore_AgentCertMetadata(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	a := sampleAgent("cert-1")
	a.CertChainPEM = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	a.CertFingerprint = "abc123"
	a.CertNotAfter = time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	a.SPIFFEID = "spiffe://example.org/agent/cert-1"
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	got, err := s.GetAgent(ctx, "cert-1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.CertChainPEM != a.CertChainPEM || got.CertFingerprint != a.CertFingerprint ||
		got.SPIFFEID != a.SPIFFEID || !got.CertNotAfter.Equal(a.CertNotAfter) {
		t.Fatalf("cert fields lost: chain=%q fp=%q spiffe=%q notAfter=%v",
			got.CertChainPEM, got.CertFingerprint, got.SPIFFEID, got.CertNotAfter)
	}

	// Update path.
	a.CertFingerprint = "def456"
	a.CertNotAfter = time.Date(2028, 6, 7, 8, 9, 10, 0, time.UTC)
	if err := s.UpdateAgent(ctx, a); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	got, err = s.GetAgent(ctx, "cert-1")
	if err != nil {
		t.Fatalf("GetAgent after update: %v", err)
	}
	if got.CertFingerprint != "def456" || !got.CertNotAfter.Equal(a.CertNotAfter) {
		t.Fatalf("cert update lost: fp=%q notAfter=%v", got.CertFingerprint, got.CertNotAfter)
	}
}

// TestSQLiteStore_AgentCertMetadata_EmptyIsNull confirms an agent with no
// cert metadata round-trips as empty/zero (the pre-migration agent case).
func TestSQLiteStore_AgentCertMetadata_EmptyIsNull(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	a := sampleAgent("nocert")
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	got, err := s.GetAgent(ctx, "nocert")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.CertChainPEM != "" || got.CertFingerprint != "" || got.SPIFFEID != "" || !got.CertNotAfter.IsZero() {
		t.Fatalf("expected empty cert fields, got chain=%q fp=%q spiffe=%q notAfter=%v",
			got.CertChainPEM, got.CertFingerprint, got.SPIFFEID, got.CertNotAfter)
	}
}

// TestMigrateAddAgentCertColumns_Upgrade asserts applySchema adds the cert
// columns to a pre-existing agents table that lacks them, idempotently.
func TestMigrateAddAgentCertColumns_Upgrade(t *testing.T) {
	ctx := context.Background()
	db, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Pre-migration agents table — no cert columns.
	if _, err := db.ExecContext(ctx, `CREATE TABLE agents (
		id TEXT PRIMARY KEY, hostname TEXT NOT NULL, os TEXT NOT NULL,
		architecture TEXT NOT NULL, ip_addresses TEXT NOT NULL,
		platform_version TEXT, agent_version TEXT, labels TEXT NOT NULL,
		status TEXT NOT NULL, registered_at TEXT NOT NULL,
		last_heartbeat_at TEXT, metrics TEXT)`); err != nil {
		t.Fatalf("create old table: %v", err)
	}

	if err := applySchema(ctx, db, BackendSQLite); err != nil {
		t.Fatalf("applySchema (upgrade): %v", err)
	}
	cols, err := sqliteColumns(ctx, db, "agents")
	if err != nil {
		t.Fatalf("sqliteColumns: %v", err)
	}
	for _, c := range []string{"cert_chain_pem", "cert_fingerprint", "cert_not_after", "spiffe_id"} {
		if !cols[c] {
			t.Errorf("column %q not added by migration", c)
		}
	}

	// Idempotent: running again is a no-op, not an error.
	if err := applySchema(ctx, db, BackendSQLite); err != nil {
		t.Fatalf("applySchema (second run): %v", err)
	}
}
