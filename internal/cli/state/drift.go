package state

import (
	"fmt"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func driftCmd(g *globals) *cobra.Command {
	flags := &inputFlags{}
	var fix bool
	cmd := &cobra.Command{
		Use:   "drift <file>",
		Short: "Detect drift; with --fix, re-apply the file to remediate",
		Long: "Runs DetectDrift on the server and renders a severity-grouped " +
			"report. With --fix and any drift present, re-submits the same YAML " +
			"via ApplyState to remediate. The runner is idempotent so in-sync " +
			"decls are cheap to re-check.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDrift(cmd, args, g, flags, fix)
		},
	}
	registerInputFlags(cmd, flags)
	cmd.Flags().BoolVar(&fix, "fix", false,
		"after detection, re-apply the same YAML to remediate drift")
	return cmd
}

func runDrift(cmd *cobra.Command, args []string, g *globals, flags *inputFlags, fix bool) error {
	ctx := cmd.Context()
	yaml, defaultSource, err := readInputYAML(args)
	if err != nil {
		return err
	}
	vars, err := parseKeyValues(flags.Variables)
	if err != nil {
		return fmt.Errorf("--variable: %w", err)
	}
	facts, err := parseKeyValues(flags.Facts)
	if err != nil {
		return fmt.Errorf("--fact: %w", err)
	}
	source := resolveSource(flags.Source, defaultSource)

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.DetectDrift(authContext(ctx, g.APIKey), &v1.DetectDriftRequest{
		YamlContent:       yaml,
		Facts:             facts,
		VariableOverrides: vars,
		Source:            source,
		ClusterId:         flags.Cluster,
		AgentId:           flags.Agent,
	})
	if err != nil {
		return fmt.Errorf("drift: %w", err)
	}
	out := cmd.OutOrStdout()
	if err := printDrift(out, g.Output, resp); err != nil {
		return err
	}

	// With --fix, only act when there's drift to act on. In-sync /
	// errors-only / skipped runs need operator inspection, not a
	// blind re-apply.
	if !fix {
		return nil
	}
	if resp.GetAggregates().GetDrifted() == 0 {
		fmt.Fprintln(out, "\nno drift to fix; skipping --fix")
		return nil
	}
	fmt.Fprintln(out, "\n--- fix ---")
	stream, err := client.ApplyState(authContext(ctx, g.APIKey), &v1.ApplyStateRequest{
		YamlContent:       yaml,
		Facts:             facts,
		VariableOverrides: vars,
		Source:            source,
		ClusterId:         flags.Cluster,
		AgentId:           flags.Agent,
	})
	if err != nil {
		return fmt.Errorf("drift --fix: %w", err)
	}
	return drainApplyStream(stream, out, g.Output)
}
