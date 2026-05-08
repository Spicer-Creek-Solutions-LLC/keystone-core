package controlplane_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// fakeSubscription captures whether Unsubscribe was called.
type fakeSubscription struct {
	mu          sync.Mutex
	unsubCalled bool
}

func (f *fakeSubscription) Unsubscribe() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unsubCalled = true
	return nil
}

// fakeSubscriber records the most recent subscribe call and exposes
// the handler so tests can drive it synchronously.
type fakeSubscriber struct {
	mu      sync.Mutex
	subj    string
	handler controlplane.MessageHandler
	sub     *fakeSubscription
	err     error
}

func (f *fakeSubscriber) Subscribe(subject string, h controlplane.MessageHandler) (controlplane.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.subj = subject
	f.handler = h
	f.sub = &fakeSubscription{}
	return f.sub, nil
}

func (f *fakeSubscriber) deliver(t *testing.T, subject string, env envelope.Envelope) error {
	t.Helper()
	f.mu.Lock()
	h := f.handler
	f.mu.Unlock()
	if h == nil {
		t.Fatal("deliver: no handler attached")
	}
	return h(context.Background(), subject, env)
}

// fakeIssuer returns a deterministic credential and records the
// agent IDs it was called with. failNext induces a one-shot error.
type fakeIssuer struct {
	mu       sync.Mutex
	called   []string
	failNext bool
	failErr  error
}

func (f *fakeIssuer) Issue(_ context.Context, agentID string) (controlplane.AgentCredentials, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return controlplane.AgentCredentials{}, f.failErr
	}
	f.called = append(f.called, agentID)
	return controlplane.AgentCredentials{
		APIKey:   "key-for-" + agentID,
		AgentID:  agentID,
		IssuedAt: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakeIssuer) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.called...)
}

// fakeValidator accepts pre-seeded (agentID, proof) pairs. Each pair
// is single-use to mirror PSK consume semantics.
type fakeValidator struct {
	mu       sync.Mutex
	accept   map[string][]byte
	consumed map[string]bool
}

func newFakeValidator() *fakeValidator {
	return &fakeValidator{accept: map[string][]byte{}, consumed: map[string]bool{}}
}

func (f *fakeValidator) seed(agentID string, proof []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accept[agentID] = append([]byte(nil), proof...)
}

func (f *fakeValidator) Validate(_ context.Context, agentID string, proof []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	want, ok := f.accept[agentID]
	if !ok {
		return controlplane.ErrPSKNotFound
	}
	if f.consumed[agentID] {
		return controlplane.ErrPSKConsumed
	}
	if string(want) != string(proof) {
		return controlplane.ErrPSKMismatch
	}
	f.consumed[agentID] = true
	return nil
}

func newBootstrapFixture(t *testing.T, opts ...func(*controlplane.BootstrapHandlerConfig)) (*controlplane.BootstrapHandler, *fakeSubscriber, *fakePublisher, *fakeIssuer, *fakeValidator) {
	t.Helper()
	store := newTestStore(t)
	sub := &fakeSubscriber{}
	pub := &fakePublisher{}
	val := newFakeValidator()
	iss := &fakeIssuer{}

	cfg := controlplane.BootstrapHandlerConfig{
		Subjects:   fakeSubjects{cluster: "default"},
		Subscriber: sub,
		Publisher:  pub,
		Store:      store,
		Validator:  val,
		Issuer:     iss,
		Clock:      func() time.Time { return time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC) },
	}
	for _, o := range opts {
		o(&cfg)
	}
	h, err := controlplane.NewBootstrapHandler(cfg)
	if err != nil {
		t.Fatalf("NewBootstrapHandler: %v", err)
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = h.Stop(context.Background()) })
	return h, sub, pub, iss, val
}

func makeRegisterEnvelope(t *testing.T, agentID string, proof []byte) envelope.Envelope {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"agent_id": agentID,
		"proof":    hex.EncodeToString(proof),
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return envelope.New(body, "kscore.default", envelope.WithMessageID("m-"+agentID))
}

func TestNewBootstrapHandler_ValidatesRequiredFields(t *testing.T) {
	store := newTestStore(t)
	good := controlplane.BootstrapHandlerConfig{
		Subjects:   fakeSubjects{cluster: "default"},
		Subscriber: &fakeSubscriber{},
		Publisher:  &fakePublisher{},
		Store:      store,
		Validator:  newFakeValidator(),
		Issuer:     &fakeIssuer{},
	}
	for _, mut := range []struct {
		name string
		f    func(*controlplane.BootstrapHandlerConfig)
	}{
		{"nil subjects", func(c *controlplane.BootstrapHandlerConfig) { c.Subjects = nil }},
		{"nil subscriber", func(c *controlplane.BootstrapHandlerConfig) { c.Subscriber = nil }},
		{"nil publisher", func(c *controlplane.BootstrapHandlerConfig) { c.Publisher = nil }},
		{"nil store", func(c *controlplane.BootstrapHandlerConfig) { c.Store = nil }},
		{"nil validator", func(c *controlplane.BootstrapHandlerConfig) { c.Validator = nil }},
		{"nil issuer", func(c *controlplane.BootstrapHandlerConfig) { c.Issuer = nil }},
	} {
		t.Run(mut.name, func(t *testing.T) {
			cfg := good
			mut.f(&cfg)
			if _, err := controlplane.NewBootstrapHandler(cfg); err == nil {
				t.Errorf("expected error for %s", mut.name)
			}
		})
	}
}

func TestBootstrapHandler_RegisterSucceeds(t *testing.T) {
	_, sub, pub, iss, val := newBootstrapFixture(t)
	val.seed("agent-1", []byte("secret-bytes"))

	subject := "kscore.default.bootstrap.agent-1.register"
	env := makeRegisterEnvelope(t, "agent-1", []byte("secret-bytes"))
	if err := sub.deliver(t, subject, env); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	calls := pub.Calls()
	if len(calls) != 1 {
		t.Fatalf("publish calls = %d, want 1", len(calls))
	}
	if want := "kscore.default.bootstrap.agent-1.response"; calls[0].subject != want {
		t.Errorf("subject = %q, want %q", calls[0].subject, want)
	}
	respEnv := calls[0].envelope
	if respEnv.CorrelationID != env.MessageID {
		t.Errorf("CorrelationID = %q, want %q", respEnv.CorrelationID, env.MessageID)
	}
	var creds controlplane.AgentCredentials
	if err := json.Unmarshal(respEnv.Payload, &creds); err != nil {
		t.Fatalf("response payload: %v", err)
	}
	if creds.APIKey == "" {
		t.Error("APIKey is empty in response")
	}
	if creds.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", creds.AgentID)
	}
	if got := iss.Calls(); len(got) != 1 || got[0] != "agent-1" {
		t.Errorf("issuer.Calls = %v, want [agent-1]", got)
	}
}

func TestBootstrapHandler_RejectsMalformedSubject(t *testing.T) {
	_, sub, pub, _, _ := newBootstrapFixture(t)
	env := makeRegisterEnvelope(t, "agent-1", []byte("x"))

	for _, badSubject := range []string{
		"kscore.default.bootstrap.register",          // missing id token
		"kscore.default.bootstrap.agent-1.foo",       // wrong suffix
		"weird.default.bootstrap.agent-1.register",   // wrong root
	} {
		if err := sub.deliver(t, badSubject, env); err != nil {
			t.Errorf("deliver(%q) = %v, want nil (handler swallows)", badSubject, err)
		}
	}
	if calls := pub.Calls(); len(calls) != 0 {
		t.Errorf("response published for malformed subject: %d", len(calls))
	}
}

func TestBootstrapHandler_RejectsAgentIDMismatch(t *testing.T) {
	_, sub, pub, _, val := newBootstrapFixture(t)
	val.seed("agent-1", []byte("x"))

	body, _ := json.Marshal(map[string]any{
		"agent_id": "agent-2", // doesn't match subject id
		"proof":    hex.EncodeToString([]byte("x")),
	})
	env := envelope.New(body, "kscore.default")
	if err := sub.deliver(t, "kscore.default.bootstrap.agent-1.register", env); err != nil {
		t.Errorf("deliver: %v", err)
	}
	if calls := pub.Calls(); len(calls) != 0 {
		t.Errorf("response published despite ID mismatch: %d", len(calls))
	}
}

func TestBootstrapHandler_RejectsBadProofHex(t *testing.T) {
	_, sub, pub, _, _ := newBootstrapFixture(t)

	body, _ := json.Marshal(map[string]any{
		"agent_id": "agent-1",
		"proof":    "not-hex-zzz",
	})
	env := envelope.New(body, "kscore.default")
	_ = sub.deliver(t, "kscore.default.bootstrap.agent-1.register", env)
	if calls := pub.Calls(); len(calls) != 0 {
		t.Errorf("response published despite bad proof: %d", len(calls))
	}
}

func TestBootstrapHandler_ValidatorRejection(t *testing.T) {
	_, sub, pub, iss, _ := newBootstrapFixture(t)
	// validator has no seed, so it returns ErrPSKNotFound

	env := makeRegisterEnvelope(t, "agent-1", []byte("x"))
	_ = sub.deliver(t, "kscore.default.bootstrap.agent-1.register", env)

	if got := iss.Calls(); len(got) != 0 {
		t.Errorf("issuer called despite validator rejection: %v", got)
	}
	if calls := pub.Calls(); len(calls) != 0 {
		t.Errorf("response published despite validator rejection: %d", len(calls))
	}
}

func TestBootstrapHandler_IssuerFailure(t *testing.T) {
	_, sub, pub, iss, val := newBootstrapFixture(t)
	val.seed("agent-1", []byte("x"))
	iss.failNext = true
	iss.failErr = errors.New("issuer down")

	env := makeRegisterEnvelope(t, "agent-1", []byte("x"))
	if err := sub.deliver(t, "kscore.default.bootstrap.agent-1.register", env); err == nil {
		t.Error("deliver: expected error from issuer failure")
	}
	if calls := pub.Calls(); len(calls) != 0 {
		t.Errorf("response published despite issuer failure: %d", len(calls))
	}
}

func TestBootstrapHandler_StopUnsubscribes(t *testing.T) {
	h, sub, _, _, _ := newBootstrapFixture(t)
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	sub.mu.Lock()
	got := sub.sub.unsubCalled
	sub.mu.Unlock()
	if !got {
		t.Error("Unsubscribe not called on Stop")
	}
}

func TestBootstrapHandler_StopIdempotent(t *testing.T) {
	h, _, _, _, _ := newBootstrapFixture(t)
	if err := h.Stop(context.Background()); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestBootstrapHandler_StartIdempotent(t *testing.T) {
	h, _, _, _, _ := newBootstrapFixture(t)
	if err := h.Start(context.Background()); err != nil {
		t.Errorf("second Start: %v", err)
	}
}

// PSKValidator unit tests

func TestPSKValidator_AcceptsValidProof(t *testing.T) {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	v := controlplane.NewPSKValidator(controlplane.PSKValidatorConfig{
		Entries: []controlplane.PSKEntry{
			{AgentID: "agent-1", Secret: []byte{0xde, 0xad, 0xbe, 0xef}, ExpiresAt: exp},
		},
	})
	if err := v.Validate(context.Background(), "agent-1", []byte{0xde, 0xad, 0xbe, 0xef}); err != nil {
		t.Errorf("Validate = %v, want nil", err)
	}
}

func TestPSKValidator_ConsumeIsSingleUse(t *testing.T) {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	v := controlplane.NewPSKValidator(controlplane.PSKValidatorConfig{
		Entries: []controlplane.PSKEntry{
			{AgentID: "agent-1", Secret: []byte("x"), ExpiresAt: exp},
		},
	})
	if err := v.Validate(context.Background(), "agent-1", []byte("x")); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := v.Validate(context.Background(), "agent-1", []byte("x")); !errors.Is(err, controlplane.ErrPSKConsumed) {
		t.Errorf("second = %v, want ErrPSKConsumed", err)
	}
}

func TestPSKValidator_RejectsMismatch(t *testing.T) {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	v := controlplane.NewPSKValidator(controlplane.PSKValidatorConfig{
		Entries: []controlplane.PSKEntry{
			{AgentID: "agent-1", Secret: []byte("right"), ExpiresAt: exp},
		},
	})
	if err := v.Validate(context.Background(), "agent-1", []byte("wrong")); !errors.Is(err, controlplane.ErrPSKMismatch) {
		t.Errorf("Validate = %v, want ErrPSKMismatch", err)
	}
	// Mismatch must NOT consume the entry — agent can retry with the
	// correct proof.
	if err := v.Validate(context.Background(), "agent-1", []byte("right")); err != nil {
		t.Errorf("retry after mismatch: %v", err)
	}
}

func TestPSKValidator_RejectsExpired(t *testing.T) {
	exp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := controlplane.NewPSKValidator(controlplane.PSKValidatorConfig{
		Entries: []controlplane.PSKEntry{
			{AgentID: "agent-1", Secret: []byte("x"), ExpiresAt: exp},
		},
		Clock: func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	if err := v.Validate(context.Background(), "agent-1", []byte("x")); !errors.Is(err, controlplane.ErrPSKExpired) {
		t.Errorf("Validate = %v, want ErrPSKExpired", err)
	}
}

func TestPSKValidator_RejectsUnknownAgent(t *testing.T) {
	v := controlplane.NewPSKValidator(controlplane.PSKValidatorConfig{})
	if err := v.Validate(context.Background(), "ghost", []byte("x")); !errors.Is(err, controlplane.ErrPSKNotFound) {
		t.Errorf("Validate = %v, want ErrPSKNotFound", err)
	}
}

func TestDecodeConfigPSKs(t *testing.T) {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []controlplane.ConfigPSK{
		{AgentID: "agent-1", Secret: "deadbeef", ExpiresAt: exp},
		{AgentID: "agent-2", Secret: "cafebabe", ExpiresAt: exp},
	}
	out, err := controlplane.DecodeConfigPSKs(in)
	if err != nil {
		t.Fatalf("DecodeConfigPSKs: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if string(out[0].Secret) != "\xde\xad\xbe\xef" {
		t.Errorf("entry[0].Secret = %x, want deadbeef", out[0].Secret)
	}
}

func TestDecodeConfigPSKs_BadHex(t *testing.T) {
	if _, err := controlplane.DecodeConfigPSKs([]controlplane.ConfigPSK{{AgentID: "x", Secret: "not-hex-zzz"}}); err == nil {
		t.Error("expected error for non-hex secret")
	}
}

// APIKeyIssuer unit test

func TestAPIKeyIssuer_IssueProducesPersistedKey(t *testing.T) {
	store := newTestStore(t)
	issuer, err := controlplane.NewAPIKeyIssuer(controlplane.APIKeyIssuerConfig{Keys: store})
	if err != nil {
		t.Fatalf("NewAPIKeyIssuer: %v", err)
	}
	creds, err := issuer.Issue(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if creds.APIKey == "" {
		t.Error("APIKey is empty")
	}
	if creds.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", creds.AgentID)
	}
}
