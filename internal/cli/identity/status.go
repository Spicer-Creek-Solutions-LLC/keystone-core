// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func statusCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show provider health + token + CA expiry snapshot",
		Long: "Single-call summary of the identity provider's state: started " +
			"flag, trust domain, root + signing expiry, watcher count, token " +
			"counts (total / unused / expired).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), cmd.OutOrStdout(), g)
		},
	}
	return cmd
}

func runStatus(ctx context.Context, out io.Writer, g *globals) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetStatus(authContext(ctx, g.APIKey), &v1.GetStatusRequest{})
	if err != nil {
		return fmt.Errorf("GetStatus: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printStatus(out, resp)
	}
}

func printStatus(out io.Writer, resp *v1.GetStatusResponse) error {
	t := newTable(out)
	t.header("FIELD", "VALUE")
	if resp.GetStarted() {
		t.row("Status", "running")
	} else {
		t.row("Status", "STOPPED — provider not running")
	}
	t.row("TrustDomain", resp.GetTrustDomain())
	if ts := resp.GetStartedAt(); ts != nil {
		startedAt := ts.AsTime()
		t.row("StartedAt", formatTimestamp(&startedAt))
		t.row("Uptime", formatDuration(time.Since(startedAt)))
	} else {
		t.row("StartedAt", "—")
	}
	if root := resp.GetRootExpiresAt(); root != nil {
		ts := root.AsTime()
		t.row("RootExpiresAt", formatTimestamp(&ts))
		t.row("RootRemaining", formatDuration(time.Until(ts)))
	}
	if signing := resp.GetSigningExpiresAt(); signing != nil {
		ts := signing.AsTime()
		t.row("SigningExpiresAt", formatTimestamp(&ts))
		t.row("SigningRemaining", formatDuration(time.Until(ts)))
	}
	t.row("Watchers", fmt.Sprintf("%d", resp.GetWatcherCount()))
	t.row("Tokens total", fmt.Sprintf("%d", resp.GetTokenTotal()))
	t.row("Tokens unused", fmt.Sprintf("%d", resp.GetTokenUnused()))
	t.row("Tokens expired", fmt.Sprintf("%d", resp.GetTokenExpired()))
	return t.flush()
}
