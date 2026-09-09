// SPDX-License-Identifier: Apache-2.0

// Command capcheck validates the Generation 1 capability catalog produced by
// reboot task R03 against the archived sources it claims to cover.
//
// It does not regenerate the catalog. R03's generator ran once; what needs to
// survive is the ability to prove the catalog's claims still hold — so that a
// later hand edit, a source document gaining a heading, or a mistaken merge is
// caught rather than silently weakening the archive record that R05 and R09
// build on.
//
// Checks, in order of what they protect:
//
//   - every item in the archived sources is present in the coverage map;
//   - the coverage map invents nothing that is not in those sources;
//   - every coverage entry names a catalog entry that exists;
//   - no catalog entry is orphaned, and no identifier is used twice;
//   - every entry's Source locator still resolves to a line in a real file; and
//   - the catalog's own status taxonomy is self-consistent.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	catalogPath  = "docs/project/FUTURE-CAPABILITIES.md"
	coveragePath = "docs/transition/capability-coverage.json"

	// generationOneFinalSHA is the archive target recorded in
	// docs/transition/manifest.json. The catalog covers the Generation 1
	// sources as they stood at that commit, so the check reads them from
	// there rather than from the working tree. Later transition tasks add
	// documents and changelog fragments of their own; those are Generation 2
	// and are not Generation 1 evidence.
	generationOneFinalSHA = "93eb147f7fcc559d31f2cce77d81e791f01673f8"
)

// Entry is one row of the catalog, plus the gap and note bullets that follow
// its domain table.
type Entry struct {
	ID     string
	Name   string
	Status string
	Scope  string
	Source string
	Gaps   []string
	Notes  []string
}

// SourceItem is one heading, checklist item, capability bullet, roadmap entry
// or changelog fragment in the archived Generation 1 sources.
type SourceItem struct {
	File    string
	Locator string
}

type coverage struct {
	SourceItemCount   int `json:"source_item_count"`
	CatalogEntryCount int `json:"catalog_entry_count"`
	Sources           map[string]struct {
		File    string `json:"file"`
		Locator string `json:"locator"`
		Entry   string `json:"entry"`
		Also    []struct {
			Entry string `json:"entry"`
		} `json:"also_entries"`
	} `json:"sources"`
}

var (
	rowRe    = regexp.MustCompile(`^\| ` + "`" + `(CAP-[A-Z]+-\d+)` + "`" + ` \| (.*?) \| (\S+) \| (\S+) \| ` + "`" + `(.*?)` + "`" + ` \|$`)
	bulletRe = regexp.MustCompile(`^- ` + "`" + `(CAP-[A-Z]+-\d+)` + "`" + ` — (.*)$`)
	lineRe   = regexp.MustCompile(`^(.*?) L(\d+)$`)
)

// parseCatalog reads the rendered catalog into entries. Gap and note bullets
// are attributed to the entry they name, not to the table they follow, so a
// reordered document does not change the result.
func parseCatalog(md string) ([]Entry, error) {
	var entries []Entry
	idx := map[string]int{}
	section := ""
	for _, line := range strings.Split(md, "\n") {
		switch {
		case strings.HasPrefix(line, "Known gaps and limitations:"):
			section = "gap"
			continue
		case strings.HasPrefix(line, "Scope and status notes:"):
			section = "note"
			continue
		case strings.HasPrefix(line, "#") || strings.HasPrefix(line, "| ID |"):
			section = ""
		}
		if m := rowRe.FindStringSubmatch(line); m != nil {
			entries = append(entries, Entry{ID: m[1], Name: m[2], Status: m[3], Scope: m[4], Source: m[5]})
			idx[m[1]] = len(entries) - 1
			continue
		}
		if m := bulletRe.FindStringSubmatch(line); m != nil && section != "" {
			i, ok := idx[m[1]]
			if !ok {
				return nil, fmt.Errorf("bullet names unknown entry %s", m[1])
			}
			if section == "gap" {
				entries[i].Gaps = append(entries[i].Gaps, m[2])
			} else {
				entries[i].Notes = append(entries[i].Notes, m[2])
			}
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no catalog rows found in %s", catalogPath)
	}
	return entries, nil
}

// enumerateSources re-derives the archived source items from the raw files,
// using the same patterns R03 used. It deliberately re-reads the documents
// rather than trusting the coverage map, which is the point of the check.
func enumerateSources(root string) ([]SourceItem, error) {
	var out []SourceItem
	add := func(file string, n int) { out = append(out, SourceItem{File: file, Locator: "L" + strconv.Itoa(n)}) }

	scan := func(file string, match func(string) bool) error {
		b, err := showAtPin(root, file)
		if err != nil {
			return err
		}
		for n, line := range strings.Split(string(b), "\n") {
			if match(line) {
				add(file, n+1)
			}
		}
		return nil
	}
	hasPrefix := func(p string) func(string) bool {
		return func(s string) bool { return strings.HasPrefix(s, p) }
	}
	checkbox := regexp.MustCompile(`^\s*[-*] \[[ x]\] `)

	if err := scan("FEATURES.md", hasPrefix("- **")); err != nil {
		return nil, err
	}
	if err := scan("docs/project/ROADMAP.md", hasPrefix("#### ")); err != nil {
		return nil, err
	}
	if err := scan("PROJECT-DETAILS.md", hasPrefix("### ")); err != nil {
		return nil, err
	}
	if err := scan("docs/project/STATE-SUPPORT-MATRIX.md", hasPrefix("### ")); err != nil {
		return nil, err
	}

	epics, err := lsAtPin(root, "epics")
	if err != nil {
		return nil, err
	}
	for _, p := range epics {
		// epics/20 is the Generation 2 epic, not Generation 1 evidence.
		if !strings.HasSuffix(p, ".md") || strings.HasPrefix(filepath.Base(p), "20-") {
			continue
		}
		if err := scan(p, checkbox.MatchString); err != nil {
			return nil, err
		}
	}

	runbooks, err := lsAtPin(root, "docs/runbooks")
	if err != nil {
		return nil, err
	}
	for _, p := range runbooks {
		if !strings.HasSuffix(p, ".md") {
			continue
		}
		if err := scan(p, hasPrefix("## ")); err != nil {
			return nil, err
		}
	}

	frags, err := lsAtPin(root, ".changes/unreleased")
	if err != nil {
		return nil, err
	}
	for _, p := range frags {
		if strings.HasSuffix(p, ".yaml") {
			out = append(out, SourceItem{File: p, Locator: "-"})
		}
	}
	return out, nil
}

// showAtPin reads a file as it stood at the pinned Generation 1 commit.
func showAtPin(root, file string) ([]byte, error) {
	// #nosec G204 -- generationOneFinalSHA is a compile-time constant and file
	// is one of this tool's own fixed source paths or a name listed by
	// git ls-tree at that commit; neither is caller-supplied.
	cmd := exec.Command("git", "show", generationOneFinalSHA+":"+file)
	cmd.Dir = root
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", generationOneFinalSHA[:9], file, err)
	}
	return b, nil
}

// lsAtPin lists the files under dir as they stood at the pinned commit.
func lsAtPin(root, dir string) ([]string, error) {
	// #nosec G204 -- generationOneFinalSHA is a compile-time constant and dir is
	// one of three literals in this file.
	cmd := exec.Command("git", "ls-tree", "--name-only", generationOneFinalSHA, dir+"/")
	cmd.Dir = root
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s %s: %w", generationOneFinalSHA[:9], dir, err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out, nil
}

// checkInvariants enforces the taxonomy the catalog documents about itself.
func checkInvariants(entries []Entry) []string {
	var problems []string
	norm := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	for _, e := range entries {
		if e.Status == "implemented" && len(e.Gaps) > 0 {
			problems = append(problems, fmt.Sprintf("%s is implemented but carries a documented gap; that is partial by definition", e.ID))
		}
		if e.Status == "partial" && len(e.Gaps) == 0 {
			problems = append(problems, fmt.Sprintf("%s is partial but names no gap", e.ID))
		}
		for _, g := range e.Gaps {
			a, b := norm(g), norm(e.Name)
			if a == b || (len(b) >= 45 && strings.HasPrefix(a, b[:45])) {
				problems = append(problems, fmt.Sprintf("%s has a gap bullet that merely restates its own name", e.ID))
				break
			}
		}
	}
	return problems
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	// #nosec G304 G703 -- capcheck is a developer tool run from a checkout;
	// root is an optional argv path and catalogPath is a constant.
	md, err := os.ReadFile(filepath.Join(root, catalogPath))
	if err != nil {
		fail("read catalog: %v", err)
	}
	entries, err := parseCatalog(string(md))
	if err != nil {
		fail("parse catalog: %v", err)
	}

	// #nosec G304 G703 -- as above; coveragePath is a constant.
	raw, err := os.ReadFile(filepath.Join(root, coveragePath))
	if err != nil {
		fail("read coverage: %v", err)
	}
	var cov coverage
	if err := json.Unmarshal(raw, &cov); err != nil {
		fail("parse coverage: %v", err)
	}

	var problems []string
	add := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	ids := map[string]bool{}
	for _, e := range entries {
		if ids[e.ID] {
			add("duplicate catalog identifier %s", e.ID)
		}
		ids[e.ID] = true
	}

	// Coverage completeness, both directions.
	want, err := enumerateSources(root)
	if err != nil {
		fail("enumerate sources: %v", err)
	}
	have := map[SourceItem]bool{}
	referenced := map[string]bool{}
	for _, s := range cov.Sources {
		have[SourceItem{File: s.File, Locator: s.Locator}] = true
		if !ids[s.Entry] {
			add("coverage names entry %s, which is not in the catalog", s.Entry)
		}
		referenced[s.Entry] = true
		for _, a := range s.Also {
			if !ids[a.Entry] {
				add("coverage also_entries names entry %s, which is not in the catalog", a.Entry)
			}
			referenced[a.Entry] = true
		}
	}
	for _, w := range want {
		if !have[w] {
			add("source item %s %s is not covered", w.File, w.Locator)
		}
	}
	wantSet := map[SourceItem]bool{}
	for _, w := range want {
		wantSet[w] = true
	}
	for _, s := range cov.Sources {
		it := SourceItem{File: s.File, Locator: s.Locator}
		if !wantSet[it] {
			add("coverage claims source item %s %s, which the sources do not contain", it.File, it.Locator)
		}
	}
	for _, e := range entries {
		if !referenced[e.ID] {
			add("catalog entry %s has no source item", e.ID)
		}
	}

	// Declared totals must match what is actually there.
	if cov.SourceItemCount != len(want) {
		add("coverage declares %d source items; the sources contain %d", cov.SourceItemCount, len(want))
	}
	if cov.CatalogEntryCount != len(entries) {
		add("coverage declares %d catalog entries; the catalog contains %d", cov.CatalogEntryCount, len(entries))
	}

	// Every Source locator still points at a real line.
	for _, e := range entries {
		m := lineRe.FindStringSubmatch(e.Source)
		if m == nil {
			continue // sources without a line reference, such as a runbook file
		}
		b, err := showAtPin(root, m[1])
		if err != nil {
			add("%s cites %s, which cannot be read at the pinned commit: %v", e.ID, m[1], err)
			continue
		}
		n, _ := strconv.Atoi(m[2])
		if n < 1 || n > len(strings.Split(string(b), "\n")) {
			add("%s cites %s line %d, which is out of range", e.ID, m[1], n)
		}
	}

	problems = append(problems, checkInvariants(entries)...)

	if len(problems) > 0 {
		sort.Strings(problems)
		fmt.Fprintf(os.Stderr, "capcheck: %d problem(s)\n\n", len(problems))
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		os.Exit(1)
	}
	fmt.Printf("capcheck: ok — %d catalog entries, %d source items covered\n", len(entries), len(want))
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "capcheck: "+format+"\n", a...)
	os.Exit(1)
}
