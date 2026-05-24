// SPDX-License-Identifier: Apache-2.0

// kscore-secrets is the Keystone Core operator CLI for the
// secrets domain (Epic 10).
//
// v1.0 subcommand surface:
//
//	get / put / delete / list
//	leases  {list, get, renew, revoke}
//	transit {encrypt, decrypt, sign, verify}
//
// All commands talk to the SecretsService gRPC hosted by
// kscore-server (default `localhost:5397`; override via
// `--server host:port`). API-key auth flows through the
// `authorization: Bearer …` header, sourced from `--api-key`
// or the `KSCORE_API_KEY` environment variable.
//
// Subcommand groups deferred to v1.x ROADMAP:
//
//	backends  — needs new ListBackends RPC
//	audit     — Epic 12 territory
//	dynamic   — needs IssueDynamicSecret RPC
//	cache     — needs Stats/Clear RPC
//	template  — substantial feature (consul-template-style)
package main

import (
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/secrets"
)

func main() {
	if err := secrets.NewCommand(secrets.Deps{}).Execute(); err != nil {
		os.Exit(1)
	}
}
