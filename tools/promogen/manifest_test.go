// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifest lays out a promo dir in a temp tree: manifest.yaml
// plus an empty tape file for each name in tapes.
func writeManifest(t *testing.T, body string, tapes ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tapes"), 0o750); err != nil {
		t.Fatalf("mkdir tapes: %v", err)
	}
	for _, name := range tapes {
		p := filepath.Join(dir, "tapes", name)
		if err := os.WriteFile(p, []byte("# tape\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	path := filepath.Join(dir, manifestName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

const validManifest = `
target_duration: 10.0
tolerance: 0.25
resolution:
  width: 1920
  height: 1080
shots:
  - id: card
    kind: card
    duration: 4.0
    caption: ""
    tape: tapes/a.tape
    feature: ""
  - id: shell
    kind: terminal
    duration: 6.0
    caption: "Declare the state."
    tape: tapes/b.tape
    feature: state-apply
`

func TestValidate_Accepts(t *testing.T) {
	m, err := LoadManifest(writeManifest(t, validManifest, "a.tape", "b.tape"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if problems := m.Validate(); len(problems) != 0 {
		t.Fatalf("Validate() = %v, want none", problems)
	}
	if got := m.TotalDuration(); got != 10.0 {
		t.Errorf("TotalDuration() = %v, want 10", got)
	}
	if got := m.Features(); len(got) != 1 || got[0] != "state-apply" {
		t.Errorf("Features() = %v, want [state-apply]", got)
	}
}

func TestValidate_Rejects(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		tapes    []string
		wantFrag string
	}{
		{
			name: "over budget",
			body: `
target_duration: 10.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: card, duration: 9.0, tape: tapes/a.tape}
  - {id: b, kind: card, duration: 9.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "want 10.00s",
		},
		{
			name: "under budget",
			body: `
target_duration: 30.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: card, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "want 30.00s",
		},
		{
			name: "duplicate id",
			body: `
target_duration: 4.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: card, duration: 2.0, tape: tapes/a.tape}
  - {id: a, kind: card, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "duplicate id",
		},
		{
			name: "unknown kind",
			body: `
target_duration: 2.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: montage, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: `kind must be`,
		},
		{
			name: "terminal shot without caption",
			body: `
target_duration: 2.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: terminal, duration: 2.0, caption: "   ", tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "require a caption",
		},
		{
			name: "missing tape file",
			body: `
target_duration: 2.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: card, duration: 2.0, tape: tapes/nope.tape}
`,
			wantFrag: "not found",
		},
		{
			name: "tape escapes the promo dir",
			body: `
target_duration: 2.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: card, duration: 2.0, tape: ../../../etc/passwd}
`,
			wantFrag: "escapes the promo directory",
		},
		{
			name: "absolute tape path",
			body: `
target_duration: 2.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: card, duration: 2.0, tape: /etc/passwd}
`,
			wantFrag: "escapes the promo directory",
		},
		{
			name: "no shots",
			body: `
target_duration: 30.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots: []
`,
			wantFrag: "no shots",
		},
		{
			name: "missing id",
			body: `
target_duration: 2.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {kind: card, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "id is required",
		},
		{
			name: "bad resolution",
			body: `
target_duration: 2.0
tolerance: 0.25
resolution: {width: 0, height: 1080}
shots:
  - {id: a, kind: card, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "resolution.width",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := LoadManifest(writeManifest(t, tt.body, tt.tapes...))
			if err != nil {
				t.Fatalf("LoadManifest: %v", err)
			}
			problems := m.Validate()
			if len(problems) == 0 {
				t.Fatalf("Validate() = none, want a problem containing %q", tt.wantFrag)
			}
			if !strings.Contains(strings.Join(problems, "\n"), tt.wantFrag) {
				t.Errorf("Validate() = %v, want one containing %q", problems, tt.wantFrag)
			}
		})
	}
}

// The budget is the reason the manifest is machine-checked at all, so
// pin the tolerance boundary explicitly.
func TestValidate_ToleranceBoundary(t *testing.T) {
	tests := []struct {
		name     string
		duration string
		wantOK   bool
	}{
		{"exactly at tolerance over", "10.25", true},
		{"just past tolerance over", "10.30", false},
		{"exactly at tolerance under", "9.75", true},
		{"just past tolerance under", "9.70", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `
target_duration: 10.0
tolerance: 0.25
resolution: {width: 1920, height: 1080}
shots:
  - {id: a, kind: card, duration: ` + tt.duration + `, tape: tapes/a.tape}
`
			m, err := LoadManifest(writeManifest(t, body, "a.tape"))
			if err != nil {
				t.Fatalf("LoadManifest: %v", err)
			}
			gotOK := len(m.Validate()) == 0
			if gotOK != tt.wantOK {
				t.Errorf("Validate() ok = %v, want %v (problems: %v)", gotOK, tt.wantOK, m.Validate())
			}
		})
	}
}

func TestLoadManifest_Errors(t *testing.T) {
	if _, err := LoadManifest(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Error("LoadManifest(missing) = nil error, want error")
	}
	bad := filepath.Join(t.TempDir(), manifestName)
	if err := os.WriteFile(bad, []byte("shots: [oops\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadManifest(bad); err == nil {
		t.Error("LoadManifest(malformed) = nil error, want error")
	}
}

// The repo's real manifest must always be valid — this is the test
// that fails when someone edits the shot list past its budget.
func TestRepoManifestIsValid(t *testing.T) {
	m, err := LoadManifest(filepath.Join("..", "..", defaultPromoDir, manifestName))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if problems := m.Validate(); len(problems) != 0 {
		t.Errorf("assets/promo/manifest.yaml is invalid:\n  %s",
			strings.Join(problems, "\n  "))
	}
}

func TestResolveTape(t *testing.T) {
	m := &Manifest{dir: filepath.Join("assets", "promo")}
	tests := []struct {
		tape    string
		wantOK  bool
		wantRel string
	}{
		{"tapes/a.tape", true, filepath.Join("assets", "promo", "tapes", "a.tape")},
		{"./tapes/a.tape", true, filepath.Join("assets", "promo", "tapes", "a.tape")},
		{"tapes/../tapes/a.tape", true, filepath.Join("assets", "promo", "tapes", "a.tape")},
		{"../secrets.env", false, ""},
		{"tapes/../../escape", false, ""},
		{"/etc/passwd", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.tape, func(t *testing.T) {
			got, ok := m.resolveTape(tt.tape)
			if ok != tt.wantOK {
				t.Fatalf("resolveTape(%q) ok = %v, want %v", tt.tape, ok, tt.wantOK)
			}
			if ok && got != tt.wantRel {
				t.Errorf("resolveTape(%q) = %q, want %q", tt.tape, got, tt.wantRel)
			}
		})
	}
}
