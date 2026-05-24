// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/backup/dest"
)

// listCmd builds the `kscore-backup list` subcommand. The --dest
// flag is a *prefix* URI: a local directory or `s3://bucket/prefix/`.
func listCmd(g *globals) *cobra.Command {
	var prefix string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List backup artifacts at a destination prefix",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), cmd.OutOrStdout(), g, prefix)
		},
	}
	cmd.Flags().StringVar(&prefix, "dest", "", "Destination prefix URI (e.g. /var/backups or s3://bucket/path/)")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

func runList(ctx context.Context, out io.Writer, g *globals, prefix string) error {
	lister, err := dest.ResolveLister(prefix, g.destConfig())
	if err != nil {
		return err
	}
	entries, err := lister.List(ctx)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(out, "no artifacts found")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSIZE\tLAST_MODIFIED")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%d\t%s\n", e.Name, e.Size, e.LastModified.UTC().Format("2006-01-02T15:04:05Z"))
	}
	return tw.Flush()
}
