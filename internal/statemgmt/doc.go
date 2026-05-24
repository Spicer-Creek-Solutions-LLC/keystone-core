// SPDX-License-Identifier: Apache-2.0

// Package statemgmt is the state management engine described in
// PROJECT-DETAILS §4.8 — the declarative configuration surface that
// applies YAML state files to agents, detects drift, and remediates.
//
// This package is being built incrementally per Epic 08. Task 1
// delivers the foundation: the Module interface every stdlib state
// type implements, the result types it returns, and a DefaultRegistry
// so modules can register themselves at init time.
//
// Later tasks layer on top:
//
//	2. State file YAML loader (StateFile / includes / variables)
//	3. text/template renderer with custom filters
//	4. Validator (module exists, params valid, requisite refs valid)
//	5. Dependency resolver + cycle detection + topological sort
//	6. State runner (Check → Apply → Test, event emission, audit)
//	7. Drift detection + DriftReport
//	8. History store (extends internal/state.Store)
//	9. gRPC StateService + REST handlers
//	10. kscorectl state CLI
//	11. ~40 base stdlib modules
//	12. Saga coordinator integration (minimal)
//	13. End-to-end integration test
//
// Modules see one fully-resolved Declaration; requisites, templating,
// and ordering are runner-level concerns and never reach a Module
// implementation.
package statemgmt
