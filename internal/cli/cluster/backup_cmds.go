// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	icluster "go.keystone-core.io/keystone-core/internal/cluster"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// snapshotExt is the conventional extension list/verify scan for.
const snapshotExt = ".kscore"

// ---- backup (remote — shared by both binaries) ----------------------------

func backupCmd(g *globals, use string) *cobra.Command {
	var output, description string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Create a cluster snapshot (leader-initiated)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBackup(cmd.Context(), cmd.OutOrStdout(), g, output, description)
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "write the snapshot to this file (default: stdout)")
	cmd.Flags().StringVar(&description, "description", "", "optional snapshot description")
	return cmd
}

func runBackup(ctx context.Context, out io.Writer, g *globals, output, description string) error {
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.CreateBackup(ctx, &v1.CreateBackupRequest{Description: description})
	if err != nil {
		return fmt.Errorf("CreateBackup: %w", err)
	}
	blob := resp.GetSnapshot()
	if len(blob) == 0 {
		return fmt.Errorf("cluster: server returned an empty snapshot")
	}
	if output == "" {
		_, werr := out.Write(blob)
		return werr
	}
	if err := os.WriteFile(output, blob, 0o600); err != nil {
		return fmt.Errorf("cluster: write %s: %w", output, err)
	}
	fmt.Fprintf(out, "backup written: %s (%d bytes)\n", output, len(blob))
	return nil
}

// ---- restore (remote — shared by both binaries) ---------------------------

func restoreCmd(g *globals, use string) *cobra.Command {
	var input string
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "Restore a cluster snapshot from a file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRestore(cmd.Context(), cmd.OutOrStdout(), g, input, force, dryRun)
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "snapshot file to restore (required)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing shard assignments")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate only; do not apply")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func runRestore(ctx context.Context, out io.Writer, g *globals, input string, force, dryRun bool) error {
	if input == "" {
		return fmt.Errorf("cluster: --input is required")
	}
	blob, err := os.ReadFile(input) //nolint:gosec // operator-supplied path
	if err != nil {
		return fmt.Errorf("cluster: read %s: %w", input, err)
	}
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.RestoreBackup(ctx, &v1.RestoreBackupRequest{
		Snapshot: blob, Force: force, DryRun: dryRun,
	})
	if err != nil {
		return fmt.Errorf("RestoreBackup: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	fmt.Fprintf(out, "success: %t\n", resp.GetSuccess())
	if d := resp.GetDetail(); d != "" {
		fmt.Fprintf(out, "detail: %s\n", d)
	}
	return nil
}

// ---- list (local — kscore-cluster-backup only) ----------------------------

func listCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "list [dir]",
		Short: "List snapshot files in a directory (local; default: .)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return runList(cmd.OutOrStdout(), g, dir)
		},
	}
}

type snapshotInfo struct {
	File    string `json:"file"`
	Cluster string `json:"cluster"`
	TakenAt string `json:"taken_at"`
	Leader  string `json:"leader_id"`
	Members int    `json:"members"`
	Shards  int    `json:"shards"`
	Valid   bool   `json:"valid"`
	Error   string `json:"error,omitempty"`
}

func runList(out io.Writer, g *globals, dir string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cluster: read dir %s: %w", dir, err)
	}
	infos := make([]snapshotInfo, 0)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != snapshotExt {
			continue
		}
		path := filepath.Join(dir, e.Name())
		infos = append(infos, inspectSnapshot(path, e.Name()))
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].File < infos[j].File })

	if g.Output == FormatJSON {
		return writeJSONAny(out, infos)
	}
	t := newTable(out)
	t.header("FILE", "CLUSTER", "TAKEN", "LEADER", "MEMBERS", "SHARDS", "VALID")
	for _, in := range infos {
		t.row(in.File, orDash(in.Cluster), orDash(in.TakenAt), orDash(in.Leader),
			fmt.Sprintf("%d", in.Members), fmt.Sprintf("%d", in.Shards),
			fmt.Sprintf("%t", in.Valid))
	}
	return t.flush()
}

func inspectSnapshot(path, name string) snapshotInfo {
	in := snapshotInfo{File: name}
	blob, err := os.ReadFile(path) //nolint:gosec // operator-supplied dir
	if err != nil {
		in.Error = err.Error()
		return in
	}
	snap, err := icluster.UnmarshalSnapshot(blob)
	if err != nil {
		in.Error = err.Error()
		return in
	}
	in.Valid = true
	in.Cluster = snap.Meta.ClusterName
	in.Leader = snap.Meta.LeaderID
	in.Members = len(snap.Members)
	in.Shards = len(snap.Shards)
	if !snap.Meta.TakenAt.IsZero() {
		in.TakenAt = snap.Meta.TakenAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return in
}

// ---- verify (local — kscore-cluster-backup only) --------------------------

func verifyCmd(g *globals) *cobra.Command {
	var input string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Validate a snapshot file (local; non-zero exit if invalid)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVerify(cmd.OutOrStdout(), g, input)
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "snapshot file to verify (required)")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func runVerify(out io.Writer, g *globals, input string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	if input == "" {
		return fmt.Errorf("cluster: --input is required")
	}
	blob, err := os.ReadFile(input) //nolint:gosec // operator-supplied path
	if err != nil {
		return fmt.Errorf("cluster: read %s: %w", input, err)
	}
	snap, err := icluster.UnmarshalSnapshot(blob)
	if err != nil {
		return fmt.Errorf("cluster: invalid snapshot %s: %w", input, err)
	}
	if g.Output == FormatJSON {
		return writeJSONAny(out, snapshotInfo{
			File: filepath.Base(input), Cluster: snap.Meta.ClusterName,
			Leader: snap.Meta.LeaderID, Members: len(snap.Members),
			Shards: len(snap.Shards), Valid: true,
		})
	}
	fmt.Fprintf(out, "valid: %s\n", input)
	fmt.Fprintf(out, "cluster: %s  leader: %s  members: %d  shards: %d\n",
		orDash(snap.Meta.ClusterName), orDash(snap.Meta.LeaderID),
		len(snap.Members), len(snap.Shards))
	return nil
}
