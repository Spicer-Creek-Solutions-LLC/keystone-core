// SPDX-License-Identifier: Apache-2.0

// kscore-identity is the Keystone Core operator CLI for the
// embedded identity provider (Epic 09).
//
// v0.1 subcommand surface:
//
//	token {create, list, revoke, cleanup}
//	ca    {info, rotate-signing, export}
//	status
//
// All commands talk to the IdentityService gRPC hosted by
// kscore-server (default `localhost:5397`; override via
// `--server host:port`). API-key auth flows through the
// `authorization: Bearer …` header, sourced from `--api-key`
// or the `KSCORE_API_KEY` environment variable.
package main

import (
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/identity"
)

func main() {
	if err := identity.NewCommand(identity.Deps{}).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
