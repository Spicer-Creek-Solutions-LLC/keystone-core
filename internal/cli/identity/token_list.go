package identity

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type tokenListOpts struct {
	agentID   string
	unused    bool
	unexpired bool
}

func tokenListCmd(g *globals) *cobra.Command {
	opts := &tokenListOpts{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List join tokens (no cleartext)",
		Long: "Lists join-token records. The cleartext Token field is " +
			"NEVER returned by this command — only ID, Prefix, AgentID, " +
			"expiry, use-count, and metadata.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTokenList(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.agentID, "agent-id", "",
		"restrict to tokens bound to this agent")
	cmd.Flags().BoolVar(&opts.unused, "unused", false,
		"show only tokens whose UsedCount < MaxUses")
	cmd.Flags().BoolVar(&opts.unexpired, "unexpired", false,
		"show only tokens whose ExpiresAt > now")
	return cmd
}

func runTokenList(ctx context.Context, out io.Writer, g *globals, opts *tokenListOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	req := &v1.ListJoinTokensRequest{
		AgentId: opts.agentID,
		Unused:  opts.unused,
	}
	if opts.unexpired {
		req.UnexpiredAt = timestamppb.New(time.Now())
	}

	resp, err := client.ListJoinTokens(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("ListJoinTokens: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printTokenList(out, resp.GetTokens())
	}
}

func printTokenList(out io.Writer, toks []*v1.JoinToken) error {
	if len(toks) == 0 {
		_, _ = fmt.Fprintln(out, "no join tokens")
		return nil
	}
	t := newTable(out)
	t.header("ID", "PREFIX", "AGENT", "EXPIRES", "USED", "METADATA")
	for _, tok := range toks {
		// Defensive guard against a wire-side bug returning
		// cleartext: refuse to print it if it leaked.
		if tok.GetToken() != "" {
			return fmt.Errorf("identity: ListJoinTokens returned cleartext Token for %q — refusing to print", tok.GetId())
		}
		expires := tok.GetExpiresAt().AsTime()
		t.row(
			tok.GetId(),
			tok.GetPrefix(),
			tok.GetAgentId(),
			formatTimestamp(&expires),
			fmt.Sprintf("%d/%d", tok.GetUsedCount(), tok.GetMaxUses()),
			formatMetadata(tok.GetMetadata()),
		)
	}
	return t.flush()
}
