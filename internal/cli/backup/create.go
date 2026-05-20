package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	bkp "go.keystone-core.io/keystone-core/internal/backup"
	"go.keystone-core.io/keystone-core/internal/backup/age"
	"go.keystone-core.io/keystone-core/internal/backup/dest"
)

// createCmd builds the `kscore-backup create` subcommand. Task 7b
// wires only the config component (Epic 18 has the storage /
// jetstream / etcd / secrets / cluster adapters tracked under a
// gate-v1.0 ROADMAP entry); the manifest grows as those land without
// any CLI change here.
func createCmd(g *globals) *cobra.Command {
	var (
		dst            string
		configs        []string
		ageRecipients  string
		clusterName    string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a backup artifact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCreate(cmd.Context(), cmd.OutOrStdout(), g, createOpts{
				dst:           dst,
				configs:       configs,
				ageRecipients: ageRecipients,
				clusterName:   clusterName,
			})
		},
	}
	cmd.Flags().StringVar(&dst, "dest", "", "Destination URI (e.g. /tmp/backup.tar or s3://bucket/key.tar)")
	cmd.Flags().StringSliceVar(&configs, "config", nil, "Config file path(s) to include (repeatable)")
	cmd.Flags().StringVar(&ageRecipients, "age-recipients", "", "Path to an age recipients file (encrypts the artifact)")
	cmd.Flags().StringVar(&clusterName, "cluster-name", "", "Stamp this cluster name into the manifest")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

type createOpts struct {
	dst           string
	configs       []string
	ageRecipients string
	clusterName   string
}

func runCreate(ctx context.Context, out io.Writer, g *globals, o createOpts) error {
	destination, err := dest.Resolve(o.dst, g.destConfig())
	if err != nil {
		return err
	}
	wc, err := destination.Open(ctx)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	closeOnce := closerOnce{w: wc}
	defer func() { _ = closeOnce.Close() }()

	var enc bkp.Encrypter
	if o.ageRecipients != "" {
		recs, err := age.LoadRecipientsFile(o.ageRecipients)
		if err != nil {
			return err
		}
		enc = &age.Encrypter{Recipients: recs}
	}
	writer, err := bkp.NewEncryptingWriter(wc, enc)
	if err != nil {
		return err
	}

	mgrOpts := []bkp.Option{bkp.WithLogger(g.logger)}
	if len(o.configs) > 0 {
		mgrOpts = append(mgrOpts, bkp.WithConfig(&FilesystemConfigCollector{Paths: o.configs}))
	}
	if o.clusterName != "" {
		mgrOpts = append(mgrOpts, bkp.WithClusterName(o.clusterName))
	}
	mgr, err := bkp.NewBackupManager(mgrOpts...)
	if err != nil {
		return err
	}

	manifest, err := mgr.CreateBackup(ctx, writer)
	if err != nil {
		return err
	}

	// Close the age-wrapped writer FIRST (flushes the cipher trailer)
	// then the underlying destination writer (commits the local file
	// or finalizes the S3 multipart upload).
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
	}
	if err := closeOnce.Close(); err != nil {
		return fmt.Errorf("close destination: %w", err)
	}

	fmt.Fprintln(out, "OK")
	return printManifestSummary(out, manifest)
}

// closerOnce wraps an io.Closer so Close is idempotent across the
// happy path (explicit Close after success) and the deferred fallback
// (cleanup on early return). Avoids a "file already closed" log on
// the happy path.
type closerOnce struct {
	w      io.Closer
	closed bool
}

func (c *closerOnce) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.w.Close()
}
