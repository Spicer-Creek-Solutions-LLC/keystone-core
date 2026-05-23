// Package cluster implements the `kscore-cluster` operator CLI and
// the `kscore-cluster-backup` DR CLI (Epic 13 task 16).
//
// Both binaries are thin wrappers over this package (the
// kscore-policy precedent). kscore-cluster talks to the
// ClusterService gRPC on a running kscore-server (default
// `localhost:9090`; override via `--server host:port`; API-key auth
// via `--api-key` or `KSCORE_API_KEY`). kscore-cluster-backup shares
// the remote backup/restore plumbing but adds local, server-free
// `list` / `verify` over snapshot files (the policy eval/validate
// precedent — CI-friendly).
//
// Scoping (matches §4.15 + the epic acceptance surface):
//
//   - `add` is an honest passthrough: members self-register on start,
//     so AddMember returns Unimplemented by contract — the CLI
//     surfaces that message rather than hiding the documented verb.
//   - `kscore-cluster-backup schedule` is deferred to a v1.x ROADMAP
//     item (epic line 47/60 tags scheduling v1.x); not implemented.
//   - `watch` (WatchMembership/WatchLeadership streams) is not in the
//     FEATURES/acceptance CLI surface — deferred as a v0.x ROADMAP
//     carry alongside the other CLI stream gaps.
//
// Live use is gated by the existing gate-v1.0 "Cluster gRPC services
// boot registration" ROADMAP item (ClusterService is not yet
// registered at kscore-server boot); the CLI itself is fully
// bufconn-tested.
package cluster

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// Output format names. Non-streaming commands default to table; json
// is the scripting-friendly alternative (kscore-policy convention).
const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Deps wires the gRPC client factory. Production uses [dialGRPC];
// tests inject a stub returning an in-process bufconn client.
type Deps struct {
	Dial func(ctx context.Context, target, apiKey string) (v1.ClusterServiceClient, io.Closer, error)
}

// globals are shared by every subcommand; populated from the root
// command's persistent flags.
type globals struct {
	Server string
	APIKey string
	Output string
	Deps   Deps
}

func newGlobals(deps Deps) *globals {
	if deps.Dial == nil {
		deps.Dial = dialGRPC
	}
	return &globals{Deps: deps}
}

func (g *globals) bindPersistent(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&g.Server, "server", "localhost:9090",
		"cluster-service gRPC address (host:port)")
	cmd.PersistentFlags().StringVar(&g.APIKey, "api-key", "",
		"API key for authentication (or set KSCORE_API_KEY)")
	cmd.PersistentFlags().StringVarP(&g.Output, "output", "o", FormatTable,
		"output format: table | json")
}

// NewClusterCommand returns the root cobra.Command for the
// kscore-cluster binary. Pass a zero Deps for the production dialer.
func NewClusterCommand(deps Deps) *cobra.Command {
	g := newGlobals(deps)
	cmd := &cobra.Command{
		Use:   "kscore-cluster",
		Short: "Keystone Core cluster operator CLI",
		Long: "Operator surface for the Keystone Core clustering control " +
			"plane.\n\nTalks to the ClusterService gRPC on a running " +
			"kscore-server (default localhost:9090). `add` is a contract " +
			"passthrough — members self-register on start, so the server " +
			"returns Unimplemented.",
	}
	g.bindPersistent(cmd)
	cmd.AddCommand(
		statusCmd(g),
		membersCmd(g),
		leaderCmd(g),
		addCmd(g),
		removeCmd(g),
		transferLeaderCmd(g),
		rebalanceCmd(g),
		backupCmd(g, "backup"),
		restoreCmd(g, "restore"),
	)
	cli.AddVersion(cmd)
	return cmd
}

// NewBackupCommand returns the root cobra.Command for the
// kscore-cluster-backup binary — the DR-focused tool. backup/restore
// reuse the shared remote plumbing; list/verify are local + server-
// free.
func NewBackupCommand(deps Deps) *cobra.Command {
	g := newGlobals(deps)
	cmd := &cobra.Command{
		Use:   "kscore-cluster-backup",
		Short: "Keystone Core cluster backup / restore CLI",
		Long: "Disaster-recovery tool for cluster snapshots.\n\nbackup / " +
			"restore talk to the ClusterService gRPC on a running " +
			"kscore-server. list / verify inspect snapshot files locally " +
			"— no server required. schedule is deferred to a future " +
			"release.",
	}
	g.bindPersistent(cmd)
	cmd.AddCommand(
		backupCmd(g, "backup"),
		restoreCmd(g, "restore"),
		listCmd(g),
		verifyCmd(g),
	)
	cli.AddVersion(cmd)
	return cmd
}
