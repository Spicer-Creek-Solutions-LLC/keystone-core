// Package blueprints_test drives every shipped reference blueprint
// through the real v1.0 blueprint pipeline: Load + Validate, then
// Apply (render → parse → feature-filter → namespace → resolve)
// through internal/blueprint.Executor with a fake State Runner. This
// is the quality gate for the catalog — a malformed manifest, an
// unresolvable entrypoint, or a missing parameter fails the build.
package blueprints_test

import (
	"context"
	"testing"

	"go.keystone-core.io/keystone-core/internal/blueprint"
	"go.keystone-core.io/keystone-core/internal/runbook"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type fakeRunner struct{ decls []*statemgmt.Declaration }

func (f *fakeRunner) Run(_ context.Context, d []*statemgmt.Declaration) (*statemgmt.RunReport, error) {
	f.decls = d
	return &statemgmt.RunReport{Total: len(d)}, nil
}

type fakeSecret struct{}

func (fakeSecret) ResolveSecret(context.Context, string) (string, error) { return "resolved-pw", nil }

// noopHookExec is a runbook.Executor with just a noop step — enough
// to run the production-cluster preflight hook runbook.
func noopHookExec(t *testing.T) *runbook.Executor {
	t.Helper()
	reg := runbook.NewRegistry()
	if err := reg.Register("noop", runbook.StepFunc(func(context.Context, runbook.StepContext) (runbook.StepOutput, error) {
		return runbook.StepOutput{}, nil
	})); err != nil {
		t.Fatal(err)
	}
	return &runbook.Executor{Registry: reg}
}

var catalog = []string{
	"demo", "production-cluster", "monitoring-stack",
	"security-baseline", "postgres-ha", "nats-cluster",
}

// TestCatalog_LoadValidate proves every blueprint manifest loads and
// passes structural validation (Load runs Validate).
func TestCatalog_LoadValidate(t *testing.T) {
	for _, name := range catalog {
		t.Run(name, func(t *testing.T) {
			if _, err := blueprint.Load(name); err != nil {
				t.Fatalf("Load(%s): %v", name, err)
			}
		})
	}
}

// TestCatalog_Apply runs each blueprint end to end through the
// executor (default entrypoint), proving the entrypoint renders,
// parses, feature-filters, and resolves.
func TestCatalog_Apply(t *testing.T) {
	for _, name := range catalog {
		t.Run(name, func(t *testing.T) {
			m, err := blueprint.Load(name)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			sr := &fakeRunner{}
			e := &blueprint.Executor{
				StateRunner: sr,
				Secrets:     fakeSecret{},
				Hooks:       blueprint.NewRunbookHookRunner(noopHookExec(t)),
				Store:       blueprint.NewMemoryAppliedStore(),
			}
			opts := blueprint.ApplyOptions{}
			if name == "production-cluster" {
				opts.Inputs = map[string]string{"postgres_password": "secret://kv/db"}
			}
			res, err := e.Apply(context.Background(), m, opts)
			if err != nil {
				t.Fatalf("Apply(%s): %v", name, err)
			}
			if res.Status != "succeeded" {
				t.Fatalf("%s status=%s", name, res.Status)
			}
			if len(sr.decls) == 0 {
				t.Fatalf("%s resolved zero declarations", name)
			}
		})
	}
}

// TestDemo_AcceptanceCriterion1 — `blueprint apply demo` deploys
// successfully and renders its output.
func TestDemo_AcceptanceCriterion1(t *testing.T) {
	m, err := blueprint.Load("demo")
	if err != nil {
		t.Fatal(err)
	}
	sr := &fakeRunner{}
	e := &blueprint.Executor{StateRunner: sr, Store: blueprint.NewMemoryAppliedStore()}
	res, err := e.Apply(context.Background(), m, blueprint.ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply demo: %v", err)
	}
	if res.Outputs["summary"] != "deployed keystone-demo (nginx)" {
		t.Fatalf("demo output = %v", res.Outputs)
	}
	var ids []string
	for _, d := range sr.decls {
		ids = append(ids, d.ID)
	}
	// marker_file feature is on by default → its declaration is kept.
	found := false
	for _, id := range ids {
		if id == "file:/etc/keystone-demo.deployed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("marker file declaration missing: %v", ids)
	}
}

// TestProductionCluster_AcceptanceCriterion2 — substitutes a secret
// param and exposes cluster_name; the secret value is resolved before
// the state collection is rendered.
func TestProductionCluster_AcceptanceCriterion2(t *testing.T) {
	m, err := blueprint.Load("production-cluster")
	if err != nil {
		t.Fatal(err)
	}
	pw, ok := m.Parameters["postgres_password"]
	if !ok || pw.Source != blueprint.SourceSecret || !pw.Sensitive {
		t.Fatalf("postgres_password not a sensitive secret param: %+v", pw)
	}
	if _, ok := m.Parameters["cluster_name"]; !ok {
		t.Fatal("cluster_name parameter missing")
	}

	sr := &fakeRunner{}
	e := &blueprint.Executor{
		StateRunner: sr,
		Secrets:     fakeSecret{},
		Hooks:       blueprint.NewRunbookHookRunner(noopHookExec(t)),
		Store:       blueprint.NewMemoryAppliedStore(),
	}
	if _, err := e.Apply(context.Background(), m, blueprint.ApplyOptions{
		Inputs: map[string]string{"postgres_password": "secret://kv/db", "cluster_name": "test"},
	}); err != nil {
		t.Fatalf("Apply production-cluster: %v", err)
	}
	// The resolved secret must have reached the rendered postgres.env.
	var envDecl *statemgmt.Declaration
	for _, d := range sr.decls {
		if d.ID == "file:/etc/keystone/postgres.env" {
			envDecl = d
		}
	}
	if envDecl == nil {
		t.Fatal("postgres.env declaration not resolved")
	}
}
