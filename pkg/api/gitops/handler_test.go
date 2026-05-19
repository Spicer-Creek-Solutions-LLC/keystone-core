package gitops

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

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
	"go.keystone-core.io/keystone-core/internal/gitops/verification"
)

// fakeEngine is the in-process [RollbackEngine] for these tests.
type fakeEngine struct {
	executed   *rollback.RollbackSpec
	executeRes *rollback.Rollback
	executeErr error
	approveRes *rollback.Rollback
	approveErr error
	rejectRes  *rollback.Rollback
	rejectErr  error
	getRes     *rollback.Rollback
	getOK      bool
	getErr     error
	listRes    []*rollback.Rollback
	listErr    error
}

func (f *fakeEngine) Execute(_ context.Context, s rollback.RollbackSpec) (*rollback.Rollback, error) {
	f.executed = &s
	return f.executeRes, f.executeErr
}
func (f *fakeEngine) ApproveRollback(context.Context, string, string) (*rollback.Rollback, error) {
	return f.approveRes, f.approveErr
}
func (f *fakeEngine) RejectRollback(context.Context, string, string, string) (*rollback.Rollback, error) {
	return f.rejectRes, f.rejectErr
}
func (f *fakeEngine) GetRollback(context.Context, string) (*rollback.Rollback, bool, error) {
	return f.getRes, f.getOK, f.getErr
}
func (f *fakeEngine) ListRollbacks(context.Context) ([]*rollback.Rollback, error) {
	return f.listRes, f.listErr
}

func newTestServer(p Providers) *httptest.Server {
	mux := http.NewServeMux()
	NewHandler(p).Register(mux)
	return httptest.NewServer(mux)
}

// doJSON sends the request, drains+closes the response body inside,
// and returns the status code + body bytes — keeps the bodyclose
// linter satisfied across the helper boundary.
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

func TestRollback_Execute_Success(t *testing.T) {
	t.Parallel()
	fe := &fakeEngine{executeRes: &rollback.Rollback{ID: "rb-1", State: rollback.StateCompleted}}
	srv := newTestServer(Providers{Rollback: fe})
	t.Cleanup(srv.Close)

	code, raw := doJSON(t, http.MethodPost, srv.URL+"/api/v1/gitops/rollback",
		`{"executor_type":"git","application":"web","strategy":"specific","revision":"c1","reason":"hotfix","config":{"repo_url":"https://r"}}`)
	if code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", code, raw)
	}
	if fe.executed.ExecutorType != "git" || fe.executed.Config["repo_url"] != "https://r" ||
		fe.executed.Request.Application != "web" || fe.executed.Request.Strategy != rollback.StrategySpecific {
		t.Errorf("engine got unexpected spec: %+v", fe.executed)
	}
	var got rollback.Rollback
	if err := json.Unmarshal(raw, &got); err != nil || got.ID != "rb-1" {
		t.Errorf("response body bad: err=%v body=%s", err, raw)
	}
}

func TestRollback_Execute_ValidationAndErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fe   *fakeEngine
		body string
		want int
	}{
		{"missing fields", &fakeEngine{}, `{"executor_type":"git"}`, http.StatusBadRequest},
		{"malformed json", &fakeEngine{}, `{`, http.StatusBadRequest},
		{"unknown executor", &fakeEngine{executeErr: rollback.ErrUnknownExecutor}, `{"executor_type":"x","application":"a","strategy":"previous"}`, http.StatusBadRequest},
		{"internal error", &fakeEngine{executeErr: errors.New("boom")}, `{"executor_type":"git","application":"a","strategy":"previous"}`, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := newTestServer(Providers{Rollback: c.fe})
			t.Cleanup(srv.Close)
			code, _ := doJSON(t, http.MethodPost, srv.URL+"/api/v1/gitops/rollback", c.body)
			if code != c.want {
				t.Errorf("status = %d, want %d", code, c.want)
			}
		})
	}
}

func TestRollback_Approve(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fe   *fakeEngine
		want int
	}{
		{"success", &fakeEngine{approveRes: &rollback.Rollback{ID: "x", State: rollback.StateCompleted}}, http.StatusOK},
		{"not found", &fakeEngine{approveErr: rollback.ErrRollbackNotFound}, http.StatusNotFound},
		{"invalid transition", &fakeEngine{approveErr: rollback.ErrInvalidTransition}, http.StatusConflict},
		{"internal error", &fakeEngine{approveErr: errors.New("db down")}, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := newTestServer(Providers{Rollback: c.fe})
			t.Cleanup(srv.Close)
			code, _ := doJSON(t, http.MethodPost, srv.URL+"/api/v1/gitops/rollbacks/x/approve", `{"approver":"alice"}`)
			if code != c.want {
				t.Errorf("status = %d, want %d", code, c.want)
			}
		})
	}
}

func TestRollback_GetAndList(t *testing.T) {
	t.Parallel()
	fe := &fakeEngine{
		getRes: &rollback.Rollback{ID: "rb-1", State: rollback.StateCompleted}, getOK: true,
		listRes: []*rollback.Rollback{{ID: "a"}, {ID: "b"}},
	}
	srv := newTestServer(Providers{Rollback: fe})
	t.Cleanup(srv.Close)

	code, raw := doJSON(t, http.MethodGet, srv.URL+"/api/v1/gitops/rollbacks/rb-1", "")
	if code != http.StatusOK || !bytes.Contains(raw, []byte(`"rb-1"`)) {
		t.Errorf("get status=%d body=%s", code, raw)
	}

	code, raw = doJSON(t, http.MethodGet, srv.URL+"/api/v1/gitops/rollbacks", "")
	if code != http.StatusOK {
		t.Fatalf("list status=%d", code)
	}
	var list []rollback.Rollback
	if err := json.Unmarshal(raw, &list); err != nil || len(list) != 2 {
		t.Errorf("list body bad: err=%v body=%s", err, raw)
	}

	fe.getOK = false
	code, _ = doJSON(t, http.MethodGet, srv.URL+"/api/v1/gitops/rollbacks/missing", "")
	if code != http.StatusNotFound {
		t.Errorf("missing status = %d, want 404", code)
	}
}

func TestRollback_Reject_StateMachineConflict(t *testing.T) {
	t.Parallel()
	fe := &fakeEngine{rejectErr: rollback.ErrInvalidTransition}
	srv := newTestServer(Providers{Rollback: fe})
	t.Cleanup(srv.Close)
	code, _ := doJSON(t, http.MethodPost, srv.URL+"/api/v1/gitops/rollbacks/x/reject",
		`{"approver":"bob","reason":"nope"}`)
	if code != http.StatusConflict {
		t.Errorf("status = %d, want 409", code)
	}
}

func TestVerification_ListGet(t *testing.T) {
	t.Parallel()
	vs := verification.NewMemoryResultStore()
	_ = vs.Save(context.Background(), &verification.StoredVerification{ID: "v1", Application: "web"})
	srv := newTestServer(Providers{Verifications: vs})
	t.Cleanup(srv.Close)

	code, raw := doJSON(t, http.MethodGet, srv.URL+"/api/v1/gitops/verifications", "")
	if code != http.StatusOK || !bytes.Contains(raw, []byte(`"v1"`)) {
		t.Errorf("list status=%d body=%s", code, raw)
	}
	code, _ = doJSON(t, http.MethodGet, srv.URL+"/api/v1/gitops/verifications/v1", "")
	if code != http.StatusOK {
		t.Errorf("get status = %d, want 200", code)
	}
	code, _ = doJSON(t, http.MethodGet, srv.URL+"/api/v1/gitops/verifications/missing", "")
	if code != http.StatusNotFound {
		t.Errorf("missing status = %d, want 404", code)
	}
}

func TestNilProviders_Return503(t *testing.T) {
	t.Parallel()
	srv := newTestServer(Providers{})
	t.Cleanup(srv.Close)
	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/v1/gitops/rollback", `{"executor_type":"git","application":"a","strategy":"previous"}`},
		{http.MethodGet, "/api/v1/gitops/rollbacks", ""},
		{http.MethodGet, "/api/v1/gitops/rollbacks/x", ""},
		{http.MethodPost, "/api/v1/gitops/rollbacks/x/approve", `{"approver":"a"}`},
		{http.MethodPost, "/api/v1/gitops/rollbacks/x/reject", `{"approver":"a","reason":"r"}`},
		{http.MethodGet, "/api/v1/gitops/verifications", ""},
		{http.MethodGet, "/api/v1/gitops/verifications/x", ""},
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
