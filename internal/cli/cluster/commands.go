package cluster

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ---- status ---------------------------------------------------------------

func statusCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cluster status: members, leader, health, quorum",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), cmd.OutOrStdout(), g)
		},
	}
}

func runStatus(ctx context.Context, out io.Writer, g *globals) error {
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	st, err := client.GetClusterStatus(ctx, &v1.GetClusterStatusRequest{})
	if err != nil {
		return fmt.Errorf("GetClusterStatus: %w", err)
	}
	lm, err := client.ListMembers(ctx, &v1.ListMembersRequest{})
	if err != nil {
		return fmt.Errorf("ListMembers: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, st)
	}
	fmt.Fprintf(out, "cluster:   %s\n", st.GetClusterId())
	fmt.Fprintf(out, "leader:    %s\n", orDash(st.GetLeaderId()))
	fmt.Fprintf(out, "members:   %d (healthy %d)\n", st.GetMemberCount(), st.GetHealthyCount())
	fmt.Fprintf(out, "quorum:    %t\n", st.GetQuorum())
	fmt.Fprintf(out, "election:  %s\n", fmtTS(st.GetLastElectionAt()))
	if len(lm.GetMembers()) > 0 {
		fmt.Fprintln(out)
		return memberTable(out, lm.GetMembers())
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---- members --------------------------------------------------------------

func membersCmd(g *globals) *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "members [id]",
		Short: "List cluster members, or show one by ID",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			return runMembers(cmd.Context(), cmd.OutOrStdout(), g, id, status)
		},
	}
	cmd.Flags().StringVar(&status, "status", "",
		"filter by status: healthy | degraded | unreachable | left")
	return cmd
}

func parseStatusFilter(s string) (v1.ClusterMemberStatus, error) {
	switch s {
	case "":
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNSPECIFIED, nil
	case "healthy":
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY, nil
	case "degraded":
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_DEGRADED, nil
	case "unreachable":
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNREACHABLE, nil
	case "left":
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_LEFT, nil
	default:
		return 0, fmt.Errorf("--status: unknown value %q", s)
	}
}

func runMembers(ctx context.Context, out io.Writer, g *globals, id, status string) error {
	stFilter, err := parseStatusFilter(status)
	if err != nil {
		return err
	}
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	if id != "" {
		resp, gerr := client.GetMember(ctx, &v1.GetMemberRequest{MemberId: id})
		if gerr != nil {
			return fmt.Errorf("GetMember: %w", gerr)
		}
		if g.Output == FormatJSON {
			return writeJSON(out, resp)
		}
		return memberTable(out, []*v1.ClusterMember{resp.GetMember()})
	}

	resp, err := client.ListMembers(ctx, &v1.ListMembersRequest{Status: stFilter})
	if err != nil {
		return fmt.Errorf("ListMembers: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	if err := memberTable(out, resp.GetMembers()); err != nil {
		return err
	}
	fmt.Fprintf(out, "total: %d\n", resp.GetTotalCount())
	return nil
}

// ---- leader ---------------------------------------------------------------

func leaderCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "leader",
		Short: "Show the current cluster leader",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLeader(cmd.Context(), cmd.OutOrStdout(), g)
		},
	}
}

func runLeader(ctx context.Context, out io.Writer, g *globals) error {
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetLeader(ctx, &v1.GetLeaderRequest{})
	if err != nil {
		return fmt.Errorf("GetLeader: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	if resp.GetLeader() == nil {
		fmt.Fprintln(out, "leader: -  (no leader elected)")
		return nil
	}
	fmt.Fprintf(out, "leader: %s  term: %d\n", resp.GetLeader().GetId(), resp.GetTerm())
	return memberTable(out, []*v1.ClusterMember{resp.GetLeader()})
}

// ---- add (contract passthrough) -------------------------------------------

func addCmd(g *globals) *cobra.Command {
	var name, address string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a member (passthrough — members self-register; server returns Unimplemented)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAdd(cmd.Context(), cmd.OutOrStdout(), g, name, address)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "member name")
	cmd.Flags().StringVar(&address, "address", "", "member address (host:port)")
	return cmd
}

func runAdd(ctx context.Context, out io.Writer, g *globals, name, address string) error {
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.AddMember(ctx, &v1.AddMemberRequest{Name: name, Address: address})
	if err != nil {
		// Expected on a real server: members self-register, so
		// AddMember is Unimplemented by contract. Surface it
		// verbatim rather than masking the documented verb.
		return fmt.Errorf("AddMember: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	return memberTable(out, []*v1.ClusterMember{resp.GetMember()})
}

// ---- remove ---------------------------------------------------------------

func removeCmd(g *globals) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove (evict) a member by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd.Context(), cmd.OutOrStdout(), g, args[0], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "force eviction even if the member appears healthy")
	return cmd
}

func runRemove(ctx context.Context, out io.Writer, g *globals, id string, force bool) error {
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	if _, err := client.RemoveMember(ctx, &v1.RemoveMemberRequest{MemberId: id, Force: force}); err != nil {
		return fmt.Errorf("RemoveMember: %w", err)
	}
	fmt.Fprintf(out, "removed: %s\n", id)
	return nil
}

// ---- transfer-leader ------------------------------------------------------

func transferLeaderCmd(g *globals) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "transfer-leader",
		Short: "Transfer leadership (optionally to a target member)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTransferLeader(cmd.Context(), cmd.OutOrStdout(), g, target)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "target member ID (optional)")
	return cmd
}

func runTransferLeader(ctx context.Context, out io.Writer, g *globals, target string) error {
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.TransferLeader(ctx, &v1.TransferLeaderRequest{TargetMemberId: target})
	if err != nil {
		return fmt.Errorf("TransferLeader: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	if nl := resp.GetNewLeader(); nl != nil {
		fmt.Fprintf(out, "transferred: new leader %s\n", nl.GetId())
		return nil
	}
	fmt.Fprintln(out, "transfer initiated")
	return nil
}

// ---- rebalance ------------------------------------------------------------

func rebalanceCmd(g *globals) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "rebalance",
		Short: "Trigger a shard rebalance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRebalance(cmd.Context(), cmd.OutOrStdout(), g, dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview only (server returns Unimplemented in v1.0)")
	return cmd
}

func runRebalance(ctx context.Context, out io.Writer, g *globals, dryRun bool) error {
	client, closer, ctx, err := g.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.Rebalance(ctx, &v1.RebalanceRequest{DryRun: dryRun})
	if err != nil {
		return fmt.Errorf("rebalance rpc: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	fmt.Fprintf(out, "reassigned agents: %d\n", resp.GetReassignedAgents())
	if d := resp.GetDetail(); d != "" {
		fmt.Fprintf(out, "detail: %s\n", d)
	}
	return nil
}
