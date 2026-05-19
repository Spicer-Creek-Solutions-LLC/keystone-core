package gitops

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
	"go.keystone-core.io/keystone-core/internal/gitops/verification"
)

// formatWorkflowResult renders a [verification.WorkflowResult] as
// human-readable text or JSON per output. Text is the default for
// terminal use; json is for `--output json` machine consumption.
func formatWorkflowResult(w io.Writer, output string, wr verification.WorkflowResult) error {
	switch strings.ToLower(output) {
	case "", "text":
		verdict := "PASS"
		if !wr.Success {
			verdict = "FAIL"
		}
		fmt.Fprintf(w, "workflow=%s verdict=%s duration=%s steps=%d\n",
			wr.Name, verdict, wr.Duration, len(wr.Steps))
		for _, s := range wr.Steps {
			tag := "ok"
			switch {
			case s.Skipped:
				tag = "skip"
			case !s.Result.Success:
				if s.Optional {
					tag = "opt-fail"
				} else {
					tag = "FAIL"
				}
			}
			fmt.Fprintf(w, "  [%s] %s (%s) duration=%s retries=%d msg=%q\n",
				tag, s.Name, s.Type, s.Result.Duration, s.Result.Retries, s.Result.Message)
		}
		return nil
	case "json":
		return writeJSON(w, wr)
	default:
		return fmt.Errorf("unknown --output %q (want text|json)", output)
	}
}

// formatRollback renders a [rollback.Rollback] as text or JSON.
func formatRollback(w io.Writer, output string, rb *rollback.Rollback) error {
	switch strings.ToLower(output) {
	case "", "text":
		fmt.Fprintf(w, "id=%s state=%s executor=%s app=%s strategy=%s\n",
			rb.ID, rb.State, rb.ExecutorType, rb.Application, rb.Strategy)
		if rb.FromRevision != "" || rb.ToRevision != "" {
			fmt.Fprintf(w, "  revisions: %s -> %s\n", rb.FromRevision, rb.ToRevision)
		}
		if rb.Error != "" {
			fmt.Fprintf(w, "  error: %s\n", rb.Error)
		}
		for _, t := range rb.Transitions {
			fmt.Fprintf(w, "  %s -> %s (%s) at %s\n", t.From, t.To, t.Event, t.At.Format("15:04:05.000"))
		}
		return nil
	case "json":
		return writeJSON(w, rb)
	default:
		return fmt.Errorf("unknown --output %q (want text|json)", output)
	}
}

// formatRollbackList renders multiple records as text or JSON.
func formatRollbackList(w io.Writer, output string, list []*rollback.Rollback) error {
	switch strings.ToLower(output) {
	case "", "text":
		if len(list) == 0 {
			fmt.Fprintln(w, "(no rollbacks)")
			return nil
		}
		for _, rb := range list {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", rb.ID, rb.State, rb.ExecutorType, rb.Application)
		}
		return nil
	case "json":
		return writeJSON(w, list)
	default:
		return fmt.Errorf("unknown --output %q (want text|json)", output)
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
