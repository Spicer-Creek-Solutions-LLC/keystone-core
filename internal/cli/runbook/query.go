// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"fmt"

	"github.com/spf13/cobra"
)

func statusCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status <execution-id>",
		Short: "Show a runbook execution's status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := d.store().Get(withContext(cmd), args[0])
			if err != nil {
				return err
			}
			printExecution(cmd, e)
			return nil
		},
	}
}

func listExecutionsCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list-executions",
		Short: "List recorded runbook executions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runs, err := d.store().List(withContext(cmd))
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(runs) == 0 {
				fmt.Fprintln(w, "(no executions)")
				return nil
			}
			for _, e := range runs {
				fmt.Fprintf(w, "%s  %s  %s\n", e.ID, e.Runbook, e.Status)
			}
			return nil
		},
	}
}

func auditCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "audit <execution-id>",
		Short: "Print a runbook execution's audit trail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := d.store().Get(withContext(cmd), args[0])
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "audit trail for execution %s (%s):\n", e.ID, e.Runbook)
			for _, t := range e.Trail {
				step := t.Step
				if step == "" {
					step = "(execution)"
				}
				fmt.Fprintf(w, "  %s  %-12s %s → %s  %s\n",
					t.At.Format("15:04:05.000"), step, t.From, t.To, t.Note)
			}
			return nil
		},
	}
}
