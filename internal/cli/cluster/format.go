// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func validateOutput(format string) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	}
	return fmt.Errorf("cluster: invalid --output %q (want %q or %q)", format, FormatTable, FormatJSON)
}

// writeJSON renders a protobuf message as indented JSON via
// protojson (the kscore-events/policy convention).
func writeJSON(w io.Writer, msg proto.Message) error {
	opts := protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: false}
	data, err := opts.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}

// writeJSONAny renders a plain Go value as indented JSON — used by
// the local list/verify commands whose payloads aren't proto
// messages.
func writeJSONAny(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}

// statusName / roleName map the proto enums to the canonical
// lowercase names (the REST DTO convention from task 15).
func statusName(s v1.ClusterMemberStatus) string {
	switch s {
	case v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY:
		return "healthy"
	case v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_DEGRADED:
		return "degraded"
	case v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNREACHABLE:
		return "unreachable"
	case v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_LEFT:
		return "left"
	default:
		return "unspecified"
	}
}

func roleName(r v1.ClusterMemberRole) string {
	switch r {
	case v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER:
		return "leader"
	case v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_FOLLOWER:
		return "follower"
	case v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEARNER:
		return "learner"
	default:
		return "unspecified"
	}
}

func fmtTS(ts *timestamppb.Timestamp) string {
	if ts == nil || !ts.IsValid() || ts.AsTime().IsZero() {
		return "-"
	}
	return ts.AsTime().UTC().Format(time.RFC3339)
}

// table is the shared tabwriter helper (identical shape to the
// kscore-events / kscore-policy table renderer).
type table struct {
	w *tabwriter.Writer
}

func newTable(w io.Writer) *table {
	return &table{w: tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)}
}

func (t *table) header(cols ...string) { fmt.Fprintln(t.w, strings.Join(cols, "\t")) }
func (t *table) row(cells ...string)   { fmt.Fprintln(t.w, strings.Join(cells, "\t")) }
func (t *table) flush() error          { return t.w.Flush() }

func memberTable(out io.Writer, members []*v1.ClusterMember) error {
	t := newTable(out)
	t.header("ID", "NAME", "ADDRESS", "ROLE", "STATUS", "VERSION")
	for _, m := range members {
		t.row(m.GetId(), m.GetName(), m.GetAddress(),
			roleName(m.GetRole()), statusName(m.GetStatus()), m.GetVersion())
	}
	return t.flush()
}
