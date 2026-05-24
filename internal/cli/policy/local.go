// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

// localInput is the on-disk shape of an `--input` file for
// `kscore-policy eval`. Mirrors policy.EvaluationInput.
type localInput struct {
	Resource  map[string]any `json:"resource,omitempty"`
	Action    string         `json:"action,omitempty"`
	User      string         `json:"user,omitempty"`
	Context   map[string]any `json:"context,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty"`
}

// detectType maps a source file path + optional --type override to
// an audit.PolicyType. .rego→opa, .cel→cel, .json→builtin.
func detectType(path, override string) (audit.PolicyType, error) {
	if override != "" {
		pt, err := audit.ParsePolicyType(override)
		if err != nil || !pt.IsKnown() {
			return "", fmt.Errorf("--type %q is not opa|cel|builtin", override)
		}
		return pt, nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".rego":
		return audit.PolicyTypeOPA, nil
	case ".cel":
		return audit.PolicyTypeCEL, nil
	case ".json":
		return audit.PolicyTypeBuiltin, nil
	}
	return "", fmt.Errorf("cannot infer policy type from %q; pass --type opa|cel|builtin", filepath.Base(path))
}

// transientEngine builds a throwaway Engine with all three real
// evaluators and the source registered as a single enabled policy.
// The local eval/validate subcommands never touch the server.
func transientEngine(path, typeOverride string) (*policy.Engine, string, error) {
	src, err := os.ReadFile(path) //nolint:gosec // operator-supplied authoring file
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}
	pt, err := detectType(path, typeOverride)
	if err != nil {
		return nil, "", err
	}
	const id = "cli-eval"
	reg := policy.NewRegistry()
	if rerr := reg.RegisterPolicy(&policy.Policy{
		ID:              id,
		Name:            filepath.Base(path),
		Type:            pt,
		Category:        policy.CategoryCustom,
		Severity:        audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit,
		Code:            string(src),
		Enabled:         true,
	}); rerr != nil {
		return nil, "", fmt.Errorf("invalid policy shape: %w", rerr)
	}
	eng, err := policy.NewEngine(reg,
		policy.WithEvaluator(audit.PolicyTypeOPA, policy.NewOPAEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeCEL, policy.NewCELEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeBuiltin, policy.NewBuiltinEvaluator()),
	)
	if err != nil {
		return nil, "", err
	}
	return eng, id, nil
}

// ---- eval (local) ---------------------------------------------------------

func evalCmd(g *globals) *cobra.Command {
	var (
		inputPath string
		typeFlag  string
	)
	cmd := &cobra.Command{
		Use:   "eval <policy-file>",
		Short: "Evaluate a policy source file locally (no server)",
		Long: "Read an OPA Rego / CEL / Builtin-JSON policy file and run it " +
			"in-process against the --input document. A policy-authoring + " +
			"CI tool — does not contact kscore-server. Type is inferred from " +
			"the extension (.rego/.cel/.json) or forced via --type.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEval(cmd.Context(), cmd.OutOrStdout(), g, args[0], inputPath, typeFlag)
		},
	}
	cmd.Flags().StringVar(&inputPath, "input", "", "JSON file with {resource,action,user,context,timestamp}")
	cmd.Flags().StringVar(&typeFlag, "type", "", "force policy type: opa | cel | builtin")
	return cmd
}

func runEval(ctx context.Context, out io.Writer, g *globals, policyPath, inputPath, typeFlag string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	eng, id, err := transientEngine(policyPath, typeFlag)
	if err != nil {
		return err
	}
	in := policy.EvaluationInput{Timestamp: time.Now().UTC()}
	if inputPath != "" {
		raw, rerr := os.ReadFile(inputPath) //nolint:gosec // operator-supplied
		if rerr != nil {
			return fmt.Errorf("read --input %s: %w", inputPath, rerr)
		}
		var li localInput
		if jerr := json.Unmarshal(raw, &li); jerr != nil {
			return fmt.Errorf("--input is not valid JSON: %w", jerr)
		}
		in.Resource = li.Resource
		in.Action = li.Action
		in.User = li.User
		in.Context = li.Context
		if !li.Timestamp.IsZero() {
			in.Timestamp = li.Timestamp
		}
	}
	res, err := eng.Evaluate(ctx, id, in)
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}
	if g.Output == FormatJSON {
		return json.NewEncoder(out).Encode(res)
	}
	verdict := "ALLOW"
	if !res.Allowed {
		verdict = "DENY"
	}
	fmt.Fprintf(out, "%s  (policy %s, %s)\n", verdict, res.PolicyName, res.Duration)
	for _, v := range res.Violations {
		fmt.Fprintf(out, "  - [%s] %s: %s\n", v.Severity, v.Rule, v.Message)
	}
	for _, wmsg := range res.Warnings {
		fmt.Fprintf(out, "  ! warning: %s\n", wmsg)
	}
	return nil
}

// ---- validate (local) -----------------------------------------------------

func validateCmd(g *globals) *cobra.Command {
	var typeFlag string
	cmd := &cobra.Command{
		Use:   "validate <policy-file>",
		Short: "Compile-check a policy source file locally (no server)",
		Long: "Parse + compile an OPA Rego / CEL / Builtin-JSON policy file " +
			"and report syntax / config errors. Exit non-zero on a compile " +
			"error so it fits CI gates. Does not contact kscore-server.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(cmd.Context(), cmd.OutOrStdout(), g, args[0], typeFlag)
		},
	}
	cmd.Flags().StringVar(&typeFlag, "type", "", "force policy type: opa | cel | builtin")
	return cmd
}

func runValidate(ctx context.Context, out io.Writer, g *globals, policyPath, typeFlag string) error {
	eng, id, err := transientEngine(policyPath, typeFlag)
	if err != nil {
		return err
	}
	// A compile error surfaces as ErrInvalidPolicy on the first
	// Evaluate (the evaluators compile lazily). A clean compile that
	// then allows/denies on empty input is still "valid" — validate
	// only cares about compilation, not the verdict.
	_, err = eng.Evaluate(ctx, id, policy.EvaluationInput{Timestamp: time.Now().UTC()})
	if err != nil && errors.Is(err, policy.ErrInvalidPolicy) {
		return fmt.Errorf("invalid: %w", err)
	}
	fmt.Fprintln(out, "valid")
	return nil
}
