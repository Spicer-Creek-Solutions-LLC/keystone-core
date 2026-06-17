// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
)

// TestEnable_RestoresFromDisabled covers the unquarantine path: a
// disabled agent is re-admitted to PENDING, after which a heartbeat is
// accepted again and promotes it to CONNECTED.
func TestEnable_RestoresFromDisabled(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	if err := mgr.Register(ctx, newAgent("a1")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Disable(ctx, "a1"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if err := mgr.Enable(ctx, "a1"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	// Re-admitted to CONNECTED, and heartbeats are accepted again
	// (no longer ErrAgentDisabled).
	rec, err := store.GetAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if rec.Status != state.AgentStatusConnected {
		t.Fatalf("status = %v, want Connected", rec.Status)
	}
	if err := mgr.Heartbeat(ctx, "a1", ""); err != nil {
		t.Fatalf("Heartbeat after Enable: %v", err)
	}
}

// TestEnable_NotRegistered returns ErrNotRegistered for an unknown agent.
func TestEnable_NotRegistered(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)

	if err := mgr.Enable(ctx, "ghost"); err != controlplane.ErrNotRegistered {
		t.Fatalf("err = %v, want ErrNotRegistered", err)
	}
}
