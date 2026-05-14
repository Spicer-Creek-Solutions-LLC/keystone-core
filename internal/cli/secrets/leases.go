package secrets

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func leasesCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "leases",
		Short: "List + manage dynamic-secret leases",
		Long: "Subcommands for the LeaseManager surface — list / get / " +
			"renew / revoke. Dynamic secrets are issued by Vault " +
			"engines (database, pki, ssh, aws, etc.); the lease manager " +
			"persists them and runs the renewal scheduler.",
	}
	cmd.AddCommand(leaseListCmd(g))
	cmd.AddCommand(leaseGetCmd(g))
	cmd.AddCommand(leaseRenewCmd(g))
	cmd.AddCommand(leaseRevokeCmd(g))
	return cmd
}

// ---- leases list -------------------------------------------------

type leaseListOpts struct {
	pathPrefix string
	pageSize   int32
	pageToken  string
}

func leaseListCmd(g *globals) *cobra.Command {
	opts := &leaseListOpts{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tracked leases",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLeaseList(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.pathPrefix, "secret-path", "",
		"restrict to leases under this secret-path prefix")
	cmd.Flags().Int32Var(&opts.pageSize, "page-size", 0,
		"maximum entries per page (0 = no limit)")
	cmd.Flags().StringVar(&opts.pageToken, "page-token", "",
		"opaque pagination cursor from the previous response")
	return cmd
}

func runLeaseList(ctx context.Context, out io.Writer, g *globals, opts *leaseListOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.ListLeases(authContext(ctx, g.APIKey), &v1.ListLeasesRequest{
		SecretPath: opts.pathPrefix,
		PageSize:   opts.pageSize,
		PageToken:  opts.pageToken,
	})
	if err != nil {
		return fmt.Errorf("ListLeases: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printLeaseList(out, resp)
	}
}

func printLeaseList(out io.Writer, resp *v1.ListLeasesResponse) error {
	leases := resp.GetLeases()
	if len(leases) == 0 {
		_, _ = fmt.Fprintln(out, "no leases")
		return nil
	}
	t := newTable(out)
	t.header("ID", "SECRET_PATH", "HOLDER", "RENEWABLE", "EXPIRES")
	for _, l := range leases {
		t.row(
			l.GetId(),
			defaultDash(l.GetSecretPath()),
			defaultDash(l.GetHolder()),
			boolToStr(l.GetRenewable()),
			formatProtoTimestamp(l.GetExpiresAt()),
		)
	}
	return t.flush()
}

// ---- leases get --------------------------------------------------

func leaseGetCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <lease-id>",
		Short: "Get a single lease by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLeaseGet(cmd.Context(), cmd.OutOrStdout(), g, args[0])
		},
	}
	return cmd
}

func runLeaseGet(ctx context.Context, out io.Writer, g *globals, id string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetLease(authContext(ctx, g.APIKey), &v1.GetLeaseRequest{LeaseId: id})
	if err != nil {
		return fmt.Errorf("GetLease: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printLease(out, resp.GetLease())
	}
}

func printLease(out io.Writer, l *v1.Lease) error {
	if l == nil {
		_, _ = fmt.Fprintln(out, "no lease")
		return nil
	}
	t := newTable(out)
	t.header("FIELD", "VALUE")
	t.row("id", l.GetId())
	t.row("secret_path", defaultDash(l.GetSecretPath()))
	t.row("holder", defaultDash(l.GetHolder()))
	t.row("renewable", boolToStr(l.GetRenewable()))
	t.row("issued_at", formatProtoTimestamp(l.GetIssuedAt()))
	t.row("expires_at", formatProtoTimestamp(l.GetExpiresAt()))
	return t.flush()
}

// ---- leases renew ------------------------------------------------

func leaseRenewCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "renew <lease-id>",
		Short: "Extend a lease's TTL via the backend",
		Long: "Calls the broker's RenewLease, which dispatches to the " +
			"backend that issued the lease + emits an audit event. The " +
			"new TTL comes from the backend; pass --increment to " +
			"request a specific extension (engines may or may not honor it).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLeaseRenew(cmd.Context(), cmd.OutOrStdout(), g, args[0])
		},
	}
	return cmd
}

func runLeaseRenew(ctx context.Context, out io.Writer, g *globals, id string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.RenewLease(authContext(ctx, g.APIKey), &v1.RenewLeaseRequest{LeaseId: id})
	if err != nil {
		return fmt.Errorf("RenewLease: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		_, _ = fmt.Fprintf(out, "renewed %q\n", id)
		return printLease(out, resp.GetLease())
	}
}

// ---- leases revoke -----------------------------------------------

func leaseRevokeCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke <lease-id>",
		Short: "Revoke a lease (idempotent)",
		Long: "Tears the credential down at the backend. Idempotent — " +
			"revoking an already-gone lease returns success.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLeaseRevoke(cmd.Context(), cmd.OutOrStdout(), g, args[0])
		},
	}
	return cmd
}

func runLeaseRevoke(ctx context.Context, out io.Writer, g *globals, id string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.RevokeLease(authContext(ctx, g.APIKey), &v1.RevokeLeaseRequest{LeaseId: id})
	if err != nil {
		return fmt.Errorf("RevokeLease: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		_, _ = fmt.Fprintf(out, "revoked %q\n", id)
		return nil
	}
}

// ---- helpers -----------------------------------------------------

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
