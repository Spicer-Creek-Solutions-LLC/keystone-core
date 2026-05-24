// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type getOpts struct {
	version       string
	refresh       bool
	showCleartext bool
}

func getCmd(g *globals) *cobra.Command {
	opts := &getOpts{}
	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Read a secret",
		Long: "Read a secret at the given path. By default the data values " +
			"render as '***' in table output; pass --show-cleartext to print " +
			"them verbatim. JSON output always carries cleartext (operators " +
			"consuming the result programmatically already have access).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), cmd.OutOrStdout(), g, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.version, "version", "",
		"specific version to fetch (KV v2 backends only)")
	cmd.Flags().BoolVar(&opts.refresh, "refresh", false,
		"bypass the cache and force a backend round-trip")
	cmd.Flags().BoolVar(&opts.showCleartext, "show-cleartext", false,
		"print data values verbatim instead of '***' (table output)")
	return cmd
}

func runGet(ctx context.Context, out io.Writer, g *globals, path string, opts *getOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	req := &v1.GetSecretRequest{Path: path, Version: opts.version}
	resp, err := client.GetSecret(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("GetSecret: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printGet(out, resp, opts.showCleartext)
	}
}

func printGet(out io.Writer, resp *v1.GetSecretResponse, showCleartext bool) error {
	meta := resp.GetMetadata()
	if meta == nil {
		_, _ = fmt.Fprintln(out, "no secret returned")
		return nil
	}

	t := newTable(out)
	t.header("FIELD", "VALUE")
	t.row("path", meta.GetPath())
	if v := meta.GetVersion(); v != "" {
		t.row("version", v)
	}
	if labels := formatLabels(meta.GetLabels()); labels != "—" {
		t.row("labels", labels)
	}
	t.row("created_at", formatProtoTimestamp(meta.GetCreatedAt()))
	t.row("updated_at", formatProtoTimestamp(meta.GetUpdatedAt()))
	if err := t.flush(); err != nil {
		return err
	}

	data := resp.GetData()
	if len(data) == 0 {
		return nil
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "DATA")
	if showCleartext {
		_, _ = fmt.Fprintln(out, cleartextDataLines(data))
	} else {
		_, _ = fmt.Fprintln(out, maskedDataLines(data))
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "(use --show-cleartext to print data values)")
	}
	return nil
}
