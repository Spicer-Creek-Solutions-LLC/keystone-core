// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/sealed"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// secretClientFixture wires a client over the fake NATS client and
// returns the pieces a test needs to answer it.
type secretClientFixture struct {
	client *SecretClient
	nats   *fakeNATSClient
	subj   fakeSubjects
	key    *ecdsa.PrivateKey
}

func newSecretClientFixture(t *testing.T, timeout time.Duration) *secretClientFixture {
	t.Helper()
	ca := newTestCA(t)
	creds := ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{})
	signer, err := NewSVIDSigner(creds)
	if err != nil {
		t.Fatalf("NewSVIDSigner: %v", err)
	}
	block, _ := pem.Decode([]byte(creds.PrivateKeyPEM))
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("key is %T, want *ecdsa.PrivateKey", parsed)
	}

	nats := newFakeNATS()
	subj := fakeSubjects{cluster: "test"}
	c, err := NewSecretClient(SecretClientConfig{
		NATS: nats, Subjects: subj, Signer: signer, Key: key,
		Timeout: timeout, Logger: testLogger(),
	})
	if err != nil {
		t.Fatalf("NewSecretClient: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop() })
	return &secretClientFixture{client: c, nats: nats, subj: subj, key: key}
}

// awaitRequest waits for the client to publish, then returns the
// nonce it used.
func (f *secretClientFixture) awaitRequest(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		envs := f.nats.captured(f.subj.SecretRequest())
		if len(envs) > 0 {
			var signed SignedRequest
			if err := json.Unmarshal(envs[0].Payload, &signed); err != nil {
				t.Fatalf("unmarshal published request: %v", err)
			}
			return signed.Nonce
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("client never published a secret request")
	return ""
}

func (f *secretClientFixture) answer(t *testing.T, resp *SecretLookupResponse) {
	t.Helper()
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	env := envelope.New(body, f.subj.Prefix(), envelope.WithCorrelationID(resp.Nonce))
	if err := f.nats.deliver(t, f.subj.SecretResponse("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}
}

func TestSecretClient_Lookup(t *testing.T) {
	f := newSecretClientFixture(t, 5*time.Second)

	type result struct {
		value string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		v, err := f.client.Lookup(context.Background(), "app/db", "password")
		done <- result{v, err}
	}()

	nonce := f.awaitRequest(t)
	box, err := sealed.Seal(&f.key.PublicKey, []byte("s3cret"), SecretAAD("agent-1", nonce))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	f.answer(t, &SecretLookupResponse{Nonce: nonce, Box: box})

	got := <-done
	if got.err != nil {
		t.Fatalf("Lookup: %v", got.err)
	}
	if got.value != "s3cret" {
		t.Errorf("Lookup() = %q, want %q", got.value, "s3cret")
	}
}

func TestSecretClient_Denied(t *testing.T) {
	f := newSecretClientFixture(t, 5*time.Second)

	done := make(chan error, 1)
	go func() {
		_, err := f.client.Lookup(context.Background(), "app/db", "password")
		done <- err
	}()

	nonce := f.awaitRequest(t)
	f.answer(t, &SecretLookupResponse{Nonce: nonce, Denied: true, Error: "path not granted to this agent"})

	err := <-done
	if !errors.Is(err, ErrSecretDenied) {
		t.Fatalf("Lookup() error = %v, want ErrSecretDenied", err)
	}
}

func TestSecretClient_ServerError(t *testing.T) {
	f := newSecretClientFixture(t, 5*time.Second)

	done := make(chan error, 1)
	go func() {
		_, err := f.client.Lookup(context.Background(), "app/db", "password")
		done <- err
	}()

	nonce := f.awaitRequest(t)
	f.answer(t, &SecretLookupResponse{Nonce: nonce, Error: "secret unavailable"})

	err := <-done
	if err == nil {
		t.Fatal("Lookup() error = nil, want an error")
	}
	if errors.Is(err, ErrSecretDenied) {
		t.Error("a server error was reported as a denial")
	}
}

// A box the client cannot open must fail rather than yield a value.
func TestSecretClient_WrongAADFails(t *testing.T) {
	f := newSecretClientFixture(t, 5*time.Second)

	done := make(chan error, 1)
	go func() {
		_, err := f.client.Lookup(context.Background(), "app/db", "password")
		done <- err
	}()

	nonce := f.awaitRequest(t)
	box, err := sealed.Seal(&f.key.PublicKey, []byte("s3cret"), SecretAAD("agent-1", "a-different-nonce"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	f.answer(t, &SecretLookupResponse{Nonce: nonce, Box: box})

	if err := <-done; err == nil {
		t.Fatal("Lookup() error = nil for a box sealed to a different request")
	}
}

func TestSecretClient_NoBox(t *testing.T) {
	f := newSecretClientFixture(t, 5*time.Second)

	done := make(chan error, 1)
	go func() {
		_, err := f.client.Lookup(context.Background(), "app/db", "password")
		done <- err
	}()

	nonce := f.awaitRequest(t)
	f.answer(t, &SecretLookupResponse{Nonce: nonce})

	if err := <-done; err == nil {
		t.Fatal("Lookup() error = nil for a response with no box")
	}
}

func TestSecretClient_Timeout(t *testing.T) {
	f := newSecretClientFixture(t, 80*time.Millisecond)
	_, err := f.client.Lookup(context.Background(), "app/db", "password")
	if err == nil {
		t.Fatal("Lookup() error = nil, want a timeout")
	}
}

func TestSecretClient_ContextCancel(t *testing.T) {
	f := newSecretClientFixture(t, 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := f.client.Lookup(ctx, "app/db", "password")
		done <- err
	}()
	f.awaitRequest(t)
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Lookup() error = %v, want context.Canceled", err)
	}
}

// A reply for a nonce nobody is waiting on is dropped, not treated as
// an answer to whatever is outstanding.
func TestSecretClient_IgnoresUnknownNonce(t *testing.T) {
	f := newSecretClientFixture(t, 200*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := f.client.Lookup(context.Background(), "app/db", "password")
		done <- err
	}()
	f.awaitRequest(t)

	box, err := sealed.Seal(&f.key.PublicKey, []byte("wrong"), SecretAAD("agent-1", "unrelated"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	f.answer(t, &SecretLookupResponse{Nonce: "unrelated", Box: box})

	if err := <-done; err == nil {
		t.Fatal("an unrelated reply satisfied the lookup")
	}
}

// Every Lookup leaves the waiter table as it found it, or a long-lived
// agent leaks one entry per render.
func TestSecretClient_WaitersAreCleanedUp(t *testing.T) {
	f := newSecretClientFixture(t, 50*time.Millisecond)
	if _, err := f.client.Lookup(context.Background(), "app/db", "password"); err == nil {
		t.Fatal("expected a timeout")
	}
	f.client.mu.Lock()
	n := len(f.client.waiters)
	f.client.mu.Unlock()
	if n != 0 {
		t.Errorf("waiters = %d after Lookup returned, want 0", n)
	}
}

func TestSecretClient_MalformedResponseIsDropped(t *testing.T) {
	f := newSecretClientFixture(t, 60*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := f.client.Lookup(context.Background(), "app/db", "password")
		done <- err
	}()
	f.awaitRequest(t)

	env := envelope.New([]byte("{not json"), f.subj.Prefix())
	if err := f.nats.deliver(t, f.subj.SecretResponse("agent-1"), env); err != nil {
		t.Fatalf("deliver returned an error: %v", err)
	}
	if err := <-done; err == nil {
		t.Fatal("a malformed reply satisfied the lookup")
	}
}

func TestNewSecretClient_Requires(t *testing.T) {
	ca := newTestCA(t)
	signer, err := NewSVIDSigner(ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{}))
	if err != nil {
		t.Fatalf("NewSVIDSigner: %v", err)
	}
	full := SecretClientConfig{
		NATS: newFakeNATS(), Subjects: fakeSubjects{cluster: "test"},
		Signer: signer, Key: signer.key,
	}
	tests := map[string]func(*SecretClientConfig){
		"nats":     func(c *SecretClientConfig) { c.NATS = nil },
		"subjects": func(c *SecretClientConfig) { c.Subjects = nil },
		"signer":   func(c *SecretClientConfig) { c.Signer = nil },
		"key":      func(c *SecretClientConfig) { c.Key = nil },
	}
	for name, drop := range tests {
		t.Run("missing "+name, func(t *testing.T) {
			cfg := full
			drop(&cfg)
			if _, err := NewSecretClient(cfg); err == nil {
				t.Errorf("NewSecretClient() error = nil without %s", name)
			}
		})
	}
}

func TestSecretClient_LookupRequiresPath(t *testing.T) {
	f := newSecretClientFixture(t, time.Second)
	if _, err := f.client.Lookup(context.Background(), "", "password"); err == nil {
		t.Error("Lookup() error = nil for an empty path")
	}
}

func TestSecretClient_StopIsIdempotent(t *testing.T) {
	f := newSecretClientFixture(t, time.Second)
	if err := f.client.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := f.client.Stop(); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}
