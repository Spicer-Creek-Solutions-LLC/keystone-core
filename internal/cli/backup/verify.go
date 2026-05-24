// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	bkp "go.keystone-core.io/keystone-core/internal/backup"
	"go.keystone-core.io/keystone-core/internal/backup/age"
	"go.keystone-core.io/keystone-core/internal/backup/dest"
)

// verifyCmd builds the `kscore-backup verify` subcommand. It reads
// an artifact, decrypts (if age-wrapped), runs the full integrity
// check via [bkp.RestoreManager.Restore] with an empty Selection (=
// verify-only mode), and prints the manifest summary.
func verifyCmd(g *globals) *cobra.Command {
	var (
		src         string
		ageIdentity string
	)
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the integrity of a backup artifact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVerify(cmd.Context(), cmd.OutOrStdout(), g, src, ageIdentity)
		},
	}
	cmd.Flags().StringVar(&src, "src", "", "Backup artifact URI (e.g. /path/to/foo.tar or s3://bucket/key.tar)")
	cmd.Flags().StringVar(&ageIdentity, "age-identity", "", "Path to an age identity file to decrypt the artifact")
	_ = cmd.MarkFlagRequired("src")
	return cmd
}

func runVerify(ctx context.Context, out io.Writer, g *globals, src, ageIdentity string) error {
	source, err := dest.ResolveSource(src, g.destConfig())
	if err != nil {
		return err
	}
	rc, err := source.Open(ctx)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = rc.Close() }()

	var reader io.Reader = rc
	if ageIdentity != "" {
		ids, err := age.LoadIdentityFile(ageIdentity)
		if err != nil {
			return err
		}
		dec := &age.Decrypter{Identities: ids}
		reader, err = bkp.NewDecryptingReader(reader, dec)
		if err != nil {
			return err
		}
	}

	mgr, err := bkp.NewRestoreManager(bkp.WithRestoreLogger(g.logger))
	if err != nil {
		return err
	}

	// Empty Selection = verify-only mode: integrity check + manifest
	// parse + populated-cluster guard (skipped when no detector
	// wired), but no handler dispatch.
	manifest, err := mgr.Restore(ctx, reader, bkp.RestoreOptions{})
	if err != nil {
		return err
	}

	return printManifestSummary(out, manifest)
}

func printManifestSummary(out io.Writer, manifest *bkp.Manifest) error {
	fmt.Fprintln(out, "OK")
	fmt.Fprintf(out, "format_version: %d\n", manifest.FormatVersion)
	fmt.Fprintf(out, "taken_at:       %s\n", manifest.TakenAt.Format("2006-01-02T15:04:05Z07:00"))
	if manifest.ClusterName != "" {
		fmt.Fprintf(out, "cluster_name:   %s\n", manifest.ClusterName)
	}
	fmt.Fprintf(out, "components:     %d\n", len(manifest.Components))
	if len(manifest.Components) == 0 {
		return nil
	}
	fmt.Fprintln(out)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPATH\tSIZE\tSHA256")
	for _, c := range manifest.Components {
		short := c.SHA256Hex
		if len(short) > 12 {
			short = short[:12]
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", c.Name, c.Path, c.Size, short)
	}
	return tw.Flush()
}
