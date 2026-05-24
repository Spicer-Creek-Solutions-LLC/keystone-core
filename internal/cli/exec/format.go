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
	out     io.Writer
	format  string
	agents  map[string]*agentState // by agent_id
	order   []string               // insertion order for table output
	batchID string
	terminal *v1.BatchTerminal
}

type agentState struct {
	AgentID    string
	Status     string
	StartedAt  time.Time
	EndedAt    time.Time
	Error      string
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
	case *v1.BatchExecuteCommandResponse_Lifecycle:
		l := e.Lifecycle
		s, ok := r.agents[l.AgentId]
		if !ok {
			s = &agentState{AgentID: l.AgentId}
			r.agents[l.AgentId] = s
			r.order = append(r.order, l.AgentId)
		}
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
	if r.terminal != nil {
		fmt.Fprintf(r.out, "\nbatch status: %s\n", batchStatusString(r.terminal.Status))
	}
	return nil
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
