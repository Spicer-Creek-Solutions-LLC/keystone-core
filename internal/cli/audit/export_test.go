// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	intaudit "go.keystone-core.io/keystone-core/internal/audit"
)

func (r *rig) seedMeta(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := r.audit.Store(ctx, intaudit.MustNewAuditEntry(intaudit.AuditEntryInput{
		PolicyID: "p-a", Action: "policy.evaluate", User: "alice",
		Allowed: true, Severity: intaudit.SeverityLow,
		Metadata: map[string]string{"region": "us-east", "token": "password=hunter2"},
	})); err != nil {
		t.Fatalf("seedMeta: %v", err)
	}
}

func TestExport_JSONL(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seed(t)
	out, err := run(t, r.deps(), "export", "--format", "jsonl")
	if err != nil {
		t.Fatalf("export jsonl: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("jsonl lines = %d, want 3\n%s", len(lines), out)
	}
	for i, ln := range lines {
		var o map[string]any
		if jerr := json.Unmarshal([]byte(ln), &o); jerr != nil {
			t.Errorf("line %d not valid JSON: %v\n%s", i, jerr, ln)
		}
	}
}

func TestExport_JSON(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seed(t)
	out, err := run(t, r.deps(), "export", "--format", "json")
	if err != nil {
		t.Fatalf("export json: %v\n%s", err, out)
	}
	var arr []map[string]any
	if jerr := json.Unmarshal([]byte(out), &arr); jerr != nil {
		t.Fatalf("not a JSON array: %v\n%s", jerr, out)
	}
	if len(arr) != 3 {
		t.Errorf("json entries = %d, want 3", len(arr))
	}
}

func TestExport_CSV(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seedMeta(t)
	out, err := run(t, r.deps(), "export", "--format", "csv")
	if err != nil {
		t.Fatalf("export csv: %v\n%s", err, out)
	}
	rows, rerr := csv.NewReader(strings.NewReader(out)).ReadAll()
	if rerr != nil {
		t.Fatalf("invalid CSV: %v\n%s", rerr, out)
	}
	if len(rows) != 2 || rows[0][0] != "id" || rows[0][13] != "metadata" {
		t.Fatalf("csv header/rows wrong: %v", rows)
	}
	var md map[string]string
	if jerr := json.Unmarshal([]byte(rows[1][13]), &md); jerr != nil || md["region"] != "us-east" {
		t.Errorf("csv metadata cell = %q (%v)", rows[1][13], jerr)
	}
}

func TestExport_RedactionFlags(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seedMeta(t)
	out, err := run(t, r.deps(), "export", "--format", "jsonl",
		"--redact-key", "token",
		"--redact-pattern", `password=\S+`,
		"--redact-user")
	if err != nil {
		t.Fatalf("export redacted: %v\n%s", err, out)
	}
	if strings.Contains(out, "hunter2") || strings.Contains(out, `"token"`) {
		t.Errorf("redaction did not strip token/pattern:\n%s", out)
	}
	if strings.Contains(out, "alice") {
		t.Errorf("--redact-user did not blank user:\n%s", out)
	}
	if !strings.Contains(out, "us-east") {
		t.Errorf("non-redacted metadata lost:\n%s", out)
	}
}

func TestExport_OutputFile(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seed(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.jsonl")
	out, err := run(t, r.deps(), "export", "--format", "jsonl", "--output-file", path)
	if err != nil {
		t.Fatalf("export --output-file: %v\n%s", err, out)
	}
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatalf("read output file: %v", rerr)
	}
	if strings.Count(strings.TrimRight(string(b), "\n"), "\n")+1 != 3 {
		t.Errorf("output file lines wrong:\n%s", string(b))
	}
}

func TestExport_FilterPassthrough(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seedMeta(t) // user alice, policy p-a
	r.seed(t)     // 3 entries user alice policy p
	out, err := run(t, r.deps(), "export", "--format", "jsonl", "--policy-id", "p-a")
	if err != nil {
		t.Fatalf("export filtered: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("policy-id filter should yield 1 entry, got %d\n%s", len(lines), out)
	}
}

func TestExport_BadFormat(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := run(t, r.deps(), "export", "--format", "yaml"); err == nil {
		t.Errorf("bad --format should error")
	}
}

func TestExport_RejectsOutputFlag(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := run(t, r.deps(), "export", "--format", "jsonl", "-o", "json"); err == nil {
		t.Errorf("export with non-default -o should error")
	}
}
