// SPDX-License-Identifier: Apache-2.0

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

// restoreCmd builds the `kscore-backup restore` subcommand. Task 7b
// wires only the config component handler (gate-v1.0 ROADMAP entry
// "Backup + restore component adapters" covers storage / etcd /
// JetStream / secrets / cluster). Components present in the artifact
// without a wired handler are silently skipped per Task-6 interest-
// list semantics; broader restore lights up as adapters land.
func restoreCmd(g *globals) *cobra.Command {
	var (
		src          string
		ageIdentity  string
		configOutDir string
		force        bool
	)
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore a backup artifact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRestore(cmd.Context(), cmd.OutOrStdout(), g, restoreOpts{
				src:          src,
				ageIdentity:  ageIdentity,
				configOutDir: configOutDir,
				force:        force,
			})
		},
	}
	cmd.Flags().StringVar(&src, "src", "", "Source URI (e.g. /tmp/backup.tar or s3://bucket/key.tar)")
	cmd.Flags().StringVar(&ageIdentity, "age-identity", "", "Path to an age identity file to decrypt the artifact")
	cmd.Flags().StringVar(&configOutDir, "config-out-dir", "", "Directory to write restored config files into")
	cmd.Flags().BoolVar(&force, "force", false, "Bypass the populated-cluster safety guard")
	_ = cmd.MarkFlagRequired("src")
	return cmd
}

type restoreOpts struct {
	src          string
	ageIdentity  string
	configOutDir string
	force        bool
}

func runRestore(ctx context.Context, out io.Writer, g *globals, o restoreOpts) error {
	source, err := dest.ResolveSource(o.src, g.destConfig())
	if err != nil {
		return err
	}
	rc, err := source.Open(ctx)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = rc.Close() }()

	var reader io.Reader = rc
	if o.ageIdentity != "" {
		ids, err := age.LoadIdentityFile(o.ageIdentity)
		if err != nil {
			return err
		}
		dec := &age.Decrypter{Identities: ids}
		reader, err = bkp.NewDecryptingReader(reader, dec)
		if err != nil {
			return err
		}
	}

	mgrOpts := []bkp.RestoreOption{bkp.WithRestoreLogger(g.logger)}
	if o.configOutDir != "" {
		mgrOpts = append(mgrOpts, bkp.WithConfigRestore(&FilesystemConfigRestore{Dir: o.configOutDir}))
	}
	mgr, err := bkp.NewRestoreManager(mgrOpts...)
	if err != nil {
		return err
	}

	manifest, err := mgr.Restore(ctx, reader, bkp.RestoreOptions{
		Force:     o.force,
		Selection: bkp.SelectAll(),
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "OK")
	return printManifestSummary(out, manifest)
}
