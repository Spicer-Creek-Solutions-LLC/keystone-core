// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ConvergeFanout is the seam StateGRPCServer uses to reach agents.
// *ConvergeDispatcher satisfies it; tests substitute a fake so the
// fan-out logic can be exercised without a bus.
type ConvergeFanout interface {
	Converge(ctx context.Context, t ConvergeTarget) (agent.ConvergeResponse, error)
}

// AgentTargetResolver resolves a Target to agent records.
// *StateGRPCServer holds one so remote apply can select hosts the same
// way batch exec does, rather than growing a second targeting dialect.
type AgentTargetResolver interface {
	Resolve(ctx context.Context, t Target) ([]state.AgentRecord, error)
}

// storeResolver adapts an AgentStore to AgentTargetResolver.
type storeResolver struct{ store state.AgentStore }

func (r storeResolver) Resolve(ctx context.Context, t Target) ([]state.AgentRecord, error) {
	return ResolveTarget(ctx, r.store, t)
}

// NewStoreResolver returns the production AgentTargetResolver.
func NewStoreResolver(store state.AgentStore) AgentTargetResolver { return storeResolver{store: store} }

// remoteRunResult is one agent's outcome, ordered for deterministic
// streaming.
type remoteRunResult struct {
	agentID string
	resp    agent.ConvergeResponse
	err     error
}

// runRemote converges every targeted agent and streams the results.
//
// Agents run CONCURRENTLY — a fleet-wide apply that walked hosts
// serially would take the sum of their runtimes, and the whole point of
// targeting a fleet is that it does not. Results are collected, sorted
// by agent id, and only then streamed, so the output is deterministic
// regardless of which host finished first; a run that emitted results
// in completion order would produce a different transcript every time
// and make diffing two runs useless.
//
// One agent's failure never aborts the others. A host that is
// unreachable, refuses the run, or fails to compile is recorded against
// itself and the remaining hosts still converge — the alternative
// leaves a fleet in a half-applied state decided by scheduling order.
func (s *StateGRPCServer) runRemote(
	ctx context.Context,
	stream interface {
		Send(*v1.ApplyStateResponse) error
	},
	runID string,
	agents []state.AgentRecord,
	req *v1.ApplyStateRequest,
	mode string,
	principal string,
) (*v1.StateRunTerminal, error) {
	results := make([]remoteRunResult, len(agents))
	var wg sync.WaitGroup
	for i := range agents {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := agents[i].ID
			resp, err := s.Converge.Converge(ctx, ConvergeTarget{
				AgentID:   id,
				RunID:     runID,
				Source:    req.GetSource(),
				Mode:      mode,
				YAML:      req.GetYamlContent(),
				Vars:      req.GetVariableOverrides(),
				Principal: principal,
				Timeout:   s.convergeTimeout(),
			})
			results[i] = remoteRunResult{agentID: id, resp: resp, err: err}
		}(i)
	}
	wg.Wait()

	sort.Slice(results, func(a, b int) bool { return results[a].agentID < results[b].agentID })

	terminal := &v1.StateRunTerminal{
		RunId:      runID,
		Aggregates: &v1.StateRunAggregates{},
	}
	anyFailed := false

	for _, r := range results {
		summary := &v1.StateAgentSummary{
			AgentId:    r.agentID,
			Aggregates: &v1.StateRunAggregates{},
		}
		switch {
		case r.err != nil:
			// Unreachable, refused, or timed out. Distinct from a
			// declaration failing, so it lands on the summary's error
			// rather than being counted as a failed declaration the
			// operator would go looking for in the results.
			anyFailed = true
			summary.Status = v1.StateRunStatus_STATE_RUN_STATUS_FAILED
			summary.ErrorMessage = r.err.Error()
		case r.resp.Rejected:
			anyFailed = true
			summary.Status = v1.StateRunStatus_STATE_RUN_STATUS_FAILED
			summary.ErrorMessage = "rejected by agent: " + r.resp.RejectReason
		case r.resp.Error != "":
			anyFailed = true
			summary.Status = v1.StateRunStatus_STATE_RUN_STATUS_FAILED
			summary.ErrorMessage = r.resp.Error
		default:
			summary.Status = v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED
		}

		for _, d := range r.resp.Results {
			if err := stream.Send(&v1.ApplyStateResponse{
				Event: &v1.ApplyStateResponse_DeclResult{
					DeclResult: declResultProto(r.agentID, d),
				},
			}); err != nil {
				return nil, err
			}
		}

		summary.Aggregates.Changed = int32(r.resp.Changed)
		summary.Aggregates.Unchanged = int32(r.resp.Unchanged)
		summary.Aggregates.Failed = int32(r.resp.Failed)
		summary.Aggregates.Skipped = int32(r.resp.Skipped)
		if r.resp.Failed > 0 {
			anyFailed = true
			summary.Status = v1.StateRunStatus_STATE_RUN_STATUS_FAILED
		}

		terminal.Aggregates.Changed += summary.Aggregates.Changed
		terminal.Aggregates.Unchanged += summary.Aggregates.Unchanged
		terminal.Aggregates.Failed += summary.Aggregates.Failed
		terminal.Aggregates.Skipped += summary.Aggregates.Skipped
		terminal.AgentSummaries = append(terminal.AgentSummaries, summary)
	}

	terminal.Status = v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED
	if anyFailed {
		terminal.Status = v1.StateRunStatus_STATE_RUN_STATUS_FAILED
	}
	return terminal, nil
}

// declResultProto maps one agent's declaration result onto the wire.
func declResultProto(agentID string, d agent.ConvergeDeclResult) *v1.StateDeclarationResult {
	return &v1.StateDeclarationResult{
		AgentId:      agentID,
		DeclId:       d.DeclID,
		Module:       d.Module,
		Outcome:      outcomeProto(d.Outcome),
		CheckDiff:    d.CheckDiff,
		ApplyChanged: d.ApplyChanged,
		ApplyDiff:    d.ApplyDiff,
		ApplyComment: d.ApplyComment,
		ErrorMessage: d.ErrorMessage,
		DurationMs:   d.DurationMs,
	}
}

// outcomeProto maps the agent's stringly outcome onto the enum. The
// agent sends strings because its wire format is JSON and it must not
// depend on the control plane's generated protos.
func outcomeProto(s string) v1.StateRunOutcome {
	switch s {
	case "changed":
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED
	case "unchanged":
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED
	case "no-op":
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_NO_OP
	case "failed":
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED
	case "drift-detected":
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED
	case "skipped":
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_SKIPPED
	default:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_UNSPECIFIED
	}
}

// convergeTimeout is the per-agent budget for a remote run.
func (s *StateGRPCServer) convergeTimeout() time.Duration {
	if s.ConvergeTimeout > 0 {
		return s.ConvergeTimeout
	}
	return defaultConvergeTimeout
}

// defaultConvergeTimeout is generous by command-dispatch standards: a
// state run may install packages, so minutes is the right order, not
// seconds.
const defaultConvergeTimeout = 10 * time.Minute

// targetFromRequest builds a Target from the request's target message.
//
// Deliberately NOT the scalar agent_id field. That one is attribution —
// "record this run against that host" — and has meant exactly that
// since it was added; promoting it to a dispatch selector would
// silently turn existing callers' bookkeeping into fleet execution.
// Targeting goes through target, and the follow-up that makes a target
// mandatory settles what agent_id means afterwards.
func targetFromRequest(req *v1.ApplyStateRequest) Target {
	return Target{
		AgentIDs:        req.GetTarget().GetAgentIds(),
		Labels:          req.GetTarget().GetLabels(),
		HostnamePattern: req.GetTarget().GetHostnamePattern(),
	}
}

// resolveRemoteTargets returns the agents a request selects, or nil
// when it selects none (the run stays on the control plane).
func (s *StateGRPCServer) resolveRemoteTargets(ctx context.Context, req *v1.ApplyStateRequest) ([]state.AgentRecord, error) {
	t := targetFromRequest(req)
	if t.isEmpty() {
		return nil, nil
	}
	if s.Resolver == nil || s.Converge == nil {
		return nil, fmt.Errorf("controlplane: remote state apply is not wired on this server")
	}
	agents, err := s.Resolver.Resolve(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("controlplane: resolve target: %w", err)
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("controlplane: target matched no agents")
	}
	return agents, nil
}

// convergePrincipal names the operator a remote run is dispatched on
// behalf of. It travels into the signature and is checked against each
// agent's principal allowlist, so an unauthenticated call must not
// borrow an authenticated one's identity — the empty string is a
// principal the allowlist will reject rather than a wildcard.
func convergePrincipal(ctx context.Context) string {
	if p := auth.PrincipalFromContext(ctx); p != nil {
		return p.ID
	}
	return ""
}
