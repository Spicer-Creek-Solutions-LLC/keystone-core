// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type listOpts struct {
	prefix    string
	pageSize  int32
	pageToken string
}

func listCmd(g *globals) *cobra.Command {
	opts := &listOpts{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secrets under a prefix (metadata only)",
		Long: "List secrets under the given prefix. Per the v1.0 contract " +
			"cleartext data is never on a list response — only path / " +
			"version / labels / updated_at.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.prefix, "prefix", "",
		"restrict to paths beginning with this prefix")
	cmd.Flags().Int32Var(&opts.pageSize, "page-size", 0,
		"maximum entries per page (0 = no limit)")
	cmd.Flags().StringVar(&opts.pageToken, "page-token", "",
		"opaque cursor from the previous response's next_page_token")
	return cmd
}

func runList(ctx context.Context, out io.Writer, g *globals, opts *listOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.ListSecrets(authContext(ctx, g.APIKey), &v1.ListSecretsRequest{
		PathPrefix: opts.prefix,
		PageSize:   opts.pageSize,
		PageToken:  opts.pageToken,
	})
	if err != nil {
		return fmt.Errorf("ListSecrets: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printList(out, resp)
	}
}

func printList(out io.Writer, resp *v1.ListSecretsResponse) error {
	entries := resp.GetSecrets()
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out, "no secrets")
		return nil
	}
	t := newTable(out)
	t.header("PATH", "VERSION", "LABELS", "UPDATED")
	for _, e := range entries {
		t.row(
			e.GetPath(),
			defaultDash(e.GetVersion()),
			formatLabels(e.GetLabels()),
			formatProtoTimestamp(e.GetUpdatedAt()),
		)
	}
	if err := t.flush(); err != nil {
		return err
	}
	if tok := resp.GetNextPageToken(); tok != "" {
		_, _ = fmt.Fprintf(out, "\n--page-token %q to fetch the next page\n", tok)
	}
	return nil
}

func defaultDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
