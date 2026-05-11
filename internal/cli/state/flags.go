package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// inputFlags is the set of flags every server-bound subcommand
// (apply / check / drift) shares. Compile / vars get reuse the
// content + variable subset but skip the agent / cluster / source
// fields that only matter once a request hits the server.
type inputFlags struct {
	Agent     string
	Cluster   string
	Source    string
	Variables []string // raw "k=v" strings; parsed via parseKeyValues
	Facts     []string
}

// localFlags is the input shape for the local-only subcommands.
type localFlags struct {
	Variables []string
	Facts     []string
}

// registerInputFlags wires the shared server-bound flags onto cmd.
func registerInputFlags(cmd *cobra.Command, f *inputFlags) {
	cmd.Flags().StringVar(&f.Agent, "agent", "",
		"target agent ID")
	cmd.Flags().StringVar(&f.Cluster, "cluster", "",
		"cluster ID (optional)")
	cmd.Flags().StringVar(&f.Source, "source", "",
		"logical source name for history (defaults to file basename)")
	cmd.Flags().StringSliceVar(&f.Variables, "variable", nil,
		"variable override (key=value); repeatable")
	cmd.Flags().StringSliceVar(&f.Facts, "fact", nil,
		"agent fact (key=value); repeatable")
}

func registerLocalFlags(cmd *cobra.Command, f *localFlags) {
	cmd.Flags().StringSliceVar(&f.Variables, "variable", nil,
		"variable override (key=value); repeatable")
	cmd.Flags().StringSliceVar(&f.Facts, "fact", nil,
		"agent fact (key=value); repeatable")
}

// parseKeyValues parses a slice of "key=value" strings into a map.
// Empty input yields nil so the proto field stays unset.
func parseKeyValues(in []string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for _, item := range in {
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value, got %q", item)
		}
		if k == "" {
			return nil, fmt.Errorf("empty key in %q", item)
		}
		out[k] = v
	}
	return out, nil
}

// readInputYAML loads the YAML file referenced by args[0]. Returns
// the bytes and the inferred default source name (basename) for the
// caller to use when --source is empty.
func readInputYAML(args []string) ([]byte, string, error) {
	if len(args) == 0 {
		return nil, "", fmt.Errorf("state: a YAML file is required")
	}
	path := args[0]
	data, err := os.ReadFile(path) //nolint:gosec // CLI input path; operators feed their own YAML files
	if err != nil {
		return nil, "", fmt.Errorf("state: read %s: %w", path, err)
	}
	return data, filepath.Base(path), nil
}

// resolveSource picks the right source name: --source wins; otherwise
// the basename of the input file.
func resolveSource(flagValue, defaultName string) string {
	if flagValue != "" {
		return flagValue
	}
	return defaultName
}
