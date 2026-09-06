// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
)

// parseKV turns repeated "key=value" flags into a map. A missing '='
// is an error (never a silent empty value).
func parseKV(pairs []string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		i := strings.IndexByte(p, '=')
		if i <= 0 {
			return nil, fmt.Errorf("invalid --param %q: want key=value", p)
		}
		out[p[:i]] = p[i+1:]
	}
	return out, nil
}

// localTarget reports whether a --target value addresses the local
// host (the only wired apply path in v1.0).
func localTarget(t string) bool {
	switch t {
	case "", "local", "localhost", "id:local":
		return true
	default:
		return false
	}
}

func applyCmd(d Deps) *cobra.Command {
	var params, enable, disable []string
	var as, entrypoint, target, server, apiKey string
	cmd := &cobra.Command{
		Use:   "apply [dir]",
		Short: "Apply a blueprint",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputs, err := parseKV(params)
			if err != nil {
				return err
			}
			// A targeted apply is the control plane's work: it holds
			// the converge dispatcher and the agent registry. Only a
			// local apply runs in this process.
			if !localTarget(target) {
				return runRemoteApply(cmd, d, remoteApplyArgs{
					name:       blueprintName(args),
					target:     target,
					inputs:     inputs,
					enable:     enable,
					disable:    disable,
					as:         as,
					entrypoint: entrypoint,
					server:     server,
					apiKey:     apiKey,
				})
			}
			ex, err := d.engine()
			if err != nil {
				return err
			}
			m, err := loadManifest(argDir(args))
			if err != nil {
				return err
			}
			res, err := ex.Apply(withContext(cmd), m, bp.ApplyOptions{
				Inputs:     inputs,
				Enable:     enable,
				Disable:    disable,
				As:         as,
				Entrypoint: entrypoint,
			})
			if res != nil {
				printResult(cmd, m.Metadata.Name, res)
			}
			return err
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&params, "param", nil, "parameter key=value (repeatable)")
	f.StringArrayVar(&enable, "enable", nil, "feature to enable (repeatable)")
	f.StringArrayVar(&disable, "disable", nil, "feature to disable (repeatable)")
	f.StringVar(&as, "as", "", "multi-instance namespace")
	f.StringVar(&entrypoint, "entrypoint", "", "named entrypoint (default: entrypoints.default)")
	f.StringVar(&target, "target", "",
		"apply target: empty or 'localhost' for this host, or id:<agent> / <label>:<value> / hostname:<glob> for a fleet")
	f.StringVar(&server, "server", "", "control-plane address for a targeted apply (default $KSCORE_SERVER)")
	f.StringVar(&apiKey, "api-key", "", "API key for a targeted apply (default $KSCORE_API_KEY)")
	return cmd
}

func rollbackCmd(d Deps) *cobra.Command {
	var server, apiKey string
	cmd := &cobra.Command{
		Use:   "rollback <run-id>",
		Short: "Roll back a recorded blueprint apply",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// A run applied to a fleet is recorded on the control
			// plane, so its rollback has to be asked for there --
			// this process has no record of it. --server (or
			// KSCORE_SERVER) is how the operator says which.
			if remoteRollbackWanted(d, server) {
				return runRemoteRollback(cmd, d, args[0], server, apiKey)
			}
			ex, err := d.engine()
			if err != nil {
				return err
			}
			res, err := ex.Rollback(withContext(cmd), args[0])
			if res != nil {
				printResult(cmd, "rollback", res)
			}
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&server, "server", "", "control-plane address; set to roll back a run recorded there (default $KSCORE_SERVER)")
	f.StringVar(&apiKey, "api-key", "", "API key for a control-plane rollback (default $KSCORE_API_KEY)")
	return cmd
}

func appliedCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "applied",
		Short: "List recorded blueprint applies",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ex, err := d.engine()
			if err != nil {
				return err
			}
			if ex.Store == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "no applied runs")
				return nil
			}
			runs, err := ex.Store.List(withContext(cmd))
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no applied runs")
				return nil
			}
			w := cmd.OutOrStdout()
			for _, r := range runs {
				parent := ""
				if r.ParentID != "" {
					parent = " parent=" + r.ParentID
				}
				fmt.Fprintf(w, "%s  %s@%s  %s  %s%s\n",
					r.ID, r.Blueprint, r.Version, r.Status, r.Entrypoint, parent)
			}
			return nil
		},
	}
}

func printResult(cmd *cobra.Command, name string, res *bp.ApplyResult) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s: %s (run %s)\n", name, res.Status, res.RunID)
	if res.Report != nil {
		fmt.Fprintf(w, "declarations: total=%d changed=%d failed=%d\n",
			res.Report.Total, res.Report.Changed, res.Report.Failed)
	}
	if len(res.Outputs) > 0 {
		keys := make([]string, 0, len(res.Outputs))
		for k := range res.Outputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "output %s = %v\n", k, res.Outputs[k])
		}
	}
}
