// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// StateEngine is the agent-side half of remote state apply: it turns a
// state file into convergence ON THIS HOST.
//
// The agent holds a complete engine — parse, render, validate, resolve,
// run — against the full stdlib registry, not a thin executor taking
// orders. That is deliberate: a state file carrying everything it needs
// is runnable on the agent without the control plane resolving anything
// for it, which keeps the agent useful when the control plane is
// unreachable and keeps `.Facts` resolving against the host they
// describe.
type StateEngine struct {
	// Registry supplies the modules. Nil falls back to
	// statemgmt.DefaultRegistry, which is what cmd/kscore-agent
	// populates via stdlib.RegisterAll.
	Registry *statemgmt.Registry
	// DeclTimeout bounds a single declaration. Zero means no per-decl
	// timeout; the request's TimeoutSeconds still bounds the run.
	DeclTimeout time.Duration
	// Secrets resolves `{{ secret "path" "key" }}` during rendering.
	// Nil means a state file referencing a secret fails with a reason
	// rather than rendering a blank password and reporting success.
	Secrets SecretResolver
}

// SecretResolver fetches one secret value. SecretClient implements it;
// so does the lazy wrapper the agent binary uses, which cannot build a
// client until bootstrap has delivered a credential.
type SecretResolver interface {
	Lookup(ctx context.Context, path, key string) (string, error)
}

// renderer builds the render pass for one converge.
//
// Per-converge rather than reused, because the secret resolver closes
// over that run's context: a lookup is a round trip to the control
// plane, and it has to be cancelled by the same deadline that bounds
// the rest of the run.
func (e *StateEngine) renderer(ctx context.Context) *statemgmt.Renderer {
	if e.Secrets == nil {
		return statemgmt.NewRenderer()
	}
	return statemgmt.NewRendererWithSecrets(func(path, key string) (string, error) {
		return e.Secrets.Lookup(ctx, path, key)
	})
}

func (e *StateEngine) registry() *statemgmt.Registry {
	if e.Registry != nil {
		return e.Registry
	}
	return statemgmt.DefaultRegistry
}

// Converge compiles the state file against facts and runs it in the
// requested mode, returning the aggregate report.
//
// Compilation happens here rather than on the control plane precisely
// so that facts are this host's. Includes are rejected for the same
// reason the server rejects them: the agent has no state library
// directory to resolve them against, and silently ignoring an include
// would converge less than the operator asked for.
func (e *StateEngine) Converge(
	ctx context.Context, mode string, yaml []byte, vars map[string]string, facts map[string]any,
) (*statemgmt.RunReport, error) {
	if len(yaml) == 0 {
		return nil, fmt.Errorf("converge: empty state file")
	}
	sf, err := statemgmt.Parse(yaml)
	if err != nil {
		return nil, fmt.Errorf("converge: parse: %w", err)
	}
	if len(sf.Includes) > 0 {
		return nil, fmt.Errorf("converge: includes are not supported over the wire; " +
			"submit a fully-resolved state file")
	}
	if len(vars) > 0 {
		if sf.Variables == nil {
			sf.Variables = map[string]any{}
		}
		for k, v := range vars {
			sf.Variables[k] = v
		}
	}
	rendered, err := e.renderer(ctx).RenderStateFile(sf, facts)
	if err != nil {
		return nil, fmt.Errorf("converge: render: %w", err)
	}
	if err := statemgmt.NewValidator(e.registry()).Validate(rendered); err != nil {
		return nil, fmt.Errorf("converge: validate: %w", err)
	}
	ordered, err := statemgmt.NewResolver().Resolve(rendered)
	if err != nil {
		return nil, fmt.Errorf("converge: resolve: %w", err)
	}

	runner := &statemgmt.Runner{Registry: e.registry(), DeclTimeout: e.DeclTimeout}
	switch mode {
	case ConvergeModeApply:
		return runner.Run(ctx, ordered)
	case ConvergeModeCheck, ConvergeModeDrift:
		// Check and drift are the same execution on the agent — Check
		// with no Apply. They differ only in how the control plane
		// records and renders the outcome.
		return runner.Check(ctx, ordered)
	default:
		return nil, fmt.Errorf("converge: unknown mode %q (want apply, check or drift)", mode)
	}
}

// handleConverge is the state-run counterpart to handleCommand:
// parse → SecurityEnforcer.ValidateConverge → compile+run → publish a
// ConvergeResponse on the converge-result subject, correlated by the
// inbound MessageID.
//
// Rejections publish Rejected=true rather than going silent, so the
// control plane reports a reason instead of waiting out a timeout.
// Shutdown semantics match handleCommand: register in the WaitGroup
// first, then run against a.commandCtx so Shutdown drains in-flight
// runs rather than killing them mid-declaration — a state run
// interrupted between Check and Apply is exactly the state you do not
// want a host left in.
func (a *Agent) handleConverge(_ context.Context, subject string, env envelope.Envelope) error {
	runCtx, ok := a.acquireCommandSlot(subject, env.MessageID)
	if !ok {
		return nil
	}
	defer a.wg.Done()

	var req ConvergeRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		a.log.Warn("agent: converge unmarshal",
			"subject", subject, "message_id", env.MessageID, "err", err)
		return nil
	}

	if a.enforcer != nil {
		if err := a.enforcer.ValidateConverge(runCtx, req); err != nil {
			a.publishConvergeResult(runCtx, env.MessageID, &ConvergeResponse{
				MessageID:    req.MessageID,
				AgentID:      a.cfg.AgentID,
				RunID:        req.RunID,
				Rejected:     true,
				RejectReason: err.Error(),
			})
			return nil
		}
	}

	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(runCtx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	started := time.Now()
	report, err := a.engine().Converge(runCtx, req.Mode, req.YAML, req.Variables, a.facts(runCtx))
	resp := &ConvergeResponse{
		MessageID:  req.MessageID,
		AgentID:    a.cfg.AgentID,
		RunID:      req.RunID,
		DurationMs: time.Since(started).Milliseconds(),
	}
	if err != nil {
		// Compilation and run failures both land here. The control
		// plane surfaces this against THIS agent, so one host's bad
		// facts do not fail the whole fleet's run.
		resp.Error = err.Error()
	}
	if report != nil {
		resp.Results = convergeResults(report)
		resp.Changed = report.Changed
		resp.Unchanged = report.Unchanged
		resp.Failed = report.Failed
		resp.Skipped = report.Skipped
	}
	a.publishConvergeResult(runCtx, env.MessageID, resp)
	return nil
}

// convergeResults flattens a RunReport into the wire shape.
func convergeResults(report *statemgmt.RunReport) []ConvergeDeclResult {
	out := make([]ConvergeDeclResult, 0, len(report.Results))
	for i := range report.Results {
		r := &report.Results[i]
		e := ConvergeDeclResult{
			DeclID:     r.DeclID,
			Module:     r.Module,
			Outcome:    r.Outcome.String(),
			DurationMs: r.Duration.Milliseconds(),
		}
		if r.Check != nil {
			e.CheckDiff = r.Check.Diff
		}
		if r.Apply != nil {
			e.ApplyChanged = r.Apply.Changed
			e.ApplyDiff = r.Apply.Diff
			e.ApplyComment = r.Apply.Comment
		}
		if r.Error != nil {
			e.ErrorMessage = r.Error.Error()
		}
		out = append(out, e)
	}
	return out
}

func (a *Agent) engine() *StateEngine {
	if a.stateEngine != nil {
		return a.stateEngine
	}
	return &StateEngine{Secrets: a.cfg.Secrets}
}

// facts builds the render context for a state run. These are the
// host's own properties, which is the whole reason compilation happens
// on the agent: `{{ .Facts.os }}` has to mean the agent's OS.
func (a *Agent) facts(ctx context.Context) map[string]any {
	facts := map[string]any{
		"agent_id": a.cfg.AgentID,
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
	}
	if a.metrics != nil {
		md := a.metrics.Metadata(ctx, a.cfg.AgentID, a.cfg.Labels)
		setFact(facts, "hostname", md.Hostname)
		setFact(facts, "os", md.OS)
		setFact(facts, "platform", md.Platform)
		setFact(facts, "platform_version", md.PlatformVersion)
		setFact(facts, "kernel_version", md.KernelVersion)
		setFact(facts, "arch", md.Architecture)
		setFact(facts, "virt_system", md.VirtSystem)
		setFact(facts, "virt_role", md.VirtRole)
	}
	// Labels are operator-assigned and the most likely thing a state
	// file branches on, so they are addressable as facts too.
	for k, v := range a.cfg.Labels {
		if k = strings.TrimSpace(k); k != "" {
			facts["label_"+k] = v
		}
	}
	return facts
}

// setFact skips empties so a missing metadata field does not shadow
// the runtime fallback with "".
func setFact(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}

func (a *Agent) publishConvergeResult(ctx context.Context, correlationID string, resp *ConvergeResponse) {
	payload, err := json.Marshal(resp)
	if err != nil {
		a.log.Warn("agent: converge result marshal",
			"agent_id", a.cfg.AgentID, "message_id", correlationID, "err", err)
		return
	}
	respEnv := envelope.New(payload, a.subjects.Prefix(),
		envelope.WithCorrelationID(correlationID),
	)
	subject := a.subjects.AgentConvergeResult(a.cfg.AgentID)
	if err := a.nats.PublishEnvelope(ctx, subject, respEnv); err != nil {
		a.log.Warn("agent: converge result publish",
			"agent_id", a.cfg.AgentID, "message_id", correlationID, "err", err)
	}
}
