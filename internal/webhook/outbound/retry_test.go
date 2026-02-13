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

func TestRetryDelivery_Success(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(5 * time.Second)
	sub := &Subscription{
		ID:  "sub-retry",
		URL: srv.URL,
	}
	event := &events.Event{ID: "evt-retry", Type: "test.retry"}

	retryDelivery(ctx, store, d, sub, []byte(`{}`), event, 3, 10*time.Millisecond)

	// Should have had 3 retry attempts (attempt 2, 3, 4 but 4 > maxRetries+1=4, so 2 and 3)
	// Actually: attempt range is 2..maxRetries+1 = 2..4, that's attempts 2, 3, 4
	// Attempt 2: fail (callCount=1 is first retry call, but we start at n<3)
	// Wait, the test httptest server callCount starts at 0 from retryDelivery perspective
	// First call in retryDelivery: callCount becomes 1 → 500
	// Second call: callCount becomes 2 → 500
	// Third call: callCount becomes 3 → 200
	assert.GreaterOrEqual(t, callCount.Load(), int32(3))

	records, err := store.ListDeliveries(ctx, "sub-retry", 10)
	require.NoError(t, err)
	// Should have at least one success record
	hasSuccess := false
	for _, r := range records {
		if r.Status == DeliverySuccess {
			hasSuccess = true
		}
	}
	assert.True(t, hasSuccess, "should have a successful delivery record")
}

func TestRetryDelivery_AllFail(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewDispatcher(5 * time.Second)
	sub := &Subscription{ID: "sub-allfail", URL: srv.URL}
	event := &events.Event{ID: "evt-allfail", Type: "test.fail"}

	retryDelivery(ctx, store, d, sub, []byte(`{}`), event, 2, 10*time.Millisecond)

	records, err := store.ListDeliveries(ctx, "sub-allfail", 10)
	require.NoError(t, err)
	assert.NotEmpty(t, records)

	// Last record should be "failed"
	assert.Equal(t, DeliveryFailed, records[0].Status)
}

func TestRetryDelivery_ContextCanceled(t *testing.T) {
	store := newTestStore(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	d := NewDispatcher(5 * time.Second)
	sub := &Subscription{ID: "sub-cancel", URL: srv.URL}
	event := &events.Event{ID: "evt-cancel", Type: "test.cancel"}

	retryDelivery(ctx, store, d, sub, []byte(`{}`), event, 5, time.Second)

	// Should exit quickly without making any retry attempts.
	// Use a fresh context for the query since the original was canceled.
	records, err := store.ListDeliveries(context.Background(), "sub-cancel", 10)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestJitter(t *testing.T) {
	d := time.Second
	j := jitter(d)
	assert.GreaterOrEqual(t, j, time.Duration(0))
	assert.Less(t, j, d/10)
}
