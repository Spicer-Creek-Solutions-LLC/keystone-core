// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// scriptedEvents returns a next() callback that yields each element
// of the slice in order, then io.EOF.
func scriptedEvents[T any](events []*T) func() (*T, error) {
	i := 0
	return func() (*T, error) {
		if i >= len(events) {
			return nil, io.EOF
		}
		ev := events[i]
		i++
		return ev, nil
	}
}

func sampleBatchStream() []*v1.BatchExecuteCommandResponse {
	t0 := time.Date(2026, 5, 10, 14, 0, 0, 0, time.UTC)
	return []*v1.BatchExecuteCommandResponse{
		{Event: &v1.BatchExecuteCommandResponse_BatchJobId{BatchJobId: "batch-1"}},
		{Event: &v1.BatchExecuteCommandResponse_Lifecycle{Lifecycle: &v1.BatchAgentLifecycle{
			AgentId: "web-1",
			Kind:    v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_START,
			At:      timestamppb.New(t0),
		}}},
		{Event: &v1.BatchExecuteCommandResponse_Lifecycle{Lifecycle: &v1.BatchAgentLifecycle{
			AgentId: "db-1",
			Kind:    v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_START,
			At:      timestamppb.New(t0),
		}}},
		{Event: &v1.BatchExecuteCommandResponse_Lifecycle{Lifecycle: &v1.BatchAgentLifecycle{
			AgentId: "web-1",
			Kind:    v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_COMPLETE,
			At:      timestamppb.New(t0.Add(time.Second)),
		}}},
		{Event: &v1.BatchExecuteCommandResponse_Lifecycle{Lifecycle: &v1.BatchAgentLifecycle{
			AgentId: "db-1",
			Kind:    v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_FAILED,
			At:      timestamppb.New(t0.Add(2 * time.Second)),
			Error:   "kaboom",
		}}},
		{Event: &v1.BatchExecuteCommandResponse_Terminal{Terminal: &v1.BatchTerminal{
			Status: v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL,
			At:     timestamppb.New(t0.Add(3 * time.Second)),
		}}},
	}
}

func TestBatchStreamRenderer_Table(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := NewBatchStreamRenderer(&buf, FormatTable)
	if err := r.Render(scriptedEvents(sampleBatchStream())); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"batch: batch-1", "web-1", "completed", "db-1", "failed", "kaboom", "partial"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestBatchStreamRenderer_JSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := NewBatchStreamRenderer(&buf, FormatJSON)
	if err := r.Render(scriptedEvents(sampleBatchStream())); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`"batch_id": "batch-1"`, `"agent_id": "web-1"`, `"agent_id": "db-1"`, `"batch_status": "partial"`, `"error": "kaboom"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestBatchStreamRenderer_YAML(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := NewBatchStreamRenderer(&buf, FormatYAML)
	if err := r.Render(scriptedEvents(sampleBatchStream())); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"batch_id: batch-1", "agent_id: web-1", "batch_status: partial"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestBatchStreamRenderer_UnknownFormat(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := NewBatchStreamRenderer(&buf, "csv")
	err := r.Render(scriptedEvents(sampleBatchStream()))
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("err = %v, want unknown-format error", err)
	}
}

func TestBatchStreamRenderer_StreamError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := NewBatchStreamRenderer(&buf, FormatTable)
	errSentinel := errors.New("broken pipe")
	err := r.Render(func() (*v1.BatchExecuteCommandResponse, error) { return nil, errSentinel })
	if err == nil || !strings.Contains(err.Error(), "stream recv") {
		t.Errorf("err = %v, want stream recv error", err)
	}
}

func TestBatchStatusString(t *testing.T) {
	t.Parallel()
	cases := map[v1.BatchJobStatus]string{
		v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED: "completed",
		v1.BatchJobStatus_BATCH_JOB_STATUS_FAILED:    "failed",
		v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL:   "partial",
		v1.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED: "cancelled",
		v1.BatchJobStatus_BATCH_JOB_STATUS_RUNNING:   "running",
		v1.BatchJobStatus_BATCH_JOB_STATUS_PENDING:   "pending",
	}
	for k, want := range cases {
		if got := batchStatusString(k); got != want {
			t.Errorf("%v → %q, want %q", k, got, want)
		}
	}
}
