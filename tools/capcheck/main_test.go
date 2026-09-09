// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

const sampleCatalog = `## Catalog

### NATS messaging

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| ` + "`CAP-NATS-001`" + ` | Embedded NATS server | implemented | Future | ` + "`FEATURES.md L89`" + ` |
| ` + "`CAP-NATS-002`" + ` | Subject hierarchy | partial | v0.6 | ` + "`FEATURES.md L91`" + ` |

Scope and status notes:

- ` + "`CAP-NATS-001`" + ` — Evidence sits on internal/nats/.

Known gaps and limitations:

- ` + "`CAP-NATS-002`" + ` — Exact per-subject permissions were never configured.
`

func TestParseCatalog(t *testing.T) {
	entries, err := parseCatalog(sampleCatalog)
	if err != nil {
		t.Fatalf("parseCatalog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].ID != "CAP-NATS-001" || entries[0].Status != "implemented" || entries[0].Scope != "Future" {
		t.Errorf("first row parsed wrong: %+v", entries[0])
	}
	if entries[0].Source != "FEATURES.md L89" {
		t.Errorf("source = %q", entries[0].Source)
	}
	// Bullets attach to the entry they name, not to the table they follow.
	if len(entries[0].Notes) != 1 || len(entries[0].Gaps) != 0 {
		t.Errorf("notes/gaps misattributed on first row: %+v", entries[0])
	}
	if len(entries[1].Gaps) != 1 || len(entries[1].Notes) != 0 {
		t.Errorf("notes/gaps misattributed on second row: %+v", entries[1])
	}
}

func TestParseCatalogRejectsEmpty(t *testing.T) {
	if _, err := parseCatalog("# nothing here\n"); err == nil {
		t.Fatal("expected an error for a catalog with no rows")
	}
}

func TestParseCatalogRejectsUnknownBullet(t *testing.T) {
	md := sampleCatalog + "\nKnown gaps and limitations:\n\n- `CAP-NATS-999` — orphan bullet.\n"
	if _, err := parseCatalog(md); err == nil {
		t.Fatal("expected an error for a bullet naming an unknown entry")
	}
}

func TestCheckInvariants(t *testing.T) {
	cases := []struct {
		name  string
		entry Entry
		want  string
	}{
		{
			name:  "implemented with a gap is partial by definition",
			entry: Entry{ID: "CAP-X-001", Name: "Thing", Status: "implemented", Gaps: []string{"half of it is missing"}},
			want:  "implemented but carries a documented gap",
		},
		{
			name:  "partial must name its gap",
			entry: Entry{ID: "CAP-X-002", Name: "Thing", Status: "partial"},
			want:  "names no gap",
		},
		{
			name:  "a gap may not merely restate the entry name",
			entry: Entry{ID: "CAP-X-003", Name: "Cross-distro state stdlib docker matrix harness", Status: "partial", Gaps: []string{"Cross-distro state stdlib docker matrix harness"}},
			want:  "restates its own name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkInvariants([]Entry{tc.entry})
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("got %v, want one problem containing %q", got, tc.want)
			}
		})
	}
}

func TestCheckInvariantsAcceptsWellFormed(t *testing.T) {
	ok := []Entry{
		{ID: "CAP-X-001", Name: "Thing", Status: "implemented"},
		{ID: "CAP-X-002", Name: "Thing two", Status: "partial", Gaps: []string{"the second half never landed"}},
		{ID: "CAP-X-003", Name: "Thing three", Status: "planned"},
		{ID: "CAP-X-004", Name: "Thing four", Status: "unknown"},
		{ID: "CAP-X-005", Name: "Domain overview", Status: "reference"},
	}
	if got := checkInvariants(ok); len(got) != 0 {
		t.Fatalf("well-formed entries reported problems: %v", got)
	}
}

func TestEnumerateSources(t *testing.T) {
	// The sources are read from the pinned Generation 1 commit, so this needs
	// the real repository rather than a synthetic tree.
	got, err := enumerateSources("../..")
	if err != nil {
		t.Skipf("pinned commit unavailable (shallow clone?): %v", err)
	}
	if len(got) < 1000 {
		t.Fatalf("got %d source items, want the full Generation 1 set", len(got))
	}
	seen := map[string]bool{}
	for _, it := range got {
		if strings.Contains(it.File, "epics/20-") {
			t.Fatalf("epic 20 is Generation 2 and must be excluded, got %+v", it)
		}
		key := it.File + " " + it.Locator
		if seen[key] {
			t.Fatalf("source item enumerated twice: %s", key)
		}
		seen[key] = true
	}
	files := map[string]bool{}
	for _, it := range got {
		files[it.File] = true
	}
	for _, want := range []string{"FEATURES.md", "docs/project/ROADMAP.md", "PROJECT-DETAILS.md", "docs/project/STATE-SUPPORT-MATRIX.md"} {
		if !files[want] {
			t.Errorf("no items enumerated from %s", want)
		}
	}
}

func TestEnumerateSourcesUnknownPin(t *testing.T) {
	if _, err := enumerateSources(t.TempDir()); err == nil {
		t.Fatal("expected an error when the pinned commit cannot be read")
	}
}

func TestCheckVocabularies(t *testing.T) {
	bad := []Entry{
		{ID: "CAP-X-001", Name: "a", Status: "banana", Scope: "Future"},
		{ID: "CAP-X-002", Name: "b", Status: "planned", Scope: "v9.9"},
	}
	got := checkVocabularies(bad)
	if len(got) != 2 {
		t.Fatalf("got %v, want two problems", got)
	}
	ok := []Entry{{ID: "CAP-X-003", Name: "c", Status: "reference", Scope: "v0.6"}}
	if got := checkVocabularies(ok); len(got) != 0 {
		t.Fatalf("well-formed entries reported problems: %v", got)
	}
}

func TestCheckTotalsCatchesAFalsifiedSummary(t *testing.T) {
	entries := []Entry{
		{ID: "CAP-X-001", Status: "implemented", Scope: "v0.6"},
		{ID: "CAP-X-002", Status: "planned", Scope: "Future"},
	}
	honest := "| implemented / partial / planned / unknown | 1 / 0 / 1 / 0 |\n| `v0.6` / `Future` | 1 / 1 |"
	if got := checkTotals(honest, entries); len(got) != 0 {
		t.Fatalf("honest totals reported problems: %v", got)
	}
	// A status changed without updating the summary is structurally valid but
	// no longer adds up; that is the point of reconciling the table.
	lying := "| implemented / partial / planned / unknown | 2 / 0 / 0 / 0 |\n| `v0.6` / `Future` | 1 / 1 |"
	if got := checkTotals(lying, entries); len(got) == 0 {
		t.Fatal("expected the falsified status totals to be caught")
	}
	badScope := "| implemented / partial / planned / unknown | 1 / 0 / 1 / 0 |\n| `v0.6` / `Future` | 600 / 103 |"
	if got := checkTotals(badScope, entries); len(got) == 0 {
		t.Fatal("expected the falsified scope totals to be caught")
	}
	if got := checkTotals("no totals here", entries); len(got) != 2 {
		t.Fatalf("expected both unreadable-totals problems, got %v", got)
	}
}

func TestParseCatalogRejectsMalformedBullet(t *testing.T) {
	md := sampleCatalog + "\nKnown gaps and limitations:\n\n- `CAP-NATS-001` a gap with no em dash\n"
	if _, err := parseCatalog(md); err == nil {
		t.Fatal("expected a malformed bullet to be rejected rather than silently ignored")
	}
}
