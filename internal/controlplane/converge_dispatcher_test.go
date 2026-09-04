// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// --- doubles ----------------------------------------------------------

type convergeSubjects struct{}

func (convergeSubjects) AgentConverge(id string) string {
	return "kscore.test.agent." + id + ".converge"
}
func (convergeSubjects) AgentConvergeResultPattern() string {
	return "kscore.test.agent.*.converge.result"
}
func (convergeSubjects) Prefix() string { return "kscore.test" }

type convergeBus struct {
	mu        sync.Mutex
	handler   MessageHandler
	published []envelope.Envelope
	subjects  []string
	pubErr    error
	subErr    error
	unsubbed  bool
	// autoReply, when set, answers every publish as if the agent had.
	autoReply func(req agent.ConvergeRequest) *agent.ConvergeResponse
}

func (b *convergeBus) Subscribe(_ string, h MessageHandler) (Subscription, error) {
	if b.subErr != nil {
		return nil, b.subErr
	}
	b.mu.Lock()
	b.handler = h
	b.mu.Unlock()
	return b, nil
}

func (b *convergeBus) Unsubscribe() error { b.unsubbed = true; return nil }

func (b *convergeBus) PublishEnvelope(_ context.Context, subject string, env envelope.Envelope) error {
	if b.pubErr != nil {
		return b.pubErr
	}
	b.mu.Lock()
	b.published = append(b.published, env)
	b.subjects = append(b.subjects, subject)
	reply := b.autoReply
	h := b.handler
	b.mu.Unlock()

	if reply == nil || h == nil {
		return nil
	}
	var req agent.ConvergeRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		return err
	}
	if resp := reply(req); resp != nil {
		body, _ := json.Marshal(resp)
		go func() {
			_ = h(context.Background(), "",
				envelope.New(body, "kscore.test", envelope.WithCorrelationID(env.MessageID)))
		}()
	}
	return nil
}

type fakeConvergeSigner struct{ calls int }

func (s *fakeConvergeSigner) ComputeConvergeHMAC(agent.ConvergeRequest) string {
	s.calls++
	return "signed"
}

func newConvergeDispatcher(t *testing.T, bus *convergeBus, signer ConvergeSigner) *ConvergeDispatcher {
	t.Helper()
	d, err := NewConvergeDispatcher(ConvergeDispatcherConfig{
		Subscriber: bus, Publisher: bus, Subjects: convergeSubjects{}, Signer: signer,
	})
	if err != nil {
		t.Fatalf("NewConvergeDispatcher: %v", err)
	}
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop() })
	return d
}

// --- tests ------------------------------------------------------------

func TestConvergeDispatcher_RoundTrip(t *testing.T) {
	bus := &convergeBus{autoReply: func(req agent.ConvergeRequest) *agent.ConvergeResponse {
		return &agent.ConvergeResponse{
			MessageID: req.MessageID, AgentID: "agent-1", RunID: req.RunID, Changed: 2,
		}
	}}
	signer := &fakeConvergeSigner{}
	d := newConvergeDispatcher(t, bus, signer)

	resp, err := d.Converge(context.Background(), ConvergeTarget{
		AgentID: "agent-1", RunID: "run-1", Mode: agent.ConvergeModeApply,
		YAML: []byte("metadata:\n  name: x\n"), Principal: "ops", Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if resp.Changed != 2 || resp.AgentID != "agent-1" {
		t.Errorf("resp = %+v, want the agent's reply", resp)
	}
	if signer.calls != 1 {
		t.Errorf("signer called %d times, want 1 — dispatches must be signed", signer.calls)
	}
	if len(bus.subjects) != 1 || bus.subjects[0] != "kscore.test.agent.agent-1.converge" {
		t.Errorf("published to %v, want the agent's converge subject", bus.subjects)
	}
}

// The waiter has to be registered before the publish. A fast agent can
// answer before Publish returns, and a reply with no waiter is dropped.
func TestConvergeDispatcher_RegistersWaiterBeforePublish(t *testing.T) {
	// autoReply fires synchronously inside PublishEnvelope, so this
	// only passes if register() already ran.
	bus := &convergeBus{autoReply: func(req agent.ConvergeRequest) *agent.ConvergeResponse {
		return &agent.ConvergeResponse{MessageID: req.MessageID, AgentID: "agent-1", Changed: 1}
	}}
	d := newConvergeDispatcher(t, bus, nil)

	resp, err := d.Converge(context.Background(), ConvergeTarget{
		AgentID: "agent-1", RunID: "run-1", Mode: agent.ConvergeModeApply, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if resp.Changed != 1 {
		t.Errorf("resp.Changed = %d — the reply raced the waiter registration", resp.Changed)
	}
}

// One unreachable agent must fail on its own budget, not hang.
func TestConvergeDispatcher_TimesOutPerAgent(t *testing.T) {
	bus := &convergeBus{} // never replies
	d := newConvergeDispatcher(t, bus, nil)

	start := time.Now()
	_, err := d.Converge(context.Background(), ConvergeTarget{
		AgentID: "silent", RunID: "run-1", Mode: agent.ConvergeModeApply,
		Timeout: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("Converge() = nil error, want a timeout for a silent agent")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want a deadline error", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second+2*time.Second {
		t.Errorf("waited %v — the per-agent budget was not honoured", elapsed)
	}
}

// Results are keyed by run+agent, so concurrent agents cannot collect
// each other's replies.
func TestConvergeDispatcher_ConcurrentAgentsGetOwnResults(t *testing.T) {
	bus := &convergeBus{autoReply: func(req agent.ConvergeRequest) *agent.ConvergeResponse {
		// Echo the agent id back out of the message id so a
		// mis-routed reply is detectable.
		return &agent.ConvergeResponse{MessageID: req.MessageID, RunID: req.RunID, AgentID: req.MessageID}
	}}
	d := newConvergeDispatcher(t, bus, nil)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		wg.Add(1)
		go func(agentID string) {
			defer wg.Done()
			resp, err := d.Converge(context.Background(), ConvergeTarget{
				AgentID: agentID, RunID: "run-1", Mode: agent.ConvergeModeApply,
				Timeout: 2 * time.Second,
			})
			if err != nil {
				errs <- err
				return
			}
			if want := "run-1:" + agentID; resp.AgentID != want {
				errs <- errors.New("agent " + agentID + " got " + resp.AgentID + "'s result")
			}
		}(id)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestConvergeDispatcher_PublishFailureSurfaces(t *testing.T) {
	bus := &convergeBus{pubErr: errors.New("nats down")}
	d := newConvergeDispatcher(t, bus, nil)
	_, err := d.Converge(context.Background(), ConvergeTarget{
		AgentID: "agent-1", RunID: "run-1", Mode: agent.ConvergeModeApply, Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("Converge() = nil error, want the publish failure surfaced")
	}
}

func TestConvergeDispatcher_Validation(t *testing.T) {
	bus := &convergeBus{}
	tests := []struct {
		name string
		cfg  ConvergeDispatcherConfig
	}{
		{"no subscriber", ConvergeDispatcherConfig{Publisher: bus, Subjects: convergeSubjects{}}},
		{"no publisher", ConvergeDispatcherConfig{Subscriber: bus, Subjects: convergeSubjects{}}},
		{"no subjects", ConvergeDispatcherConfig{Subscriber: bus, Publisher: bus}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewConvergeDispatcher(tt.cfg); err == nil {
				t.Error("NewConvergeDispatcher() = nil error, want a validation failure")
			}
		})
	}

	d := newConvergeDispatcher(t, bus, nil)
	if _, err := d.Converge(context.Background(), ConvergeTarget{RunID: "r"}); err == nil {
		t.Error("Converge() without an agent id = nil error, want an error")
	}
}

// Stop releases waiters rather than leaving a caller blocked until its
// own timeout on a shutting-down control plane.
func TestConvergeDispatcher_StopReleasesWaiters(t *testing.T) {
	bus := &convergeBus{}
	d, err := NewConvergeDispatcher(ConvergeDispatcherConfig{
		Subscriber: bus, Publisher: bus, Subjects: convergeSubjects{},
	})
	if err != nil {
		t.Fatalf("NewConvergeDispatcher: %v", err)
	}
	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := d.Converge(context.Background(), ConvergeTarget{
			AgentID: "agent-1", RunID: "run-1", Mode: agent.ConvergeModeApply, Timeout: 30 * time.Second,
		})
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("Converge returned nil after Stop, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Error("Stop did not release the waiter")
	}
	if !bus.unsubbed {
		t.Error("Stop did not unsubscribe")
	}
}
