// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// BlueprintConvergeRunner applies a rendered blueprint on a fixed set
// of agents, over the same converge path `state apply --target` uses.
//
// It satisfies blueprint.StateFileRunner, which hands it the state
// FILE rather than resolved declarations. That is the point: each
// agent parses and renders the file itself, so `.Facts` describe the
// host being converged. A blueprint's own parameter rendering has
// already happened centrally, which is correct -- parameters are
// operator input, not properties of the target.
//
// The agent set is fixed at construction rather than passed per call.
// One blueprint apply is one operator intent against one resolved
// fleet; re-resolving a target between entrypoints could send the
// rollback entrypoint to a different set of hosts than the apply.
type BlueprintConvergeRunner struct {
	converge ConvergeFanout
	agents   []string

	principal string
	source    string
	runID     string
	timeout   time.Duration
}

// BlueprintConvergeConfig constructs a BlueprintConvergeRunner.
type BlueprintConvergeConfig struct {
	Converge ConvergeFanout
	// Agents is the resolved target set. Must be non-empty: a remote
	// apply with no hosts is a mistake worth surfacing, not a no-op
	// that reports success.
	Agents    []string
	Principal string
	// Source names the blueprint in run history.
	Source  string
	RunID   string
	Timeout time.Duration
}

// ErrNoAgents is returned when a remote blueprint apply resolves to no
// agents.
var ErrNoAgents = errors.New("controlplane: blueprint apply matched no agents")

// NewBlueprintConvergeRunner validates cfg.
func NewBlueprintConvergeRunner(cfg BlueprintConvergeConfig) (*BlueprintConvergeRunner, error) {
	if cfg.Converge == nil {
		return nil, errors.New("controlplane: blueprint runner: Converge is required")
	}
	if len(cfg.Agents) == 0 {
		return nil, ErrNoAgents
	}
	agents := append([]string(nil), cfg.Agents...)
	sort.Strings(agents)
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultConvergeTimeout
	}
	return &BlueprintConvergeRunner{
		converge:  cfg.Converge,
		agents:    agents,
		principal: cfg.Principal,
		source:    cfg.Source,
		runID:     cfg.RunID,
		timeout:   timeout,
	}, nil
}

// Agents returns the resolved target set, for the AppliedRun record.
func (r *BlueprintConvergeRunner) Agents() []string {
	return append([]string(nil), r.agents...)
}

// RunFile converges every targeted agent against the state file and
// merges the reports.
//
// One agent failing does not abort the others, matching remote state
// apply: a fleet-wide operation should tell an operator which hosts
// are wrong, not stop at the first. A run in which any agent failed
// still returns a non-nil report, so the caller can see the whole
// picture, alongside an error so it is not mistaken for success.
func (r *BlueprintConvergeRunner) RunFile(ctx context.Context, stateFile []byte) (*statemgmt.RunReport, error) {
	type outcome struct {
		agentID string
		resp    agent.ConvergeResponse
		err     error
	}
	results := make([]outcome, len(r.agents))

	var wg sync.WaitGroup
	for i, agentID := range r.agents {
		wg.Add(1)
		go func(i int, agentID string) {
			defer wg.Done()
			resp, err := r.converge.Converge(ctx, ConvergeTarget{
				AgentID:   agentID,
				RunID:     r.runID,
				Source:    r.source,
				Mode:      agent.ConvergeModeApply,
				YAML:      stateFile,
				Principal: r.principal,
				Timeout:   r.timeout,
			})
			results[i] = outcome{agentID: agentID, resp: resp, err: err}
		}(i, agentID)
	}
	wg.Wait()

	merged := &statemgmt.RunReport{}
	var failures []string
	for _, o := range results {
		switch {
		case o.err != nil:
			failures = append(failures, fmt.Sprintf("%s: %v", o.agentID, o.err))
			merged.Failed++
			continue
		case o.resp.Rejected:
			failures = append(failures, fmt.Sprintf("%s: rejected: %s", o.agentID, o.resp.RejectReason))
			merged.Failed++
			continue
		case o.resp.Error != "":
			failures = append(failures, fmt.Sprintf("%s: %s", o.agentID, o.resp.Error))
			merged.Failed++
			continue
		}
		merged.Changed += o.resp.Changed
		merged.Unchanged += o.resp.Unchanged
		merged.Failed += o.resp.Failed
		merged.Skipped += o.resp.Skipped
		for _, d := range o.resp.Results {
			merged.Results = append(merged.Results, blueprintDeclResult(o.agentID, d))
		}
	}

	// Sorted so two runs of the same blueprint over the same fleet
	// produce the same transcript regardless of which agent answered
	// first.
	sort.SliceStable(merged.Results, func(i, j int) bool {
		return merged.Results[i].DeclID < merged.Results[j].DeclID
	})

	if len(failures) > 0 {
		return merged, fmt.Errorf("%d of %d agents failed: %v",
			len(failures), len(r.agents), failures)
	}
	return merged, nil
}

// blueprintDeclResult converts one agent's declaration outcome,
// prefixing the id with the agent so a merged report says which host
// each line came from. Without it a fleet report is a list of
// identical-looking declarations with no way to tell them apart.
func blueprintDeclResult(agentID string, d agent.ConvergeDeclResult) statemgmt.DeclarationResult {
	out := statemgmt.DeclarationResult{
		DeclID:   agentID + "|" + d.DeclID,
		Module:   d.Module,
		Duration: time.Duration(d.DurationMs) * time.Millisecond,
	}
	out.Outcome = blueprintOutcome(d.Outcome)
	if d.ErrorMessage != "" {
		out.Error = errors.New(d.ErrorMessage)
	}
	return out
}

// blueprintOutcome maps the agent's stringly outcome back onto the
// enum. The agent sends strings because its wire format is JSON and it
// must not depend on the control plane's types; this is the inverse of
// Outcome.String(). An unrecognised value becomes OutcomeFailed rather
// than the zero value, which is "unchanged" -- silently reporting an
// unknown outcome as a success is the worst available default.
func blueprintOutcome(s string) statemgmt.Outcome {
	switch s {
	case "changed":
		return statemgmt.OutcomeChanged
	case "unchanged":
		return statemgmt.OutcomeUnchanged
	case "no-op":
		return statemgmt.OutcomeNoOp
	case "drift-detected":
		return statemgmt.OutcomeDriftDetected
	case "skipped":
		return statemgmt.OutcomeSkipped
	default:
		return statemgmt.OutcomeFailed
	}
}
