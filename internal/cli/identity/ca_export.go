package identity

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type caExportOpts struct {
	what string
}

func caExportCmd(g *globals) *cobra.Command {
	opts := &caExportOpts{what: "root"}
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Print CA material — PEM (root|signing) or JWKS (bundle)",
		Long: "Dumps the requested CA material to stdout: --what root or " +
			"--what signing returns a PEM-encoded certificate; --what bundle " +
			"returns the SPIFFE JWKS-format trust bundle (a JSON document, not " +
			"PEM — operators that need PEM derive it via --what root).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCAExport(cmd.Context(), cmd.OutOrStdout(), g, opts)
		},
	}
	cmd.Flags().StringVar(&opts.what, "what", "root",
		"material to export: root | signing | bundle")
	return cmd
}

func runCAExport(ctx context.Context, out io.Writer, g *globals, opts *caExportOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}

	whatEnum, err := parseExportWhat(opts.what)
	if err != nil {
		return err
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.ExportCA(authContext(ctx, g.APIKey), &v1.ExportCARequest{
		What: whatEnum,
	})
	if err != nil {
		return fmt.Errorf("ExportCA: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		// `--output json --what bundle` is a no-op wrapper around
		// the bundle's own JSON; for PEM, we wrap it in a
		// {"pem": "..."} envelope so it can be machine-parsed.
		return writeJSONAny(out, map[string]string{
			"what": opts.what,
			"pem":  string(resp.GetPem()),
		})
	default:
		_, err := out.Write(resp.GetPem())
		return err
	}
}

func parseExportWhat(s string) (v1.ExportCARequest_What, error) {
	switch s {
	case "root":
		return v1.ExportCARequest_WHAT_ROOT, nil
	case "signing":
		return v1.ExportCARequest_WHAT_SIGNING, nil
	case "bundle":
		return v1.ExportCARequest_WHAT_BUNDLE, nil
	default:
		return v1.ExportCARequest_WHAT_UNSPECIFIED,
			fmt.Errorf("identity: --what %q must be one of root | signing | bundle", s)
	}
}
