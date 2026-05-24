// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/logging"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// Attribute-key namespace. Mirrors the metric naming convention
// (kscore_<domain>_*) so a Grafana panel that filters on
// kscore_agents_total{agent="a1"} drops naturally onto a trace search
// of kscore.agent.id="a1".
//
// New attribute keys MUST start with one of the kscore.<domain>. prefixes
// below — OTel semantic conventions reserve a wide root-level keyspace
// (db.*, http.*, rpc.*, service.*) that we don't want to fight.
const (
	// Agent domain.
	AttrAgentID       = "kscore.agent.id"
	AttrAgentHostname = "kscore.agent.hostname"
	AttrAgentOS       = "kscore.agent.os"
	AttrAgentVersion  = "kscore.agent.version"
	AttrAgentStatus   = "kscore.agent.status"

	// Command (a.k.a. "job") domain. The epic names the helper JobAttrs
	// but the persistent type is CommandRecord; we keep the attribute
	// keys aligned to the actual data model so traces and metrics agree.
	AttrCommandID       = "kscore.command.id"
	AttrCommandAgent    = "kscore.command.agent"
	AttrCommandType     = "kscore.command.type"
	AttrCommandStatus   = "kscore.command.status"
	AttrCommandExitCode = "kscore.command.exit_code"
	AttrCommandUser     = "kscore.command.user"

	// State (declarative-management) domain.
	AttrStateDeclID = "kscore.state.decl_id"
	AttrStateModule = "kscore.state.module"
	AttrStateName   = "kscore.state.name"
	AttrStateState  = "kscore.state.state"

	// Event-bus domain.
	AttrEventID       = "kscore.event.id"
	AttrEventType     = "kscore.event.type"
	AttrEventSeverity = "kscore.event.severity"
	AttrEventSource   = "kscore.event.source"
	AttrEventSubject  = "kscore.event.subject"
	AttrEventCategory = "kscore.event.category"

	// Cross-cutting: the request-scoped correlation ID flowed through
	// context, HTTP / gRPC metadata, and NATS message headers. Lives
	// outside the per-domain prefixes because every span at every
	// layer can carry it.
	AttrCorrelationID = "kscore.correlation_id"

	// Policy / audit domain.
	AttrPolicyID              = "kscore.policy.id"
	AttrPolicyName            = "kscore.policy.name"
	AttrPolicyAllowed         = "kscore.policy.allowed"
	AttrPolicyAction          = "kscore.policy.action"
	AttrPolicyEnforcementMode = "kscore.policy.enforcement_mode"
	AttrPolicyResourceType    = "kscore.policy.resource_type"
)

// AgentAttrs builds span attributes for an agent. Returns nil for a nil
// argument so callers can pass through lookup results without a guard.
// Empty string fields are omitted so the span doesn't carry blank
// key/value pairs into the exporter.
func AgentAttrs(rec *state.AgentRecord) []attribute.KeyValue {
	if rec == nil {
		return nil
	}
	out := make([]attribute.KeyValue, 0, 5)
	out = appendString(out, AttrAgentID, rec.ID)
	out = appendString(out, AttrAgentHostname, rec.Hostname)
	out = appendString(out, AttrAgentOS, rec.OS)
	out = appendString(out, AttrAgentVersion, rec.AgentVersion)
	out = appendString(out, AttrAgentStatus, string(rec.Status))
	if len(out) == 0 {
		return nil
	}
	return out
}

// JobAttrs builds span attributes for a command execution. The epic
// names this JobAttrs; the underlying type is CommandRecord (commands
// and "jobs" are synonymous in this codebase). ExitCode is recorded
// only when the command has terminated.
func JobAttrs(rec *state.CommandRecord) []attribute.KeyValue {
	if rec == nil {
		return nil
	}
	out := make([]attribute.KeyValue, 0, 6)
	out = appendString(out, AttrCommandID, rec.ID)
	out = appendString(out, AttrCommandAgent, rec.AgentID)
	out = appendString(out, AttrCommandType, rec.Command)
	out = appendString(out, AttrCommandStatus, string(rec.Status))
	out = appendString(out, AttrCommandUser, rec.User)
	if isTerminalCommand(rec.Status) {
		out = append(out, attribute.Int(AttrCommandExitCode, rec.ExitCode))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// StateAttrs builds span attributes for a state-management declaration.
func StateAttrs(decl *statemgmt.Declaration) []attribute.KeyValue {
	if decl == nil {
		return nil
	}
	out := make([]attribute.KeyValue, 0, 4)
	out = appendString(out, AttrStateDeclID, decl.ID)
	out = appendString(out, AttrStateModule, decl.Module)
	out = appendString(out, AttrStateName, decl.Name)
	out = appendString(out, AttrStateState, decl.State)
	if len(out) == 0 {
		return nil
	}
	return out
}

// EventAttrs builds span attributes for an event-bus message. Value
// receiver: callers usually have the Event by value at the publish
// site, and a zero Event yields nil rather than an empty slice.
func EventAttrs(e events.Event) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, 6)
	out = appendString(out, AttrEventID, e.ID)
	out = appendString(out, AttrEventType, string(e.Type))
	if e.Severity != events.SeverityUnknown {
		out = appendString(out, AttrEventSeverity, e.Severity.String())
	}
	out = appendString(out, AttrEventSource, e.Source)
	out = appendString(out, AttrEventSubject, e.Subject)
	if e.Type != "" {
		out = appendString(out, AttrEventCategory, string(e.Type.Category()))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CorrelationIDAttr returns the kscore.correlation_id attribute when
// ctx carries one (via logging.CorrelationIDFromContext), or nil.
// Callers pass the result to span.SetAttributes at span start so the
// HTTP/gRPC/NATS request boundary identifier flows into every span
// the request creates.
func CorrelationIDAttr(ctx context.Context) []attribute.KeyValue {
	id := logging.CorrelationIDFromContext(ctx)
	if id == "" {
		return nil
	}
	return []attribute.KeyValue{attribute.String(AttrCorrelationID, id)}
}

// PolicyAttrs builds span attributes for one audit entry. Allowed is
// always recorded (boolean has no "zero" interpretation operators can
// rely on otherwise).
func PolicyAttrs(entry audit.AuditEntry) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, 6)
	out = appendString(out, AttrPolicyID, entry.PolicyID)
	out = appendString(out, AttrPolicyName, entry.PolicyName)
	out = append(out, attribute.Bool(AttrPolicyAllowed, entry.Allowed))
	out = appendString(out, AttrPolicyAction, entry.Action)
	out = appendString(out, AttrPolicyEnforcementMode, string(entry.EnforcementMode))
	out = appendString(out, AttrPolicyResourceType, entry.ResourceType)
	return out
}

// appendString appends a string-valued attribute only when value is
// non-empty. Keeps the call sites a single line per field.
func appendString(out []attribute.KeyValue, key, value string) []attribute.KeyValue {
	if value == "" {
		return out
	}
	return append(out, attribute.String(key, value))
}

// isTerminalCommand reports whether s is one of the terminal command
// statuses (completed / failed / timeout / cancelled). Pre-terminal
// states (pending / running) don't carry a meaningful ExitCode.
func isTerminalCommand(s state.CommandStatus) bool {
	switch s {
	case state.CommandStatusCompleted,
		state.CommandStatusFailed,
		state.CommandStatusTimeout,
		state.CommandStatusCancelled:
		return true
	default:
		return false
	}
}
