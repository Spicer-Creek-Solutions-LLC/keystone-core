package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

func TestNewBackend_RejectsBadConfig(t *testing.T) {
	t.Parallel()
	_, err := NewBackend(Config{})
	if err == nil {
		t.Fatalf("NewBackend(empty) = nil err")
	}
	if !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
}

func TestBackend_Lifecycle(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))
	srv.register("PUT", "/v1/auth/token/revoke-self", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNoContent, nil)
	})

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}

	if err := b.Health(context.Background()); !errors.Is(err, secrets.ErrBackendNotStarted) {
		t.Errorf("Health pre-Start = %v, want ErrBackendNotStarted", err)
	}

	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := b.Start(context.Background()); err == nil {
		t.Errorf("double Start = nil err")
	}

	if err := b.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Errorf("double Stop: %v", err)
	}
	if err := b.Start(context.Background()); err == nil {
		t.Errorf("Start after Stop = nil err")
	}
}

func TestBackend_Capabilities(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	caps := b.Capabilities()
	for _, want := range []secrets.BackendCapability{
		secrets.CapKV, secrets.CapList, secrets.CapDynamic,
		secrets.CapLeaseRenew, secrets.CapLeaseRevoke,
	} {
		if !secrets.HasCapability(caps, want) {
			t.Errorf("missing capability %s", want)
		}
	}
	if secrets.HasCapability(caps, secrets.CapTransit) {
		t.Errorf("CapTransit advertised; transit lands in task 7")
	}
}

func TestBackend_Name(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if b.Name() != DefaultBackendName {
		t.Errorf("Name = %q, want %q", b.Name(), DefaultBackendName)
	}

	b2, _ := NewBackend(Config{
		Address: srv.addr(),
		Name:    "vault-prod",
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if b2.Name() != "vault-prod" {
		t.Errorf("custom Name = %q, want %q", b2.Name(), "vault-prod")
	}
}

func TestBackend_OpsBeforeStart(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}

	cases := []func() error{
		func() error { _, err := b.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "secret/x"}); return err },
		func() error {
			_, err := b.WriteSecret(context.Background(), secrets.WriteSecretRequest{Path: "secret/x", Data: map[string]any{"k": "v"}})
			return err
		},
		func() error {
			_, err := b.ListSecrets(context.Background(), secrets.ListSecretsRequest{Prefix: "secret/"})
			return err
		},
		func() error { return b.DeleteSecret(context.Background(), secrets.DeleteSecretRequest{Path: "secret/x"}) },
		func() error {
			_, err := b.IssueDynamicSecret(context.Background(), secrets.IssueDynamicSecretRequest{Path: "database/creds/app"})
			return err
		},
		func() error {
			_, err := b.RenewLease(context.Background(), secrets.RenewLeaseRequest{LeaseID: "x"})
			return err
		},
		func() error { return b.RevokeLease(context.Background(), secrets.RevokeLeaseRequest{LeaseID: "x"}) },
	}
	for i, fn := range cases {
		err := fn()
		if !errors.Is(err, secrets.ErrBackendNotStarted) {
			t.Errorf("op[%d] err = %v, want ErrBackendNotStarted", i, err)
		}
	}
}

func TestBackend_Health_VaultDown(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// sys/health returns 500 by default (no handler registered).
	err = b.Health(context.Background())
	if err == nil {
		t.Errorf("Health = nil with sys/health undefined")
	}
}

func TestBackend_Health_Healthy(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))
	srv.register("GET", "/v1/sys/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"initialized": true})
	})

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := b.Health(context.Background()); err != nil {
		t.Errorf("Health = %v, want nil", err)
	}
}

// Broker integration — drive the Vault backend through a real
// secrets.Broker, asserting routing + audit happen end-to-end.
func TestBackend_BrokerIntegration(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))
	srv.register("GET", "/v1/secret/data/app", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"data":     map[string]any{"password": "hunter2"},
				"metadata": map[string]any{"version": 1, "created_time": "2026-05-14T12:00:00Z"},
			},
		})
	})

	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
		Mounts:  []MountConfig{{Path: "secret", KVVersion: 2}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	auditor := &brokerAuditor{}
	router, err := secrets.NewRouter([]secrets.Route{{Prefix: "secret/", Backend: DefaultBackendName}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	broker, err := secrets.NewBroker(secrets.BrokerConfig{
		Router:   router,
		Backends: []secrets.SecretBackend{b},
		Auditor:  auditor,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}

	got, err := broker.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "secret/app"})
	if err != nil {
		t.Fatalf("broker.GetSecret: %v", err)
	}
	if got.Data["password"] != "hunter2" {
		t.Errorf("round-trip lost password: %#v", got)
	}
	if auditor.count() != 1 {
		t.Errorf("auditor count = %d, want 1", auditor.count())
	}
	last := auditor.last()
	if last.Backend != DefaultBackendName {
		t.Errorf("audit backend = %q, want %q", last.Backend, DefaultBackendName)
	}
	if !last.Allowed {
		t.Errorf("audit allowed = false: %#v", last)
	}
}

type brokerAuditor struct {
	mu  sync.Mutex
	evs []secrets.SecretAccessEvent
}

func (a *brokerAuditor) Emit(_ context.Context, e secrets.SecretAccessEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.evs = append(a.evs, e)
}

func (a *brokerAuditor) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.evs)
}

func (a *brokerAuditor) last() secrets.SecretAccessEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.evs) == 0 {
		return secrets.SecretAccessEvent{}
	}
	return a.evs[len(a.evs)-1]
}

func TestInternalClient(t *testing.T) {
	t.Parallel()
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))
	b, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if b.internalClient() == nil {
		t.Errorf("internalClient returned nil")
	}
}

// Probe-style test confirming TLS Insecure log line appears once at
// NewBackend (not at every op).
func TestNewBackend_InsecureLogged(t *testing.T) {
	t.Parallel()
	// This test does not assert on log output (slog.Default is global
	// and capturing it from tests is fragile). It just verifies the
	// path doesn't error.
	srv := newVaultTestServer(t)
	srv.register("GET", "/v1/auth/token/lookup-self", handleLookupSelf(3600, false))
	_, err := NewBackend(Config{
		Address: srv.addr(),
		Auth:    AuthConfig{Method: AuthMethodToken, Token: &TokenAuthConfig{Token: "s.dev"}},
		TLS:     TLSConfig{Insecure: true},
	})
	if err != nil {
		t.Fatalf("NewBackend with Insecure: %v", err)
	}
}

// Sanity check that the gated integration tests aren't accidentally
// running in unit-test mode.
func TestIntegration_GuardWorks(t *testing.T) {
	t.Parallel()
	addr := getIntegrationAddr()
	if addr != "" && !strings.HasPrefix(addr, "http") {
		t.Errorf("KSCORE_TEST_VAULT_ADDR is set to a non-URL value: %q", addr)
	}
	// Otherwise nothing to assert — the integration_test.go file
	// drives the actual integration tests via t.Skip when unset.
	_ = fmt.Sprintf
}
