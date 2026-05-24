// kscore-policy is the Keystone Core operator + authoring CLI for
// the policy domain (Epic 12 task 14).
//
// v1.0 subcommand surface:
//
//	list / show / compliance / violations   (remote — PolicyService gRPC)
//	eval / validate                          (local — in-process evaluators)
//
// list/show/compliance/violations talk to the PolicyService gRPC on
// the running kscore-server (default `localhost:5397`; override via
// `--server host:port`). API-key auth flows through the
// `authorization: Bearer …` header from `--api-key` or
// `KSCORE_API_KEY`. eval/validate read a policy source file and run
// the evaluator in-process — no server, CI-friendly.
//
// Subcommands deferred to v1.x ROADMAP:
//
//	check — fold of validate + dry-run; scope undefined for v1.0.
//	test  — policy unit-test harness; spec fuzzy, no acceptance line.
package main

import (
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/policy"
)

func main() {
	if err := policy.NewCommand(policy.Deps{}).Execute(); err != nil {
		os.Exit(1)
	}
}
