// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// fakeController records calls and returns scripted errors, so the
// quarantine RPC behaviour can be tested without a live ConnectionManager.
type fakeController struct {
	disableErr, enableErr error
	disabledID, enabledID string
}

func (f *fakeController) Disable(_ context.Context, id string) error {
	f.disabledID = id
	return f.disableErr
}

func (f *fakeController) Enable(_ context.Context, id string) error {
	f.enabledID = id
	return f.enableErr
}

func newQuarantineServer(t *testing.T, ctrl controlplane.AgentController) *controlplane.GRPCServer {
	t.Helper()
	store := newTestStore(t)
	disp, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{Store: store})
	if err != nil {
		t.Fatalf("NewBatchDispatcher: %v", err)
	}
	srv, err := controlplane.NewGRPCServer(controlplane.GRPCServerConfig{
		Dispatcher: disp,
		Store:      store,
		Controller: ctrl,
	})
	if err != nil {
		t.Fatalf("NewGRPCServer: %v", err)
	}
	return srv
}

func TestQuarantineAgent_Success(t *testing.T) {
	t.Parallel()
	fc := &fakeController{}
	srv := newQuarantineServer(t, fc)
	resp, err := srv.QuarantineAgent(context.Background(),
		&v1.QuarantineAgentRequest{AgentId: "a1", Reason: "compromised"})
	if err != nil {
		t.Fatalf("QuarantineAgent: %v", err)
	}
	if fc.disabledID != "a1" {
		t.Errorf("Disable called with %q, want a1", fc.disabledID)
	}
	if resp.GetStatus() != v1.AgentStatus_AGENT_STATUS_DISABLED {
		t.Errorf("status = %v, want DISABLED", resp.GetStatus())
	}
}

func TestUnquarantineAgent_Success(t *testing.T) {
	t.Parallel()
	fc := &fakeController{}
	srv := newQuarantineServer(t, fc)
	resp, err := srv.UnquarantineAgent(context.Background(),
		&v1.UnquarantineAgentRequest{AgentId: "a1"})
	if err != nil {
		t.Fatalf("UnquarantineAgent: %v", err)
	}
	if fc.enabledID != "a1" {
		t.Errorf("Enable called with %q, want a1", fc.enabledID)
	}
	if resp.GetStatus() != v1.AgentStatus_AGENT_STATUS_CONNECTED {
		t.Errorf("status = %v, want CONNECTED", resp.GetStatus())
	}
}

func TestQuarantineAgent_NotFound(t *testing.T) {
	t.Parallel()
	srv := newQuarantineServer(t, &fakeController{disableErr: controlplane.ErrNotRegistered})
	_, err := srv.QuarantineAgent(context.Background(),
		&v1.QuarantineAgentRequest{AgentId: "ghost"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

func TestQuarantineAgent_NilControllerUnimplemented(t *testing.T) {
	t.Parallel()
	srv := newQuarantineServer(t, nil)
	_, err := srv.QuarantineAgent(context.Background(),
		&v1.QuarantineAgentRequest{AgentId: "a1"})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("code = %v, want Unimplemented", status.Code(err))
	}
}

func TestQuarantineAgent_EmptyID(t *testing.T) {
	t.Parallel()
	fc := &fakeController{}
	srv := newQuarantineServer(t, fc)
	_, err := srv.QuarantineAgent(context.Background(),
		&v1.QuarantineAgentRequest{AgentId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if fc.disabledID != "" {
		t.Errorf("Disable should not be called on empty id, got %q", fc.disabledID)
	}
}

// compile-time assertion that *ConnectionManager satisfies the interface
// the operator server depends on.
var _ controlplane.AgentController = (*controlplane.ConnectionManager)(nil)
