package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func issuedToken(t *testing.T) (*Store, EnrollmentToken) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	tok := EnrollmentToken{TokenID: "t1", AgentID: "a1", AgentName: "web-1", BootstrapPublicKey: "UBOOT",
		IssuedAt: time.Unix(100, 0), ExpiresAt: time.Unix(1000, 0), IssuedByUID: 1}
	if err := s.IssueEnrollment(context.Background(), tok, AuditRecord{Action: "issued", Result: "issued", At: time.Unix(100, 0)}); err != nil {
		t.Fatal(err)
	}
	return s, tok
}

func count(t *testing.T, s *Store, q string) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// S1 records the halves once; the second attempt is refused and writes no
// audit record, so the caller must read and compare the recorded halves.
func TestRecordCredentialOnce(t *testing.T) {
	s, tok := issuedToken(t)
	ctx := context.Background()
	c := Credential{NATSPublicKey: "UAGENT", SigningPublicKey: []byte{1}, EncryptionPublicKey: []byte{2}, PermanentJWT: "jwt", At: time.Unix(200, 0)}
	if err := s.RecordCredential(ctx, tok.TokenID, c, AuditRecord{Action: "cred", Result: "issued", At: c.At}); err != nil {
		t.Fatal(err)
	}
	other := c
	other.NATSPublicKey = "UOTHER"
	if err := s.RecordCredential(ctx, tok.TokenID, other, AuditRecord{Action: "cred", Result: "issued", At: c.At}); !errors.Is(err, ErrAlreadyRecorded) {
		t.Fatalf("second record: %v", err)
	}
	r, err := s.Enrollment(ctx, tok.TokenID)
	if err != nil {
		t.Fatal(err)
	}
	if r.NATSPublicKey != "UAGENT" || r.PermanentJWT != "jwt" || r.State != TokenIssued {
		t.Fatalf("record %+v", r)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM audit WHERE action = 'cred'`); n != 1 {
		t.Fatalf("%d credential records", n)
	}
}

// S4 and S5 are one write: the agent active and the token spent together,
// once, and not before S1.
func TestActivateIsOneWriteOnce(t *testing.T) {
	s, tok := issuedToken(t)
	ctx := context.Background()
	if err := s.Activate(ctx, tok.TokenID, time.Unix(300, 0), AuditRecord{Action: "act", Result: "active", At: time.Unix(300, 0)}); !errors.Is(err, ErrNotActivatable) {
		t.Fatalf("activation before S1: %v", err)
	}
	c := Credential{NATSPublicKey: "UAGENT", SigningPublicKey: []byte{1}, EncryptionPublicKey: []byte{2}, PermanentJWT: "jwt", At: time.Unix(200, 0)}
	if err := s.RecordCredential(ctx, tok.TokenID, c, AuditRecord{Action: "cred", Result: "issued", At: c.At}); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, tok.TokenID, time.Unix(300, 0), AuditRecord{Action: "act", Result: "active", At: time.Unix(300, 0)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, tok.TokenID, time.Unix(400, 0), AuditRecord{Action: "act", Result: "active", At: time.Unix(400, 0)}); !errors.Is(err, ErrNotActivatable) {
		t.Fatalf("second activation: %v", err)
	}
	r, _ := s.EnrollmentByAgent(ctx, "a1")
	if r.State != TokenSpent || r.SpentAt.Unix() != 300 {
		t.Fatalf("token %+v", r)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM agents WHERE agent_id = 'a1' AND state = 'active' AND nats_public_key = 'UAGENT' AND agent_name = 'web-1'`); n != 1 {
		t.Fatalf("%d active agent rows", n)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM audit WHERE action = 'act'`); n != 1 {
		t.Fatalf("%d activation records", n)
	}
}
