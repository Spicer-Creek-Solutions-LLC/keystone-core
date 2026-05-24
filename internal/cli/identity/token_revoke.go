// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func tokenRevokeCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Delete a join token by ID",
		Long: "Revokes (deletes) a join token by its server-assigned ID. " +
			"Returns success even if the token was already gone — the call is " +
			"idempotent from the operator's perspective (NotFound is mapped to " +
			"exit code 0 with a warning).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenRevoke(cmd.Context(), cmd.OutOrStdout(), g, args[0])
		},
	}
	return cmd
}

func runTokenRevoke(ctx context.Context, out io.Writer, g *globals, id string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("identity: token id is required")
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	_, err = client.DeleteJoinToken(authContext(ctx, g.APIKey), &v1.DeleteJoinTokenRequest{
		Id: id,
	})
	if err != nil {
		// Treat NotFound as idempotent — exit 0 with a notice. Any
		// other error propagates.
		if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
			switch g.Output {
			case FormatJSON:
				return writeJSONAny(out, map[string]string{
					"id":     id,
					"status": "not_found",
					"note":   "token already absent — DeleteJoinToken is idempotent",
				})
			default:
				_, _ = fmt.Fprintf(out, "token %q not found (already revoked?)\n", id)
				return nil
			}
		}
		return fmt.Errorf("DeleteJoinToken: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSONAny(out, map[string]string{
			"id":     id,
			"status": "revoked",
		})
	default:
		_, _ = fmt.Fprintf(out, "revoked: %s\n", id)
		return nil
	}
}

