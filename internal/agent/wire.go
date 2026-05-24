// SPDX-License-Identifier: Apache-2.0

package agent

// CommandResponse is the JSON payload the agent publishes inside an
// Envelope on kscore.{cluster}.agent.{id}.response after processing
// a command. The control plane's CommandDispatcher correlates by
// the inbound CommandRequest.MessageID (also stamped on the
// response Envelope's CorrelationID).
//
// Mirrors Executor.ExecuteResult plus identity fields and the
// rejection-path metadata. Duration is surfaced as int64 ms so
// dashboards can plot without parsing time.Duration's stringly
// form.
//
// Rejected=true means SecurityEnforcer.Validate refused the command
// (HMAC, principal allowlist, command rules, MaxArgsBytes); the
// reason is in RejectReason. Rejected=false means the executor
// ran; ExitCode + Stdout + Stderr + TimedOut + Error reflect the
// outcome.
type CommandResponse struct {
	MessageID       string `json:"message_id"`
	AgentID         string `json:"agent_id"`
	ExitCode        int    `json:"exit_code"`
	Stdout          []byte `json:"stdout,omitempty"`
	Stderr          []byte `json:"stderr,omitempty"`
	DurationMs      int64  `json:"duration_ms"`
	TimedOut        bool   `json:"timed_out"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	Error           string `json:"error,omitempty"`
	Rejected        bool   `json:"rejected"`
	RejectReason    string `json:"reject_reason,omitempty"`
}
