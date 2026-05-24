// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
)

func seedResolverAgents(t *testing.T, store state.Store) {
	t.Helper()
	now := time.Now()
	agents := []*state.AgentRecord{
		{ID: "web-1", Hostname: "web-prod-1", OS: "linux", Architecture: "amd64",
			Labels: map[string]string{"role": "web", "env": "prod"},
			Status: state.AgentStatusConnected, RegisteredAt: now, LastHeartbeatAt: now},
		{ID: "web-2", Hostname: "web-prod-2", OS: "linux", Architecture: "amd64",
			Labels: map[string]string{"role": "web", "env": "prod"},
			Status: state.AgentStatusConnected, RegisteredAt: now, LastHeartbeatAt: now},
		{ID: "db-1", Hostname: "db-prod-1", OS: "linux", Architecture: "arm64",
			Labels: map[string]string{"role": "db", "env": "prod"},
			Status: state.AgentStatusConnected, RegisteredAt: now, LastHeartbeatAt: now},
		{ID: "web-3-dev", Hostname: "web-dev-3", OS: "linux", Architecture: "amd64",
			Labels: map[string]string{"role": "web", "env": "dev"},
			Status: state.AgentStatusConnected, RegisteredAt: now, LastHeartbeatAt: now},
	}
	for _, a := range agents {
		if err := store.CreateAgent(context.Background(), a); err != nil {
			t.Fatalf("seed %s: %v", a.ID, err)
		}
	}
}

func TestResolveTarget_Empty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	got, err := controlplane.ResolveTarget(context.Background(), store, controlplane.Target{})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("empty target: got %v, want nil", got)
	}
}

func TestResolveTarget_AgentIDsOnly(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedResolverAgents(t, store)
	got, err := controlplane.ResolveTarget(context.Background(), store,
		controlplane.Target{AgentIDs: []string{"web-1", "db-1", "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (web-1, db-1; missing ignored)", len(got))
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids["web-1"] || !ids["db-1"] {
		t.Errorf("got = %v, want web-1 + db-1", ids)
	}
}

func TestResolveTarget_LabelsAreANDed(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedResolverAgents(t, store)
	got, err := controlplane.ResolveTarget(context.Background(), store,
		controlplane.Target{Labels: map[string]string{"role": "web", "env": "prod"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (web-1, web-2)", len(got))
	}
}

func TestResolveTarget_HostnameGlob(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedResolverAgents(t, store)
	got, err := controlplane.ResolveTarget(context.Background(), store,
		controlplane.Target{HostnamePattern: "web-prod-*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
}

func TestResolveTarget_AllThreeDimensionsANDed(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedResolverAgents(t, store)
	got, err := controlplane.ResolveTarget(context.Background(), store,
		controlplane.Target{
			AgentIDs:        []string{"web-1", "web-2", "web-3-dev"},
			Labels:          map[string]string{"env": "prod"},
			HostnamePattern: "web-prod-*",
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (web-3-dev filtered out by env)", len(got))
	}
}

func TestResolveTarget_BadGlob(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, err := controlplane.ResolveTarget(context.Background(), store,
		controlplane.Target{HostnamePattern: "[unclosed"})
	if err == nil {
		t.Error("expected error for malformed glob")
	}
}

func TestResolveTarget_LabelMissing(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	seedResolverAgents(t, store)
	got, err := controlplane.ResolveTarget(context.Background(), store,
		controlplane.Target{Labels: map[string]string{"role": "cache"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got = %d, want 0 (no cache agents)", len(got))
	}
}
