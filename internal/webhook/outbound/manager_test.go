package outbound

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// mockSubscriber implements events.EventSubscriber for testing.
type mockSubscriber struct {
	handler events.EventHandler
}

func (m *mockSubscriber) Subscribe(subject string, handler events.EventHandler) (*events.Subscription, error) {
	m.handler = handler
	return &events.Subscription{ID: "test-sub", Subject: subject, Active: true}, nil
}

func (m *mockSubscriber) SubscribeQueue(subject, queue string, handler events.EventHandler) (*events.Subscription, error) {
	m.handler = handler
	return &events.Subscription{ID: "test-sub", Subject: subject, Queue: queue, Active: true}, nil
}

func (m *mockSubscriber) Close() error { return nil }

// emit simulates an event arriving on the subscriber.
func (m *mockSubscriber) emit(event *events.Event) {
	if m.handler != nil {
		m.handler(event)
	}
}

func TestManager_StartStop(t *testing.T) {
	store := newTestStore(t)
	sub := &mockSubscriber{}
	d := NewDispatcher(5 * time.Second)

	mgr := NewManager(store, d, sub, ManagerConfig{MaxRetries: 1, RetryBackoff: 10 * time.Millisecond})
	require.NoError(t, mgr.Start(context.Background()))
	require.NoError(t, mgr.Stop())
}

func TestManager_DeliverMatchingEvent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	var deliveryCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveryCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	require.NoError(t, store.Create(ctx, &Subscription{
		ID:         "sub-match",
		Name:       "test",
		URL:        srv.URL,
		Events:     []string{"agent.*"},
		Enabled:    true,
		Headers:    map[string]string{},
		MaxRetries: 0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}))

	sub := &mockSubscriber{}
	d := NewDispatcher(5 * time.Second)
	mgr := NewManager(store, d, sub, ManagerConfig{MaxRetries: 0, RetryBackoff: time.Millisecond})
	require.NoError(t, mgr.Start(ctx))

	sub.emit(&events.Event{
		ID:   "evt-1",
		Type: events.EventTypeAgentConnect,
		Time: time.Now(),
	})

	// Wait for async delivery
	require.Eventually(t, func() bool {
		return deliveryCount.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, mgr.Stop())

	records, err := store.ListDeliveries(ctx, "sub-match", 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(records), 1)
	assert.Equal(t, DeliverySuccess, records[0].Status)
}

func TestManager_NoMatchSkipsDelivery(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	var deliveryCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveryCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	require.NoError(t, store.Create(ctx, &Subscription{
		ID:      "sub-nomatch",
		Name:    "test",
		URL:     srv.URL,
		Events:  []string{"policy.*"},
		Enabled: true, Headers: map[string]string{},
		CreatedAt: now, UpdatedAt: now,
	}))

	sub := &mockSubscriber{}
	d := NewDispatcher(5 * time.Second)
	mgr := NewManager(store, d, sub, ManagerConfig{MaxRetries: 0})
	require.NoError(t, mgr.Start(ctx))

	sub.emit(&events.Event{
		ID:   "evt-2",
		Type: events.EventTypeAgentConnect,
		Time: time.Now(),
	})

	// Give time for any potential delivery
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), deliveryCount.Load())

	require.NoError(t, mgr.Stop())
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		eventType string
		filters   []string
		want      bool
	}{
		{"agent.connect", []string{"agent.*"}, true},
		{"agent.disconnect", []string{"agent.*"}, true},
		{"state.drift", []string{"agent.*"}, false},
		{"state.drift", []string{"state.*", "agent.*"}, true},
		{"state.drift", []string{"state.drift"}, true},
		{"agent.connect", []string{"*"}, true},
		{"agent.connect", []string{}, false},
		{"job.complete", []string{"job.*"}, true},
	}
	for _, tc := range tests {
		got := matchesFilter(tc.eventType, tc.filters)
		assert.Equal(t, tc.want, got, "matchesFilter(%q, %v)", tc.eventType, tc.filters)
	}
}
