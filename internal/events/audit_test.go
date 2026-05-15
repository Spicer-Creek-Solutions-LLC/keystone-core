package events

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// auditStubPublisher records Publish + PublishAsync calls and lets
// tests inject errors. Mutex-guarded so race detector stays happy
// across the few concurrent assertions.
type auditStubPublisher struct {
	mu             sync.Mutex
	published      []Event
	publishedAsync []Event
	emitErr        error
	emitAsyncErr   error
}

func (s *auditStubPublisher) Start(context.Context) error { return nil }
func (s *auditStubPublisher) Publish(_ context.Context, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emitErr != nil {
		return s.emitErr
	}
	s.published = append(s.published, e)
	return nil
}
func (s *auditStubPublisher) PublishAsync(_ context.Context, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emitAsyncErr != nil {
		return s.emitAsyncErr
	}
	s.publishedAsync = append(s.publishedAsync, e)
	return nil
}
func (s *auditStubPublisher) Stop(context.Context) error { return nil }

// ---- AuditOutcome --------------------------------------------------------

func TestAuditOutcome_IsValid(t *testing.T) {
	t.Parallel()
	if !AuditOutcomeAllowed.IsValid() {
		t.Errorf("allowed not valid")
	}
	if !AuditOutcomeDenied.IsValid() {
		t.Errorf("denied not valid")
	}
	for _, bad := range []AuditOutcome{"", "permit", "block", "ALLOWED"} {
		if bad.IsValid() {
			t.Errorf("%q reported valid", bad)
		}
	}
}

// ---- NewAuditEvent — defaults ------------------------------------------

func TestNewAuditEvent_AllowedDefaultsToPolicyPass(t *testing.T) {
	t.Parallel()
	e, err := NewAuditEvent(AuditEventInput{
		Source:  "secrets-broker",
		Action:  "get_secret",
		Outcome: AuditOutcomeAllowed,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.Type != EventTypePolicyPass {
		t.Errorf("type = %s, want policy.pass", e.Type)
	}
	if e.Severity != SeverityInfo {
		t.Errorf("severity = %s, want info", e.Severity)
	}
	if e.Tags[AuditTagOutcome] != string(AuditOutcomeAllowed) {
		t.Errorf("outcome tag = %s", e.Tags[AuditTagOutcome])
	}
}

func TestNewAuditEvent_DeniedDefaultsToPolicyViolation(t *testing.T) {
	t.Parallel()
	e, err := NewAuditEvent(AuditEventInput{
		Source:  "secrets-broker",
		Action:  "get_secret",
		Outcome: AuditOutcomeDenied,
		Reason:  "capability refused",
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.Type != EventTypePolicyViolation {
		t.Errorf("type = %s, want policy.violation", e.Type)
	}
	if e.Severity != SeverityWarn {
		t.Errorf("severity = %s, want warn", e.Severity)
	}
	if e.Tags[AuditTagReason] != "capability refused" {
		t.Errorf("reason tag = %q", e.Tags[AuditTagReason])
	}
}

func TestNewAuditEvent_ExplicitTypeOverride(t *testing.T) {
	t.Parallel()
	e, err := NewAuditEvent(AuditEventInput{
		Type:    EventTypeUserCommand,
		Source:  "kscore-secrets",
		Action:  "put_secret",
		Outcome: AuditOutcomeAllowed,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.Type != EventTypeUserCommand {
		t.Errorf("type = %s, want user.command", e.Type)
	}
}

func TestNewAuditEvent_ExplicitSeverityOverride(t *testing.T) {
	t.Parallel()
	e, err := NewAuditEvent(AuditEventInput{
		Source:   "secrets-broker",
		Action:   "get_secret",
		Outcome:  AuditOutcomeDenied,
		Severity: SeverityCritical,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.Severity != SeverityCritical {
		t.Errorf("severity = %s, want critical (explicit override)", e.Severity)
	}
}

// ---- NewAuditEvent — tag stamping --------------------------------------

func TestNewAuditEvent_CanonicalTagsStamped(t *testing.T) {
	t.Parallel()
	e, err := NewAuditEvent(AuditEventInput{
		Source:       "secrets-broker",
		Actor:        "spiffe://kscore.local/agent/agent-1",
		Action:       "get_secret",
		Resource:     "kv/app/db",
		ResourceType: "secret",
		Outcome:      AuditOutcomeAllowed,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	want := map[string]string{
		AuditTagActor:        "spiffe://kscore.local/agent/agent-1",
		AuditTagAction:       "get_secret",
		AuditTagResource:     "kv/app/db",
		AuditTagResourceType: "secret",
		AuditTagOutcome:      "allowed",
	}
	for k, v := range want {
		if got := e.Tags[k]; got != v {
			t.Errorf("tag %q = %q, want %q", k, got, v)
		}
	}
	if _, has := e.Tags[AuditTagReason]; has {
		t.Errorf("reason tag should be absent on allowed event: %q", e.Tags[AuditTagReason])
	}
}

func TestNewAuditEvent_OptionalFieldsOmittedFromTags(t *testing.T) {
	t.Parallel()
	// No Actor / Resource / ResourceType — those tag keys must NOT
	// appear (vs. appearing with empty string values).
	e, err := NewAuditEvent(AuditEventInput{
		Source:  "system",
		Action:  "startup",
		Outcome: AuditOutcomeAllowed,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, k := range []string{AuditTagActor, AuditTagResource, AuditTagResourceType, AuditTagReason} {
		if _, has := e.Tags[k]; has {
			t.Errorf("tag %q should be absent when input field is empty", k)
		}
	}
}

// ---- ExtraTags merge precedence ----------------------------------------

func TestNewAuditEvent_ExtraTagsCanonicalWins(t *testing.T) {
	t.Parallel()
	e, err := NewAuditEvent(AuditEventInput{
		Source:  "secrets-broker",
		Actor:   "spiffe-actor",
		Action:  "get_secret",
		Outcome: AuditOutcomeAllowed,
		ExtraTags: map[string]string{
			AuditTagActor:  "extra-actor",  // collision — canonical wins
			AuditTagAction: "extra-action", // collision — canonical wins
			"region":       "us-east",      // non-collision — preserved
			"team":         "platform",
		},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.Tags[AuditTagActor] != "spiffe-actor" {
		t.Errorf("ExtraTags overrode canonical actor: %q", e.Tags[AuditTagActor])
	}
	if e.Tags[AuditTagAction] != "get_secret" {
		t.Errorf("ExtraTags overrode canonical action: %q", e.Tags[AuditTagAction])
	}
	if e.Tags["region"] != "us-east" {
		t.Errorf("non-canonical ExtraTags lost: region=%q", e.Tags["region"])
	}
	if e.Tags["team"] != "platform" {
		t.Errorf("non-canonical ExtraTags lost: team=%q", e.Tags["team"])
	}
}

// ---- NewAuditEvent — validation -----------------------------------------

func TestNewAuditEvent_RejectsMissingFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   AuditEventInput
	}{
		{"empty source", AuditEventInput{Action: "get_secret", Outcome: AuditOutcomeAllowed}},
		{"empty action", AuditEventInput{Source: "x", Outcome: AuditOutcomeAllowed}},
		{"empty outcome", AuditEventInput{Source: "x", Action: "get_secret"}},
		{"bogus outcome", AuditEventInput{Source: "x", Action: "get_secret", Outcome: "permit"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewAuditEvent(c.in)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidEvent) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidEvent)", err)
			}
		})
	}
}

func TestNewAuditEvent_RejectsBadExplicitType(t *testing.T) {
	t.Parallel()
	_, err := NewAuditEvent(AuditEventInput{
		Type:    EventType("bogus"),
		Source:  "x",
		Action:  "get_secret",
		Outcome: AuditOutcomeAllowed,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("err = %v", err)
	}
}

func TestNewAuditEvent_DataPropagates(t *testing.T) {
	t.Parallel()
	e, err := NewAuditEvent(AuditEventInput{
		Source:  "secrets-broker",
		Action:  "write_secret",
		Outcome: AuditOutcomeAllowed,
		Data:    map[string]any{"masked_payload": map[string]any{"password": "***"}},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if v, ok := e.Data["masked_payload"]; !ok || v == nil {
		t.Errorf("data lost: %+v", e.Data)
	}
}

// ---- AuditEmitter -------------------------------------------------------

func TestNewAuditEmitter_RejectsNilPublisher(t *testing.T) {
	t.Parallel()
	_, err := NewAuditEmitter(nil, nil)
	if err == nil {
		t.Errorf("expected error for nil publisher")
	}
}

func TestAuditEmitter_Emit_SyncDelegatesToPublisher(t *testing.T) {
	t.Parallel()
	pub := &auditStubPublisher{}
	emitter, err := NewAuditEmitter(pub, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	in := AuditEventInput{
		Source:   "secrets-broker",
		Actor:    "spiffe://kscore.local/agent/agent-1",
		Action:   "get_secret",
		Resource: "kv/app/db",
		Outcome:  AuditOutcomeAllowed,
	}
	if err := emitter.Emit(context.Background(), in); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 1 {
		t.Fatalf("publisher saw %d, want 1", len(pub.published))
	}
	got := pub.published[0]
	if got.Type != EventTypePolicyPass {
		t.Errorf("published type = %s", got.Type)
	}
	if got.Tags[AuditTagAction] != "get_secret" {
		t.Errorf("action tag = %q", got.Tags[AuditTagAction])
	}
}

func TestAuditEmitter_Emit_PublisherErrorPropagates(t *testing.T) {
	t.Parallel()
	pub := &auditStubPublisher{emitErr: errors.New("nats down")}
	emitter, _ := NewAuditEmitter(pub, nil)
	err := emitter.Emit(context.Background(), AuditEventInput{
		Source:  "secrets-broker",
		Action:  "get_secret",
		Outcome: AuditOutcomeAllowed,
	})
	if err == nil {
		t.Fatalf("expected publisher error to propagate")
	}
	if emitter.FailedPublishes() != 1 {
		t.Errorf("FailedPublishes = %d, want 1", emitter.FailedPublishes())
	}
}

func TestAuditEmitter_Emit_RejectsInvalidInput(t *testing.T) {
	t.Parallel()
	pub := &auditStubPublisher{}
	emitter, _ := NewAuditEmitter(pub, nil)
	err := emitter.Emit(context.Background(), AuditEventInput{
		// Missing Source / Action / Outcome — should fail before
		// reaching the publisher.
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 0 {
		t.Errorf("invalid input reached publisher: %+v", pub.published)
	}
}

func TestAuditEmitter_EmitAsync_DelegatesToPublishAsync(t *testing.T) {
	t.Parallel()
	pub := &auditStubPublisher{}
	emitter, _ := NewAuditEmitter(pub, nil)
	if err := emitter.EmitAsync(context.Background(), AuditEventInput{
		Source:  "x",
		Action:  "get_secret",
		Outcome: AuditOutcomeAllowed,
	}); err != nil {
		t.Fatalf("EmitAsync: %v", err)
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.publishedAsync) != 1 {
		t.Errorf("PublishAsync called %d times, want 1", len(pub.publishedAsync))
	}
	if len(pub.published) != 0 {
		t.Errorf("sync Publish unexpectedly called: %d", len(pub.published))
	}
}

func TestAuditEmitter_EmitAsync_RejectsInvalidInput(t *testing.T) {
	t.Parallel()
	pub := &auditStubPublisher{}
	emitter, _ := NewAuditEmitter(pub, nil)
	if err := emitter.EmitAsync(context.Background(), AuditEventInput{Source: "x"}); err == nil {
		t.Errorf("expected validation error")
	}
}

// ---- canonical tag keys are stable -------------------------------------

func TestCanonicalAuditTagKeys_StableSpellings(t *testing.T) {
	t.Parallel()
	// Pin the canonical spellings so a future rename has to update
	// this test deliberately. Consumers across multiple binaries
	// read these keys; drift = silent breakage.
	pinned := map[string]string{
		"AuditTagActor":        AuditTagActor,
		"AuditTagAction":       AuditTagAction,
		"AuditTagResource":     AuditTagResource,
		"AuditTagResourceType": AuditTagResourceType,
		"AuditTagOutcome":      AuditTagOutcome,
		"AuditTagReason":       AuditTagReason,
	}
	want := map[string]string{
		"AuditTagActor":        "actor",
		"AuditTagAction":       "action",
		"AuditTagResource":     "resource",
		"AuditTagResourceType": "resource_type",
		"AuditTagOutcome":      "outcome",
		"AuditTagReason":       "reason",
	}
	for name, got := range pinned {
		if got != want[name] {
			t.Errorf("%s = %q, want %q (canonical spelling drifted)", name, got, want[name])
		}
	}
}

