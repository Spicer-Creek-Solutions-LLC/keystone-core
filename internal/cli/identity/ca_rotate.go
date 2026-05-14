package identity

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func caRotateCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate-signing",
		Short: "Force an immediate signing-CA rotation",
		Long: "Calls RotateSigningCA on the live provider — mints a fresh " +
			"signing CA, persists it, rebuilds the trust bundle, and notifies " +
			"every watcher. Old leaves continue to verify (their chain still " +
			"includes the old signing CA cert under the unchanged root).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCARotate(cmd.Context(), cmd.OutOrStdout(), g)
		},
	}
	return cmd
}

func runCARotate(ctx context.Context, out io.Writer, g *globals) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.RotateSigningCA(authContext(ctx, g.APIKey), &v1.RotateSigningCARequest{})
	if err != nil {
		return fmt.Errorf("RotateSigningCA: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		_, _ = fmt.Fprintf(out, "signing CA rotated\n")
		_, _ = fmt.Fprintf(out, "new JWT kid: %s\n", resp.GetNewJwtKid())
		return nil
	}
}
