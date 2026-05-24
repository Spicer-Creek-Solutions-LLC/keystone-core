// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	intpolicy "go.keystone-core.io/keystone-core/internal/policy"
	"go.keystone-core.io/keystone-core/internal/state"
	pkgpolicy "go.keystone-core.io/keystone-core/pkg/api/policy"
)

type recordingAuditor struct {
	mu sync.Mutex
	n  int
}

func (r *recordingAuditor) Emit(context.Context, audit.AuditEntry) {
	r.mu.Lock()
	r.n++
	r.mu.Unlock()
}
func (r *recordingAuditor) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

type rig struct {
	srv      *httptest.Server
	engine   *intpolicy.Engine
	auditLog audit.AuditStore
	auditor  *recordingAuditor
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "policy.db")},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	reg := intpolicy.NewRegistry()
	eng, err := intpolicy.NewEngine(reg,
		intpolicy.WithEvaluator(audit.PolicyTypeBuiltin, intpolicy.NewBuiltinEvaluator()),
		intpolicy.WithEvaluator(audit.PolicyTypeOPA, intpolicy.NewOPAEvaluator()),
	)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	auditLog := audit.NewSQLAuditStore(st)
	gen, err := intpolicy.NewReportGenerator(auditLog, intpolicy.NewControlMapping())
	if err != nil {
		t.Fatalf("NewReportGenerator: %v", err)
	}
	rec := &recordingAuditor{}

	mux := http.NewServeMux()
	pkgpolicy.NewHandler(eng, gen, auditLog, rec).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &rig{srv: srv, engine: eng, auditLog: auditLog, auditor: rec}
}

func newDisabledRig(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	pkgpolicy.NewHandler(nil, nil, nil, nil).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func (r *rig) regBuiltin(t *testing.T, id, code string, enabled bool) {
	t.Helper()
	if err := r.engine.Registry().RegisterPolicy(&intpolicy.Policy{
		ID: id, Name: id, Type: audit.PolicyTypeBuiltin,
		Category: intpolicy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Code: code, Enabled: enabled,
	}); err != nil {
		t.Fatalf("RegisterPolicy %s: %v", id, err)
	}
}

// doJSON issues the request, optionally decodes the JSON body into
// out, closes the body, and returns the status code. No
// *http.Response escapes the helper, so the body is provably closed
// (keeps the bodyclose linter happy without per-call-site defers).
func doJSON(t *testing.T, srv *httptest.Server, method, path string, body, out any) int {
	t.Helper()
	var br *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		br = bytes.NewBuffer(b)
	} else {
		br = &bytes.Buffer{}
	}
	req, err := http.NewRequest(method, srv.URL+path, br)
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
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("decode %s: %v", string(b), err)
		}
	}
	return resp.StatusCode
}

// doRaw posts a raw (possibly invalid-JSON) body and returns the
// status code, closing the body.
func doRaw(t *testing.T, srv *httptest.Server, method, path, raw string) int {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestEvaluate_PolicyAllow_EmitsAudit(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.regBuiltin(t, "only-read", `{"rule":"allowed-actions","allowed":["read"]}`, true)

	var out struct {
		Result struct {
			Allowed bool `json:"allowed"`
		} `json:"result"`
	}
	st := doJSON(t, r.srv, "POST", "/api/v1/policies/evaluate", map[string]any{
		"policy_id": "only-read",
		"input":     map[string]any{"action": "read", "user": "alice"},
	}, &out)
	if st != http.StatusOK {
		t.Fatalf("status = %d, want 200", st)
	}
	if !out.Result.Allowed {
		t.Errorf("allowed = false, want true")
	}
	if r.auditor.count() != 1 {
		t.Errorf("audit emissions = %d, want 1", r.auditor.count())
	}
}

func TestEvaluate_PolicyDeny(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.regBuiltin(t, "only-read", `{"rule":"allowed-actions","allowed":["read"]}`, true)

	var out struct {
		Result struct {
			Allowed    bool `json:"allowed"`
			Violations []struct {
				Rule string `json:"rule"`
			} `json:"violations"`
		} `json:"result"`
	}
	doJSON(t, r.srv, "POST", "/api/v1/policies/evaluate", map[string]any{
		"policy_id": "only-read",
		"input":     map[string]any{"action": "delete"},
	}, &out)
	if out.Result.Allowed || len(out.Result.Violations) == 0 {
		t.Errorf("expected deny+violations, got %+v", out.Result)
	}
}

func TestEvaluate_PolicySet_AllowedAll(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.regBuiltin(t, "a", `{"rule":"allowed-actions","allowed":["read"]}`, true)
	r.regBuiltin(t, "b", `{"rule":"allowed-actions","allowed":["read","write"]}`, true)
	if err := r.engine.Registry().RegisterPolicySet(&intpolicy.PolicySet{
		ID: "set", Name: "set", PolicyIDs: []string{"a", "b"}, Enabled: true,
	}); err != nil {
		t.Fatalf("RegisterPolicySet: %v", err)
	}
	var out struct {
		Results    []map[string]any `json:"results"`
		AllowedAll *bool            `json:"allowed_all"`
	}
	doJSON(t, r.srv, "POST", "/api/v1/policies/evaluate", map[string]any{
		"policy_set_id": "set",
		"input":         map[string]any{"action": "write"},
	}, &out)
	if len(out.Results) != 2 || out.AllowedAll == nil || *out.AllowedAll {
		t.Errorf("set eval: results=%d allowed_all=%v", len(out.Results), out.AllowedAll)
	}
	if r.auditor.count() != 2 {
		t.Errorf("audit emissions = %d, want 2 (one per member)", r.auditor.count())
	}
}

func TestEvaluate_BadRequests(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.regBuiltin(t, "disabled", `{"rule":"allowed-actions","allowed":["x"]}`, false)

	cases := []struct {
		name string
		body any
		want int
	}{
		{"neither id", map[string]any{"input": map[string]any{}}, http.StatusBadRequest},
		{"both ids", map[string]any{"policy_id": "a", "policy_set_id": "b"}, http.StatusBadRequest},
		{"missing policy", map[string]any{"policy_id": "ghost"}, http.StatusNotFound},
		{"disabled policy", map[string]any{"policy_id": "disabled"}, http.StatusConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := doJSON(t, r.srv, "POST", "/api/v1/policies/evaluate", c.body, nil)
			if st != c.want {
				t.Errorf("status = %d, want %d", st, c.want)
			}
		})
	}

	if st := doRaw(t, r.srv, "POST", "/api/v1/policies/evaluate", "{not json"); st != http.StatusBadRequest {
		t.Errorf("bad JSON status = %d, want 400", st)
	}
}

func TestGetAndListPolicies(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.regBuiltin(t, "p-b", `{"rule":"allowed-actions","allowed":["x"]}`, true)
	r.regBuiltin(t, "p-a", `{"rule":"allowed-actions","allowed":["x"]}`, true)

	var p struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if st := doJSON(t, r.srv, "GET", "/api/v1/policies/p-a", nil, &p); st != http.StatusOK {
		t.Fatalf("get status = %d", st)
	}
	if p.ID != "p-a" || p.Type != "builtin" {
		t.Errorf("policy DTO = %+v", p)
	}
	if st := doJSON(t, r.srv, "GET", "/api/v1/policies/ghost", nil, nil); st != http.StatusNotFound {
		t.Errorf("missing get = %d, want 404", st)
	}

	var lst struct {
		Policies []struct {
			ID string `json:"id"`
		} `json:"policies"`
		TotalCount int `json:"total_count"`
	}
	doJSON(t, r.srv, "GET", "/api/v1/policies", nil, &lst)
	if lst.TotalCount != 2 || lst.Policies[0].ID != "p-a" {
		t.Errorf("list = %+v", lst)
	}
}

func TestListPolicies_Pagination(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	for _, id := range []string{"p1", "p2", "p3"} {
		r.regBuiltin(t, id, `{"rule":"allowed-actions","allowed":["x"]}`, true)
	}
	var p1 struct {
		Policies      []map[string]any `json:"policies"`
		NextPageToken string           `json:"next_page_token"`
	}
	doJSON(t, r.srv, "GET", "/api/v1/policies?page_size=2", nil, &p1)
	if len(p1.Policies) != 2 || p1.NextPageToken == "" {
		t.Fatalf("page1: %d entries token=%q", len(p1.Policies), p1.NextPageToken)
	}
	var p2 struct {
		Policies      []map[string]any `json:"policies"`
		NextPageToken string           `json:"next_page_token"`
	}
	doJSON(t, r.srv, "GET", "/api/v1/policies?page_size=2&page_token="+p1.NextPageToken, nil, &p2)
	if len(p2.Policies) != 1 || p2.NextPageToken != "" {
		t.Errorf("page2: %d entries token=%q", len(p2.Policies), p2.NextPageToken)
	}
	if st := doJSON(t, r.srv, "GET", "/api/v1/policies?page_token=abc", nil, nil); st != http.StatusBadRequest {
		t.Errorf("bad token status = %d, want 400", st)
	}
}

func TestPolicySets_GetAndList(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.regBuiltin(t, "m", `{"rule":"allowed-actions","allowed":["x"]}`, true)
	if err := r.engine.Registry().RegisterPolicySet(&intpolicy.PolicySet{
		ID: "s1", Name: "s1", PolicyIDs: []string{"m"}, Enabled: true,
	}); err != nil {
		t.Fatalf("RegisterPolicySet: %v", err)
	}
	var ps struct {
		ID        string   `json:"id"`
		PolicyIDs []string `json:"policy_ids"`
	}
	doJSON(t, r.srv, "GET", "/api/v1/policy-sets/s1", nil, &ps)
	if ps.ID != "s1" || len(ps.PolicyIDs) != 1 {
		t.Errorf("policy set DTO = %+v", ps)
	}
	var lst struct {
		PolicySets []map[string]any `json:"policy_sets"`
		TotalCount int              `json:"total_count"`
	}
	doJSON(t, r.srv, "GET", "/api/v1/policy-sets", nil, &lst)
	if lst.TotalCount != 1 {
		t.Errorf("policy-sets list total = %d, want 1", lst.TotalCount)
	}
}

func TestViolationsAndAuditLog(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	_ = r.auditLog.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
		PolicyID: "p", Action: "policy.evaluate", Allowed: true, Severity: audit.SeverityLow,
	}))
	_ = r.auditLog.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
		PolicyID: "p", Action: "policy.evaluate", Allowed: false, Severity: audit.SeverityHigh,
		Violations: []audit.Violation{{Rule: "r", Message: "no", Severity: audit.SeverityHigh}},
	}))

	var al struct {
		Entries []map[string]any `json:"entries"`
	}
	doJSON(t, r.srv, "GET", "/api/v1/policies/audit", nil, &al)
	if len(al.Entries) != 2 {
		t.Errorf("audit log entries = %d, want 2", len(al.Entries))
	}

	var vl struct {
		Entries []struct {
			Allowed bool `json:"allowed"`
		} `json:"entries"`
	}
	doJSON(t, r.srv, "GET", "/api/v1/policies/violations", nil, &vl)
	if len(vl.Entries) != 1 || vl.Entries[0].Allowed {
		t.Errorf("violations = %+v, want 1 denied", vl.Entries)
	}
}

func TestCompliance(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		_ = r.auditLog.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p", Action: "policy.evaluate", Allowed: i != 0,
			Severity: audit.SeverityHigh,
		}))
	}
	path := "/api/v1/policies/compliance?since=" +
		now.Add(-time.Hour).Format(time.RFC3339) + "&until=" + now.Add(time.Hour).Format(time.RFC3339)
	var rep struct {
		TotalEvaluations     int     `json:"total_evaluations"`
		CompliantEvaluations int     `json:"compliant_evaluations"`
		ComplianceRate       float64 `json:"compliance_rate"`
	}
	if st := doJSON(t, r.srv, "GET", path, nil, &rep); st != http.StatusOK {
		t.Fatalf("compliance status = %d", st)
	}
	if rep.TotalEvaluations != 3 || rep.CompliantEvaluations != 2 {
		t.Errorf("report = %+v", rep)
	}
}

func TestCompliance_InvalidQuery(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if st := doJSON(t, r.srv, "GET", "/api/v1/policies/compliance", nil, nil); st != http.StatusBadRequest {
		t.Errorf("missing since status = %d, want 400", st)
	}
	st := doJSON(t, r.srv, "GET", "/api/v1/policies/compliance?since="+
		time.Now().UTC().Format(time.RFC3339)+"&framework=sarbanes", nil, nil)
	if st != http.StatusBadRequest {
		t.Errorf("bad framework status = %d, want 400", st)
	}
}

func TestDisabledComponents503(t *testing.T) {
	t.Parallel()
	srv := newDisabledRig(t)
	paths := []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/v1/policies/evaluate", map[string]any{"policy_id": "x"}},
		{"GET", "/api/v1/policies/x", nil},
		{"GET", "/api/v1/policies", nil},
		{"GET", "/api/v1/policy-sets", nil},
		{"GET", "/api/v1/policies/violations", nil},
		{"GET", "/api/v1/policies/audit", nil},
		{"GET", "/api/v1/policies/compliance", nil},
	}
	for _, p := range paths {
		if st := doJSON(t, srv, p.method, p.path, p.body, nil); st != http.StatusServiceUnavailable {
			t.Errorf("%s %s status = %d, want 503", p.method, p.path, st)
		}
	}
}
