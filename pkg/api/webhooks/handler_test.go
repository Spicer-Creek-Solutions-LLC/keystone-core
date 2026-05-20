package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
)

// fakeManager satisfies WebhookManager for handler tests.
type fakeManager struct {
	refreshCalls int
	testRes      *outbound.DeliveryRecord
	testErr      error
}

func (f *fakeManager) Refresh(context.Context) error { f.refreshCalls++; return nil }
func (f *fakeManager) TestSubscription(_ context.Context, _ string) (*outbound.DeliveryRecord, error) {
	return f.testRes, f.testErr
}

func newTestServer(p Providers) *httptest.Server {
	mux := http.NewServeMux()
	NewHandler(p).Register(mux)
	return httptest.NewServer(mux)
}

func doJSON(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	var br io.Reader
	if body != "" {
		br = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, br)
	if err != nil {
		t.Fatalf("req: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func TestSubscription_Create_ReturnsCleartextSecretOnce(t *testing.T) {
	t.Parallel()
	store := outbound.NewMemoryStore()
	fm := &fakeManager{}
	srv := newTestServer(Providers{Store: store, Manager: fm})
	t.Cleanup(srv.Close)

	code, raw := doJSON(t, http.MethodPost, srv.URL+"/api/v1/webhooks/subscriptions",
		`{"name":"slack","url":"https://hooks/x","secret":"shhh","events":["state.drift","policy.violation"]}`)
	if code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", code, raw)
	}
	var created outbound.Subscription
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.Secret != "shhh" {
		t.Errorf("create response Secret = %q, want cleartext shhh (§4.14 cleartext-on-creation)", created.Secret)
	}
	if !created.Enabled {
		t.Error("Enabled default did not apply (want true)")
	}
	if fm.refreshCalls != 1 {
		t.Errorf("Manager.Refresh calls = %d, want 1 after create", fm.refreshCalls)
	}

	// Now read it back — Get must mask.
	code, raw = doJSON(t, http.MethodGet, srv.URL+"/api/v1/webhooks/subscriptions/"+created.ID, "")
	if code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	var got outbound.Subscription
	_ = json.Unmarshal(raw, &got)
	if got.Secret != maskedSecret {
		t.Errorf("get response Secret = %q, want %q (§4.14 mask)", got.Secret, maskedSecret)
	}
}

func TestSubscription_Create_Validation(t *testing.T) {
	t.Parallel()
	srv := newTestServer(Providers{Store: outbound.NewMemoryStore()})
	t.Cleanup(srv.Close)
	cases := []struct {
		name string
		body string
		want int
	}{
		{"missing name", `{"url":"https://x"}`, http.StatusBadRequest},
		{"missing url", `{"name":"x"}`, http.StatusBadRequest},
		{"malformed", `{`, http.StatusBadRequest},
		{"unknown field", `{"name":"x","url":"u","bogus":true}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			code, _ := doJSON(t, http.MethodPost, srv.URL+"/api/v1/webhooks/subscriptions", c.body)
			if code != c.want {
				t.Errorf("status = %d, want %d", code, c.want)
			}
		})
	}
}

func TestSubscription_ListMasks(t *testing.T) {
	t.Parallel()
	store := outbound.NewMemoryStore()
	_ = store.CreateSubscription(context.Background(), &outbound.Subscription{
		ID: "s1", Name: "a", URL: "https://a", Secret: "raw", Enabled: true,
	})
	srv := newTestServer(Providers{Store: store})
	t.Cleanup(srv.Close)

	code, raw := doJSON(t, http.MethodGet, srv.URL+"/api/v1/webhooks/subscriptions", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if !bytes.Contains(raw, []byte(`"***"`)) {
		t.Errorf("list response missing masked secret: %s", raw)
	}
	if bytes.Contains(raw, []byte(`"raw"`)) {
		t.Errorf("list response leaked cleartext secret: %s", raw)
	}
}

func TestSubscription_Patch_PartialAndMasks(t *testing.T) {
	t.Parallel()
	store := outbound.NewMemoryStore()
	_ = store.CreateSubscription(context.Background(), &outbound.Subscription{
		ID: "s1", Name: "old", URL: "https://o", Secret: "s", Enabled: true,
		Events: []string{"state.drift"}, MaxRetries: 1, TimeoutSec: 5,
	})
	fm := &fakeManager{}
	srv := newTestServer(Providers{Store: store, Manager: fm})
	t.Cleanup(srv.Close)

	code, raw := doJSON(t, http.MethodPatch, srv.URL+"/api/v1/webhooks/subscriptions/s1",
		`{"name":"new","enabled":false,"max_retries":7}`)
	if code != http.StatusOK {
		t.Fatalf("patch status = %d body=%s", code, raw)
	}
	var got outbound.Subscription
	_ = json.Unmarshal(raw, &got)
	if got.Name != "new" || got.Enabled || got.MaxRetries != 7 {
		t.Errorf("patch did not apply partial fields: %+v", got)
	}
	if got.URL != "https://o" || got.TimeoutSec != 5 || len(got.Events) != 1 {
		t.Errorf("patch clobbered unset fields: %+v", got)
	}
	if got.Secret != maskedSecret {
		t.Errorf("patch response Secret = %q, want masked", got.Secret)
	}
	if fm.refreshCalls != 1 {
		t.Errorf("Manager.Refresh calls = %d, want 1 after patch", fm.refreshCalls)
	}
}

func TestSubscription_Delete(t *testing.T) {
	t.Parallel()
	store := outbound.NewMemoryStore()
	_ = store.CreateSubscription(context.Background(), &outbound.Subscription{ID: "s1", Name: "a", URL: "u", Enabled: true})
	fm := &fakeManager{}
	srv := newTestServer(Providers{Store: store, Manager: fm})
	t.Cleanup(srv.Close)

	code, _ := doJSON(t, http.MethodDelete, srv.URL+"/api/v1/webhooks/subscriptions/s1", "")
	if code != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", code)
	}
	if fm.refreshCalls != 1 {
		t.Errorf("Manager.Refresh calls = %d, want 1 after delete", fm.refreshCalls)
	}
	// 404 on second delete.
	code, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/webhooks/subscriptions/s1", "")
	if code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", code)
	}
}

func TestSubscription_Test_DispatchesAndReturnsDelivery(t *testing.T) {
	t.Parallel()
	fm := &fakeManager{testRes: &outbound.DeliveryRecord{
		ID: "d1", SubscriptionID: "s1", EventType: "webhook.test",
		Status: outbound.DeliverySuccess, StatusCode: 200, Attempt: 1,
	}}
	srv := newTestServer(Providers{Store: outbound.NewMemoryStore(), Manager: fm})
	t.Cleanup(srv.Close)

	code, raw := doJSON(t, http.MethodPost, srv.URL+"/api/v1/webhooks/subscriptions/s1/test", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d body=%s", code, raw)
	}
	var got outbound.DeliveryRecord
	_ = json.Unmarshal(raw, &got)
	if got.Status != outbound.DeliverySuccess || got.EventType != "webhook.test" {
		t.Errorf("test response = %+v", got)
	}

	// Internal error path
	fm.testErr = errors.New("boom")
	fm.testRes = nil
	code, _ = doJSON(t, http.MethodPost, srv.URL+"/api/v1/webhooks/subscriptions/s1/test", "")
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", code)
	}
}

func TestSubscription_Deliveries(t *testing.T) {
	t.Parallel()
	store := outbound.NewMemoryStore()
	_ = store.CreateSubscription(context.Background(), &outbound.Subscription{ID: "s1", Name: "a", URL: "u", Enabled: true})
	for i := 0; i < 3; i++ {
		_ = store.SaveDelivery(context.Background(), &outbound.DeliveryRecord{
			ID:             "d" + string(rune('0'+i)),
			SubscriptionID: "s1", EventType: "state.drift",
			Status: outbound.DeliverySuccess,
		})
	}
	srv := newTestServer(Providers{Store: store})
	t.Cleanup(srv.Close)

	code, raw := doJSON(t, http.MethodGet, srv.URL+"/api/v1/webhooks/subscriptions/s1/deliveries", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	var got []outbound.DeliveryRecord
	if err := json.Unmarshal(raw, &got); err != nil || len(got) != 3 {
		t.Errorf("list deliveries: err=%v len=%d", err, len(got))
	}

	// ?limit=2
	code, raw = doJSON(t, http.MethodGet, srv.URL+"/api/v1/webhooks/subscriptions/s1/deliveries?limit=2", "")
	_ = json.Unmarshal(raw, &got)
	if code != http.StatusOK || len(got) != 2 {
		t.Errorf("limit=2 status=%d len=%d", code, len(got))
	}
}

func TestSubscription_NotFound(t *testing.T) {
	t.Parallel()
	srv := newTestServer(Providers{Store: outbound.NewMemoryStore()})
	t.Cleanup(srv.Close)
	for _, path := range []string{
		"/api/v1/webhooks/subscriptions/missing",
		"/api/v1/webhooks/subscriptions/missing",
	} {
		code, _ := doJSON(t, http.MethodGet, srv.URL+path, "")
		if code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
	code, _ := doJSON(t, http.MethodPatch, srv.URL+"/api/v1/webhooks/subscriptions/missing", `{"name":"x"}`)
	if code != http.StatusNotFound {
		t.Errorf("PATCH missing = %d, want 404", code)
	}
}

func TestNilProviders_Return503(t *testing.T) {
	t.Parallel()
	srv := newTestServer(Providers{})
	t.Cleanup(srv.Close)
	cases := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/webhooks/subscriptions", ""},
		{http.MethodPost, "/api/v1/webhooks/subscriptions", `{"name":"a","url":"u"}`},
		{http.MethodGet, "/api/v1/webhooks/subscriptions/x", ""},
		{http.MethodPatch, "/api/v1/webhooks/subscriptions/x", `{"name":"y"}`},
		{http.MethodDelete, "/api/v1/webhooks/subscriptions/x", ""},
		{http.MethodPost, "/api/v1/webhooks/subscriptions/x/test", ""},
		{http.MethodGet, "/api/v1/webhooks/subscriptions/x/deliveries", ""},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			t.Parallel()
			code, _ := doJSON(t, c.method, srv.URL+c.path, c.body)
			if code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", code)
			}
		})
	}
}
