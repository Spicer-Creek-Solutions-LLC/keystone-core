// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"sort"
	"testing"

	"go.opentelemetry.io/otel/attribute"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/logging"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func keys(kvs []attribute.KeyValue) []string {
	out := make([]string, len(kvs))
	for i, kv := range kvs {
		out[i] = string(kv.Key)
	}
	sort.Strings(out)
	return out
}

func find(kvs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, kv := range kvs {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestAgentAttrs_Nil(t *testing.T) {
	if got := AgentAttrs(nil); got != nil {
		t.Errorf("AgentAttrs(nil) = %v, want nil", got)
	}
}

func TestAgentAttrs_HappyPath(t *testing.T) {
	rec := &state.AgentRecord{
		ID:           "a-1",
		Hostname:     "node-01",
		OS:           "linux",
		AgentVersion: "0.5.0",
		Status:       state.AgentStatusConnected,
	}
	got := AgentAttrs(rec)
	wantKeys := []string{
		AttrAgentHostname, AttrAgentID, AttrAgentOS, AttrAgentStatus, AttrAgentVersion,
	}
	sort.Strings(wantKeys)
	if gotK := keys(got); !equalStrings(gotK, wantKeys) {
		t.Errorf("keys = %v, want %v", gotK, wantKeys)
	}
	if v, _ := find(got, AttrAgentID); v.AsString() != "a-1" {
		t.Errorf("agent.id = %q, want a-1", v.AsString())
	}
}

func TestAgentAttrs_EmptyFieldsSkipped(t *testing.T) {
	rec := &state.AgentRecord{ID: "a-1"}
	got := AgentAttrs(rec)
	if want := []string{AttrAgentID}; !equalStrings(keys(got), want) {
		t.Errorf("keys = %v, want %v", keys(got), want)
	}
}

func TestAgentAttrs_AllEmpty_ReturnsNil(t *testing.T) {
	if got := AgentAttrs(&state.AgentRecord{}); got != nil {
		t.Errorf("AgentAttrs(empty) = %v, want nil", got)
	}
}

func TestJobAttrs_Nil(t *testing.T) {
	if got := JobAttrs(nil); got != nil {
		t.Errorf("JobAttrs(nil) = %v, want nil", got)
	}
}

func TestJobAttrs_Terminal_IncludesExitCode(t *testing.T) {
	rec := &state.CommandRecord{
		ID:       "c-1",
		AgentID:  "a-1",
		Command:  "exec",
		User:     "root",
		Status:   state.CommandStatusFailed,
		ExitCode: 42,
	}
	got := JobAttrs(rec)
	if v, ok := find(got, AttrCommandExitCode); !ok || v.AsInt64() != 42 {
		t.Errorf("exit_code = %v ok=%v, want 42", v.AsInt64(), ok)
	}
	if v, _ := find(got, AttrCommandStatus); v.AsString() != string(state.CommandStatusFailed) {
		t.Errorf("status = %q, want failed", v.AsString())
	}
}

func TestJobAttrs_NonTerminal_OmitsExitCode(t *testing.T) {
	rec := &state.CommandRecord{
		ID:      "c-1",
		AgentID: "a-1",
		Command: "exec",
		Status:  state.CommandStatusRunning,
	}
	got := JobAttrs(rec)
	if _, ok := find(got, AttrCommandExitCode); ok {
		t.Errorf("exit_code present for running command; want omitted")
	}
}

func TestStateAttrs_Nil(t *testing.T) {
	if got := StateAttrs(nil); got != nil {
		t.Errorf("StateAttrs(nil) = %v, want nil", got)
	}
}

func TestStateAttrs_HappyPath(t *testing.T) {
	decl := &statemgmt.Declaration{
		ID:     "file:/etc/nginx/nginx.conf",
		Module: "file",
		Name:   "/etc/nginx/nginx.conf",
		State:  "present",
	}
	got := StateAttrs(decl)
	want := []string{AttrStateDeclID, AttrStateModule, AttrStateName, AttrStateState}
	sort.Strings(want)
	if gotK := keys(got); !equalStrings(gotK, want) {
		t.Errorf("keys = %v, want %v", gotK, want)
	}
}

func TestEventAttrs_HappyPath(t *testing.T) {
	e, err := events.NewEvent(events.EventTypeAgentConnect, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	e.Severity = events.SeverityInfo
	e.Subject = "kscore.default.events.agent.agent.connect"
	got := EventAttrs(e)
	want := []string{
		AttrEventCategory, AttrEventID, AttrEventSeverity, AttrEventSource, AttrEventSubject, AttrEventType,
	}
	sort.Strings(want)
	if gotK := keys(got); !equalStrings(gotK, want) {
		t.Errorf("keys = %v, want %v", gotK, want)
	}
	if v, _ := find(got, AttrEventType); v.AsString() != string(events.EventTypeAgentConnect) {
		t.Errorf("event.type = %q, want %q", v.AsString(), events.EventTypeAgentConnect)
	}
}

func TestEventAttrs_ZeroEvent_ReturnsNil(t *testing.T) {
	if got := EventAttrs(events.Event{}); got != nil {
		t.Errorf("EventAttrs(zero) = %v, want nil", got)
	}
}

func TestEventAttrs_UnknownSeverity_Omitted(t *testing.T) {
	e := events.Event{
		ID:       "e-1",
		Type:     events.EventTypeAgentConnect,
		Source:   "src",
		Severity: events.SeverityUnknown,
	}
	got := EventAttrs(e)
	if _, ok := find(got, AttrEventSeverity); ok {
		t.Errorf("severity present despite Unknown; want omitted")
	}
}

func TestPolicyAttrs_AllowedAlwaysRecorded(t *testing.T) {
	entry := audit.AuditEntry{
		PolicyID:        "p-1",
		PolicyName:      "policy-a",
		Allowed:         false,
		Action:          "policy.evaluate",
		EnforcementMode: audit.EnforcementModeEnforce,
		ResourceType:    "secret",
	}
	got := PolicyAttrs(entry)
	if v, ok := find(got, AttrPolicyAllowed); !ok || v.AsBool() != false {
		t.Errorf("allowed = %v ok=%v, want false", v.AsBool(), ok)
	}
	if v, _ := find(got, AttrPolicyName); v.AsString() != "policy-a" {
		t.Errorf("policy.name = %q, want policy-a", v.AsString())
	}
}

func TestPolicyAttrs_ZeroEntry_StillHasAllowed(t *testing.T) {
	got := PolicyAttrs(audit.AuditEntry{})
	if v, ok := find(got, AttrPolicyAllowed); !ok || v.AsBool() != false {
		t.Errorf("allowed = %v ok=%v, want false (zero value recorded)", v.AsBool(), ok)
	}
}

func TestAttributeKeyNamespace_AllKscorePrefix(t *testing.T) {
	all := []string{
		AttrAgentID, AttrAgentHostname, AttrAgentOS, AttrAgentVersion, AttrAgentStatus,
		AttrCommandID, AttrCommandAgent, AttrCommandType, AttrCommandStatus, AttrCommandExitCode, AttrCommandUser,
		AttrStateDeclID, AttrStateModule, AttrStateName, AttrStateState,
		AttrEventID, AttrEventType, AttrEventSeverity, AttrEventSource, AttrEventSubject, AttrEventCategory,
		AttrPolicyID, AttrPolicyName, AttrPolicyAllowed, AttrPolicyAction, AttrPolicyEnforcementMode, AttrPolicyResourceType,
		AttrCorrelationID,
	}
	for _, k := range all {
		if len(k) < len("kscore.") || k[:len("kscore.")] != "kscore." {
			t.Errorf("attribute key %q lacks kscore. prefix", k)
		}
	}
}

func TestCorrelationIDAttr_PresentInCtx(t *testing.T) {
	ctx := logging.WithCorrelationID(context.Background(), "abc-123")
	got := CorrelationIDAttr(ctx)
	if len(got) != 1 {
		t.Fatalf("attr count = %d, want 1", len(got))
	}
	if got[0].Key != AttrCorrelationID {
		t.Errorf("key = %q, want %q", got[0].Key, AttrCorrelationID)
	}
	if got[0].Value.AsString() != "abc-123" {
		t.Errorf("value = %q, want abc-123", got[0].Value.AsString())
	}
}

func TestCorrelationIDAttr_EmptyCtx_ReturnsNil(t *testing.T) {
	if got := CorrelationIDAttr(context.Background()); got != nil {
		t.Errorf("attr = %v, want nil", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
