// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strconv"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
)

// commandAuditResourceType is the [audit.AuditEntry.ResourceType]
// every command-exec emission carries. Consumers filter on
// `resource_type == "command"` to scope to the exec channel.
const commandAuditResourceType = "command"

// newCommandTerminalEmitter builds a
// [controlplane.TerminalCommandFunc] that emits an
// [audit.AuditEntry] through auditor for every command reaching a
// terminal state. §4.12 "every sensitive op MUST emit"; command
// exec is one of the five sensitive ops.
//
// Mapping:
//
//   - Action       ← "command.exec"
//   - ResourceType ← "command"
//   - User         ← principal (dispatch-time issuer); falls back to record.User (target system user) when empty
//   - Allowed      ← Status == Completed AND ExitCode == 0
//   - Severity     ← Low on success; High on failure / timeout / cancel
//   - Duration     ← record.CompletedAt - record.StartedAt
//   - Metadata     ← {agent_id, command, exit_code, status}
//   - Violations   ← stderr summary on failure
func newCommandTerminalEmitter(auditor audit.Auditor) controlplane.TerminalCommandFunc {
	return func(ctx context.Context, principal string, rec *state.CommandRecord, result state.CommandResult) {
		if auditor == nil || rec == nil {
			return
		}
		allowed := result.Status == state.CommandStatusCompleted && result.ExitCode == 0
		severity := audit.SeverityLow
		var violations []audit.Violation
		if !allowed {
			severity = audit.SeverityHigh
			msg := "command failed"
			if result.Stderr != "" {
				msg = result.Stderr
			}
			violations = []audit.Violation{{
				Rule:     "command.exec",
				Message:  msg,
				Severity: severity,
			}}
		}
		user := principal
		if user == "" {
			user = rec.User
		}
		metadata := map[string]string{
			"command_id": rec.ID,
			"agent_id":   rec.AgentID,
			"command":    rec.Command,
			"status":     string(result.Status),
			"exit_code":  strconv.Itoa(result.ExitCode),
		}
		var duration int64
		if !rec.StartedAt.IsZero() && !result.CompletedAt.IsZero() {
			duration = result.CompletedAt.Sub(rec.StartedAt).Nanoseconds()
		}
		entry, err := audit.NewAuditEntry(audit.AuditEntryInput{
			Action:       "command.exec",
			ResourceType: commandAuditResourceType,
			Allowed:      allowed,
			Severity:     severity,
			Violations:   violations,
			User:         user,
			Metadata:     metadata,
		})
		if err != nil {
			return
		}
		if duration > 0 {
			entry.Duration = result.CompletedAt.Sub(rec.StartedAt)
		}
		if !rec.StartedAt.IsZero() {
			entry.Timestamp = rec.StartedAt.UTC()
		}
		auditor.Emit(ctx, entry)
	}
}
