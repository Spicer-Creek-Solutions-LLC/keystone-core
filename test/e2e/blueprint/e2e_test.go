// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Package blueprinte2e is the Epic 15 task-13 end-to-end integration
// suite. It wires the REAL Epic-15 engines exactly as a server would
// compose them — blueprint.Executor + runbook.Executor with the v1.0
// step set + BrokerSecretResolver — and drives them against the REAL
// shipped catalog (modules/examples/blueprints) plus real runbook /
// saga flows. A recording StateRunner stands in for host convergence
// (applying production-cluster's package/service/sysctl declarations
// for real is not CI-safe and is not what proves the integration);
// the literal multi-container docker-compose convergence form is a
// gate-v1.0 ROADMAP item.
//
// Build-tagged `integration` (run via `make test-integration`), so
// it is excluded from the default `go test ./...`.
package blueprinte2e

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	"go.keystone-core.io/keystone-core/internal/runbook"
	"go.keystone-core.io/keystone-core/internal/runbook/steps"
	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/pkg/saga"
)

const catalog = "../../../modules/examples/blueprints"

// recordingRunner captures every declaration dispatched to the State
// Runner and reports them all as applied/changed.
type recordingRunner struct {
	mu    sync.Mutex
	decls []*statemgmt.Declaration
}

func (r *recordingRunner) Run(_ context.Context, d []*statemgmt.Declaration) (*statemgmt.RunReport, error) {
	r.mu.Lock()
	r.decls = append(r.decls, d...)
	r.mu.Unlock()
	return &statemgmt.RunReport{Total: len(d), Changed: len(d)}, nil
}

func (r *recordingRunner) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.decls))
	for _, d := range r.decls {
		out = append(out, d.ID)
	}
	return out
}

// fakeBroker satisfies the BrokerSecretResolver's secret getter.
type fakeBroker struct{ val string }

func (f fakeBroker) GetSecret(context.Context, secrets.GetSecretRequest) (*secrets.Secret, error) {
	return &secrets.Secret{Data: map[string]any{"value": f.val}}, nil
}

// newStack wires the real engines the way cmd/kscore-server would,
// plus a hook-step recorder so we can prove the preflight runbook
// actually executed end to end.
func newStack(t *testing.T) (*bp.Executor, *recordingRunner, *int) {
	t.Helper()
	reg := runbook.NewRegistry()
	if err := steps.RegisterAll(reg, steps.Deps{}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	var hookSteps int
	var mu sync.Mutex
	// Override noop with a recorder: production-cluster's preflight
	// hook is a runbook of noop steps, so this fires only if the
	// real RunbookHookRunner really ran it.
	if err := reg.Register("noop", runbook.StepFunc(func(context.Context, runbook.StepContext) (runbook.StepOutput, error) {
		mu.Lock()
		hookSteps++
		mu.Unlock()
		return runbook.StepOutput{}, nil
	})); err != nil {
		t.Fatalf("override noop: %v", err)
	}
	rr := &recordingRunner{}
	ex := &bp.Executor{
		StateRunner: rr,
		Secrets:     bp.NewBrokerSecretResolver(fakeBroker{val: "s3cr3t"}),
		Hooks:       bp.NewRunbookHookRunner(&runbook.Executor{Registry: reg}),
		Store:       bp.NewMemoryAppliedStore(),
	}
	return ex, rr, &hookSteps
}

func has(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// TestE2E_ProductionCluster_ApplyHooksRollback is the headline
// acceptance: apply production-cluster with a secret-sourced
// password, the preflight hook runs as a runbook, every declaration
// reaches the runner, the secret is substituted into rendered state,
// and rollback reverts.
func TestE2E_ProductionCluster_ApplyHooksRollback(t *testing.T) {
	ex, rr, hookSteps := newStack(t)
	m, err := bp.Load(filepath.Join(catalog, "production-cluster"))
	if err != nil {
		t.Fatalf("Load production-cluster: %v", err)
	}

	res, err := ex.Apply(context.Background(), m, bp.ApplyOptions{
		Inputs: map[string]string{
			"postgres_password": "secret://kv/db",
			"cluster_name":      "test",
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "succeeded" {
		t.Fatalf("status=%s", res.Status)
	}

	// Preflight hook (a runbook with 2 noop steps) actually ran.
	if *hookSteps < 2 {
		t.Fatalf("preflight hook did not run end to end (hookSteps=%d)", *hookSteps)
	}

	// Every production-cluster declaration was applied.
	ids := rr.ids()
	for _, want := range []string{
		"package:etcd", "package:postgresql", "package:nats-server",
		"sysctl:net.core.somaxconn",
		"file:/etc/keystone/postgres.env", "file:/etc/keystone/nats-jetstream.conf",
		"file:/etc/keystone/cluster.marker",
		"service:etcd", "service:postgresql", "service:nats-server",
	} {
		if !has(ids, want) {
			t.Errorf("declaration %q not applied; got %v", want, ids)
		}
	}

	// Secret substituted into rendered state — cleartext present,
	// the secret:// reference gone.
	var pgEnv *statemgmt.Declaration
	for _, d := range rr.decls {
		if d.ID == "file:/etc/keystone/postgres.env" {
			pgEnv = d
		}
	}
	if pgEnv == nil {
		t.Fatal("postgres.env declaration missing")
	}
	content, _ := pgEnv.Params["content"].(string)
	if !strings.Contains(content, "s3cr3t") || strings.Contains(content, "secret://") {
		t.Fatalf("secret not substituted into rendered state: %q", content)
	}

	// Rollback reverts via entrypoints.rollback.
	rb, err := ex.Rollback(context.Background(), res.RunID)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rb.Status != "succeeded" {
		t.Fatalf("rollback status=%s", rb.Status)
	}
}

// TestE2E_Demo_Apply backs acceptance #1 (apply demo succeeds).
func TestE2E_Demo_Apply(t *testing.T) {
	ex, rr, _ := newStack(t)
	m, err := bp.Load(filepath.Join(catalog, "demo"))
	if err != nil {
		t.Fatalf("Load demo: %v", err)
	}
	res, err := ex.Apply(context.Background(), m, bp.ApplyOptions{})
	if err != nil || res.Status != "succeeded" {
		t.Fatalf("apply demo: status=%v err=%v", res.Status, err)
	}
	if res.Outputs["summary"] != "deployed keystone-demo (nginx)" {
		t.Fatalf("demo output=%v", res.Outputs)
	}
	if !has(rr.ids(), "file:/etc/keystone-demo.deployed") {
		t.Fatalf("demo marker not applied: %v", rr.ids())
	}
}

// TestE2E_Runbook_TemplatingAndOnFailure backs the runbook
// acceptance lines: step 2 reads step 1's output, and a step failure
// triggers the onFailure chain.
func TestE2E_Runbook_TemplatingAndOnFailure(t *testing.T) {
	reg := runbook.NewRegistry()
	_ = reg.Register("produce", runbook.StepFunc(func(context.Context, runbook.StepContext) (runbook.StepOutput, error) {
		return runbook.StepOutput{Outputs: map[string]any{"pid": "42"}}, nil
	}))
	var consumed string
	_ = reg.Register("consume", runbook.StepFunc(func(_ context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
		consumed, _ = sc.Config["from"].(string)
		return runbook.StepOutput{}, nil
	}))
	var cleaned bool
	_ = reg.Register("boom", runbook.StepFunc(func(context.Context, runbook.StepContext) (runbook.StepOutput, error) {
		return runbook.StepOutput{}, errors.New("kaboom")
	}))
	_ = reg.Register("cleanup", runbook.StepFunc(func(context.Context, runbook.StepContext) (runbook.StepOutput, error) {
		cleaned = true
		return runbook.StepOutput{}, nil
	}))
	e := &runbook.Executor{Registry: reg}

	rbk := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "tmpl"},
		Spec: runbook.Spec{Steps: []runbook.Step{
			{Type: "produce", Name: "s1"},
			{Type: "consume", Name: "s2", DependsOn: []string{"s1"},
				Config: map[string]any{"from": "{{ .steps.s1.outputs.pid }}"}},
		}},
	}
	exec, err := e.Execute(context.Background(), rbk, nil)
	if err != nil || exec.Status != runbook.StatusSucceeded {
		t.Fatalf("templating runbook: status=%v err=%v", exec.Status, err)
	}
	if consumed != "42" {
		t.Fatalf("step 2 did not read step 1 output: %q", consumed)
	}
	if len(exec.Trail) == 0 {
		t.Fatal("execution has no audit trail")
	}

	failRB := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "fail"},
		Spec: runbook.Spec{
			OnFailure: []string{"cleanup"},
			Steps: []runbook.Step{
				{Type: "boom", Name: "b"},
				{Type: "cleanup", Name: "cleanup"},
			},
		},
	}
	if _, err := e.Execute(context.Background(), failRB, nil); !errors.Is(err, runbook.ErrExecutionFailed) {
		t.Fatalf("expected ErrExecutionFailed, got %v", err)
	}
	if !cleaned {
		t.Fatal("onFailure chain did not run")
	}
}

// TestE2E_Saga_Compensation backs the saga acceptance: steps 1-3
// succeed, step 4 fails, 3→2→1 compensate in reverse.
func TestE2E_Saga_Compensation(t *testing.T) {
	var order []string
	mk := func(name string, fail bool) saga.Step {
		return saga.Step{
			Name: name,
			Action: func(_ context.Context, d any) (any, error) {
				if fail {
					return d, errors.New(name + " failed")
				}
				return d, nil
			},
			Compensate: func(context.Context, any) error {
				order = append(order, name)
				return nil
			},
		}
	}
	c := &saga.Coordinator{}
	exec, err := c.Run(context.Background(), "rollout", []saga.Step{
		mk("s1", false), mk("s2", false), mk("s3", false), mk("s4", true),
	}, nil)
	if err == nil {
		t.Fatal("expected saga failure")
	}
	if exec.Status != saga.StatusFailed {
		t.Fatalf("saga status=%v want failed", exec.Status)
	}
	want := []string{"s3", "s2", "s1"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("compensation order=%v want %v", order, want)
	}
}
