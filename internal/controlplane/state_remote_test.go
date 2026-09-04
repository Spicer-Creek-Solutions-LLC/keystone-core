// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// fakeFanout answers per agent id, recording call order.
type fakeFanout struct {
	mu       sync.Mutex
	calls    []string
	respond  func(id string) (agent.ConvergeResponse, error)
	inFlight int
	maxPar   int
}

func (f *fakeFanout) Converge(_ context.Context, t ConvergeTarget) (agent.ConvergeResponse, error) {
	f.mu.Lock()
	f.calls = append(f.calls, t.AgentID)
	f.inFlight++
	if f.inFlight > f.maxPar {
		f.maxPar = f.inFlight
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.inFlight--; f.mu.Unlock() }()
	return f.respond(t.AgentID)
}

// collectStream captures what the server would send to the client.
type collectStream struct {
	mu     sync.Mutex
	events []*v1.ApplyStateResponse
}

func (c *collectStream) Send(ev *v1.ApplyStateResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return nil
}

func (c *collectStream) declResults() []*v1.StateDeclarationResult {
	var out []*v1.StateDeclarationResult
	for _, e := range c.events {
		if d := e.GetDeclResult(); d != nil {
			out = append(out, d)
		}
	}
	return out
}

func agentRecords(ids ...string) []state.AgentRecord {
	recs := make([]state.AgentRecord, 0, len(ids))
	for _, id := range ids {
		recs = append(recs, state.AgentRecord{ID: id})
	}
	return recs
}

func okResponse(id string, changed int) agent.ConvergeResponse {
	return agent.ConvergeResponse{
		AgentID: id, Changed: changed,
		Results: []agent.ConvergeDeclResult{
			{DeclID: "file:/etc/app.env", Module: "file", Outcome: "changed", ApplyChanged: true},
		},
	}
}

// Every declaration result must name the host it ran on. Without it a
// fleet-wide run reports N results per declaration with no way to tell
// them apart.
func TestRunRemote_AttributesResultsToAgents(t *testing.T) {
	fan := &fakeFanout{respond: func(id string) (agent.ConvergeResponse, error) {
		return okResponse(id, 1), nil
	}}
	s := &StateGRPCServer{Converge: fan}
	stream := &collectStream{}

	terminal, err := s.runRemote(context.Background(), stream, "run-1",
		agentRecords("web-2", "web-1"), &v1.ApplyStateRequest{}, agent.ConvergeModeApply, "ops")
	if err != nil {
		t.Fatalf("runRemote: %v", err)
	}

	got := stream.declResults()
	if len(got) != 2 {
		t.Fatalf("got %d decl results, want 2", len(got))
	}
	// Sorted by agent id, not completion order — otherwise two runs of
	// the same file produce different transcripts and diffing is
	// useless.
	if got[0].GetAgentId() != "web-1" || got[1].GetAgentId() != "web-2" {
		t.Errorf("results attributed/ordered as %q,%q; want web-1,web-2",
			got[0].GetAgentId(), got[1].GetAgentId())
	}
	if terminal.GetAggregates().GetChanged() != 2 {
		t.Errorf("aggregate Changed = %d, want 2 (summed across hosts)",
			terminal.GetAggregates().GetChanged())
	}
	if len(terminal.GetAgentSummaries()) != 2 {
		t.Errorf("agent summaries = %d, want one per host", len(terminal.GetAgentSummaries()))
	}
}

// A fleet apply that walked hosts serially would take the sum of their
// runtimes, which defeats the point of targeting a fleet.
func TestRunRemote_DispatchesConcurrently(t *testing.T) {
	release := make(chan struct{})
	var reached sync.WaitGroup
	reached.Add(4)
	fan := &fakeFanout{respond: func(id string) (agent.ConvergeResponse, error) {
		reached.Done()
		<-release // hold every agent until all four have arrived
		return okResponse(id, 0), nil
	}}
	s := &StateGRPCServer{Converge: fan}

	done := make(chan struct{})
	go func() {
		_, _ = s.runRemote(context.Background(), &collectStream{}, "run-1",
			agentRecords("a", "b", "c", "d"), &v1.ApplyStateRequest{}, agent.ConvergeModeApply, "ops")
		close(done)
	}()
	reached.Wait() // would deadlock if dispatch were serial
	close(release)
	<-done

	if fan.maxPar < 4 {
		t.Errorf("max concurrent dispatches = %d, want 4", fan.maxPar)
	}
}

// One host failing must not abort the others — otherwise the fleet is
// left half-applied according to scheduling order.
func TestRunRemote_OneAgentFailureDoesNotAbortOthers(t *testing.T) {
	fan := &fakeFanout{respond: func(id string) (agent.ConvergeResponse, error) {
		if id == "broken" {
			return agent.ConvergeResponse{}, errors.New("unreachable")
		}
		return okResponse(id, 1), nil
	}}
	s := &StateGRPCServer{Converge: fan}
	stream := &collectStream{}

	terminal, err := s.runRemote(context.Background(), stream, "run-1",
		agentRecords("alpha", "broken", "zulu"), &v1.ApplyStateRequest{}, agent.ConvergeModeApply, "ops")
	if err != nil {
		t.Fatalf("runRemote: %v", err)
	}
	if len(fan.calls) != 3 {
		t.Errorf("dispatched to %d agents, want all 3 attempted", len(fan.calls))
	}
	// The healthy hosts still converged and still reported.
	if terminal.GetAggregates().GetChanged() != 2 {
		t.Errorf("Changed = %d, want 2 from the healthy hosts",
			terminal.GetAggregates().GetChanged())
	}
	if terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_FAILED {
		t.Error("run status = completed, want failed when a host failed")
	}

	// The operator has to be able to tell WHICH host to go look at.
	var broken *v1.StateAgentSummary
	for _, sum := range terminal.GetAgentSummaries() {
		if sum.GetAgentId() == "broken" {
			broken = sum
		}
	}
	if broken == nil {
		t.Fatal("no summary for the failing host")
	}
	if !strings.Contains(broken.GetErrorMessage(), "unreachable") {
		t.Errorf("summary error = %q, want it to name the failure", broken.GetErrorMessage())
	}
}

// A refusal is a distinct outcome from a declaration failing, and must
// be reported as such rather than as a silent success.
func TestRunRemote_RejectionIsReported(t *testing.T) {
	fan := &fakeFanout{respond: func(id string) (agent.ConvergeResponse, error) {
		return agent.ConvergeResponse{AgentID: id, Rejected: true, RejectReason: "hmac invalid"}, nil
	}}
	s := &StateGRPCServer{Converge: fan}

	terminal, err := s.runRemote(context.Background(), &collectStream{}, "run-1",
		agentRecords("a"), &v1.ApplyStateRequest{}, agent.ConvergeModeApply, "ops")
	if err != nil {
		t.Fatalf("runRemote: %v", err)
	}
	if terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_FAILED {
		t.Error("a rejected run reported as completed")
	}
	if got := terminal.GetAgentSummaries()[0].GetErrorMessage(); !strings.Contains(got, "hmac invalid") {
		t.Errorf("summary error = %q, want the rejection reason", got)
	}
}

// A targeted request against a server with no dispatch wiring must be
// refused, not silently converge the control plane instead of the fleet.
func TestResolveRemoteTargets_UnwiredServerRefuses(t *testing.T) {
	s := &StateGRPCServer{}
	_, err := s.resolveRemoteTargets(context.Background(), &v1.ApplyStateRequest{
		Target: &v1.Target{AgentIds: []string{"a"}},
	})
	if err == nil {
		t.Fatal("targeted request on an unwired server = nil error")
	}
	if !strings.Contains(err.Error(), "not wired") {
		t.Errorf("err = %v, want it to say remote apply is not wired", err)
	}
}

// An untargeted request stays local — that is still the default and
// this commit must not change it.
func TestResolveRemoteTargets_EmptyTargetStaysLocal(t *testing.T) {
	s := &StateGRPCServer{}
	agents, err := s.resolveRemoteTargets(context.Background(), &v1.ApplyStateRequest{})
	if err != nil {
		t.Fatalf("untargeted request errored: %v", err)
	}
	if agents != nil {
		t.Errorf("agents = %v, want nil so the run stays on the control plane", agents)
	}
}

// agent_id is attribution, not targeting. Promoting it would silently
// turn existing callers' bookkeeping into fleet execution.
func TestResolveRemoteTargets_AgentIDIsNotATarget(t *testing.T) {
	s := &StateGRPCServer{}
	agents, err := s.resolveRemoteTargets(context.Background(), &v1.ApplyStateRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("agent_id-only request errored: %v", err)
	}
	if agents != nil {
		t.Errorf("agents = %v, want nil — agent_id records attribution, it does not dispatch", agents)
	}
}

func TestOutcomeProto(t *testing.T) {
	tests := map[string]v1.StateRunOutcome{
		"changed":        v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED,
		"unchanged":      v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED,
		"no-op":          v1.StateRunOutcome_STATE_RUN_OUTCOME_NO_OP,
		"failed":         v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED,
		"drift-detected": v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED,
		"skipped":        v1.StateRunOutcome_STATE_RUN_OUTCOME_SKIPPED,
		"nonsense":       v1.StateRunOutcome_STATE_RUN_OUTCOME_UNSPECIFIED,
	}
	for in, want := range tests {
		if got := outcomeProto(in); got != want {
			t.Errorf("outcomeProto(%q) = %v, want %v", in, got, want)
		}
	}
}
