package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/protocol"
)

// A denial is sent only once its audit record is durable. With the audit table
// unwritable, the same refused request is recorded nowhere and answered not at
// all; with it writable, it is recorded and answered with a bare denial.
func TestDenialIsAnsweredOnlyOnceRecorded(t *testing.T) {
	f := newFixture(t)
	srv, st := f.open(t)
	svc := &Service{srv: srv, store: st, minter: srv.Issuer.minter, now: time.Now}
	unknown := strings.Repeat("0", 64)

	reply := svc.respond(context.Background(), unknown, []byte("not an envelope"))
	if reply == nil || reply.Kind != KindDenied {
		t.Fatalf("control: a refused request with a writable audit was answered %+v", reply)
	}
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM audit WHERE action = ?`, ActionDenied).Scan(&n); err != nil || n != 1 {
		t.Fatalf("control: %d denial records (%v)", n, err)
	}

	if _, err := st.DB().Exec(`CREATE TRIGGER no_audit BEFORE INSERT ON audit BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if reply := svc.respond(context.Background(), unknown, []byte("not an envelope")); reply != nil {
		t.Fatalf("a denial whose record failed was answered: %+v", reply)
	}
}

// A hold whose journal cannot be written is a local fault, not a refused
// token; only running out of time at a hold is a refusal.
func TestAHoldsJournalFailureIsNotARefusal(t *testing.T) {
	cfg := config.AgentFaults{RecordPath: filepath.Join(t.TempDir(), "missing", "faults.jsonl"), HoldBeforeRequest: 10 * time.Millisecond}
	a := &Agent{faults: faults{cfg}}
	err := a.hold(context.Background(), "enrollment_agent_hold_before_request_ms")
	if err == nil || errors.Is(err, ErrRefused) {
		t.Fatalf("an unwritable journal: %v, want a local error", err)
	}
	cfg.RecordPath = filepath.Join(t.TempDir(), "faults.jsonl")
	a = &Agent{faults: faults{cfg}}
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := a.hold(expired, "enrollment_agent_hold_before_request_ms"); !errors.Is(err, ErrRefused) {
		t.Fatalf("expiry at a hold: %v, want ErrRefused", err)
	}
}

// If the after-activation hold's reached record cannot be written, S6 is not
// answered until it is: the gate stays held, and the hold then runs.
func TestUnrecordedActivationHoldFailsClosed(t *testing.T) {
	f := newFixture(t)
	srv, st := f.open(t)
	svc := &Service{srv: srv, store: st, minter: srv.Issuer.minter, now: time.Now, holdAfterActivation: 200 * time.Millisecond}
	ctx := context.Background()
	b, err := srv.Issuer.Issue(ctx, Actor{UID: 1}, "web-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := GenerateAgentKeys()
	if err != nil {
		t.Fatal(err)
	}
	request := func(kind string) []byte {
		payload, _ := json.Marshal(Request{
			NATSPublicKey: keys.NATSPublicKey(), Kind: kind,
			SigningPublicKey:    EncodeKey(keys.Signing.Verifying().Bytes()),
			EncryptionPublicKey: EncodeKey(keys.Decryption.Recipient().Bytes()),
		})
		env, err := Seal(keys.Signing, protocol.ClassEnrollmentRequest, b.AgentID, b.TokenID, payload, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return env
	}
	if r := svc.respond(ctx, b.TokenID, request(KindCredential)); r == nil || r.Kind != KindCredential {
		t.Fatalf("S1: %+v", r)
	}
	if _, err := st.DB().Exec(`CREATE TRIGGER no_reached BEFORE INSERT ON audit WHEN NEW.action LIKE 'fault.reached:%'
		BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	proof, err := Seal(keys.Signing, protocol.ClassPresence, b.AgentID, "", []byte("{}"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	go svc.onPresence(&nats.Msg{Subject: PresenceSubject(b.AgentID), Data: proof})
	for i := 0; ; i++ {
		if rec, _ := st.Enrollment(ctx, b.TokenID); rec.State == "spent" {
			break
		}
		if i > 100 {
			t.Fatal("control: activation never committed")
		}
		time.Sleep(20 * time.Millisecond)
	}

	answered := make(chan *Reply, 1)
	go func() { answered <- svc.respond(ctx, b.TokenID, request(KindConfirmation)) }()
	select {
	case r := <-answered:
		t.Fatalf("S6 was answered with the hold's reached record unwritten: %+v", r)
	case <-time.After(time.Second):
	}
	if _, err := st.DB().Exec(`DROP TRIGGER no_reached`); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-answered:
		if r == nil || r.Kind != KindConfirmation || r.Identity != StateActive {
			t.Fatalf("after the record was written: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("S6 was never answered once the record could be written")
	}
	var n int
	st.DB().QueryRow(`SELECT COUNT(*) FROM audit WHERE action = ?`, ActionFaultReached+FaultHoldAfterActivation).Scan(&n)
	if n != 1 {
		t.Fatalf("%d reached records", n)
	}
}
