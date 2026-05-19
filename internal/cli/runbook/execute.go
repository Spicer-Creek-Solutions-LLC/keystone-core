package runbook

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	rb "go.keystone-core.io/keystone-core/internal/runbook"
)

func executeCmd(d Deps) *cobra.Command {
	var inputs []string
	cmd := &cobra.Command{
		Use:   "execute <runbook.yaml>",
		Short: "Execute a runbook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.Executor == nil {
				return ErrEngineNotConfigured
			}
			r, err := rb.Load(args[0])
			if err != nil {
				return err
			}
			in := make(map[string]any, len(inputs))
			for _, p := range inputs {
				i := strings.IndexByte(p, '=')
				if i <= 0 {
					return fmt.Errorf("invalid --input %q: want key=value", p)
				}
				in[p[:i]] = p[i+1:]
			}
			exec, runErr := d.Executor.Execute(withContext(cmd), r, in)
			if exec != nil {
				_ = d.store().Save(withContext(cmd), exec)
				printExecution(cmd, exec)
			}
			return runErr
		},
	}
	cmd.Flags().StringArrayVar(&inputs, "input", nil, "runbook input key=value (repeatable)")
	return cmd
}

func printExecution(cmd *cobra.Command, e *rb.Execution) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "execution %s  runbook=%s  status=%s\n", e.ID, e.Runbook, e.Status)
	for _, s := range e.Steps {
		line := fmt.Sprintf("  step %-20s %s", s.Name, s.Status)
		if s.Attempts > 1 {
			line += fmt.Sprintf(" (attempts=%d)", s.Attempts)
		}
		if s.Error != nil {
			line += "  err=" + s.Error.Error()
		}
		fmt.Fprintln(w, line)
	}
	if e.Error != nil {
		fmt.Fprintf(w, "error: %v\n", e.Error)
	}
}
