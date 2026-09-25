package enrollment

import (
	"context"
	"strings"
	"testing"
	"time"
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
