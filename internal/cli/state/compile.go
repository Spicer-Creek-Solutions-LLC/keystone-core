// SPDX-License-Identifier: Apache-2.0

package state

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func compileCmd(g *globals) *cobra.Command {
	flags := &localFlags{}
	cmd := &cobra.Command{
		Use:   "compile <file>",
		Short: "Compile a YAML state file locally (Parse → Render → Validate → Resolve)",
		Long: "Runs the full client-side pipeline and prints the ordered " +
			"declaration list. Does not contact the server. Useful for " +
			"verifying require chains and templating before submission.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompile(cmd, args, g, flags)
		},
	}
	registerLocalFlags(cmd, flags)
	return cmd
}

func runCompile(cmd *cobra.Command, args []string, g *globals, flags *localFlags) error {
	decls, _, err := compileLocal(args, flags)
	if err != nil {
		return err
	}
	return printCompile(cmd.OutOrStdout(), g.Output, decls)
}

// compileLocal runs the engine pipeline. Returns the ordered
// declarations and the rendered Variables map so vars get can reuse
// the same code path.
//
// Note: validation needs a Registry. Compile is intentionally
// permissive — it uses an empty Registry, which means
// module-existence checks fail with "module not registered". We
// short-circuit those into warnings rather than errors so authors
// can lint requisites + templating without having any modules
// installed locally. Real validation happens server-side at apply
// time when the stdlib registry is populated.
func compileLocal(args []string, flags *localFlags) ([]*statemgmt.Declaration, map[string]any, error) {
	yaml, _, err := readInputYAML(args)
	if err != nil {
		return nil, nil, err
	}
	vars, err := parseKeyValues(flags.Variables)
	if err != nil {
		return nil, nil, fmt.Errorf("--variable: %w", err)
	}
	facts, err := parseKeyValues(flags.Facts)
	if err != nil {
		return nil, nil, fmt.Errorf("--fact: %w", err)
	}

	sf, err := statemgmt.Parse(yaml)
	if err != nil {
		return nil, nil, err
	}
	if len(sf.Includes) > 0 {
		return nil, nil, fmt.Errorf("compile: includes not supported in v1.0; got %v", sf.Includes)
	}
	if len(vars) > 0 {
		if sf.Variables == nil {
			sf.Variables = map[string]any{}
		}
		for k, v := range vars {
			sf.Variables[k] = v
		}
	}
	rendered, err := statemgmt.NewRenderer().RenderStateFile(sf, mapStringToAny(facts))
	if err != nil {
		return nil, nil, err
	}
	if err := validateCompile(rendered); err != nil {
		return nil, nil, err
	}
	ordered, err := statemgmt.NewResolver().Resolve(rendered)
	if err != nil {
		return nil, nil, err
	}
	return ordered, rendered.Variables, nil
}

// validateCompile runs the Validator but suppresses
// module-not-registered issues since local compile has no real
// modules. Other validation issues (requisite shape, missing fields,
// requisite refs) still produce errors so authors get useful
// feedback.
func validateCompile(sf *statemgmt.StateFile) error {
	err := statemgmt.NewValidator(statemgmt.NewRegistry()).Validate(sf)
	if err == nil {
		return nil
	}
	ve, ok := err.(*statemgmt.ValidationError)
	if !ok {
		return err
	}
	var kept []statemgmt.ValidationIssue
	for _, iss := range ve.Issues {
		// Drop "module %q not registered" issues — they're the
		// expected outcome of compiling against an empty Registry.
		// Also drop the matching State issue: when the module is
		// not registered, ValidStates is unknown so the engine
		// can't validate State and produces a follow-on issue.
		if iss.Field == "Module" {
			continue
		}
		if iss.Field == "State" {
			continue
		}
		kept = append(kept, iss)
	}
	if len(kept) == 0 {
		return nil
	}
	return &statemgmt.ValidationError{Issues: kept}
}

// mapStringToAny widens map[string]string to map[string]any for the
// Renderer's facts argument.
func mapStringToAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
