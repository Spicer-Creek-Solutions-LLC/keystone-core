// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newLazy(t *testing.T, store *CredentialStore) (*LazySecretClient, *fakeNATSClient) {
	t.Helper()
	nats := newFakeNATS()
	l, err := NewLazySecretClient(LazySecretClientConfig{
		Store:    store,
		NATS:     nats,
		Subjects: fakeSubjects{cluster: "test"},
		Timeout:  50 * time.Millisecond,
		Logger:   testLogger(),
	})
	if err != nil {
		t.Fatalf("NewLazySecretClient: %v", err)
	}
	t.Cleanup(func() { _ = l.Stop() })
	return l, nats
}

// Construction must not touch the store: on a first boot there is no
// credential yet, and failing here would disable secret rendering on
// exactly the run that enrolled the host.
func TestLazySecretClient_ConstructsWithoutACredential(t *testing.T) {
	store := &CredentialStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	l, nats := newLazy(t, store)

	if nats.subscribed(fakeSubjects{cluster: "test"}.SecretResponse("agent-1")) {
		t.Error("constructing the lazy client subscribed to NATS")
	}
	_ = l
}

func TestLazySecretClient_NoCredentialYet(t *testing.T) {
	store := &CredentialStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	l, _ := newLazy(t, store)

	_, err := l.Lookup(context.Background(), "app/db", "password")
	if err == nil {
		t.Fatal("Lookup() error = nil with no credential")
	}
	if !strings.Contains(err.Error(), "bootstrap") {
		t.Errorf("error = %v, want it to point at bootstrap", err)
	}
}

// An API-key-only credential means the control plane runs with
// identity disabled. The message should say that, not report a
// signing failure the operator would go hunting for in the agent.
func TestLazySecretClient_APIKeyOnlyCredential(t *testing.T) {
	store := &CredentialStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if err := store.Save(&Credentials{APIKey: "k", AgentID: "agent-1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	l, _ := newLazy(t, store)

	_, err := l.Lookup(context.Background(), "app/db", "password")
	if err == nil {
		t.Fatal("Lookup() error = nil for an API-key-only credential")
	}
	if !strings.Contains(err.Error(), "identity.enabled") {
		t.Errorf("error = %v, want it to name identity.enabled", err)
	}
}

func TestLazySecretClient_BuildsOnFirstUse(t *testing.T) {
	ca := newTestCA(t)
	store := &CredentialStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if err := store.Save(ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{})); err != nil {
		t.Fatalf("seed: %v", err)
	}
	subj := fakeSubjects{cluster: "test"}
	l, nats := newLazy(t, store)

	// Times out (nothing answers), but the client must have been built
	// and subscribed on the way.
	if _, err := l.Lookup(context.Background(), "app/db", "password"); err == nil {
		t.Fatal("expected a timeout")
	}
	if !nats.subscribed(subj.SecretResponse("agent-1")) {
		t.Error("first Lookup did not subscribe to the response subject")
	}
	if n := len(nats.captured(subj.SecretRequest())); n != 1 {
		t.Errorf("published %d requests, want 1", n)
	}
}

// The client is built once, not per lookup: a second Lookup must not
// subscribe again.
func TestLazySecretClient_BuildsOnce(t *testing.T) {
	ca := newTestCA(t)
	store := &CredentialStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if err := store.Save(ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{})); err != nil {
		t.Fatalf("seed: %v", err)
	}
	subj := fakeSubjects{cluster: "test"}
	l, nats := newLazy(t, store)

	for i := 0; i < 2; i++ {
		if _, err := l.Lookup(context.Background(), "app/db", "password"); err == nil {
			t.Fatal("expected a timeout")
		}
	}
	subs := 0
	for _, s := range nats.subjects {
		if s == subj.SecretResponse("agent-1") {
			subs++
		}
	}
	if subs != 1 {
		t.Errorf("subscribed %d times, want 1", subs)
	}
	if n := len(nats.captured(subj.SecretRequest())); n != 2 {
		t.Errorf("published %d requests, want 2 (one per lookup)", n)
	}
}

func TestLazySecretClient_StopIsSafeBeforeUse(t *testing.T) {
	store := &CredentialStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	l, _ := newLazy(t, store)
	if err := l.Stop(); err != nil {
		t.Errorf("Stop before any lookup: %v", err)
	}
	if err := l.Stop(); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestNewLazySecretClient_Requires(t *testing.T) {
	full := LazySecretClientConfig{
		Store:    &CredentialStore{Path: "/tmp/x"},
		NATS:     newFakeNATS(),
		Subjects: fakeSubjects{cluster: "test"},
	}
	tests := map[string]func(*LazySecretClientConfig){
		"store":    func(c *LazySecretClientConfig) { c.Store = nil },
		"nats":     func(c *LazySecretClientConfig) { c.NATS = nil },
		"subjects": func(c *LazySecretClientConfig) { c.Subjects = nil },
	}
	for name, drop := range tests {
		t.Run("missing "+name, func(t *testing.T) {
			cfg := full
			drop(&cfg)
			if _, err := NewLazySecretClient(cfg); err == nil {
				t.Errorf("NewLazySecretClient() error = nil without %s", name)
			}
		})
	}
}

// The engine must fail a state file that references a secret when no
// resolver is configured, rather than rendering a blank value.
func TestStateEngine_SecretWithoutResolver(t *testing.T) {
	e := &StateEngine{Registry: engineRegistry(t)}
	yaml := []byte("metadata:\n  name: s\n  version: \"1.0\"\n\nfile:\n  " +
		filepath.Join(t.TempDir(), "app.env") +
		":\n    state: present\n    content: \"P={{ secret \\\"app/db\\\" \\\"password\\\" }}\"\n")

	_, err := e.Converge(context.Background(), ConvergeModeApply, yaml, nil, map[string]any{})
	if err == nil {
		t.Fatal("Converge() error = nil for a secret reference with no resolver")
	}
	if !strings.Contains(err.Error(), "not available here") {
		t.Errorf("error = %v, want the unavailable-resolver message", err)
	}
}

// With a resolver, the value lands in the rendered declaration.
func TestStateEngine_SecretIsRendered(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app.env")
	e := &StateEngine{Registry: engineRegistry(t), Secrets: stubResolver{value: "s3cret"}}
	yaml := []byte("metadata:\n  name: s\n  version: \"1.0\"\n\nfile:\n  " + target +
		":\n    state: present\n    content: \"P={{ secret \\\"app/db\\\" \\\"password\\\" }}\\n\"\n")

	report, err := e.Converge(context.Background(), ConvergeModeApply, yaml, nil, map[string]any{})
	if err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if report.Failed != 0 {
		t.Fatalf("failed = %d, want 0: %+v", report.Failed, report.Results)
	}
	got, err := readFileString(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if got != "P=s3cret\n" {
		t.Errorf("file content = %q, want %q", got, "P=s3cret\n")
	}
}

func readFileString(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

type stubResolver struct {
	value string
	err   error
}

func (s stubResolver) Lookup(context.Context, string, string) (string, error) {
	return s.value, s.err
}
