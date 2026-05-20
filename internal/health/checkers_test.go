package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

type natsPinger struct{ err error }

func (n natsPinger) Health(context.Context) error { return n.err }

type dbPinger struct{ err error }

func (d dbPinger) Ping(context.Context) error { return d.err }

type jsPinger struct{ err error }

func (j jsPinger) Check(context.Context) error { return j.err }

func TestPingChecker_NameAndInterval(t *testing.T) {
	c := NewPingChecker("x", nil, 5*time.Second)
	if c.Name() != "x" {
		t.Errorf("Name = %q", c.Name())
	}
	if c.Interval() != 5*time.Second {
		t.Errorf("Interval = %s", c.Interval())
	}
}

func TestPingChecker_NilFn_AlwaysHealthy(t *testing.T) {
	c := NewPingChecker("noop", nil, 0)
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}
}

func TestPingChecker_NilReceiver_Safe(t *testing.T) {
	var c *PingChecker
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("nil *PingChecker.Check = %v, want nil", err)
	}
}

func TestNewNATSChecker_DelegatesToPinger(t *testing.T) {
	c := NewNATSChecker(natsPinger{}, time.Second)
	if c.Name() != "nats" {
		t.Errorf("Name = %q, want nats", c.Name())
	}
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}

	wantErr := errors.New("disconnected")
	c2 := NewNATSChecker(natsPinger{err: wantErr}, time.Second)
	if err := c2.Check(context.Background()); !errors.Is(err, wantErr) {
		t.Errorf("Check = %v, want %v", err, wantErr)
	}
}

func TestNewDBChecker_DelegatesToPinger(t *testing.T) {
	c := NewDBChecker(dbPinger{}, time.Second)
	if c.Name() != "db" {
		t.Errorf("Name = %q", c.Name())
	}
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}
}

func TestNewJetStreamChecker_DelegatesToPinger(t *testing.T) {
	c := NewJetStreamChecker(jsPinger{}, time.Second)
	if c.Name() != "jetstream" {
		t.Errorf("Name = %q", c.Name())
	}
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}
	wantErr := errors.New("no jetstream")
	c2 := NewJetStreamChecker(jsPinger{err: wantErr}, time.Second)
	if err := c2.Check(context.Background()); !errors.Is(err, wantErr) {
		t.Errorf("Check = %v, want %v", err, wantErr)
	}
}

func TestJetStreamPingerFunc_Adapts(t *testing.T) {
	called := false
	p := JetStreamPingerFunc(func(context.Context) error { called = true; return nil })
	if err := p.Check(context.Background()); err != nil {
		t.Fatalf("Check = %v", err)
	}
	if !called {
		t.Fatal("func not invoked")
	}
}

func TestNewCustomChecker(t *testing.T) {
	called := false
	c := NewCustomChecker("my-thing", func(context.Context) error {
		called = true
		return nil
	}, 10*time.Second)
	if c.Name() != "my-thing" {
		t.Errorf("Name = %q", c.Name())
	}
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check = %v", err)
	}
	if !called {
		t.Error("custom fn not invoked")
	}
}

func TestNoBackend_Constructors(t *testing.T) {
	tests := []struct {
		name string
		c    Checker
	}{
		{"nats", NewNATSChecker(nil, 0)},
		{"db", NewDBChecker(nil, 0)},
		{"jetstream", NewJetStreamChecker(nil, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.c.Name() != tt.name {
				t.Errorf("Name = %q", tt.c.Name())
			}
			err := tt.c.Check(context.Background())
			if err == nil {
				t.Fatalf("Check = nil, want no-backend error")
			}
			if !errors.Is(err, err) || !contains(err.Error(), "no backend configured") {
				t.Errorf("err = %v, want no-backend message", err)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
