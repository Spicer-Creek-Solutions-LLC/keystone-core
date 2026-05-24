// SPDX-License-Identifier: Apache-2.0

// Package policy implements the v1.0 policy engine (PROJECT-DETAILS
// §4.12). It is the evaluation half of the audit-and-policy pair;
// the recording half is internal/audit.
//
// Layering (Epic 12 build order):
//
//	task 5  — this skeleton: Policy / PolicySet / Binding data model,
//	          in-memory Registry, Evaluator seam, Engine coordinator.
//	          Engine.Evaluate* return ErrNoEvaluator.
//	task 6  — OPAEvaluator     (wraps open-policy-agent/opa/rego)
//	task 7  — CELEvaluator     (wraps google/cel-go)
//	task 8  — BuiltinEvaluator (13 hardcoded rules)
//	task 9  — Engine.Evaluate* dispatch + aggregation + tests
//	task 10 — Enforcer (v1.0 always allows; post-v1.0 honors EnforcementMode)
//
// Dependency direction: policy → audit (shared Severity /
// EnforcementMode / PolicyType enums + the Violation type). audit
// never imports policy. A policy evaluation yields audit.Violation
// values that flow into the audit log without translation.
package policy
