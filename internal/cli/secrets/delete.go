// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func deleteCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <path>",
		Short: "Delete a secret",
		Long: "Delete a secret at the given path. v1.0 wire format does " +
			"not distinguish soft vs hard delete — backends apply their " +
			"native default (Vault KV v2: soft delete; encrypted-file: " +
			"hard remove from the in-memory state + atomic rewrite).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), cmd.OutOrStdout(), g, args[0])
		},
	}
	return cmd
}

func runDelete(ctx context.Context, out io.Writer, g *globals, path string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.DeleteSecret(authContext(ctx, g.APIKey), &v1.DeleteSecretRequest{Path: path})
	if err != nil {
		return fmt.Errorf("DeleteSecret: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		_, _ = fmt.Fprintf(out, "deleted %q\n", path)
		return nil
	}
}
