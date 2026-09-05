// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// fanoutStub records what each agent was asked to converge and returns
// a scripted response per agent.
type fanoutStub struct {
	mu       sync.Mutex
	seen     map[string][]byte
	respond  func(agentID string) (agent.ConvergeResponse, error)
	inflight int
	maxSeen  int
}

func (f *fanoutStub) Converge(_ context.Context, t controlplane.ConvergeTarget) (agent.ConvergeResponse, error) {
	f.mu.Lock()
	if f.seen == nil {
		f.seen = map[string][]byte{}
	}
	f.seen[t.AgentID] = append([]byte(nil), t.YAML...)
	f.inflight++
	if f.inflight > f.maxSeen {
		f.maxSeen = f.inflight
	}
	f.mu.Unlock()

	resp, err := f.respond(t.AgentID)

	f.mu.Lock()
	f.inflight--
	f.mu.Unlock()
	return resp, err
}

func okResponse(agentID string) (agent.ConvergeResponse, error) {
	return agent.ConvergeResponse{
		AgentID: agentID, Changed: 1,
		Results: []agent.ConvergeDeclResult{
			{DeclID: "file:/etc/app.conf", Module: "file", Outcome: "changed"},
		},
	}, nil
}

func newRunner(t *testing.T, f *fanoutStub, agents ...string) *controlplane.BlueprintConvergeRunner {
	t.Helper()
	r, err := controlplane.NewBlueprintConvergeRunner(controlplane.BlueprintConvergeConfig{
		Converge: f, Agents: agents, Source: "demo.yaml", RunID: "run-1", Principal: "admin",
	})
	if err != nil {
		t.Fatalf("NewBlueprintConvergeRunner: %v", err)
	}
	return r
}

// Every targeted agent receives the same state file, verbatim. If the
// blueprint were resolved per agent they could diverge.
func TestBlueprintRunner_SendsTheFileToEveryAgent(t *testing.T) {
	f := &fanoutStub{respond: okResponse}
	r := newRunner(t, f, "agent-2", "agent-1")

	file := []byte("file:\n  /etc/app.conf:\n    state: present\n")
	report, err := r.RunFile(context.Background(), file)
	if err != nil {
		t.Fatalf("RunFile: %v", err)
	}
	if len(f.seen) != 2 {
		t.Fatalf("converged %d agents, want 2", len(f.seen))
	}
	for _, id := range []string{"agent-1", "agent-2"} {
		got, ok := f.seen[id]
		if !ok {
			t.Errorf("%s never received the state file", id)
			continue
		}
		if string(got) != string(file) {
			t.Errorf("%s received a different file:\n got %q\nwant %q", id, got, file)
		}
	}
	if report.Changed != 2 {
		t.Errorf("merged Changed = %d, want 2 (one per agent)", report.Changed)
	}
}

// A merged report must say which host each line came from, or a fleet
// report is a list of identical-looking declarations.
func TestBlueprintRunner_AttributesResultsToAgents(t *testing.T) {
	f := &fanoutStub{respond: okResponse}
	r := newRunner(t, f, "agent-1", "agent-2")

	report, err := r.RunFile(context.Background(), []byte("file: {}\n"))
	if err != nil {
		t.Fatalf("RunFile: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(report.Results))
	}
	for _, res := range report.Results {
		if !strings.Contains(res.DeclID, "|") {
			t.Errorf("result %q carries no agent attribution", res.DeclID)
		}
	}
	// Sorted, so the transcript is stable across runs.
	if report.Results[0].DeclID > report.Results[1].DeclID {
		t.Error("merged results are not sorted")
	}
	if report.Results[0].Outcome != statemgmt.OutcomeChanged {
		t.Errorf("outcome = %v, want changed", report.Results[0].Outcome)
	}
}

// One host failing must not stop the others: an operator needs to know
// which hosts are wrong, not just the first.
func TestBlueprintRunner_OneAgentFailingDoesNotAbortTheRest(t *testing.T) {
	f := &fanoutStub{respond: func(agentID string) (agent.ConvergeResponse, error) {
		if agentID == "agent-2" {
			return agent.ConvergeResponse{}, errors.New("unreachable")
		}
		return okResponse(agentID)
	}}
	r := newRunner(t, f, "agent-1", "agent-2", "agent-3")

	report, err := r.RunFile(context.Background(), []byte("file: {}\n"))
	if err == nil {
		t.Fatal("RunFile() error = nil, want the failure surfaced")
	}
	if !strings.Contains(err.Error(), "agent-2") {
		t.Errorf("error = %v, want it to name the failing agent", err)
	}
	// The report still exists, so the operator sees the whole picture.
	if report == nil {
		t.Fatal("report = nil; a partial failure must still report")
	}
	if len(f.seen) != 3 {
		t.Errorf("converged %d agents, want all 3 attempted", len(f.seen))
	}
	if report.Changed != 2 {
		t.Errorf("Changed = %d, want 2 from the two healthy agents", report.Changed)
	}
}

// A rejection is a policy outcome, not a transport error, and must be
// counted as a failure rather than silently dropped.
func TestBlueprintRunner_RejectionCountsAsFailure(t *testing.T) {
	f := &fanoutStub{respond: func(agentID string) (agent.ConvergeResponse, error) {
		return agent.ConvergeResponse{AgentID: agentID, Rejected: true, RejectReason: "not permitted"}, nil
	}}
	r := newRunner(t, f, "agent-1")

	report, err := r.RunFile(context.Background(), []byte("file: {}\n"))
	if err == nil {
		t.Fatal("RunFile() error = nil for a rejected converge")
	}
	if !strings.Contains(err.Error(), "not permitted") {
		t.Errorf("error = %v, want the reject reason", err)
	}
	if report.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Failed)
	}
}

func TestBlueprintRunner_AgentSideErrorCountsAsFailure(t *testing.T) {
	f := &fanoutStub{respond: func(agentID string) (agent.ConvergeResponse, error) {
		return agent.ConvergeResponse{AgentID: agentID, Error: "compile failed"}, nil
	}}
	r := newRunner(t, f, "agent-1")

	if _, err := r.RunFile(context.Background(), []byte("file: {}\n")); err == nil {
		t.Fatal("RunFile() error = nil for an agent-side error")
	}
}

// Agents are converged concurrently; a fleet apply that serialised
// would scale with fleet size.
func TestBlueprintRunner_ConvergesConcurrently(t *testing.T) {
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(3)
	f := &fanoutStub{respond: func(agentID string) (agent.ConvergeResponse, error) {
		started.Done()
		<-release
		return okResponse(agentID)
	}}
	r := newRunner(t, f, "agent-1", "agent-2", "agent-3")

	done := make(chan error, 1)
	go func() {
		_, err := r.RunFile(context.Background(), []byte("file: {}\n"))
		done <- err
	}()
	started.Wait() // all three in flight at once, or this blocks
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("RunFile: %v", err)
	}
	if f.maxSeen < 3 {
		t.Errorf("max concurrent converges = %d, want 3", f.maxSeen)
	}
}

// A remote apply with no hosts is a mistake worth surfacing, not a
// no-op that reports success.
func TestNewBlueprintConvergeRunner_Requires(t *testing.T) {
	t.Run("no agents", func(t *testing.T) {
		_, err := controlplane.NewBlueprintConvergeRunner(controlplane.BlueprintConvergeConfig{
			Converge: &fanoutStub{respond: okResponse},
		})
		if !errors.Is(err, controlplane.ErrNoAgents) {
			t.Errorf("error = %v, want ErrNoAgents", err)
		}
	})

	t.Run("no converge fanout", func(t *testing.T) {
		_, err := controlplane.NewBlueprintConvergeRunner(controlplane.BlueprintConvergeConfig{
			Agents: []string{"agent-1"},
		})
		if err == nil {
			t.Error("error = nil without a Converge fanout")
		}
	})
}

// Agents() feeds the AppliedRun record, so rollback reaches the hosts
// the apply actually ran on.
func TestBlueprintRunner_AgentsIsSortedAndCopied(t *testing.T) {
	f := &fanoutStub{respond: okResponse}
	r := newRunner(t, f, "agent-3", "agent-1", "agent-2")

	got := r.Agents()
	want := []string{"agent-1", "agent-2", "agent-3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Agents() = %v, want %v", got, want)
		}
	}
	got[0] = "mutated"
	if r.Agents()[0] != "agent-1" {
		t.Error("Agents() handed out a slice the caller can mutate")
	}
}
