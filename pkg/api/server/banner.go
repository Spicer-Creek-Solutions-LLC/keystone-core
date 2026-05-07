package server

import (
	"fmt"
	"strings"

	"go.keystone-core.io/keystone-core/pkg/version"
)

// banner formats the human-readable startup banner emitted at the end
// of the 21-step init (step 20). The formatter is split out so task 9
// can extend it without disturbing the orchestration code.
func banner(info version.Info, addrs Addrs, authMode, storageBackend string, warnings []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "kscore-server %s (commit %s, built %s)\n",
		info.Version, info.GitCommit, info.BuildDate)
	fmt.Fprintf(&b, "  gRPC:    %s\n", addrs.GRPC)
	fmt.Fprintf(&b, "  HTTP:    %s\n", addrs.HTTP)
	fmt.Fprintf(&b, "  auth:    %s\n", authMode)
	fmt.Fprintf(&b, "  storage: %s\n", storageBackend)
	for _, w := range warnings {
		fmt.Fprintf(&b, "  WARNING: %s\n", w)
	}
	return strings.TrimRight(b.String(), "\n")
}
