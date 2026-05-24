// SPDX-License-Identifier: Apache-2.0

package blueprint_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ibp "go.keystone-core.io/keystone-core/internal/blueprint"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/pkg/api/blueprint"
)

type fakeCat struct {
	list []*ibp.Manifest
	get  *ibp.Manifest
}

func (f fakeCat) List(context.Context) ([]*ibp.Manifest, error) { return f.list, nil }
func (f fakeCat) Get(_ context.Context, name string) (*ibp.Manifest, error) {
	if f.get == nil {
		return nil, errors.New("not found: " + name)
	}
	return f.get, nil
}

type fakeApplier struct {
	res *ibp.ApplyResult
	err error
}

func (f fakeApplier) Apply(context.Context, string, ibp.ApplyOptions) (*ibp.ApplyResult, error) {
	return f.res, f.err
}

func serve(p blueprint.Providers, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	blueprint.NewHandler(p).Register(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func sampleManifest() *ibp.Manifest {
	return &ibp.Manifest{
		Metadata:    ibp.Metadata{Name: "demo", Version: "1.0.0", Description: "demo bp"},
		Parameters:  map[string]ibp.ParamSpec{"pw": {Type: "string", Sensitive: true, Source: ibp.SourceSecret}},
		Entrypoints: ibp.Entrypoints{Default: "apply.yaml"},
	}
}

func TestBlueprintHandler_Unwired503(t *testing.T) {
	for _, path := range []string{
		"GET /api/v1/blueprints", "GET /api/v1/blueprints/demo",
		"POST /api/v1/blueprints/demo/apply",
	} {
		parts := strings.SplitN(path, " ", 2)
		rec := serve(blueprint.Providers{}, parts[0], parts[1], "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: code=%d want 503", path, rec.Code)
		}
	}
}

func TestBlueprintHandler_ListGet(t *testing.T) {
	p := blueprint.Providers{Catalog: fakeCat{list: []*ibp.Manifest{sampleManifest()}, get: sampleManifest()}}

	rec := serve(p, "GET", "/api/v1/blueprints", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"total_count":1`) {
		t.Fatalf("list code=%d body=%s", rec.Code, rec.Body)
	}

	rec = serve(p, "GET", "/api/v1/blueprints/demo", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"demo"`) {
		t.Fatalf("get code=%d body=%s", rec.Code, rec.Body)
	}
	// The manifest DTO must NOT echo secret param values — only names.
	if strings.Contains(rec.Body.String(), "Source") || strings.Contains(rec.Body.String(), "secret://") {
		t.Fatalf("response leaked param internals: %s", rec.Body)
	}

	rec = serve(blueprint.Providers{Catalog: fakeCat{}}, "GET", "/api/v1/blueprints/missing", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing code=%d want 404", rec.Code)
	}
}

func TestBlueprintHandler_Apply(t *testing.T) {
	okRes := &ibp.ApplyResult{RunID: "r1", Status: "succeeded",
		Report: &statemgmt.RunReport{Total: 3, Changed: 2}, Outputs: map[string]any{"summary": "ok"}}

	rec := serve(blueprint.Providers{Applier: fakeApplier{res: okRes}},
		"POST", "/api/v1/blueprints/demo/apply", `{"params":{"pw":"secret://kv/db"}}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"run_id":"r1"`) {
		t.Fatalf("apply code=%d body=%s", rec.Code, rec.Body)
	}
	// The request's secret param value must not be echoed back.
	if strings.Contains(rec.Body.String(), "secret://kv/db") {
		t.Fatalf("apply response leaked secret input: %s", rec.Body)
	}

	// apply ran but ended failed → 409 with the result envelope.
	failRes := &ibp.ApplyResult{RunID: "r2", Status: "failed", Report: &statemgmt.RunReport{Failed: 1}}
	rec = serve(blueprint.Providers{Applier: fakeApplier{res: failRes, err: ibp.ErrApplyFailed}},
		"POST", "/api/v1/blueprints/demo/apply", `{}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"status":"failed"`) {
		t.Fatalf("failed-apply code=%d body=%s", rec.Code, rec.Body)
	}

	// setup error, no result → 500
	rec = serve(blueprint.Providers{Applier: fakeApplier{err: errors.New("boom")}},
		"POST", "/api/v1/blueprints/demo/apply", `{}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("setup-err code=%d want 500", rec.Code)
	}

	// bad JSON body → 400
	rec = serve(blueprint.Providers{Applier: fakeApplier{res: okRes}},
		"POST", "/api/v1/blueprints/demo/apply", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json code=%d want 400", rec.Code)
	}
}
