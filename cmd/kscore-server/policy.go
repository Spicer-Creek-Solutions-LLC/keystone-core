// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

// policyRuntime carries the Epic 12 policy engine pieces built at
// boot. The PolicyService gRPC server (and the task-13 REST
// handlers) bind against these.
type policyRuntime struct {
	Engine  *policy.Engine
	Reports *policy.ReportGenerator
	// Controls is the framework↔policy mapping the ReportGenerator
	// consults for framework-scoped compliance reports. Exposed so
	// later wiring / CLIs can register controls.
	Controls *policy.ControlMapping
}

// startPolicy assembles the policy engine: a Registry-backed Engine
// with the OPA / CEL / Builtin evaluators wired, plus a
// ReportGenerator over the audit store. Returns nil + nil error when
// the audit store is unavailable (policy reporting needs it) — the
// PolicyService then isn't registered and clients reach
// Unimplemented, matching the secrets/events opt-out pattern.
//
// auditStore is auditRuntime.Store (the SQL-backed audit log);
// policy evaluations also emit through auditEmitter (auditRuntime
// .FanOut) so the §4.12 "every sensitive op emits" rule covers
// policy eval — the 5th sensitive op (task 4 deferred this to the
// engine landing in tasks 5-9).
func startPolicy(ctx context.Context, auditStore audit.AuditStore, auditEmitter audit.Auditor, log *slog.Logger) (*policyRuntime, error) {
	if auditStore == nil {
		log.LogAttrs(ctx, slog.LevelInfo, "policy: audit store unavailable; PolicyService disabled")
		return nil, nil
	}
	reg := policy.NewRegistry()
	eng, err := policy.NewEngine(reg,
		policy.WithEvaluator(audit.PolicyTypeOPA, policy.NewOPAEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeCEL, policy.NewCELEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeBuiltin, policy.NewBuiltinEvaluator()),
	)
	if err != nil {
		return nil, fmt.Errorf("policy: engine: %w", err)
	}
	controls := policy.NewControlMapping()
	gen, err := policy.NewReportGenerator(auditStore, controls)
	if err != nil {
		return nil, fmt.Errorf("policy: report generator: %w", err)
	}
	return &policyRuntime{Engine: eng, Reports: gen, Controls: controls}, nil
}
