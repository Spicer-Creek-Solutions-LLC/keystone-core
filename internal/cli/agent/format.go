// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func validateOutput(format string) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	}
	return fmt.Errorf("agent: invalid --output %q (want %q or %q)", format, FormatTable, FormatJSON)
}

// renderAgentList writes the agents to w in the chosen format.
func renderAgentList(w io.Writer, resp *v1.ListAgentsResponse, format string) error {
	switch format {
	case FormatJSON:
		opts := protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: false}
		data, err := opts.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		_, err = io.WriteString(w, "\n")
		return err
	case FormatTable:
		return writeTable(w, resp.GetAgents())
	}
	return fmt.Errorf("agent: unsupported --output %q", format)
}

func writeTable(w io.Writer, agents []*v1.Agent) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tSTATUS\tHOSTNAME\tOS\tLAST-HEARTBEAT\tLABELS"); err != nil {
		return err
	}
	for _, a := range agents {
		hb := "-"
		if t := a.GetLastHeartbeatAt(); t != nil {
			hb = t.AsTime().UTC().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			a.GetId(), statusName(a.GetStatus()), a.GetHostname(), a.GetOs(), hb, formatLabels(a.GetLabels()),
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// statusName maps the AgentStatus enum to canonical lowercase names
// (matches the kscore-cluster + REST DTO convention).
func statusName(s v1.AgentStatus) string {
	switch s {
	case v1.AgentStatus_AGENT_STATUS_PENDING:
		return "pending"
	case v1.AgentStatus_AGENT_STATUS_CONNECTED:
		return "connected"
	case v1.AgentStatus_AGENT_STATUS_STALE:
		return "stale"
	case v1.AgentStatus_AGENT_STATUS_DISABLED:
		return "disabled"
	default:
		return "unspecified"
	}
}

// formatLabels renders a label map as comma-separated key=value
// pairs in deterministic key order. Empty map renders as "-".
func formatLabels(m map[string]string) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + m[k]
	}
	return strings.Join(parts, ",")
}
