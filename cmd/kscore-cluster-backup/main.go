// kscore-cluster-backup is the Keystone Core cluster disaster-
// recovery CLI (Epic 13 task 16).
//
//	backup / restore   talk to the ClusterService gRPC on a running
//	                   kscore-server (default `localhost:5397`).
//	list / verify      inspect snapshot files locally — no server
//	                   required (CI-friendly, the kscore-policy
//	                   eval/validate precedent).
//
// `schedule` (automated periodic backups) is deferred to a future
// release per the epic scope; it is intentionally not registered.
package main

import (
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/cluster"
)

func main() {
	if err := cluster.NewBackupCommand(cluster.Deps{}).Execute(); err != nil {
		os.Exit(1)
	}
}
