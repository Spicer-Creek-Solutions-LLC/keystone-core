package token

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type mockStore struct {
	deleteExpiredCalls atomic.Int32
}

func (m *mockStore) Create(_ context.Context, _ *JoinToken) error              { return nil }
func (m *mockStore) GetByID(_ context.Context, _ string) (*JoinToken, error)   { return nil, nil }
func (m *mockStore) Lookup(_ context.Context, _ string) (*JoinToken, error)    { return nil, nil }
func (m *mockStore) List(_ context.Context) ([]*JoinToken, error)              { return nil, nil }
func (m *mockStore) Revoke(_ context.Context, _ string) error                  { return nil }
func (m *mockStore) IncrementUses(_ context.Context, _ string) error           { return nil }
func (m *mockStore) DeleteExpired(_ context.Context) (int, error) {
	m.deleteExpiredCalls.Add(1)
	return 0, nil
}

var _ Store = (*mockStore)(nil)

func TestCleaner_StartStop(t *testing.T) {
	store := &mockStore{}
	c := NewCleaner(store, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.Start(ctx)

	// Wait for at least one cleanup cycle.
	time.Sleep(50 * time.Millisecond)

	c.Stop()

	calls := store.deleteExpiredCalls.Load()
	if calls == 0 {
		t.Error("expected DeleteExpired to be called at least once")
	}
}

func TestCleaner_ContextCancel(t *testing.T) {
	store := &mockStore{}
	c := NewCleaner(store, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	go c.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("cleaner did not stop after context cancellation")
	}
}

func TestCleaner_DefaultInterval(t *testing.T) {
	c := NewCleaner(&mockStore{}, 0)
	if c.interval != DefaultCleanupInterval {
		t.Errorf("interval = %v, want %v", c.interval, DefaultCleanupInterval)
	}
}

func TestCleaner_NegativeInterval(t *testing.T) {
	c := NewCleaner(&mockStore{}, -5*time.Second)
	if c.interval != DefaultCleanupInterval {
		t.Errorf("interval = %v, want %v", c.interval, DefaultCleanupInterval)
	}
}

func TestCleaner_Done(t *testing.T) {
	c := NewCleaner(&mockStore{}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	go c.Start(ctx)

	cancel()

	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("Done channel not closed after stop")
	}
}
