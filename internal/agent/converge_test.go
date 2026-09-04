// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// engineRegistry returns a registry with the stdlib registered, which
// is what cmd/kscore-agent gives the real engine.
func engineRegistry(t *testing.T) *statemgmt.Registry {
	t.Helper()
	reg := statemgmt.NewRegistry()
	if err := stdlib.RegisterAll(reg); err != nil {
		t.Fatalf("stdlib.RegisterAll: %v", err)
	}
	return reg
}

// The engine converges the host it runs on — the whole point of
// compiling agent-side rather than shipping resolved declarations.
func TestStateEngine_Converge_Apply(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app.env")

	yaml := []byte(`
metadata:
  name: converge-test
  version: "1.0"

file:
  ` + target + `:
    state: present
    mode: "0600"
    content: |
      HELLO=world
`)

	e := &StateEngine{Registry: engineRegistry(t)}
	report, err := e.Converge(context.Background(), ConvergeModeApply, yaml, nil, nil)
	if err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if report.Failed != 0 {
		t.Fatalf("report.Failed = %d, want 0: %+v", report.Failed, report.Results)
	}
	if report.Changed != 1 {
		t.Errorf("report.Changed = %d, want 1", report.Changed)
	}
	body, err := os.ReadFile(target) //nolint:gosec // t.TempDir path
	if err != nil {
		t.Fatalf("read converged file: %v", err)
	}
	if !strings.Contains(string(body), "HELLO=world") {
		t.Errorf("converged file = %q, want it to contain HELLO=world", body)
	}
}

// Idempotence is the property that makes re-applying safe, so assert
// the second run reports no change rather than merely not failing.
func TestStateEngine_Converge_Idempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "idem.txt")
	yaml := []byte("metadata:\n  name: idem\n  version: \"1.0\"\n\nfile:\n  " +
		target + ":\n    state: present\n    content: |\n      once\n")

	e := &StateEngine{Registry: engineRegistry(t)}
	if _, err := e.Converge(context.Background(), ConvergeModeApply, yaml, nil, nil); err != nil {
		t.Fatalf("first Converge: %v", err)
	}
	report, err := e.Converge(context.Background(), ConvergeModeApply, yaml, nil, nil)
	if err != nil {
		t.Fatalf("second Converge: %v", err)
	}
	if report.Changed != 0 {
		t.Errorf("second apply Changed = %d, want 0 (not idempotent)", report.Changed)
	}
}

// Variables sent by the control plane override the state file's own
// defaults. This is the path a credential travels: the file carries a
// placeholder, the value arrives at dispatch.
func TestStateEngine_Converge_VariableOverride(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "vars.env")
	yaml := []byte(`
metadata:
  name: vars
  version: "1.0"

variables:
  greeting: PLACEHOLDER

file:
  ` + target + `:
    state: present
    content: |
      GREETING={{ .Vars.greeting }}
`)

	e := &StateEngine{Registry: engineRegistry(t)}
	if _, err := e.Converge(context.Background(), ConvergeModeApply, yaml,
		map[string]string{"greeting": "overridden"}, nil); err != nil {
		t.Fatalf("Converge: %v", err)
	}
	body, _ := os.ReadFile(target) //nolint:gosec // t.TempDir path
	if !strings.Contains(string(body), "GREETING=overridden") {
		t.Errorf("rendered = %q, want the override applied", body)
	}
	if strings.Contains(string(body), "PLACEHOLDER") {
		t.Errorf("rendered = %q, still carries the placeholder", body)
	}
}

// Facts resolve against the host running the engine. Rendering these
// on the control plane would describe the wrong machine, which is the
// reason compilation is agent-side.
func TestStateEngine_Converge_FactsRender(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "facts.txt")
	yaml := []byte("metadata:\n  name: facts\n  version: \"1.0\"\n\nfile:\n  " +
		target + ":\n    state: present\n    content: |\n      OS={{ .Facts.os }}\n")

	e := &StateEngine{Registry: engineRegistry(t)}
	if _, err := e.Converge(context.Background(), ConvergeModeApply, yaml, nil,
		map[string]any{"os": "plan9"}); err != nil {
		t.Fatalf("Converge: %v", err)
	}
	body, _ := os.ReadFile(target) //nolint:gosec // t.TempDir path
	if !strings.Contains(string(body), "OS=plan9") {
		t.Errorf("rendered = %q, want the supplied fact", body)
	}
}

// Check mode must not touch the host.
func TestStateEngine_Converge_CheckDoesNotApply(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "check-only.txt")
	yaml := []byte("metadata:\n  name: chk\n  version: \"1.0\"\n\nfile:\n  " +
		target + ":\n    state: present\n    content: |\n      nope\n")

	e := &StateEngine{Registry: engineRegistry(t)}
	if _, err := e.Converge(context.Background(), ConvergeModeCheck, yaml, nil, nil); err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("check mode created the file; it must only report")
	}
}

func TestStateEngine_Converge_Errors(t *testing.T) {
	e := &StateEngine{Registry: engineRegistry(t)}
	tests := []struct {
		name     string
		mode     string
		yaml     string
		wantFrag string
	}{
		{"empty state file", ConvergeModeApply, "", "empty state file"},
		{"unparseable", ConvergeModeApply, "\tnot: [yaml", "parse"},
		{"unknown mode", "montage", "metadata:\n  name: x\n  version: \"1.0\"\n", "unknown mode"},
		{
			// Includes need a state library the agent does not have.
			// Silently ignoring one would converge less than asked.
			name:     "includes rejected",
			mode:     ConvergeModeApply,
			yaml:     "metadata:\n  name: x\n  version: \"1.0\"\nincludes:\n  - other.yaml\n",
			wantFrag: "includes are not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.Converge(context.Background(), tt.mode, []byte(tt.yaml), nil, nil)
			if err == nil {
				t.Fatalf("Converge() = nil error, want one containing %q", tt.wantFrag)
			}
			if !strings.Contains(err.Error(), tt.wantFrag) {
				t.Errorf("Converge() = %v, want it to contain %q", err, tt.wantFrag)
			}
		})
	}
}

// A nil Registry falls back to DefaultRegistry, which is what the real
// agent relies on after stdlib.RegisterAll populates it.
func TestStateEngine_DefaultRegistryFallback(t *testing.T) {
	e := &StateEngine{}
	if e.registry() != statemgmt.DefaultRegistry {
		t.Error("registry() did not fall back to DefaultRegistry")
	}
}

// --- handler + authz --------------------------------------------------

// signConverge signs req the way the control plane will, so the test
// exercises the real verification path rather than a disabled one.
func signConverge(t *testing.T, e *SecurityEnforcer, req ConvergeRequest) ConvergeRequest {
	t.Helper()
	req.Signature = e.ComputeConvergeHMAC(req)
	return req
}

func TestConverge_HMACRoundTrip(t *testing.T) {
	e, err := NewSecurityEnforcer(SecurityPolicy{HMACSecret: []byte("s3cret")}, nil)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	req := signConverge(t, e, ConvergeRequest{
		MessageID: "m-1", Principal: "ops", RunID: "run-1",
		Mode: ConvergeModeApply, YAML: []byte("metadata:\n  name: x\n"),
		Variables: map[string]string{"b": "2", "a": "1"},
	})
	if err := e.ValidateConverge(context.Background(), req); err != nil {
		t.Fatalf("ValidateConverge on a correctly signed request: %v", err)
	}
}

// Every signed field must actually be covered by the signature —
// otherwise an attacker rewrites the part that decides what runs.
func TestConverge_HMACCoversPayload(t *testing.T) {
	e, err := NewSecurityEnforcer(SecurityPolicy{HMACSecret: []byte("s3cret")}, nil)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	base := signConverge(t, e, ConvergeRequest{
		MessageID: "m-1", Principal: "ops", RunID: "run-1", Source: "app.yaml",
		Mode: ConvergeModeApply, YAML: []byte("metadata:\n  name: x\n"),
		Variables: map[string]string{"a": "1"}, TimeoutSeconds: 30,
	})

	tamper := map[string]func(*ConvergeRequest){
		"yaml":      func(r *ConvergeRequest) { r.YAML = []byte("metadata:\n  name: evil\n") },
		"mode":      func(r *ConvergeRequest) { r.Mode = ConvergeModeCheck },
		"principal": func(r *ConvergeRequest) { r.Principal = "someone-else" },
		"run id":    func(r *ConvergeRequest) { r.RunID = "run-2" },
		"source":    func(r *ConvergeRequest) { r.Source = "other.yaml" },
		"timeout":   func(r *ConvergeRequest) { r.TimeoutSeconds = 3600 },
		"variables": func(r *ConvergeRequest) { r.Variables = map[string]string{"a": "2"} },
	}
	for name, mutate := range tamper {
		t.Run(name, func(t *testing.T) {
			req := base
			mutate(&req)
			if err := e.ValidateConverge(context.Background(), req); err == nil {
				t.Errorf("tampering with %s was not detected by the signature", name)
			}
		})
	}
}

// Variable maps iterate in random order, so an unsorted canonical
// encoding produces a signature that verifies only sometimes.
func TestConverge_HMACStableAcrossMapOrder(t *testing.T) {
	e, err := NewSecurityEnforcer(SecurityPolicy{HMACSecret: []byte("s3cret")}, nil)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	req := ConvergeRequest{
		MessageID: "m-1", Mode: ConvergeModeApply,
		Variables: map[string]string{"a": "1", "b": "2", "c": "3", "d": "4", "e": "5"},
	}
	want := e.ComputeConvergeHMAC(req)
	for i := 0; i < 50; i++ {
		if got := e.ComputeConvergeHMAC(req); got != want {
			t.Fatalf("signature not stable across map iteration order: %s != %s", got, want)
		}
	}
}

// A state run is not a lesser privilege than a command, so it uses the
// same principal allowlist.
func TestConverge_PrincipalAllowlist(t *testing.T) {
	e, err := NewSecurityEnforcer(SecurityPolicy{PrincipalAllowlist: []string{"ops"}}, nil)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	if err := e.ValidateConverge(context.Background(), ConvergeRequest{Principal: "ops"}); err != nil {
		t.Errorf("allowed principal rejected: %v", err)
	}
	if err := e.ValidateConverge(context.Background(), ConvergeRequest{Principal: "intruder"}); err == nil {
		t.Error("principal outside the allowlist was accepted")
	}
}

// --- end-to-end over the fake bus -------------------------------------

// The agent must subscribe to BOTH subjects. A half-subscribed agent
// that answers commands but ignores state runs is the failure this
// guards: it looks healthy and silently does nothing.
func TestAgent_SubscribesToConverge(t *testing.T) {
	a, nats, subj, _ := newAgentWithEnforcer(t, SecurityPolicy{DefaultPolicy: PolicyAllow})
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	for _, want := range []string{subj.AgentCommand("agent-1"), subj.AgentConverge("agent-1")} {
		if !nats.subscribed(want) {
			t.Errorf("agent did not subscribe to %q", want)
		}
	}
	if nats.handlerFor(subj.AgentConverge("agent-1")) == nil {
		t.Error("no handler attached to the converge subject")
	}
}

// The full path: signed request in on the converge subject, host
// converged, result out on the converge-result subject.
func TestAgent_ConvergeFlow_RoundTrip(t *testing.T) {
	a, nats, subj, enf := newAgentWithEnforcer(t, SecurityPolicy{
		HMACSecret:    []byte("secret-1"),
		DefaultPolicy: PolicyAllow,
	})
	a.stateEngine = &StateEngine{Registry: engineRegistry(t)}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	target := filepath.Join(t.TempDir(), "converged.env")
	req := ConvergeRequest{
		MessageID: "run-msg-1",
		Principal: "admin",
		RunID:     "run-1",
		Source:    "app-env.yaml",
		Mode:      ConvergeModeApply,
		YAML: []byte("metadata:\n  name: e2e\n  version: \"1.0\"\n\nfile:\n  " +
			target + ":\n    state: present\n    content: |\n      SHIPPED=true\n"),
		TimeoutSeconds: 10,
	}
	req.Signature = enf.ComputeConvergeHMAC(req)

	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("run-msg-1"))
	if err := nats.deliver(t, subj.AgentConverge("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	envs := nats.collectFor(t, subj.AgentConvergeResult("agent-1"), 1, 3*time.Second)
	var resp ConvergeResponse
	if err := json.Unmarshal(envs[0].Payload, &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if resp.Rejected {
		t.Fatalf("Rejected = true, want false (reason=%q)", resp.RejectReason)
	}
	if resp.Error != "" {
		t.Fatalf("Error = %q, want empty", resp.Error)
	}
	if resp.RunID != "run-1" || resp.AgentID != "agent-1" {
		t.Errorf("RunID/AgentID = %q/%q, want run-1/agent-1", resp.RunID, resp.AgentID)
	}
	if resp.Changed != 1 {
		t.Errorf("Changed = %d, want 1", resp.Changed)
	}
	if len(resp.Results) != 1 || resp.Results[0].Outcome != "changed" {
		t.Errorf("Results = %+v, want one changed declaration", resp.Results)
	}
	if envs[0].CorrelationID != "run-msg-1" {
		t.Errorf("CorrelationID = %q, want run-msg-1", envs[0].CorrelationID)
	}
	// The point of the whole exercise: the file exists on this host.
	if _, err := os.Stat(target); err != nil {
		t.Errorf("declared file not converged on the agent host: %v", err)
	}
}

// A rejected run answers rather than going silent, so the control
// plane reports a reason instead of waiting out a timeout.
func TestAgent_ConvergeFlow_BadSignatureIsRejected(t *testing.T) {
	a, nats, subj, _ := newAgentWithEnforcer(t, SecurityPolicy{
		HMACSecret:    []byte("secret-1"),
		DefaultPolicy: PolicyAllow,
	})
	a.stateEngine = &StateEngine{Registry: engineRegistry(t)}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	target := filepath.Join(t.TempDir(), "must-not-exist.env")
	req := ConvergeRequest{
		MessageID: "run-msg-2",
		Principal: "admin",
		RunID:     "run-2",
		Mode:      ConvergeModeApply,
		YAML: []byte("metadata:\n  name: nope\n  version: \"1.0\"\n\nfile:\n  " +
			target + ":\n    state: present\n    content: |\n      nope\n"),
		Signature: "deadbeef",
	}
	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("run-msg-2"))
	if err := nats.deliver(t, subj.AgentConverge("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	envs := nats.collectFor(t, subj.AgentConvergeResult("agent-1"), 1, 3*time.Second)
	var resp ConvergeResponse
	if err := json.Unmarshal(envs[0].Payload, &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !resp.Rejected {
		t.Error("Rejected = false, want true for a bad signature")
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("a rejected run still converged the host")
	}
}
