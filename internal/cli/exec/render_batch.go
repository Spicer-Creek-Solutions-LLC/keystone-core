// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// RenderBatchJob writes a single BatchJob in the requested format.
func RenderBatchJob(out io.Writer, b *v1.BatchJob, format string) error {
	if b == nil {
		return fmt.Errorf("exec: no batch")
	}
	switch format {
	case FormatJSON:
		return jsonEncode(out, batchJobMap(b))
	case FormatYAML:
		return yamlEncode(out, batchJobMap(b))
	case FormatTable, "":
		fmt.Fprintf(out, "batch: %s\n", b.Id)
		fmt.Fprintf(out, "status: %s\n", batchStatusString(b.Status))
		fmt.Fprintf(out, "command: %s\n", b.Command)
		if len(b.Args) > 0 {
			fmt.Fprintf(out, "args: %v\n", b.Args)
		}
		fmt.Fprintf(out, "agents: %d total, %d completed, %d successful, %d failed\n",
			b.TotalAgents, b.CompletedAgents, b.SuccessfulAgents, b.FailedAgents)
		fmt.Fprintf(out, "created_at: %s\n", protoTime(b.CreatedAt))
		if b.StartedAt != nil {
			fmt.Fprintf(out, "started_at: %s\n", protoTime(b.StartedAt))
		}
		if b.CompletedAt != nil {
			fmt.Fprintf(out, "completed_at: %s\n", protoTime(b.CompletedAt))
		}
		return nil
	default:
		return fmt.Errorf("exec: unknown output format %q", format)
	}
}

// RenderBatchList writes a list of BatchJobs in the requested format.
func RenderBatchList(out io.Writer, batches []*v1.BatchJob, format string) error {
	switch format {
	case FormatJSON:
		list := make([]map[string]any, 0, len(batches))
		for _, b := range batches {
			list = append(list, batchJobMap(b))
		}
		return jsonEncode(out, list)
	case FormatYAML:
		list := make([]map[string]any, 0, len(batches))
		for _, b := range batches {
			list = append(list, batchJobMap(b))
		}
		return yamlEncode(out, list)
	case FormatTable, "":
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tSTATUS\tTOTAL\tSUCCEEDED\tFAILED\tCREATED")
		for _, b := range batches {
			fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\n",
				b.Id, batchStatusString(b.Status),
				b.TotalAgents, b.SuccessfulAgents, b.FailedAgents,
				protoTime(b.CreatedAt))
		}
		return tw.Flush()
	default:
		return fmt.Errorf("exec: unknown output format %q", format)
	}
}

// OutputRenderOpts controls how RenderAgentResults emits stdout/stderr.
type OutputRenderOpts struct {
	IncludeStdout bool
	IncludeStderr bool
	// Raw writes the single agent's bytes straight to the writer with
	// no framing. Caller must verify len(results) <= 1 before setting.
	Raw    bool
	Format string
}

// RenderAgentResults writes per-agent output. Format applies only when
// Raw is false. Empty results emits nothing in table mode and an empty
// list in JSON/YAML.
func RenderAgentResults(out io.Writer, results []*v1.BatchAgentResult, opts OutputRenderOpts) error {
	if opts.Raw {
		if len(results) == 0 {
			return nil
		}
		r := results[0]
		switch {
		case opts.IncludeStderr:
			_, err := out.Write(r.GetStderr())
			return err
		default:
			_, err := out.Write(r.GetStdout())
			return err
		}
	}
	switch opts.Format {
	case FormatJSON:
		list := make([]map[string]any, 0, len(results))
		for _, r := range results {
			list = append(list, agentResultMap(r, opts))
		}
		return jsonEncode(out, list)
	case FormatYAML:
		list := make([]map[string]any, 0, len(results))
		for _, r := range results {
			list = append(list, agentResultMap(r, opts))
		}
		return yamlEncode(out, list)
	case FormatTable, "":
		for i, r := range results {
			if i > 0 {
				fmt.Fprintln(out)
			}
			if opts.IncludeStdout {
				fmt.Fprintf(out, "=== agent %s stdout (exit %d) ===\n",
					r.AgentId, r.ExitCode)
				if _, err := out.Write(r.GetStdout()); err != nil {
					return err
				}
				if !endsWithNewline(r.GetStdout()) {
					fmt.Fprintln(out)
				}
				if r.GetStdoutTruncated() {
					fmt.Fprintln(out, "[stdout truncated by agent output cap]")
				}
			}
			if opts.IncludeStderr {
				fmt.Fprintf(out, "=== agent %s stderr (exit %d) ===\n",
					r.AgentId, r.ExitCode)
				if _, err := out.Write(r.GetStderr()); err != nil {
					return err
				}
				if !endsWithNewline(r.GetStderr()) {
					fmt.Fprintln(out)
				}
				if r.GetStderrTruncated() {
					fmt.Fprintln(out, "[stderr truncated by agent output cap]")
				}
			}
			if r.Error != "" {
				fmt.Fprintf(out, "agent %s error: %s\n", r.AgentId, r.Error)
			}
		}
		return nil
	default:
		return fmt.Errorf("exec: unknown output format %q", opts.Format)
	}
}

// RenderPreviewStream consumes a dry-run BatchExecuteCommand stream
// and prints the matched agent set.
func RenderPreviewStream(out io.Writer, next func() (*v1.BatchExecuteCommandResponse, error), format string) error {
	var preview *v1.BatchPreview
	for {
		ev, err := next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("exec: stream recv: %w", err)
		}
		if p := ev.GetPreview(); p != nil {
			preview = p
		}
	}
	ids := []string{}
	if preview != nil {
		ids = preview.AgentIds
	}
	switch format {
	case FormatJSON:
		return jsonEncode(out, map[string]any{"matched_agents": ids})
	case FormatYAML:
		return yamlEncode(out, map[string]any{"matched_agents": ids})
	case FormatTable, "":
		fmt.Fprintf(out, "matched %d agent(s):\n", len(ids))
		for _, id := range ids {
			fmt.Fprintf(out, "  %s\n", id)
		}
		return nil
	default:
		return fmt.Errorf("exec: unknown output format %q", format)
	}
}

// ---- helpers --------------------------------------------------------------

func batchJobMap(b *v1.BatchJob) map[string]any {
	out := map[string]any{
		"id":                b.Id,
		"status":            batchStatusString(b.Status),
		"command":           b.Command,
		"args":              b.Args,
		"total_agents":      b.TotalAgents,
		"completed_agents":  b.CompletedAgents,
		"successful_agents": b.SuccessfulAgents,
		"failed_agents":     b.FailedAgents,
	}
	if b.CreatedAt != nil {
		out["created_at"] = b.CreatedAt.AsTime()
	}
	if b.StartedAt != nil {
		out["started_at"] = b.StartedAt.AsTime()
	}
	if b.CompletedAt != nil {
		out["completed_at"] = b.CompletedAt.AsTime()
	}
	return out
}

func agentResultMap(r *v1.BatchAgentResult, opts OutputRenderOpts) map[string]any {
	out := map[string]any{
		"agent_id":  r.AgentId,
		"success":   r.Success,
		"exit_code": r.ExitCode,
	}
	if r.Error != "" {
		out["error"] = r.Error
	}
	if opts.IncludeStdout {
		out["stdout"] = string(r.GetStdout())
		if r.GetStdoutTruncated() {
			out["stdout_truncated"] = true
		}
	}
	if opts.IncludeStderr {
		out["stderr"] = string(r.GetStderr())
		if r.GetStderrTruncated() {
			out["stderr_truncated"] = true
		}
	}
	return out
}

func protoTime(t *timestamppb.Timestamp) string {
	if t == nil {
		return ""
	}
	return t.AsTime().Format(time.RFC3339)
}

func jsonEncode(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func yamlEncode(out io.Writer, v any) error {
	enc := yaml.NewEncoder(out)
	defer func() { _ = enc.Close() }()
	enc.SetIndent(2)
	return enc.Encode(v)
}

func endsWithNewline(b []byte) bool {
	return len(b) > 0 && b[len(b)-1] == '\n'
}
