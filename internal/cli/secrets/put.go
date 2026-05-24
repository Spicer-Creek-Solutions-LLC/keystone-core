// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type putOpts struct {
	data   []string
	labels []string
	ttl    time.Duration
}

func putCmd(g *globals) *cobra.Command {
	opts := &putOpts{}
	cmd := &cobra.Command{
		Use:   "put <path>",
		Short: "Write a secret",
		Long: "Write a secret. Use --data key=value (repeatable) for the " +
			"payload and --label key=value (repeatable) for metadata. " +
			"Optional --ttl applies for backends that honor per-secret " +
			"expiry (currently propagated as `ttl_seconds` metadata; the " +
			"file backend honors it natively in a v1.x ROADMAP entry).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPut(cmd.Context(), cmd.OutOrStdout(), g, args[0], opts)
		},
	}
	cmd.Flags().StringSliceVar(&opts.data, "data", nil,
		"data entry as key=value (repeatable)")
	cmd.Flags().StringSliceVar(&opts.labels, "label", nil,
		"label entry as key=value (repeatable)")
	cmd.Flags().DurationVar(&opts.ttl, "ttl", 0,
		"per-secret TTL (e.g. 5m); 0 = no expiry")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

func runPut(ctx context.Context, out io.Writer, g *globals, path string, opts *putOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}

	data, err := keyValSliceToMap(opts.data)
	if err != nil {
		return err
	}
	labels, err := keyValSliceToMap(opts.labels)
	if err != nil {
		return err
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	req := &v1.WriteSecretRequest{
		Path:       path,
		Data:       data,
		Labels:     labels,
		TtlSeconds: int32(opts.ttl / time.Second), // #nosec G115 -- caller-supplied TTL
	}
	resp, err := client.WriteSecret(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("WriteSecret: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		return printPut(out, resp)
	}
}

func printPut(out io.Writer, resp *v1.WriteSecretResponse) error {
	meta := resp.GetMetadata()
	if meta == nil {
		_, _ = fmt.Fprintln(out, "secret written")
		return nil
	}
	t := newTable(out)
	t.header("FIELD", "VALUE")
	t.row("path", meta.GetPath())
	if v := meta.GetVersion(); v != "" {
		t.row("version", v)
	}
	if labels := formatLabels(meta.GetLabels()); labels != "—" {
		t.row("labels", labels)
	}
	t.row("updated_at", formatProtoTimestamp(meta.GetUpdatedAt()))
	return t.flush()
}

// keyValSliceToMap parses a slice of `key=value` entries.
func keyValSliceToMap(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		k, v, err := parseKeyVal(e)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}
