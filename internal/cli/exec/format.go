// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"go.yaml.in/yaml/v3"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// BatchStreamRenderer accumulates events from a BatchExecuteCommand
// stream and writes the final report in the requested format.
//
// Streams are consumed via the next() callback so tests can drive the
// renderer without a real gRPC client.
type BatchStreamRenderer struct {
	out      io.Writer
	format   string
	agents   map[string]*agentState // by agent_id
	order    []string               // insertion order for table output
	batchID  string
	terminal *v1.BatchTerminal
	// showOutput renders each agent's captured stdout/stderr after the
	// status table. Off by default: a fleet-wide command can produce a
	// lot of output, and the status summary is what most dispatches are
	// checked for.
	showOutput bool
}

type agentState struct {
	AgentID   string
	Status    string
	StartedAt time.Time
	EndedAt   time.Time
	Error     string
	Stdout    []byte
	Stderr    []byte
}

// NewBatchStreamRenderer returns a renderer that writes to out in the
// given format.
func NewBatchStreamRenderer(out io.Writer, format string) *BatchStreamRenderer {
	return &BatchStreamRenderer{
		out:    out,
		format: format,
		agents: map[string]*agentState{},
	}
}

// WithOutput makes the renderer include each agent's captured
// stdout/stderr. The stream already carries BatchAgentOutput chunks —
// they were simply being dropped, which forced operators through
// `exec list` + `exec output <id>` to read what a command actually
// printed.
func (r *BatchStreamRenderer) WithOutput(show bool) *BatchStreamRenderer {
	r.showOutput = show
	return r
}

// BatchID returns the batch job id observed on the stream, or "".
func (r *BatchStreamRenderer) BatchID() string { return r.batchID }

// agent returns the tracked state for id, creating it in stream order.
func (r *BatchStreamRenderer) agent(id string) *agentState {
	s, ok := r.agents[id]
	if !ok {
		s = &agentState{AgentID: id}
		r.agents[id] = s
		r.order = append(r.order, id)
	}
	return s
}

// Render pumps next() until io.EOF, accumulating events. On EOF it
// writes the terminal report. Any other error (non-EOF) is returned
// unrendered.
func (r *BatchStreamRenderer) Render(next func() (*v1.BatchExecuteCommandResponse, error)) error {
	for {
		ev, err := next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return r.flush()
			}
			return fmt.Errorf("exec: stream recv: %w", err)
		}
		r.observe(ev)
	}
}

// observe folds one event into the accumulated state.
func (r *BatchStreamRenderer) observe(ev *v1.BatchExecuteCommandResponse) {
	switch e := ev.Event.(type) {
	case *v1.BatchExecuteCommandResponse_BatchJobId:
		r.batchID = e.BatchJobId
	case *v1.BatchExecuteCommandResponse_Output:
		// Declared by the proto but not emitted by the server today —
		// per-agent output is stored and read back via
		// ListBatchAgentResults (see runDispatch's --show-output path).
		// Folded in anyway so the renderer is correct if the server
		// ever does stream it.
		if o := e.Output; o != nil && o.Output != nil {
			s := r.agent(o.AgentId)
			s.Stdout = append(s.Stdout, o.Output.Stdout...)
			s.Stderr = append(s.Stderr, o.Output.Stderr...)
		}
	case *v1.BatchExecuteCommandResponse_Lifecycle:
		l := e.Lifecycle
		s := r.agent(l.AgentId)
		switch l.Kind {
		case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_START:
			s.Status = "running"
			if l.At != nil {
				s.StartedAt = l.At.AsTime()
			}
		case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_COMPLETE:
			s.Status = "completed"
			if l.At != nil {
				s.EndedAt = l.At.AsTime()
			}
		case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_FAILED:
			s.Status = "failed"
			s.Error = l.Error
			if l.At != nil {
				s.EndedAt = l.At.AsTime()
			}
		}
	case *v1.BatchExecuteCommandResponse_Terminal:
		r.terminal = e.Terminal
	}
}

// flush writes the accumulated report.
func (r *BatchStreamRenderer) flush() error {
	switch r.format {
	case FormatJSON:
		return r.flushJSON()
	case FormatYAML:
		return r.flushYAML()
	case FormatTable, "":
		return r.flushTable()
	default:
		return fmt.Errorf("exec: unknown output format %q", r.format)
	}
}

// flushTable writes a human-readable per-agent table + summary.
func (r *BatchStreamRenderer) flushTable() error {
	if r.batchID != "" {
		fmt.Fprintf(r.out, "batch: %s\n", r.batchID)
	}
	if len(r.order) > 0 {
		tw := tabwriter.NewWriter(r.out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "AGENT\tSTATUS\tDURATION\tERROR")
		for _, id := range r.order {
			s := r.agents[id]
			dur := ""
			if !s.StartedAt.IsZero() && !s.EndedAt.IsZero() {
				dur = s.EndedAt.Sub(s.StartedAt).Round(time.Millisecond).String()
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.AgentID, s.Status, dur, s.Error)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if r.showOutput {
		r.flushAgentOutput()
	}
	if r.terminal != nil {
		fmt.Fprintf(r.out, "\nbatch status: %s\n", batchStatusString(r.terminal.Status))
	}
	return nil
}

// flushAgentOutput prints each agent's captured streams beneath the
// status table, in the same order. Matches the layout `exec output`
// already uses so the two read the same.
func (r *BatchStreamRenderer) flushAgentOutput() {
	for _, id := range r.order {
		s := r.agents[id]
		if len(s.Stdout) == 0 && len(s.Stderr) == 0 {
			continue
		}
		if len(s.Stdout) > 0 {
			fmt.Fprintf(r.out, "\n=== agent %s stdout ===\n%s", s.AgentID, ensureNewline(s.Stdout))
		}
		if len(s.Stderr) > 0 {
			fmt.Fprintf(r.out, "\n=== agent %s stderr ===\n%s", s.AgentID, ensureNewline(s.Stderr))
		}
	}
}

// ensureNewline appends a trailing newline when the captured stream
// lacks one, so the next header does not run onto the output's last
// line.
func ensureNewline(b []byte) string {
	if len(b) > 0 && b[len(b)-1] != '\n' {
		return string(b) + "\n"
	}
	return string(b)
}

// flushJSON dumps the structured report.
func (r *BatchStreamRenderer) flushJSON() error {
	enc := json.NewEncoder(r.out)
	enc.SetIndent("", "  ")
	return enc.Encode(r.report())
}

// flushYAML dumps the structured report.
func (r *BatchStreamRenderer) flushYAML() error {
	enc := yaml.NewEncoder(r.out)
	defer func() { _ = enc.Close() }()
	enc.SetIndent(2)
	return enc.Encode(r.report())
}

// report builds the json/yaml shape from accumulated state. Agents
// sorted by ID for deterministic output.
func (r *BatchStreamRenderer) report() map[string]any {
	ids := append([]string(nil), r.order...)
	sort.Strings(ids)

	agents := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		s := r.agents[id]
		entry := map[string]any{
			"agent_id": s.AgentID,
			"status":   s.Status,
		}
		if !s.StartedAt.IsZero() {
			entry["started_at"] = s.StartedAt
		}
		if !s.EndedAt.IsZero() {
			entry["ended_at"] = s.EndedAt
		}
		if s.Error != "" {
			entry["error"] = s.Error
		}
		if r.showOutput {
			entry["stdout"] = string(s.Stdout)
			entry["stderr"] = string(s.Stderr)
		}
		agents = append(agents, entry)
	}

	out := map[string]any{
		"batch_id": r.batchID,
		"agents":   agents,
	}
	if r.terminal != nil {
		out["batch_status"] = batchStatusString(r.terminal.Status)
		if r.terminal.At != nil {
			out["completed_at"] = r.terminal.At.AsTime()
		}
	}
	return out
}

// batchStatusString renders the proto enum without its prefix.
func batchStatusString(s v1.BatchJobStatus) string {
	switch s {
	case v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED:
		return "completed"
	case v1.BatchJobStatus_BATCH_JOB_STATUS_FAILED:
		return "failed"
	case v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL:
		return "partial"
	case v1.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED:
		return "cancelled"
	case v1.BatchJobStatus_BATCH_JOB_STATUS_RUNNING:
		return "running"
	case v1.BatchJobStatus_BATCH_JOB_STATUS_PENDING:
		return "pending"
	default:
		return "unspecified"
	}
}

// The single-agent ExecuteCommand RPC is not used by 11a — `run`,
// `async`, and `script` all dispatch via BatchExecuteCommand (with a
// single-agent Target when needed). A dedicated single-result renderer
// will land in a future task that uses ExecuteCommand directly.
