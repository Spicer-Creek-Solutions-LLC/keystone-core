// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"errors"
	"os"
	"testing"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type stubCatalog struct {
	m   *bp.Manifest
	err error
}

func (s stubCatalog) Get(context.Context, string) (*bp.Manifest, error) { return s.m, s.err }

// recordingLocal notes that the LOCAL path ran, which is what
// distinguishes an untargeted apply from a targeted one.
type recordingLocal struct{ ran bool }

func (r *recordingLocal) Run(context.Context, []*statemgmt.Declaration) (*statemgmt.RunReport, error) {
	r.ran = true
	return &statemgmt.RunReport{}, nil
}

// minimalManifest is the smallest blueprint the executor will apply:
// one entrypoint writing one file.
func minimalManifest(t *testing.T) *bp.Manifest {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir+"/apply.yaml", "file:\n  /tmp/bp-test.conf:\n    state: present\n")
	return &bp.Manifest{
		Metadata:    bp.Metadata{Name: "demo", Version: "1.0"},
		SourcePath:  dir,
		Entrypoints: bp.Entrypoints{Default: "apply.yaml"},
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// An untargeted apply converges the control-plane host.
func TestBlueprintApplier_NoAgentsRunsLocally(t *testing.T) {
	local := &recordingLocal{}
	f := &fanoutStub{respond: okResponse}
	a := &controlplane.BlueprintApplier{
		Catalog: stubCatalog{m: minimalManifest(t)}, Local: local, Converge: f,
	}

	if _, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !local.ran {
		t.Error("an untargeted apply did not use the local runner")
	}
	if len(f.seen) != 0 {
		t.Errorf("an untargeted apply reached %d agents, want 0", len(f.seen))
	}
}

// A targeted apply sends the rendered file to the agents and must not
// touch the local runner -- converging the control plane when the
// operator named other hosts is the failure this guards.
func TestBlueprintApplier_AgentsRunRemotely(t *testing.T) {
	local := &recordingLocal{}
	f := &fanoutStub{respond: okResponse}
	a := &controlplane.BlueprintApplier{
		Catalog: stubCatalog{m: minimalManifest(t)}, Local: local, Converge: f,
	}

	_, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{
		Agents: []string{"agent-1", "agent-2"},
		Target: "id:agent-1,agent-2",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if local.ran {
		t.Error("a targeted apply used the LOCAL runner; it converged the wrong host")
	}
	if len(f.seen) != 2 {
		t.Errorf("reached %d agents, want 2", len(f.seen))
	}
	// Each agent must receive a state FILE it can parse itself.
	for id, body := range f.seen {
		if _, err := statemgmt.Parse(body); err != nil {
			t.Errorf("%s received something that is not a parseable state file: %v", id, err)
		}
	}
}

// The run record must survive for rollback to have something to target.
func TestBlueprintApplier_RecordsTargetAndAgents(t *testing.T) {
	store := bp.NewMemoryAppliedStore()
	a := &controlplane.BlueprintApplier{
		Catalog:  stubCatalog{m: minimalManifest(t)},
		Converge: &fanoutStub{respond: okResponse},
		Store:    store,
	}

	res, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{
		Agents: []string{"agent-2", "agent-1"},
		Target: "role:web",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	run, err := store.Get(context.Background(), res.RunID)
	if err != nil {
		t.Fatalf("Get run: %v", err)
	}
	if run.Target != "role:web" {
		t.Errorf("recorded Target = %q, want %q", run.Target, "role:web")
	}
	if len(run.Agents) != 2 {
		t.Fatalf("recorded Agents = %v, want two", run.Agents)
	}
}

func TestBlueprintApplier_Unwired(t *testing.T) {
	t.Run("no catalog", func(t *testing.T) {
		a := &controlplane.BlueprintApplier{}
		if _, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{}); !errors.Is(err, controlplane.ErrNoCatalog) {
			t.Errorf("error = %v, want ErrNoCatalog", err)
		}
	})

	// A targeted apply with no converge path must fail, never fall
	// back to the local host.
	t.Run("targeted with no converge path", func(t *testing.T) {
		local := &recordingLocal{}
		a := &controlplane.BlueprintApplier{Catalog: stubCatalog{m: minimalManifest(t)}, Local: local}
		_, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{Agents: []string{"agent-1"}})
		if !errors.Is(err, controlplane.ErrRemoteApplyDisabled) {
			t.Errorf("error = %v, want ErrRemoteApplyDisabled", err)
		}
		if local.ran {
			t.Error("a targeted apply fell back to the local runner")
		}
	})

	t.Run("untargeted with no local runner", func(t *testing.T) {
		a := &controlplane.BlueprintApplier{
			Catalog:  stubCatalog{m: minimalManifest(t)},
			Converge: &fanoutStub{respond: okResponse},
		}
		_, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{})
		if !errors.Is(err, controlplane.ErrLocalApplyDisabled) {
			t.Errorf("error = %v, want ErrLocalApplyDisabled", err)
		}
	})

	t.Run("catalog lookup failure", func(t *testing.T) {
		a := &controlplane.BlueprintApplier{Catalog: stubCatalog{err: errors.New("no such blueprint")}}
		if _, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{}); err == nil {
			t.Error("error = nil for a catalog miss")
		}
	})
}

// A rollback must reach the hosts that received the apply, taken from
// the run record. Re-resolving the original target could reach an
// agent that joined afterwards, or miss one whose labels changed.
func TestBlueprintApplier_RollbackUsesRecordedAgents(t *testing.T) {
	store := bp.NewMemoryAppliedStore()
	f := &fanoutStub{respond: okResponse}
	local := &recordingLocal{}
	a := &controlplane.BlueprintApplier{
		Catalog: stubCatalog{m: rollbackManifest(t)}, Converge: f, Local: local, Store: store,
	}

	applied, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{
		Agents: []string{"agent-1", "agent-2"}, Target: "role:web",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	f.seen = nil // forget the apply's traffic

	if _, err := a.Rollback(context.Background(), applied.RunID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if local.ran {
		t.Error("a rollback of a fleet apply ran on the LOCAL host")
	}
	if len(f.seen) != 2 {
		t.Fatalf("rollback reached %d agents, want the same 2 as the apply", len(f.seen))
	}
	for _, id := range []string{"agent-1", "agent-2"} {
		if _, ok := f.seen[id]; !ok {
			t.Errorf("%s did not receive the rollback", id)
		}
	}
}

// A locally-applied run rolls back locally.
func TestBlueprintApplier_RollbackOfLocalRunStaysLocal(t *testing.T) {
	store := bp.NewMemoryAppliedStore()
	f := &fanoutStub{respond: okResponse}
	local := &recordingLocal{}
	a := &controlplane.BlueprintApplier{
		Catalog: stubCatalog{m: rollbackManifest(t)}, Converge: f, Local: local, Store: store,
	}

	applied, err := a.Apply(context.Background(), "demo", bp.ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := a.Rollback(context.Background(), applied.RunID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(f.seen) != 0 {
		t.Errorf("a local run's rollback reached %d agents, want 0", len(f.seen))
	}
}

func TestBlueprintApplier_RollbackErrors(t *testing.T) {
	t.Run("no store", func(t *testing.T) {
		a := &controlplane.BlueprintApplier{Catalog: stubCatalog{m: rollbackManifest(t)}}
		if _, err := a.Rollback(context.Background(), "run-1"); !errors.Is(err, controlplane.ErrNoAppliedStore) {
			t.Errorf("error = %v, want ErrNoAppliedStore", err)
		}
	})

	t.Run("unknown run", func(t *testing.T) {
		a := &controlplane.BlueprintApplier{
			Catalog: stubCatalog{m: rollbackManifest(t)}, Store: bp.NewMemoryAppliedStore(),
		}
		if _, err := a.Rollback(context.Background(), "nope"); err == nil {
			t.Error("error = nil for an unknown run id")
		}
	})
}

// rollbackManifest writes a real blueprint directory and loads it.
//
// Executor.Rollback reloads the manifest from SourcePath, so a
// hand-built struct is not enough -- the fixture has to exist on disk
// the way a catalog entry does.
func rollbackManifest(t *testing.T) *bp.Manifest {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir+"/blueprint.yaml", `metadata:
  name: demo
  version: 1.0.0
  description: rollback fixture

entrypoints:
  default: apply.yaml
  rollback: rollback.yaml
`)
	writeFile(t, dir+"/apply.yaml", "file:\n  /tmp/bp-rb.conf:\n    state: present\n")
	writeFile(t, dir+"/rollback.yaml", "file:\n  /tmp/bp-rb.conf:\n    state: absent\n")
	m, err := bp.Load(dir)
	if err != nil {
		t.Fatalf("load fixture blueprint: %v", err)
	}
	return m
}
