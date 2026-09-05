// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/sealed"
	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// ---- fixtures --------------------------------------------------------

type secretCA struct {
	cert *x509.Certificate
	key  crypto.Signer
	pem  string
}

func newSecretCA(t *testing.T) *secretCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "secret-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("ca cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}
	return &secretCA{cert: cert, key: key,
		pem: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))}
}

// agentKit is one agent's credential plus the pieces a test needs to
// act as that agent.
type agentKit struct {
	id     string
	signer *agent.SVIDSigner
	key    *ecdsa.PrivateKey
}

func (ca *secretCA) kit(t *testing.T, agentID string) *agentKit {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	u, err := url.Parse("spiffe://example.org/agent/" + agentID)
	if err != nil {
		t.Fatalf("uri: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: agentID},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         []*url.URL{u},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, key.Public(), ca.key)
	if err != nil {
		t.Fatalf("leaf cert: %v", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	signer, err := agent.NewSVIDSigner(&agent.Credentials{
		APIKey:         "k",
		AgentID:        agentID,
		CertChainPEM:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) + ca.pem,
		PrivateKeyPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		TrustBundlePEM: ca.pem,
	})
	if err != nil {
		t.Fatalf("NewSVIDSigner: %v", err)
	}
	return &agentKit{id: agentID, signer: signer, key: key}
}

type secretSubjects struct{}

func (secretSubjects) SecretRequest() string { return "kscore.test.secret.request" }
func (secretSubjects) SecretResponse(id string) string {
	return "kscore.test.secret." + id + ".response"
}
func (secretSubjects) Prefix() string { return "kscore.test" }

type stubBroker struct {
	secret *secrets.Secret
	err    error
}

func (s *stubBroker) GetSecret(_ context.Context, _ secrets.GetSecretRequest) (*secrets.Secret, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.secret, nil
}

type stubAgents struct {
	rec *state.AgentRecord
	err error
}

func (s *stubAgents) GetAgent(_ context.Context, _ string) (*state.AgentRecord, error) {
	return s.rec, s.err
}

// harness wires a handler over fakes and exposes the send/receive pair.
type harness struct {
	t    *testing.T
	ca   *secretCA
	sub  *fakeSubscriber
	pub  *fakePublisher
	stop func()
}

func newHarness(t *testing.T, grants *secrets.AgentGrants, broker controlplane.SecretReader, agents controlplane.AgentLabelSource) *harness {
	t.Helper()
	ca := newSecretCA(t)
	roots, err := controlplane.SVIDRootsFromPEM(ca.pem)
	if err != nil {
		t.Fatalf("roots: %v", err)
	}
	sub := &fakeSubscriber{}
	pub := &fakePublisher{}
	h, err := controlplane.NewSecretRequestHandler(controlplane.SecretHandlerConfig{
		Subscriber: sub,
		Publisher:  pub,
		Subjects:   secretSubjects{},
		Verifier:   &controlplane.SVIDVerifier{Roots: roots},
		Grants:     grants,
		Broker:     broker,
		Agents:     agents,
	})
	if err != nil {
		t.Fatalf("NewSecretRequestHandler: %v", err)
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return &harness{t: t, ca: ca, sub: sub, pub: pub, stop: func() { _ = h.Stop() }}
}

// ask sends a signed lookup and returns the single reply.
func (h *harness) ask(kit *agentKit, path, key string) (*agent.SecretLookupResponse, string) {
	h.t.Helper()
	payload, err := json.Marshal(agent.SecretLookupRequest{Path: path, Key: key})
	if err != nil {
		h.t.Fatalf("marshal: %v", err)
	}
	signed, err := kit.signer.Sign(payload)
	if err != nil {
		h.t.Fatalf("Sign: %v", err)
	}
	body, err := json.Marshal(signed)
	if err != nil {
		h.t.Fatalf("marshal signed: %v", err)
	}
	env := envelope.New(body, "kscore.test", envelope.WithMessageID(signed.Nonce))
	if err := h.sub.deliver(h.t, secretSubjects{}.SecretRequest(), env); err != nil {
		h.t.Fatalf("deliver: %v", err)
	}
	calls := h.pub.Calls()
	if len(calls) == 0 {
		return nil, signed.Nonce
	}
	var resp agent.SecretLookupResponse
	if err := json.Unmarshal(calls[len(calls)-1].envelope.Payload, &resp); err != nil {
		h.t.Fatalf("unmarshal response: %v", err)
	}
	return &resp, signed.Nonce
}

func grantsFor(t *testing.T, rules []secrets.AgentGrant) *secrets.AgentGrants {
	t.Helper()
	g, err := secrets.NewAgentGrants(rules)
	if err != nil {
		t.Fatalf("NewAgentGrants: %v", err)
	}
	return g
}

func dbSecret() *secrets.Secret {
	return &secrets.Secret{Path: "app/db", Data: map[string]any{"password": "s3cret", "username": "app"}}
}

// sealedOpen unseals a response box with key.
func sealedOpen(t *testing.T, key crypto.PrivateKey, resp *agent.SecretLookupResponse, aad []byte) (string, error) {
	t.Helper()
	plaintext, err := sealed.Open(key, resp.Box, aad)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// ---- tests -----------------------------------------------------------

func TestSecretHandler_ServesGrantedPath(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{"app/"}}})
	h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, nil)
	defer h.stop()

	kit := h.ca.kit(t, "agent-1")
	resp, nonce := h.ask(kit, "app/db", "password")
	if resp == nil {
		t.Fatal("no response published")
	}
	if resp.Denied || resp.Error != "" {
		t.Fatalf("denied=%v error=%q, want a sealed value", resp.Denied, resp.Error)
	}
	if resp.Box == nil {
		t.Fatal("response carried no box")
	}

	// Only the asking agent can read it.
	plaintext, err := sealedOpen(t, kit.key, resp, agent.SecretAAD("agent-1", nonce))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if plaintext != "s3cret" {
		t.Errorf("value = %q, want %q", plaintext, "s3cret")
	}
}

// The reply goes out on a subject every agent can subscribe to, so the
// sealing has to be what protects it.
func TestSecretHandler_ReplyIsUnreadableByAnotherAgent(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{"app/"}}})
	h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, nil)
	defer h.stop()

	kit := h.ca.kit(t, "agent-1")
	eavesdropper := h.ca.kit(t, "agent-2")

	resp, nonce := h.ask(kit, "app/db", "password")
	if resp == nil || resp.Box == nil {
		t.Fatal("no sealed response")
	}
	if _, err := sealedOpen(t, eavesdropper.key, resp, agent.SecretAAD("agent-1", nonce)); err == nil {
		t.Fatal("another agent opened the reply")
	}

	// The cleartext must not appear anywhere in what was published.
	for _, c := range h.pub.Calls() {
		if strings.Contains(string(c.envelope.Payload), "s3cret") {
			t.Fatal("the published envelope contains the cleartext secret")
		}
	}
}

func TestSecretHandler_DeniesUngrantedPath(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{"app/"}}})
	h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, nil)
	defer h.stop()

	resp, _ := h.ask(h.ca.kit(t, "agent-1"), "other/root-ca", "password")
	if resp == nil {
		t.Fatal("no response published")
	}
	if !resp.Denied {
		t.Errorf("denied = false, want true for an ungranted path")
	}
	if resp.Box != nil {
		t.Error("a denial carried a box")
	}
}

func TestSecretHandler_DeniesAnotherAgentsGrant(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{"app/"}}})
	h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, nil)
	defer h.stop()

	// agent-2 holds a legitimate certificate; the grant is not its own.
	resp, _ := h.ask(h.ca.kit(t, "agent-2"), "app/db", "password")
	if resp == nil {
		t.Fatal("no response published")
	}
	if !resp.Denied {
		t.Error("agent-2 was served a path granted only to agent-1")
	}
}

// With no grants configured, nothing is readable.
func TestSecretHandler_NoGrantsDeniesEverything(t *testing.T) {
	h := newHarness(t, nil, &stubBroker{secret: dbSecret()}, nil)
	defer h.stop()

	resp, _ := h.ask(h.ca.kit(t, "agent-1"), "app/db", "password")
	if resp == nil {
		t.Fatal("no response published")
	}
	if !resp.Denied {
		t.Error("a server with no grants served a secret")
	}
}

// A request that does not authenticate gets no reply at all: there is
// no verified agent to address one to.
func TestSecretHandler_UnverifiedRequestGetsNoReply(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{AgentIDs: []string{"*"}, Paths: []string{"app/"}}})
	h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, nil)
	defer h.stop()

	foreign := newSecretCA(t) // not the CA the verifier trusts
	resp, _ := h.ask(foreign.kit(t, "agent-1"), "app/db", "password")
	if resp != nil {
		t.Fatalf("a reply was published for an untrusted certificate: %+v", resp)
	}
	if len(h.pub.Calls()) != 0 {
		t.Error("handler published something for an unverified request")
	}
}

func TestSecretHandler_MalformedPayloadGetsNoReplyOrAnError(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{AgentIDs: []string{"*"}, Paths: []string{"app/"}}})
	h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, nil)
	defer h.stop()

	env := envelope.New([]byte("{not json"), "kscore.test", envelope.WithMessageID("x"))
	if err := h.sub.deliver(t, secretSubjects{}.SecretRequest(), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if len(h.pub.Calls()) != 0 {
		t.Error("handler replied to an unparseable request")
	}
}

// Labels come from the control plane's record, never the request, so a
// label-scoped grant follows the record.
func TestSecretHandler_LabelGrantUsesTheStoredRecord(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{
		Labels: map[string]string{"role": "web"}, Paths: []string{"app/"},
	}})

	t.Run("record carries the label", func(t *testing.T) {
		agents := &stubAgents{rec: &state.AgentRecord{ID: "agent-1", Labels: map[string]string{"role": "web"}}}
		h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, agents)
		defer h.stop()
		resp, _ := h.ask(h.ca.kit(t, "agent-1"), "app/db", "password")
		if resp == nil || resp.Denied {
			t.Errorf("labelled agent was denied: %+v", resp)
		}
	})

	t.Run("record lacks the label", func(t *testing.T) {
		agents := &stubAgents{rec: &state.AgentRecord{ID: "agent-1", Labels: map[string]string{"role": "db"}}}
		h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, agents)
		defer h.stop()
		resp, _ := h.ask(h.ca.kit(t, "agent-1"), "app/db", "password")
		if resp == nil || !resp.Denied {
			t.Errorf("agent with the wrong label was served: %+v", resp)
		}
	})

	// A verified certificate with no record fails closed rather than
	// evaluating label rules against no labels.
	t.Run("no record at all", func(t *testing.T) {
		agents := &stubAgents{err: errors.New("not found")}
		h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, agents)
		defer h.stop()
		resp, _ := h.ask(h.ca.kit(t, "agent-1"), "app/db", "password")
		if resp == nil || !resp.Denied {
			t.Errorf("agent with no record was served: %+v", resp)
		}
	})
}

func TestSecretHandler_BrokerFailureIsNotADenial(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{"app/"}}})
	h := newHarness(t, grants, &stubBroker{err: errors.New("backend down")}, nil)
	defer h.stop()

	resp, _ := h.ask(h.ca.kit(t, "agent-1"), "app/db", "password")
	if resp == nil {
		t.Fatal("no response published")
	}
	if resp.Denied {
		t.Error("a backend failure was reported as a policy denial")
	}
	if resp.Error == "" {
		t.Error("a backend failure reported no error")
	}
	// The broker's own message could name internal infrastructure.
	if strings.Contains(resp.Error, "backend down") {
		t.Errorf("broker error text leaked to the agent: %q", resp.Error)
	}
}

func TestSecretHandler_KeySelection(t *testing.T) {
	grants := grantsFor(t, []secrets.AgentGrant{{AgentIDs: []string{"agent-1"}, Paths: []string{"app/"}}})

	t.Run("missing key", func(t *testing.T) {
		h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, nil)
		defer h.stop()
		resp, _ := h.ask(h.ca.kit(t, "agent-1"), "app/db", "nonexistent")
		if resp == nil || resp.Error == "" {
			t.Errorf("missing key produced no error: %+v", resp)
		}
	})

	// Guessing among several fields would sometimes hand back the wrong
	// credential, which is worse than refusing.
	t.Run("no key with several fields", func(t *testing.T) {
		h := newHarness(t, grants, &stubBroker{secret: dbSecret()}, nil)
		defer h.stop()
		resp, _ := h.ask(h.ca.kit(t, "agent-1"), "app/db", "")
		if resp == nil || resp.Error == "" {
			t.Errorf("ambiguous key produced no error: %+v", resp)
		}
	})

	t.Run("no key with one field", func(t *testing.T) {
		single := &secrets.Secret{Path: "app/token", Data: map[string]any{"value": "t0ken"}}
		h := newHarness(t, grants, &stubBroker{secret: single}, nil)
		defer h.stop()
		kit := h.ca.kit(t, "agent-1")
		resp, nonce := h.ask(kit, "app/token", "")
		if resp == nil || resp.Box == nil {
			t.Fatalf("single-field secret was not served: %+v", resp)
		}
		got, err := sealedOpen(t, kit.key, resp, agent.SecretAAD("agent-1", nonce))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if got != "t0ken" {
			t.Errorf("value = %q, want %q", got, "t0ken")
		}
	})

	t.Run("non-scalar field", func(t *testing.T) {
		structured := &secrets.Secret{Path: "app/cfg", Data: map[string]any{"cfg": map[string]any{"a": 1}}}
		h := newHarness(t, grants, &stubBroker{secret: structured}, nil)
		defer h.stop()
		resp, _ := h.ask(h.ca.kit(t, "agent-1"), "app/cfg", "cfg")
		if resp == nil || resp.Error == "" {
			t.Errorf("structured field produced no error: %+v", resp)
		}
	})
}

func TestNewSecretRequestHandler_Requires(t *testing.T) {
	ca := newSecretCA(t)
	roots, err := controlplane.SVIDRootsFromPEM(ca.pem)
	if err != nil {
		t.Fatalf("roots: %v", err)
	}
	full := controlplane.SecretHandlerConfig{
		Subscriber: &fakeSubscriber{},
		Publisher:  &fakePublisher{},
		Subjects:   secretSubjects{},
		Verifier:   &controlplane.SVIDVerifier{Roots: roots},
		Broker:     &stubBroker{},
	}
	tests := map[string]func(*controlplane.SecretHandlerConfig){
		"subscriber": func(c *controlplane.SecretHandlerConfig) { c.Subscriber = nil },
		"publisher":  func(c *controlplane.SecretHandlerConfig) { c.Publisher = nil },
		"subjects":   func(c *controlplane.SecretHandlerConfig) { c.Subjects = nil },
		"verifier":   func(c *controlplane.SecretHandlerConfig) { c.Verifier = nil },
		"broker":     func(c *controlplane.SecretHandlerConfig) { c.Broker = nil },
	}
	for name, drop := range tests {
		t.Run("missing "+name, func(t *testing.T) {
			cfg := full
			drop(&cfg)
			if _, err := controlplane.NewSecretRequestHandler(cfg); err == nil {
				t.Errorf("NewSecretRequestHandler() error = nil without %s", name)
			}
		})
	}

	// Grants may be nil -- that is a server that denies every lookup,
	// not a misconfigured one.
	t.Run("nil grants is allowed", func(t *testing.T) {
		if _, err := controlplane.NewSecretRequestHandler(full); err != nil {
			t.Errorf("NewSecretRequestHandler() error = %v with nil grants", err)
		}
	})
}
