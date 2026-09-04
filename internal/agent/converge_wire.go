// SPDX-License-Identifier: Apache-2.0

package agent

// ConvergeRequest is the control plane's request that this agent
// converge itself against a state file. It is the state-management
// counterpart to CommandRequest and travels the same shape of path:
// signed by the control plane, published on the agent's converge
// subject, validated by SecurityEnforcer before anything runs.
//
// The YAML is forwarded verbatim rather than pre-compiled
// declarations. Compilation has to happen HERE, because rendering
// resolves `.Facts` — and the facts that matter are this host's, not
// the control plane's. A server-side compile would render every
// declaration against the wrong machine.
//
// Signature is hex-encoded HMAC-SHA-256 over canonicalConverge(req),
// excluded from the canonical itself.
type ConvergeRequest struct {
	MessageID string `json:"message_id"`
	Principal string `json:"principal"`
	// RunID ties every agent's results back to the one state run the
	// operator started, mirroring batch exec's batch_job_id.
	RunID string `json:"run_id"`
	// Source is the logical name of the state file, for run history.
	Source string `json:"source"`
	// Mode is apply | check | drift.
	Mode string `json:"mode"`
	// YAML is the state file, verbatim.
	YAML []byte `json:"yaml"`
	// Variables override the state file's own `variables:` block.
	Variables      map[string]string `json:"variables,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Signature      string            `json:"signature"`
}

// Converge modes. Mirrors state.StateRunMode without importing it —
// the agent package stays free of the control plane's storage types.
const (
	ConvergeModeApply = "apply"
	ConvergeModeCheck = "check"
	ConvergeModeDrift = "drift"
)

// ConvergeDeclResult is one declaration's outcome on this agent.
// Field-for-field the subset of statemgmt's per-declaration result
// that the control plane needs to fill a StateDeclarationResult.
type ConvergeDeclResult struct {
	DeclID       string `json:"decl_id"`
	Module       string `json:"module"`
	Outcome      string `json:"outcome"`
	CheckDiff    string `json:"check_diff,omitempty"`
	ApplyChanged bool   `json:"apply_changed"`
	ApplyDiff    string `json:"apply_diff,omitempty"`
	ApplyComment string `json:"apply_comment,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	DurationMs   int64  `json:"duration_ms"`
}

// ConvergeResponse is the terminal payload the agent publishes on its
// converge-result subject.
//
// One response carrying every declaration's result, rather than a
// stream of per-declaration messages. That matches the command path,
// where CommandResponse also arrives whole at the end, and it keeps
// the correlation model simple: one request, one reply, keyed by
// MessageID. The cost is that a remote apply's results appear
// together rather than progressively, which is noted in the state
// docs.
//
// Rejected=true means SecurityEnforcer refused the request before any
// declaration ran; Error is set when compilation or the run itself
// failed.
type ConvergeResponse struct {
	MessageID    string               `json:"message_id"`
	AgentID      string               `json:"agent_id"`
	RunID        string               `json:"run_id"`
	Results      []ConvergeDeclResult `json:"results,omitempty"`
	Changed      int                  `json:"changed"`
	Unchanged    int                  `json:"unchanged"`
	Failed       int                  `json:"failed"`
	Skipped      int                  `json:"skipped"`
	DurationMs   int64                `json:"duration_ms"`
	Error        string               `json:"error,omitempty"`
	Rejected     bool                 `json:"rejected"`
	RejectReason string               `json:"reject_reason,omitempty"`
}
