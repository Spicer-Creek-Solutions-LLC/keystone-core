// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// newBootstrapAgent builds an agent with a PSK and a credential store
// under t.TempDir(), which is the shape every test here needs.
func newBootstrapAgent(t *testing.T) (*Agent, *fakeNATSClient, fakeSubjects, *CredentialStore) {
	t.Helper()
	a, nats, subj, _ := newAgentWithEnforcer(t, SecurityPolicy{
		HMACSecret:    []byte("secret-1"),
		DefaultPolicy: PolicyAllow,
	})
	store := &CredentialStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	a.cfg.BootstrapPSK = "1111"
	a.creds = store
	return a, nats, subj, store
}

func deliverCredentials(t *testing.T, nats *fakeNATSClient, subj fakeSubjects, c Credentials) {
	t.Helper()
	body, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("bootstrap-1"))
	if err := nats.deliver(t, subj.BootstrapResponse("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}
}

func TestAgent_BootstrapResponse_StoresCredentials(t *testing.T) {
	a, nats, subj, store := newBootstrapAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	chain := testCertPEM(t, time.Now().Add(time.Hour))
	deliverCredentials(t, nats, subj, Credentials{
		APIKey:         "issued-key",
		AgentID:        "agent-1",
		IssuedAt:       time.Now(),
		CertChainPEM:   chain,
		PrivateKeyPEM:  "-----BEGIN PRIVATE KEY-----\nk\n-----END PRIVATE KEY-----\n",
		TrustBundlePEM: "-----BEGIN CERTIFICATE-----\nt\n-----END CERTIFICATE-----\n",
	})

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.APIKey != "issued-key" {
		t.Errorf("APIKey = %q, want %q", got.APIKey, "issued-key")
	}
	if !got.HasSVID() {
		t.Error("stored credentials lost the SVID")
	}
	if got.CertChainPEM != chain {
		t.Error("stored cert chain does not match what was delivered")
	}
}

// The register publish must not go out before the response
// subscription exists: the PSK is single-use, so a reply that lands
// with nobody listening strands the agent permanently.
func TestAgent_Start_SubscribesResponseBeforePublishingRegister(t *testing.T) {
	a, nats, subj, _ := newBootstrapAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	if nats.publishCount(subj.BootstrapRegister("agent-1")) != 1 {
		t.Fatal("Start did not publish the bootstrap register")
	}
	// The fake records handlers at Subscribe time, so a handler being
	// present means the subscription preceded the publish above.
	if !nats.hasHandler(subj.BootstrapResponse("agent-1")) {
		t.Error("no handler on the bootstrap response subject after Start")
	}
}

func TestAgent_Start_SkipsRegisterWhenCredentialed(t *testing.T) {
	a, nats, subj, store := newBootstrapAgent(t)
	if err := store.Save(&Credentials{
		APIKey:        "already-issued",
		AgentID:       "agent-1",
		CertChainPEM:  testCertPEM(t, time.Now().Add(time.Hour)),
		PrivateKeyPEM: "k",
	}); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	if n := nats.publishCount(subj.BootstrapRegister("agent-1")); n != 0 {
		t.Errorf("bootstrap register published %d times, want 0 when already credentialed", n)
	}
}

func TestAgent_Start_ReBootstrapsOnExpiredCredentials(t *testing.T) {
	a, nats, subj, store := newBootstrapAgent(t)
	if err := store.Save(&Credentials{
		APIKey:        "stale",
		AgentID:       "agent-1",
		CertChainPEM:  testCertPEM(t, time.Now().Add(-time.Hour)),
		PrivateKeyPEM: "k",
	}); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	if n := nats.publishCount(subj.BootstrapRegister("agent-1")); n != 1 {
		t.Errorf("bootstrap register published %d times, want 1 for an expired credential", n)
	}
}

func TestAgent_BootstrapResponse_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		creds   *Credentials
	}{
		{name: "malformed json", payload: []byte("{not json")},
		{name: "wrong agent", creds: &Credentials{APIKey: "k", AgentID: "agent-2"}},
		{name: "no api key", creds: &Credentials{AgentID: "agent-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, nats, subj, store := newBootstrapAgent(t)
			if err := a.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

			payload := tt.payload
			if payload == nil {
				var err error
				payload, err = json.Marshal(tt.creds)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
			}
			env := envelope.New(payload, subj.Prefix(), envelope.WithMessageID("bootstrap-1"))
			// A rejected response is dropped, not errored: returning an
			// error here would make NATS redeliver a payload that will
			// never become valid.
			if err := nats.deliver(t, subj.BootstrapResponse("agent-1"), env); err != nil {
				t.Fatalf("deliver returned an error: %v", err)
			}
			if _, err := store.Load(); !errors.Is(err, ErrNoCredentials) {
				t.Errorf("credentials were stored for a rejected response (err = %v)", err)
			}
		})
	}
}

func TestAgent_BootstrapResponse_NoStoreConfigured(t *testing.T) {
	a, nats, subj, _ := newBootstrapAgent(t)
	a.creds = nil
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	// Must degrade to a warning, not a crash — an agent with no
	// configured path still has to run.
	deliverCredentials(t, nats, subj, Credentials{APIKey: "k", AgentID: "agent-1"})
}

func TestAgent_HaveValidCredentials(t *testing.T) {
	t.Run("no store", func(t *testing.T) {
		a, _, _, _ := newBootstrapAgent(t)
		a.creds = nil
		if a.haveValidCredentials() {
			t.Error("haveValidCredentials() = true with no store")
		}
	})

	t.Run("corrupt file re-bootstraps", func(t *testing.T) {
		a, _, _, store := newBootstrapAgent(t)
		if err := os.WriteFile(store.Path, []byte("{broken"), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if a.haveValidCredentials() {
			t.Error("haveValidCredentials() = true for an unreadable file")
		}
	})

	t.Run("api-key-only credential counts", func(t *testing.T) {
		a, _, _, store := newBootstrapAgent(t)
		if err := store.Save(&Credentials{APIKey: "k", AgentID: "agent-1"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if !a.haveValidCredentials() {
			t.Error("haveValidCredentials() = false for a valid API-key-only credential")
		}
	})
}
