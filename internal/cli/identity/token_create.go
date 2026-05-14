package identity

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type tokenCreateOpts struct {
	agentID  string
	ttl      time.Duration
	maxUses  int
	metadata []string // repeated --metadata key=value flags
}

// tokenCreateCmd builds the `token create` subcommand.
func tokenCreateCmd(g *globals) *cobra.Command {
	opts := &tokenCreateOpts{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Mint a new join token (cleartext returned once)",
		Long: "Mints a join token via the IdentityService. The cleartext value " +
			"is printed exactly once — copy it to the new agent's bootstrap " +
			"configuration. Subsequent `token list` calls return everything " +
			"EXCEPT the cleartext.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTokenCreate(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.agentID, "agent-id", "",
		"agent identifier to bind the token to (required)")
	cmd.Flags().DurationVar(&opts.ttl, "ttl", 0,
		"token lifetime (default 5m; max 24h)")
	cmd.Flags().IntVar(&opts.maxUses, "max-uses", 0,
		"how many agents may use this token (default 1)")
	cmd.Flags().StringSliceVar(&opts.metadata, "metadata", nil,
		"operator metadata, repeatable: --metadata role=web --metadata env=prod")
	_ = cmd.MarkFlagRequired("agent-id")
	return cmd
}

func runTokenCreate(ctx context.Context, out io.Writer, g *globals, opts *tokenCreateOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	metadata, err := parseMetadataFlag(opts.metadata)
	if err != nil {
		return err
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.CreateJoinToken(authContext(ctx, g.APIKey), &v1.CreateJoinTokenRequest{
		AgentId:    opts.agentID,
		TtlSeconds: int64(opts.ttl / time.Second),
		MaxUses:    int32(opts.maxUses), //nolint:gosec // operator-supplied small int
		Metadata:   metadata,
	})
	if err != nil {
		return fmt.Errorf("CreateJoinToken: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printCreatedToken(out, resp.GetToken())
	}
}

// printCreatedToken renders the create response with a clear
// "shown once" banner so operators don't miss the cleartext.
func printCreatedToken(out io.Writer, tok *v1.JoinToken) error {
	_, _ = fmt.Fprintln(out, "Join token created — the cleartext below is shown ONCE.")
	_, _ = fmt.Fprintln(out, "Pass it to the new agent's bootstrap configuration; the")
	_, _ = fmt.Fprintln(out, "server keeps only the salted hash.")
	_, _ = fmt.Fprintln(out)

	t := newTable(out)
	t.header("FIELD", "VALUE")
	t.row("ID", tok.GetId())
	t.row("Token", tok.GetToken())
	t.row("Prefix", tok.GetPrefix())
	t.row("AgentID", tok.GetAgentId())
	t.row("TTL", (time.Duration(tok.GetTtlSeconds()) * time.Second).String())
	t.row("MaxUses", fmt.Sprintf("%d", tok.GetMaxUses()))
	created := tok.GetCreatedAt().AsTime()
	expires := tok.GetExpiresAt().AsTime()
	t.row("Created", formatTimestamp(&created))
	t.row("Expires", formatTimestamp(&expires))
	t.row("Metadata", formatMetadata(tok.GetMetadata()))
	return t.flush()
}

// parseMetadataFlag parses repeated `--metadata key=value` flags
// into a map. Rejects entries missing `=` or with empty keys.
func parseMetadataFlag(in []string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for _, raw := range in {
		k, v, ok := strings.Cut(raw, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("identity: --metadata %q must be key=value", raw)
		}
		out[k] = v
	}
	return out, nil
}
