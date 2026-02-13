package outbound

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAPI(t *testing.T) (*APIHandler, *SQLiteStore) {
	t.Helper()
	store := newTestStore(t)
	return NewAPIHandler(store), store
}

func TestAPI_CreateSubscription(t *testing.T) {
	h, _ := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"name":"test","url":"https://example.com/hook","events":["agent.*"],"secret":"s3c"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/subscriptions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "test", resp["name"])
	assert.Equal(t, "***", resp["secret"])
	assert.NotEmpty(t, resp["id"])
}

func TestAPI_CreateSubscription_Validation(t *testing.T) {
	h, _ := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	tests := []struct {
		name string
		body string
		want string
	}{
		{"missing name", `{"url":"https://x.com","events":["*"]}`, "name is required"},
		{"missing url", `{"name":"test","events":["*"]}`, "url is required"},
		{"invalid url", `{"name":"test","url":"not-a-url","events":["*"]}`, "invalid url"},
		{"missing events", `{"name":"test","url":"https://x.com"}`, "events is required"},
		{"invalid json", `{bad`, "invalid JSON"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/subscriptions", bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tc.want)
		})
	}
}

func TestAPI_ListSubscriptions(t *testing.T) {
	h, store := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, store.Create(ctx, &Subscription{
		ID: "s1", Name: "one", URL: "https://a.com", Secret: "secret",
		Events: []string{"*"}, Enabled: true, Headers: map[string]string{},
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, store.Create(ctx, &Subscription{
		ID: "s2", Name: "two", URL: "https://b.com",
		Events: []string{"*"}, Enabled: true, Headers: map[string]string{},
		CreatedAt: now, UpdatedAt: now,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/subscriptions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp, 2)
	// Secret should be masked
	for _, s := range resp {
		if s["id"] == "s1" {
			assert.Equal(t, "***", s["secret"])
		}
		if s["id"] == "s2" {
			assert.Equal(t, "", s["secret"])
		}
	}
}

func TestAPI_GetSubscription(t *testing.T) {
	h, store := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, store.Create(ctx, &Subscription{
		ID: "get-1", Name: "getter", URL: "https://g.com",
		Events: []string{"*"}, Enabled: true, Headers: map[string]string{},
		CreatedAt: now, UpdatedAt: now,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/subscriptions/get-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "getter", resp["name"])
}

func TestAPI_GetSubscription_NotFound(t *testing.T) {
	h, _ := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/subscriptions/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_UpdateSubscription(t *testing.T) {
	h, store := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, store.Create(ctx, &Subscription{
		ID: "upd-1", Name: "original", URL: "https://o.com",
		Events: []string{"*"}, Enabled: true, Headers: map[string]string{},
		CreatedAt: now, UpdatedAt: now,
	}))

	body := `{"name":"updated","enabled":false}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/webhooks/subscriptions/upd-1", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "updated", resp["name"])
	assert.Equal(t, false, resp["enabled"])
}

func TestAPI_DeleteSubscription(t *testing.T) {
	h, store := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, store.Create(ctx, &Subscription{
		ID: "del-1", Name: "doomed", URL: "https://d.com",
		Events: []string{"*"}, Enabled: true, Headers: map[string]string{},
		CreatedAt: now, UpdatedAt: now,
	}))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/webhooks/subscriptions/del-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/subscriptions/del-1", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_DeleteSubscription_NotFound(t *testing.T) {
	h, _ := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/webhooks/subscriptions/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPI_TestSubscription(t *testing.T) {
	var received bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h, store := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, store.Create(ctx, &Subscription{
		ID: "test-1", Name: "tester", URL: srv.URL, Secret: "key",
		Events: []string{"*"}, Enabled: true, Headers: map[string]string{},
		TimeoutSec: 5, CreatedAt: now, UpdatedAt: now,
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/subscriptions/test-1/test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, received)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])
}

func TestAPI_Deliveries(t *testing.T) {
	h, store := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ctx := context.Background()

	require.NoError(t, store.RecordDelivery(ctx, &DeliveryRecord{
		ID: "d1", SubscriptionID: "sub-hist", EventType: "agent.connect",
		EventID: "e1", Status: DeliverySuccess, StatusCode: 200,
		Attempt: 1, DeliveredAt: time.Now(),
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/subscriptions/sub-hist/deliveries?limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var records []DeliveryRecord
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &records))
	assert.Len(t, records, 1)
}

func TestAPI_MethodNotAllowed(t *testing.T) {
	h, _ := newTestAPI(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/webhooks/subscriptions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
