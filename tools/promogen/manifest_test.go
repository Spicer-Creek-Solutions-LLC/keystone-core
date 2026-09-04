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
defaults:
  tolerance: 0.25
  resolution:
    width: 1920
    height: 1080
reels:
  - id: promo
    title: "test reel"
    output: test-reel
    target_duration: 10.0
    square_cut: true
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
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 10.0
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
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 30.0
    shots:
      - {id: a, kind: card, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "want 30.00s",
		},
		{
			name: "duplicate id",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 4.0
    shots:
      - {id: a, kind: card, duration: 2.0, tape: tapes/a.tape}
      - {id: a, kind: card, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "duplicate id within the reel",
		},
		{
			name: "unknown kind",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 2.0
    shots:
      - {id: a, kind: montage, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: `kind must be`,
		},
		{
			name: "terminal shot without caption",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 2.0
    shots:
      - {id: a, kind: terminal, duration: 2.0, caption: "   ", tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "require a caption",
		},
		{
			name: "missing tape file",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 2.0
    shots:
      - {id: a, kind: card, duration: 2.0, tape: tapes/nope.tape}
`,
			wantFrag: "not found",
		},
		{
			name: "tape escapes the promo dir",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 2.0
    shots:
      - {id: a, kind: card, duration: 2.0, tape: ../../../etc/passwd}
`,
			wantFrag: "escapes the promo directory",
		},
		{
			name: "absolute tape path",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 2.0
    shots:
      - {id: a, kind: card, duration: 2.0, tape: /etc/passwd}
`,
			wantFrag: "escapes the promo directory",
		},
		{
			name: "no shots",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 30.0
    shots: []
`,
			wantFrag: "declares no shots",
		},
		{
			name: "missing id",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 2.0
    shots:
      - {kind: card, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "id is required",
		},
		{
			name: "bad resolution",
			body: `
defaults:
  tolerance: 0.25
  resolution: {width: 0, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 2.0
    shots:
      - {id: a, kind: card, duration: 2.0, tape: tapes/a.tape}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "resolution width and height",
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
defaults:
  tolerance: 0.25
  resolution: {width: 1920, height: 1080}
reels:
  - id: r1
    output: out1
    target_duration: 10.0
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

// --- reel-level rules -------------------------------------------------

func TestValidate_ReelRules(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		tapes    []string
		wantFrag string
	}{
		{
			name: "duplicate reel id",
			body: `
defaults: {tolerance: 0.25, resolution: {width: 1920, height: 1080}}
reels:
  - {id: r1, output: a, target_duration: 2.0, shots: [{id: s, kind: card, duration: 2.0, tape: tapes/a.tape}]}
  - {id: r1, output: b, target_duration: 2.0, shots: [{id: s, kind: card, duration: 2.0, tape: tapes/a.tape}]}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "duplicate reel id",
		},
		{
			// Two reels writing one file silently clobber each other —
			// the second render wins and the first vanishes.
			name: "duplicate output",
			body: `
defaults: {tolerance: 0.25, resolution: {width: 1920, height: 1080}}
reels:
  - {id: r1, output: same, target_duration: 2.0, shots: [{id: s, kind: card, duration: 2.0, tape: tapes/a.tape}]}
  - {id: r2, output: same, target_duration: 2.0, shots: [{id: s, kind: card, duration: 2.0, tape: tapes/a.tape}]}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "would write the same file",
		},
		{
			name: "missing output",
			body: `
defaults: {tolerance: 0.25, resolution: {width: 1920, height: 1080}}
reels:
  - {id: r1, target_duration: 2.0, shots: [{id: s, kind: card, duration: 2.0, tape: tapes/a.tape}]}
`,
			tapes:    []string{"a.tape"},
			wantFrag: "output is required",
		},
		{
			name: "no reels",
			body: `
defaults: {tolerance: 0.25, resolution: {width: 1920, height: 1080}}
reels: []
`,
			wantFrag: "declares no reels",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := LoadManifest(writeManifest(t, tt.body, tt.tapes...))
			if err != nil {
				t.Fatalf("LoadManifest: %v", err)
			}
			problems := m.Validate()
			if !strings.Contains(strings.Join(problems, "\n"), tt.wantFrag) {
				t.Errorf("Validate() = %v, want one containing %q", problems, tt.wantFrag)
			}
		})
	}
}

// Budgets are per reel, so one reel busting its budget must not be
// masked by another reel being under — the whole point of the split.
func TestValidate_BudgetsAreIndependent(t *testing.T) {
	body := `
defaults: {tolerance: 0.25, resolution: {width: 1920, height: 1080}}
reels:
  - {id: ok, output: a, target_duration: 10.0, shots: [{id: s, kind: card, duration: 10.0, tape: tapes/a.tape}]}
  - {id: over, output: b, target_duration: 10.0, shots: [{id: s, kind: card, duration: 25.0, tape: tapes/a.tape}]}
`
	m, err := LoadManifest(writeManifest(t, body, "a.tape"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	problems := strings.Join(m.Validate(), "\n")
	if !strings.Contains(problems, `reel "over"`) {
		t.Errorf("Validate() did not flag the over-budget reel: %v", problems)
	}
	if strings.Contains(problems, `reel "ok"`) {
		t.Errorf("Validate() flagged the in-budget reel: %v", problems)
	}
}

// Shot ids are unique within a reel, not globally: two reels can both
// sensibly open on a shot called "hook".
func TestValidate_ShotIDsScopedToReel(t *testing.T) {
	body := `
defaults: {tolerance: 0.25, resolution: {width: 1920, height: 1080}}
reels:
  - {id: r1, output: a, target_duration: 2.0, shots: [{id: hook, kind: card, duration: 2.0, tape: tapes/a.tape}]}
  - {id: r2, output: b, target_duration: 2.0, shots: [{id: hook, kind: card, duration: 2.0, tape: tapes/a.tape}]}
`
	m, err := LoadManifest(writeManifest(t, body, "a.tape"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if problems := m.Validate(); len(problems) != 0 {
		t.Errorf("Validate() = %v, want none — shot ids are reel-scoped", problems)
	}
}

func TestReelDefaultsInheritance(t *testing.T) {
	d := Defaults{Tolerance: 0.25, Resolution: Resolution{Width: 1920, Height: 1080}}

	inherit := Reel{}
	if got := inherit.EffectiveTolerance(d); got != 0.25 {
		t.Errorf("EffectiveTolerance() = %v, want the default 0.25", got)
	}
	if got := inherit.EffectiveResolution(d); got != d.Resolution {
		t.Errorf("EffectiveResolution() = %+v, want the default %+v", got, d.Resolution)
	}

	override := Reel{Tolerance: 1.5, Resolution: Resolution{Width: 1280, Height: 720}}
	if got := override.EffectiveTolerance(d); got != 1.5 {
		t.Errorf("EffectiveTolerance() = %v, want the override 1.5", got)
	}
	if got := override.EffectiveResolution(d); got.Width != 1280 || got.Height != 720 {
		t.Errorf("EffectiveResolution() = %+v, want the override 1280x720", got)
	}

	// A half-specified resolution fills the missing axis from defaults.
	partial := Reel{Resolution: Resolution{Width: 1080}}
	if got := partial.EffectiveResolution(d); got.Width != 1080 || got.Height != 1080 {
		t.Errorf("EffectiveResolution() = %+v, want 1080x1080", got)
	}
}

func TestManifestReelLookup(t *testing.T) {
	m, err := LoadManifest(writeManifest(t, validManifest, "a.tape", "b.tape"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if r := m.Reel("promo"); r == nil || r.Output != "test-reel" {
		t.Errorf("Reel(\"promo\") = %+v, want the promo reel", r)
	}
	if r := m.Reel("absent"); r != nil {
		t.Errorf("Reel(\"absent\") = %+v, want nil", r)
	}
	if got := len(m.AllShots()); got != 2 {
		t.Errorf("AllShots() = %d shots, want 2", got)
	}
}

// docs_page is only worth having if it is checked: without this the
// field is a comment, and a clip silently stops being shown the first
// time someone restructures a page.
func TestValidateDocsPages(t *testing.T) {
	root := t.TempDir()
	page := filepath.Join(root, "page.md")
	if err := os.WriteFile(page, []byte("intro\n{{< clip name=\"good-clip\" >}}\n"), 0o600); err != nil {
		t.Fatalf("write page: %v", err)
	}

	tests := []struct {
		name     string
		reel     Reel
		wantFrag string
	}{
		{
			name: "embedded",
			reel: Reel{ID: "a", Output: "good-clip", DocsPage: "page.md"},
		},
		{
			name:     "page exists but does not embed the clip",
			reel:     Reel{ID: "b", Output: "other-clip", DocsPage: "page.md"},
			wantFrag: "does not embed the clip",
		},
		{
			name:     "page missing",
			reel:     Reel{ID: "c", Output: "good-clip", DocsPage: "absent.md"},
			wantFrag: "not found",
		},
		{
			name: "no docs_page is fine — not every reel is embedded",
			reel: Reel{ID: "d", Output: "good-clip"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{Reels: []Reel{tt.reel}}
			problems := m.ValidateDocsPages(root)
			if tt.wantFrag == "" {
				if len(problems) != 0 {
					t.Errorf("ValidateDocsPages() = %v, want none", problems)
				}
				return
			}
			if len(problems) == 0 || !strings.Contains(problems[0], tt.wantFrag) {
				t.Errorf("ValidateDocsPages() = %v, want one containing %q", problems, tt.wantFrag)
			}
		})
	}
}

// The repo's own reels must stay embedded where they claim to be.
func TestRepoDocsPagesEmbedTheirClips(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	m, err := LoadManifest(filepath.Join(repoRoot, defaultPromoDir, manifestName))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if problems := m.ValidateDocsPages(repoRoot); len(problems) != 0 {
		t.Errorf("docs pages out of sync with the manifest:\n  %s",
			strings.Join(problems, "\n  "))
	}
}
