package runbook_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	irb "go.keystone-core.io/keystone-core/internal/runbook"
	"go.keystone-core.io/keystone-core/pkg/api/runbook"
)

type fakeCatalog struct {
	list []*irb.Runbook
	get  *irb.Runbook
	err  error
}

func (f fakeCatalog) List(context.Context) ([]*irb.Runbook, error) { return f.list, f.err }
func (f fakeCatalog) Get(_ context.Context, id string) (*irb.Runbook, error) {
	if f.get == nil {
		return nil, errors.New("not found: " + id)
	}
	return f.get, nil
}

type fakeRunner struct {
	exec *irb.Execution
	err  error
}

func (f fakeRunner) Execute(context.Context, string, map[string]any) (*irb.Execution, error) {
	return f.exec, f.err
}

type fakeStore struct{ e *irb.Execution }

func (f fakeStore) Get(_ context.Context, id string) (*irb.Execution, error) {
	if f.e == nil {
		return nil, errors.New("execution not found: " + id)
	}
	return f.e, nil
}

func serve(p runbook.Providers, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	runbook.NewHandler(p).Register(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func sampleRB() *irb.Runbook {
	return &irb.Runbook{
		Metadata: irb.Metadata{Name: "db-restart", Version: "1.0.0"},
		Spec:     irb.Spec{Steps: []irb.Step{{Type: "noop", Name: "stop"}}},
	}
}

func TestRunbookHandler_Unwired503(t *testing.T) {
	for _, path := range []string{
		"GET /api/v1/runbooks", "GET /api/v1/runbooks/x",
		"POST /api/v1/runbooks", "GET /api/v1/executions/abc",
	} {
		parts := strings.SplitN(path, " ", 2)
		rec := serve(runbook.Providers{}, parts[0], parts[1], "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: code=%d want 503", path, rec.Code)
		}
	}
}

func TestRunbookHandler_ListGet(t *testing.T) {
	p := runbook.Providers{Catalog: fakeCatalog{list: []*irb.Runbook{sampleRB()}, get: sampleRB()}}

	rec := serve(p, "GET", "/api/v1/runbooks", "")
	if rec.Code != 200 {
		t.Fatalf("list code=%d", rec.Code)
	}
	var lr struct {
		Runbooks   []map[string]any `json:"runbooks"`
		TotalCount int              `json:"total_count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &lr)
	if lr.TotalCount != 1 || lr.Runbooks[0]["name"] != "db-restart" {
		t.Fatalf("list body=%s", rec.Body)
	}

	rec = serve(p, "GET", "/api/v1/runbooks/db-restart", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "db-restart") {
		t.Fatalf("get code=%d body=%s", rec.Code, rec.Body)
	}

	rec = serve(runbook.Providers{Catalog: fakeCatalog{}}, "GET", "/api/v1/runbooks/missing", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing code=%d want 404", rec.Code)
	}
}

func TestRunbookHandler_Execute(t *testing.T) {
	okExec := &irb.Execution{ID: "e1", Runbook: "db-restart", Status: irb.StatusSucceeded}

	rec := serve(runbook.Providers{Runner: fakeRunner{exec: okExec}}, "POST", "/api/v1/runbooks", `{"runbook":"db-restart"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("execute code=%d body=%s", rec.Code, rec.Body)
	}

	failExec := &irb.Execution{ID: "e2", Status: irb.StatusFailed}
	rec = serve(runbook.Providers{Runner: fakeRunner{exec: failExec, err: irb.ErrExecutionFailed}},
		"POST", "/api/v1/runbooks", `{"runbook":"x"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"failed"`) {
		t.Fatalf("failed-exec code=%d body=%s", rec.Code, rec.Body)
	}

	rec = serve(runbook.Providers{Runner: fakeRunner{err: errors.New("boom")}}, "POST", "/api/v1/runbooks", `{"runbook":"x"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("setup-err code=%d want 500", rec.Code)
	}

	rec = serve(runbook.Providers{Runner: fakeRunner{exec: okExec}}, "POST", "/api/v1/runbooks", `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json code=%d want 400", rec.Code)
	}
	rec = serve(runbook.Providers{Runner: fakeRunner{exec: okExec}}, "POST", "/api/v1/runbooks", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing runbook code=%d want 400", rec.Code)
	}
}

func TestRunbookHandler_Execution(t *testing.T) {
	e := &irb.Execution{ID: "e1", Runbook: "rb", Status: irb.StatusSucceeded}
	rec := serve(runbook.Providers{Store: fakeStore{e: e}}, "GET", "/api/v1/executions/e1", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"id":"e1"`) {
		t.Fatalf("execution code=%d body=%s", rec.Code, rec.Body)
	}
	rec = serve(runbook.Providers{Store: fakeStore{}}, "GET", "/api/v1/executions/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing execution code=%d want 404", rec.Code)
	}
}
