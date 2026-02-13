package outbound

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	store, err := NewSQLiteStore(db)
	require.NoError(t, err)
	return store
}

func TestNewSQLiteStore_NilDB(t *testing.T) {
	_, err := NewSQLiteStore(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection required")
}

func TestSQLiteStore_CRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	sub := &Subscription{
		ID:         "sub-1",
		Name:       "Test Hook",
		URL:        "https://example.com/hook",
		Secret:     "s3cret",
		Events:     []string{"agent.*", "state.drift"},
		Enabled:    true,
		Headers:    map[string]string{"X-Custom": "val"},
		MaxRetries: 5,
		TimeoutSec: 15,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Create
	require.NoError(t, store.Create(ctx, sub))

	// Get
	got, err := store.Get(ctx, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, sub.Name, got.Name)
	assert.Equal(t, sub.URL, got.URL)
	assert.Equal(t, sub.Secret, got.Secret)
	assert.Equal(t, sub.Events, got.Events)
	assert.True(t, got.Enabled)
	assert.Equal(t, "val", got.Headers["X-Custom"])
	assert.Equal(t, 5, got.MaxRetries)
	assert.Equal(t, 15, got.TimeoutSec)

	// Get not found
	_, err = store.Get(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// List
	subs, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	// Update
	sub.Name = "Updated Hook"
	sub.UpdatedAt = now.Add(time.Minute)
	require.NoError(t, store.Update(ctx, sub))
	got, err = store.Get(ctx, "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "Updated Hook", got.Name)

	// Update not found
	notFound := &Subscription{ID: "nope", Events: []string{}, Headers: map[string]string{}, UpdatedAt: now}
	assert.Error(t, store.Update(ctx, notFound))

	// Delete
	require.NoError(t, store.Delete(ctx, "sub-1"))
	_, err = store.Get(ctx, "sub-1")
	assert.Error(t, err)

	// Delete not found
	assert.Error(t, store.Delete(ctx, "sub-1"))
}

func TestSQLiteStore_ListEnabled(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	for _, tc := range []struct {
		id      string
		enabled bool
	}{
		{"e1", true},
		{"e2", false},
		{"e3", true},
	} {
		require.NoError(t, store.Create(ctx, &Subscription{
			ID:        tc.id,
			Name:      tc.id,
			URL:       "https://example.com/" + tc.id,
			Events:    []string{"*"},
			Enabled:   tc.enabled,
			Headers:   map[string]string{},
			CreatedAt: now,
			UpdatedAt: now,
		}))
	}

	enabled, err := store.ListEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, enabled, 2)
}

func TestSQLiteStore_Deliveries(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Record deliveries
	for i := 0; i < 5; i++ {
		require.NoError(t, store.RecordDelivery(ctx, &DeliveryRecord{
			ID:             fmt.Sprintf("del-%d", i),
			SubscriptionID: "sub-1",
			EventType:      "agent.connect",
			EventID:        fmt.Sprintf("evt-%d", i),
			Status:         DeliverySuccess,
			StatusCode:     200,
			Attempt:        1,
			DeliveredAt:    now.Add(time.Duration(i) * time.Minute),
		}))
	}

	// List with limit
	records, err := store.ListDeliveries(ctx, "sub-1", 3)
	require.NoError(t, err)
	assert.Len(t, records, 3)
	// Should be ordered by delivered_at DESC
	assert.Equal(t, "del-4", records[0].ID)

	// Default limit
	records, err = store.ListDeliveries(ctx, "sub-1", 0)
	require.NoError(t, err)
	assert.Len(t, records, 5)

	// Empty result for unknown subscription
	records, err = store.ListDeliveries(ctx, "unknown", 10)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestSQLiteStore_DeleteOldDeliveries(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	require.NoError(t, store.RecordDelivery(ctx, &DeliveryRecord{
		ID: "old-1", SubscriptionID: "s1", EventType: "x", EventID: "e1",
		Status: DeliverySuccess, DeliveredAt: old,
	}))
	require.NoError(t, store.RecordDelivery(ctx, &DeliveryRecord{
		ID: "recent-1", SubscriptionID: "s1", EventType: "x", EventID: "e2",
		Status: DeliverySuccess, DeliveredAt: recent,
	}))

	deleted, err := store.DeleteOldDeliveries(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	records, err := store.ListDeliveries(ctx, "s1", 10)
	require.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, "recent-1", records[0].ID)
}

func TestSQLiteStore_DeleteCascadesDeliveries(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, store.Create(ctx, &Subscription{
		ID: "sub-del", Name: "test", URL: "https://example.com",
		Events: []string{"*"}, Enabled: true, Headers: map[string]string{},
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, store.RecordDelivery(ctx, &DeliveryRecord{
		ID: "d1", SubscriptionID: "sub-del", EventType: "x", EventID: "e1",
		Status: DeliverySuccess, DeliveredAt: now,
	}))

	require.NoError(t, store.Delete(ctx, "sub-del"))

	records, err := store.ListDeliveries(ctx, "sub-del", 10)
	require.NoError(t, err)
	assert.Empty(t, records)
}
