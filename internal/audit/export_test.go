// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

func sampleEntries() []audit.AuditEntry {
	t0 := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	return []audit.AuditEntry{
		{
			ID: "e1", Timestamp: t0, PolicyID: "p-a", PolicyName: "Pa",
			PolicyType: audit.PolicyTypeBuiltin, ResourceType: "secret",
			Allowed: true, Duration: 25 * time.Millisecond,
			EnforcementMode: audit.EnforcementModeAudit, Severity: audit.SeverityLow,
			User: "alice", Action: "policy.evaluate",
			Metadata: map[string]string{"region": "us-east", "token": "password=hunter2"},
		},
		{
			ID: "e2", Timestamp: t0.Add(time.Minute), PolicyID: "p-b", PolicyName: "Pb",
			PolicyType: audit.PolicyTypeOPA, ResourceType: "lease",
			Allowed: false, Duration: 5 * time.Millisecond,
			EnforcementMode: audit.EnforcementModeAudit, Severity: audit.SeverityHigh,
			User: "bob", Action: "policy.evaluate",
			Violations: []audit.Violation{
				{Rule: "r1", Message: "denied because password=secret123", Severity: audit.SeverityHigh},
			},
		},
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"json", "JSONL", " csv "} {
		if _, err := audit.ParseFormat(ok); err != nil {
			t.Errorf("ParseFormat(%q) err: %v", ok, err)
		}
	}
	if _, err := audit.ParseFormat("yaml"); err == nil {
		t.Errorf("ParseFormat(yaml) should error")
	}
	if _, err := audit.NewExporter(&bytes.Buffer{}, audit.Format("xml"), nil); err == nil {
		t.Errorf("NewExporter bad format should error")
	}
}

func TestExport_JSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := audit.Export(&buf, audit.FormatJSON, nil, sampleEntries()); err != nil {
		t.Fatalf("Export: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not a valid JSON array: %v\n%s", err, buf.String())
	}
	if len(got) != 2 || got[0]["id"] != "e1" || got[1]["id"] != "e2" {
		t.Errorf("json entries = %+v", got)
	}
	// Enum fields are canonical lowercase strings.
	if got[1]["severity"] != "high" || got[0]["policy_type"] != "builtin" {
		t.Errorf("enum encoding wrong: %+v", got[1])
	}
}

func TestExport_JSON_Empty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := audit.Export(&buf, audit.FormatJSON, nil, nil); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Errorf("empty JSON export = %q, want []", buf.String())
	}
}

func TestExport_JSONL(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := audit.Export(&buf, audit.FormatJSONL, nil, sampleEntries()); err != nil {
		t.Fatalf("Export: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("jsonl lines = %d, want 2\n%s", len(lines), buf.String())
	}
	for i, ln := range lines {
		var o map[string]any
		if err := json.Unmarshal([]byte(ln), &o); err != nil {
			t.Errorf("line %d not valid JSON: %v\n%s", i, err, ln)
		}
	}
}

func TestExport_JSONL_Empty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := audit.Export(&buf, audit.FormatJSONL, nil, nil); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty JSONL export = %q, want empty", buf.String())
	}
}

func TestExport_CSV(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := audit.Export(&buf, audit.FormatCSV, nil, sampleEntries()); err != nil {
		t.Fatalf("Export: %v", err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("csv rows = %d (header+2), want 3", len(rows))
	}
	wantHeader := []string{
		"id", "timestamp", "policy_id", "policy_name", "policy_type",
		"resource_type", "allowed", "duration_ns", "enforcement_mode",
		"severity", "user", "action", "violations_count", "metadata",
	}
	if strings.Join(rows[0], ",") != strings.Join(wantHeader, ",") {
		t.Errorf("csv header = %v", rows[0])
	}
	// e1: allowed=true, no violations, metadata json with 2 keys.
	r1 := rows[1]
	if r1[0] != "e1" || r1[6] != "true" || r1[12] != "0" {
		t.Errorf("e1 row = %v", r1)
	}
	var md map[string]string
	if err := json.Unmarshal([]byte(r1[13]), &md); err != nil || md["region"] != "us-east" {
		t.Errorf("e1 metadata cell = %q (%v)", r1[13], err)
	}
	// e2: allowed=false, violations_count=1, metadata empty {}.
	r2 := rows[2]
	if r2[6] != "false" || r2[12] != "1" || r2[13] != "{}" {
		t.Errorf("e2 row = %v", r2)
	}
}

func TestExport_RedactionApplied(t *testing.T) {
	t.Parallel()
	rc, err := audit.NewRedactionConfig(audit.RedactionConfigInput{
		RedactMetadataKeys: []string{"token"},
		RedactPatterns:     []string{`password=\S+`},
		RedactUser:         true,
	})
	if err != nil {
		t.Fatalf("NewRedactionConfig: %v", err)
	}
	var buf bytes.Buffer
	if err := audit.Export(&buf, audit.FormatJSONL, rc, sampleEntries()); err != nil {
		t.Fatalf("Export: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "hunter2") || strings.Contains(out, "secret123") {
		t.Errorf("redaction did not strip the password pattern:\n%s", out)
	}
	if strings.Contains(out, `"token"`) {
		t.Errorf("redaction did not drop the token metadata key:\n%s", out)
	}
	if strings.Contains(out, "alice") || strings.Contains(out, "bob") {
		t.Errorf("RedactUser did not blank the user field:\n%s", out)
	}
	// Non-redacted data still present.
	if !strings.Contains(out, "us-east") {
		t.Errorf("non-redacted metadata lost:\n%s", out)
	}
}

func TestExport_NilRedactionPassthrough(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := audit.Export(&buf, audit.FormatJSONL, nil, sampleEntries()); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(buf.String(), "hunter2") || !strings.Contains(buf.String(), "alice") {
		t.Errorf("nil redaction should pass data through unchanged:\n%s", buf.String())
	}
}

func TestExporter_StreamingLifecycle(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp, err := audit.NewExporter(&buf, audit.FormatJSON, nil)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	if err := exp.Begin(); err != nil {
		t.Fatal(err)
	}
	for _, e := range sampleEntries() {
		if err := exp.WriteEntry(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := exp.End(); err != nil {
		t.Fatal(err)
	}
	// End is idempotent.
	if err := exp.End(); err != nil {
		t.Errorf("second End: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("streamed JSON invalid: %v\n%s", err, buf.String())
	}
	if len(got) != 2 {
		t.Errorf("streamed entries = %d", len(got))
	}
}
