// SPDX-License-Identifier: Apache-2.0

package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	internalevents "go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/state"
	pkgevents "go.keystone-core.io/keystone-core/pkg/api/events"
)

// testRig wires the REST handler against a real SQL-backed
// EventStore + a stub publisher that records emits.
type testRig struct {
	srv   *httptest.Server
	store internalevents.EventStore
	pub   *stubPub
}

type stubPub struct {
	mu        sync.Mutex
	published []internalevents.Event
	err       error
}

func (s *stubPub) Start(context.Context) error { return nil }
func (s *stubPub) Publish(_ context.Context, e internalevents.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.published = append(s.published, e)
	return nil
}
func (s *stubPub) PublishAsync(ctx context.Context, e internalevents.Event) error {
	return s.Publish(ctx, e)
}
func (s *stubPub) Stop(context.Context) error { return nil }

func (s *stubPub) snapshot() []internalevents.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]internalevents.Event, len(s.published))
	copy(out, s.published)
	return out
}

func newRig(t *testing.T) *testRig {
	t.Helper()
	stateStore, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "events.db")},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = stateStore.Close() })

	store := internalevents.NewSQLEventStore(stateStore)
	pub := &stubPub{}

	mux := http.NewServeMux()
	pkgevents.NewHandler(store, pub).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testRig{srv: srv, store: store, pub: pub}
}

func newDisabledRig(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	pkgevents.NewHandler(nil, nil).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func doRequest(t *testing.T, srv *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var bodyReader *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		bodyReader = bytes.NewBuffer(b)
	} else {
		bodyReader = &bytes.Buffer{}
	}
	req, err := http.NewRequest(method, srv.URL+path, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// ---- List ----------------------------------------------------------------

func TestREST_List_Empty(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := doRequest(t, r.srv, "GET", "/api/v1/events", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
	var out struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Events) != 0 {
		t.Errorf("len = %d, want 0", len(out.Events))
	}
}

func TestREST_List_FilterByType(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	for _, typ := range []internalevents.EventType{
		internalevents.EventTypeAgentConnect,
		internalevents.EventTypeJobStart,
		internalevents.EventTypeAgentConnect,
	} {
		_ = r.store.Store(ctx, internalevents.MustNewEvent(typ, "src"))
	}

	resp := doRequest(t, r.srv, "GET", "/api/v1/events?type=agent.connect", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		Events []map[string]any `json:"events"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Events) != 2 {
		t.Errorf("len = %d, want 2", len(out.Events))
	}
}

func TestREST_List_FilterByCategory(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	for _, typ := range []internalevents.EventType{
		internalevents.EventTypeAgentConnect,
		internalevents.EventTypeAgentHeartbeat,
		internalevents.EventTypeJobStart,
	} {
		_ = r.store.Store(ctx, internalevents.MustNewEvent(typ, "src"))
	}
	resp := doRequest(t, r.srv, "GET", "/api/v1/events?category=agent", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		Events []map[string]any `json:"events"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Events) != 2 {
		t.Errorf("len = %d, want 2", len(out.Events))
	}
}

func TestREST_List_TypeAndCategoryMutuallyExclusive(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := doRequest(t, r.srv, "GET", "/api/v1/events?type=agent.connect&category=agent", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestREST_List_FilterByMinSeverity(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	severities := []internalevents.Severity{
		internalevents.SeverityInfo, internalevents.SeverityWarn,
		internalevents.SeverityError, internalevents.SeverityInfo,
	}
	for _, sev := range severities {
		e := internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "src")
		e.Severity = sev
		_ = r.store.Store(ctx, e)
	}
	resp := doRequest(t, r.srv, "GET", "/api/v1/events?min_severity=warn", nil)
	defer resp.Body.Close()
	var out struct {
		Events []map[string]any `json:"events"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Events) != 2 {
		t.Errorf("warn+ = %d, want 2", len(out.Events))
	}
}

func TestREST_List_FilterByTag(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	for _, role := range []string{"web", "db", "web"} {
		e := internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "src")
		e.Tags = map[string]string{"role": role}
		_ = r.store.Store(ctx, e)
	}
	resp := doRequest(t, r.srv, "GET", "/api/v1/events?tag.role=web", nil)
	defer resp.Body.Close()
	var out struct {
		Events []map[string]any `json:"events"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Events) != 2 {
		t.Errorf("tag.role=web → %d, want 2", len(out.Events))
	}
}

func TestREST_List_InvalidLimit(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := doRequest(t, r.srv, "GET", "/api/v1/events?limit=abc", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// ---- Get ----------------------------------------------------------------

func TestREST_Get_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	in := internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "agent-1")
	in.Tags = map[string]string{"role": "web"}
	in.Data = map[string]any{"k": "v"}
	_ = r.store.Store(ctx, in)

	resp := doRequest(t, r.srv, "GET", "/api/v1/events/"+in.ID, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["id"] != in.ID {
		t.Errorf("id = %v", out["id"])
	}
	if out["severity"] != "info" {
		t.Errorf("severity = %v, want info", out["severity"])
	}
}

func TestREST_Get_NotFound(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := doRequest(t, r.srv, "GET", "/api/v1/events/ghost", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// ---- Emit ----------------------------------------------------------------

func TestREST_Emit_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	body := map[string]any{
		"type":     "agent.connect",
		"source":   "agent-1",
		"severity": "warn",
		"tags":     map[string]string{"role": "web"},
		"data":     map[string]any{"latency_ms": 12.5},
	}
	resp := doRequest(t, r.srv, "POST", "/api/v1/events", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		EventID string `json:"event_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.EventID == "" {
		t.Fatalf("EventID empty")
	}
	pubs := r.pub.snapshot()
	if len(pubs) != 1 {
		t.Fatalf("publisher saw %d events", len(pubs))
	}
	if pubs[0].Severity != internalevents.SeverityWarn {
		t.Errorf("severity not propagated: %s", pubs[0].Severity)
	}
	if pubs[0].Tags["role"] != "web" {
		t.Errorf("tags lost: %v", pubs[0].Tags)
	}
}

func TestREST_Emit_MissingType(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	body := map[string]any{"source": "agent-1"}
	resp := doRequest(t, r.srv, "POST", "/api/v1/events", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestREST_Emit_BadType(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	body := map[string]any{"type": "bogus", "source": "x"}
	resp := doRequest(t, r.srv, "POST", "/api/v1/events", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestREST_Emit_PublisherError(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.pub.err = errors.New("publisher down")
	body := map[string]any{"type": "agent.connect", "source": "x"}
	resp := doRequest(t, r.srv, "POST", "/api/v1/events", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ---- Types ----------------------------------------------------------------

func TestREST_Types(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := doRequest(t, r.srv, "GET", "/api/v1/events/types", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		Types []string `json:"types"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Types) != 30 {
		t.Errorf("len = %d, want 30 (§4.9's 29 + system.rebooted)", len(out.Types))
	}
}

// ---- Stats ----------------------------------------------------------------

func TestREST_Stats(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = r.store.Store(ctx, internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "src"))
	}
	for i := 0; i < 2; i++ {
		e := internalevents.MustNewEvent(internalevents.EventTypeJobStart, "src")
		e.Severity = internalevents.SeverityWarn
		_ = r.store.Store(ctx, e)
	}

	resp := doRequest(t, r.srv, "GET", "/api/v1/events/stats", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out struct {
		Total      int64            `json:"total"`
		ByType     map[string]int64 `json:"by_type"`
		BySeverity map[string]int64 `json:"by_severity"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Total != 5 {
		t.Errorf("total = %d, want 5", out.Total)
	}
	if out.ByType["agent.connect"] != 3 {
		t.Errorf("agent.connect = %d", out.ByType["agent.connect"])
	}
	if out.BySeverity["warn"] != 2 {
		t.Errorf("warn = %d", out.BySeverity["warn"])
	}
}

func TestREST_Stats_BadSince(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	resp := doRequest(t, r.srv, "GET", "/api/v1/events/stats?since=not-a-time", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// ---- 503 when disabled ----------------------------------------------------

func TestREST_503_WhenDisabled(t *testing.T) {
	t.Parallel()
	srv := newDisabledRig(t)
	cases := []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/v1/events", nil},
		{"GET", "/api/v1/events/abc", nil},
		{"POST", "/api/v1/events", map[string]any{"type": "agent.connect", "source": "x"}},
		{"GET", "/api/v1/events/stats", nil},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s %s", c.method, c.path), func(t *testing.T) {
			resp := doRequest(t, srv, c.method, c.path, c.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", resp.StatusCode)
			}
		})
	}

	// /types stays up even when disabled — taxonomy is a constant.
	resp := doRequest(t, srv, "GET", "/api/v1/events/types", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/types should always work: %d", resp.StatusCode)
	}
}

// ---- Bad JSON ----------------------------------------------------------

func TestREST_Emit_BadJSON(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	req, _ := http.NewRequest("POST", r.srv.URL+"/api/v1/events", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d", resp.StatusCode)
	}
}
