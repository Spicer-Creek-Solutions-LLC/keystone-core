// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/identity"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// newVerifyProvider builds a started EmbeddedProvider sufficient for SVID
// issuance + trust-bundle lookup (no attestors/join-tokens needed here).
func newVerifyProvider(t *testing.T) *identity.EmbeddedProvider {
	t.Helper()
	caStorage, err := identity.NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	p, err := identity.NewEmbeddedProvider(identity.EmbeddedProviderConfig{
		CAConfig:        identity.DefaultCAConfig(identity.DefaultTrustDomain),
		Storage:         caStorage,
		RotatorInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("provider.Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	})
	return p
}

func issueAgentChainPEM(t *testing.T, p *identity.EmbeddedProvider, agentID string) string {
	t.Helper()
	id, err := identity.AgentID(p.TrustDomain(), agentID)
	if err != nil {
		t.Fatalf("AgentID: %v", err)
	}
	svid, err := p.IssueX509SVID(context.Background(), identity.IssueX509SVIDRequest{ID: id, TTL: 30 * time.Minute})
	if err != nil {
		t.Fatalf("IssueX509SVID: %v", err)
	}
	var b strings.Builder
	for _, c := range svid.Chain() {
		if err := pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}); err != nil {
			t.Fatalf("pem.Encode: %v", err)
		}
	}
	return b.String()
}

func newVerifyServer(t *testing.T, store state.Store, verifier controlplane.CertVerifier) *controlplane.GRPCServer {
	t.Helper()
	disp, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{Store: store})
	if err != nil {
		t.Fatalf("NewBatchDispatcher: %v", err)
	}
	srv, err := controlplane.NewGRPCServer(controlplane.GRPCServerConfig{
		Dispatcher: disp,
		Store:      store,
		Verifier:   verifier,
	})
	if err != nil {
		t.Fatalf("NewGRPCServer: %v", err)
	}
	return srv
}

func storeAgent(t *testing.T, store state.Store, id, chainPEM string) {
	t.Helper()
	rec := &state.AgentRecord{
		ID:           id,
		Hostname:     "host-" + id,
		OS:           "linux",
		Architecture: "amd64",
		IPAddresses:  []string{},
		Labels:       map[string]string{},
		Status:       state.AgentStatusConnected,
		RegisteredAt: time.Now().UTC(),
		CertChainPEM: chainPEM,
	}
	if err := store.CreateAgent(context.Background(), rec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
}

func TestVerifyAgent_OK(t *testing.T) {
	provider := newVerifyProvider(t)
	store := newTestStore(t)
	storeAgent(t, store, "agent-1", issueAgentChainPEM(t, provider, "agent-1"))
	srv := newVerifyServer(t, store, provider)

	resp, err := srv.VerifyAgent(context.Background(), &v1.VerifyAgentRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("VerifyAgent: %v", err)
	}
	if !resp.GetHasCert() || !resp.GetOk() || !resp.GetChainValid() || !resp.GetSpiffeMatch() {
		t.Fatalf("unexpected verdict: %+v", resp)
	}
	if resp.GetExpired() || resp.GetExpiresAt() == nil {
		t.Errorf("expiry fields wrong: expired=%v expiresAt=%v", resp.GetExpired(), resp.GetExpiresAt())
	}
}

func TestVerifyAgent_NoStoredCert(t *testing.T) {
	provider := newVerifyProvider(t)
	store := newTestStore(t)
	storeAgent(t, store, "agent-2", "") // no cert
	srv := newVerifyServer(t, store, provider)

	resp, err := srv.VerifyAgent(context.Background(), &v1.VerifyAgentRequest{AgentId: "agent-2"})
	if err != nil {
		t.Fatalf("VerifyAgent: %v", err)
	}
	if resp.GetHasCert() || resp.GetOk() {
		t.Errorf("expected has_cert=false ok=false, got %+v", resp)
	}
}

func TestVerifyAgent_NotFound(t *testing.T) {
	provider := newVerifyProvider(t)
	srv := newVerifyServer(t, newTestStore(t), provider)
	_, err := srv.VerifyAgent(context.Background(), &v1.VerifyAgentRequest{AgentId: "ghost"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

func TestVerifyAgent_EmptyID(t *testing.T) {
	provider := newVerifyProvider(t)
	srv := newVerifyServer(t, newTestStore(t), provider)
	_, err := srv.VerifyAgent(context.Background(), &v1.VerifyAgentRequest{AgentId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestVerifyAgent_NilVerifierUnimplemented(t *testing.T) {
	srv := newVerifyServer(t, newTestStore(t), nil)
	_, err := srv.VerifyAgent(context.Background(), &v1.VerifyAgentRequest{AgentId: "agent-1"})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("code = %v, want Unimplemented", status.Code(err))
	}
}
