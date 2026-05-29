// SPDX-License-Identifier: Apache-2.0

// kscore-cluster is the Keystone Core cluster operator CLI
// (Epic 13 task 16).
//
// Subcommands talk to the ClusterService gRPC on a running
// kscore-server (default `localhost:5397`; override via
// `--server host:port`). API-key auth flows through the
// `authorization: Bearer …` header from `--api-key` or
// `KSCORE_API_KEY`.
//
//	status / members / leader                  (read)
//	add / remove / transfer-leader / rebalance (mutate)
//	backup / restore                           (snapshot)
//
// `add` is a contract passthrough — members self-register on start,
// so the server returns Unimplemented. Backup scheduling +
// membership/leadership watch streams are deferred (see the package
// doc / ROADMAP).
package main

import (
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/cluster"
)

func main() {
	if err := cluster.NewClusterCommand(cluster.Deps{}).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
