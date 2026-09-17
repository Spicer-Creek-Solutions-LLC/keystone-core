// SPDX-License-Identifier: Apache-2.0

// Command capcheck validates the Generation 1 capability catalog produced by
// reboot task R03 against the archived sources it claims to cover.
//
// It does not regenerate the catalog. R03's generator ran once; what needs to
// survive is the ability to prove the catalog's claims still hold — so that a
// later hand edit of either artifact, or a mistaken merge, is caught rather
// than silently weakening the archive record that R05 and R09 build on. The
// archived sources are read at an immutable commit, so they cannot drift; what
// can drift is the catalog and the coverage map that describe them.
//
// Checks, in order of what they protect:
//
//   - every item in the archived sources is present in the coverage map;
//   - the coverage map invents nothing that is not in those sources;
//   - every coverage entry names a catalog entry that exists;
//   - no catalog entry is orphaned, and no identifier is used twice;
//   - every entry's Source locator resolves, and agrees with the coverage map;
//   - the declared totals match what the artifacts actually contain; and
//   - the catalog's own status and scope vocabularies are self-consistent.
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
		File     string `json:"file"`
		Locator  string `json:"locator"`
		Entry    string `json:"entry"`
		Relation string `json:"relation"`
		Kind     string `json:"kind"`
		Text     string `json:"text"`
		Also     []struct {
			Entry    string `json:"entry"`
			Relation string `json:"relation"`
		} `json:"also_entries"`
	} `json:"sources"`
}

// The catalog documents these vocabularies about itself; an unrecognised value
// means the table was hand-edited into a state the taxonomy does not define.
var (
	validStatus = map[string]bool{"implemented": true, "partial": true, "planned": true, "unknown": true, "reference": true}
	validScope  = map[string]bool{"v0.6": true, "Future": true}
	validRel    = map[string]bool{"primary": true, "refinement": true, "duplicate": true, "domain-overview": true}
	validKind   = map[string]bool{"capability-bullet": true, "roadmap-entry": true, "epic-checkbox": true,
		"runbook-section": true, "details-section": true, "state-module": true, "changelog-fragment": true}
)

var (
	rowRe         = regexp.MustCompile(`^\| ` + "`" + `(CAP-[A-Z]+-\d+)` + "`" + ` \| (.*?) \| (\S+) \| (\S+) \| ` + "`" + `(.*?)` + "`" + ` \|$`)
	bulletRe      = regexp.MustCompile(`^- ` + "`" + `(CAP-[A-Z]+-\d+)` + "`" + ` — (.*)$`)
	lineRe        = regexp.MustCompile(`^(.*?) L(\d+)$`)
	totalsRe      = regexp.MustCompile(`\| implemented / partial / planned / unknown \| (\d+) / (\d+) / (\d+) / (\d+) \|`)
	scopeTotalsRe = regexp.MustCompile("\\| `v0.6` / `Future` \\| (\\d+) / (\\d+) \\|")
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
		if section != "" && strings.HasPrefix(line, "- ") && !bulletRe.MatchString(line) {
			return nil, fmt.Errorf("malformed bullet in a %s block: %q", section, line)
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
	cmd := gitAt(root, "show", generationOneFinalSHA+":"+file)
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
	cmd := gitAt(root, "ls-tree", "--name-only", generationOneFinalSHA, dir+"/")
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

// checkVocabularies rejects a status or scope value the taxonomy does not
// define, which is what a hand edit of the table looks like.
func checkVocabularies(entries []Entry) []string {
	var problems []string
	for _, e := range entries {
		if !validStatus[e.Status] {
			problems = append(problems, fmt.Sprintf("%s has status %q, which the taxonomy does not define", e.ID, e.Status))
		}
		if !validScope[e.Scope] {
			problems = append(problems, fmt.Sprintf("%s has scope %q, which the taxonomy does not define", e.ID, e.Scope))
		}
	}
	return problems
}

// checkTotals reconciles the declared summary against the rows themselves. It
// doubles as a checksum over the status column: changing one entry's status
// without updating the table is caught here even though the row stays
// structurally valid.
func checkTotals(md string, entries []Entry) []string {
	var problems []string
	counted := map[string]int{}
	scoped := map[string]int{}
	for _, e := range entries {
		counted[e.Status]++
		scoped[e.Scope]++
	}
	if m := totalsRe.FindStringSubmatch(md); m != nil {
		for i, k := range []string{"implemented", "partial", "planned", "unknown"} {
			n, _ := strconv.Atoi(m[i+1])
			if n != counted[k] {
				problems = append(problems, fmt.Sprintf("the totals table declares %d %s entries; the catalog has %d", n, k, counted[k]))
			}
		}
	} else {
		problems = append(problems, "the totals table could not be read, so its declared counts cannot be reconciled")
	}
	if m := scopeTotalsRe.FindStringSubmatch(md); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a != scoped["v0.6"] || b != scoped["Future"] {
			problems = append(problems, fmt.Sprintf("the totals table declares %d v0.6 and %d Future; the catalog has %d and %d", a, b, scoped["v0.6"], scoped["Future"]))
		}
	} else {
		problems = append(problems, "the scope totals row could not be read, so its declared counts cannot be reconciled")
	}
	return problems
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

	problems = append(problems, checkVocabularies(entries)...)

	for _, e := range entries {
		if strings.TrimSpace(e.Name) == "" {
			add("%s has an empty capability name", e.ID)
		}
	}

	// A Source cell must agree with the coverage item that created the entry.
	primaryCount := map[string]int{}
	primaryOf := map[string]string{}
	for _, s := range cov.Sources {
		if s.Relation == "primary" {
			primaryOf[s.Entry] = s.File + " " + s.Locator
			primaryCount[s.Entry]++
		}
		if !validKind[s.Kind] {
			add("coverage kind %q is not one of the documented source kinds", s.Kind)
		}
		if !validRel[s.Relation] {
			add("coverage relation %q is not one of the documented relations", s.Relation)
		}
		for _, a := range s.Also {
			// also_entries exists only for the RFC 0001 execution-boundary split,
			// which is a duplicate of an entry that already has its own primary.
			if a.Relation != "duplicate" {
				add("coverage also_entries relation for %s is %q; only \"duplicate\" is defined", a.Entry, a.Relation)
			}
			if s.Relation != "primary" {
				add("coverage item %s %s carries also_entries but is not a primary", s.File, s.Locator)
			}
		}
	}
	// The coverage item an entry cites must not be a bare refinement. Demoting a
	// primary would otherwise leave the entry looking merely unsourced rather
	// than mis-sourced.
	relationAt := map[string]string{}
	entryAt := map[string]string{}
	for _, s := range cov.Sources {
		relationAt[s.File+" "+s.Locator] = s.Relation
		entryAt[s.File+" "+s.Locator] = s.Entry
	}
	for _, e := range entries {
		// domain-overview and duplicate are legitimate sole backings for an
		// entry; a refinement is not, because a refinement by definition adds
		// detail to an entry that some other item created.
		if rel, ok := relationAt[e.Source]; ok && entryAt[e.Source] == e.ID && rel == "refinement" && primaryOf[e.ID] == "" {
			add("%s cites %q, but that coverage item is recorded as a refinement and the entry has no primary", e.ID, e.Source)
		}
	}

	for _, e := range entries {
		want, ok := primaryOf[e.ID]
		if !ok {
			continue // entries backed only by duplicate or domain-overview items
		}
		got := strings.TrimSuffix(e.Source, " -")
		if got != want && e.Source != strings.TrimSuffix(want, " -") {
			add("%s cites %q but its primary coverage item is %q", e.ID, e.Source, want)
		}
	}

	// A coverage item's recorded text must still match the line it cites, so a
	// rewritten coverage map cannot quietly describe something the sources do
	// not say.
	norm := func(x string) string { return strings.Join(strings.Fields(x), " ") }
	pinned := map[string][]string{}
	for _, sc := range cov.Sources {
		if sc.Locator == "-" || sc.Text == "" {
			continue // changelog fragments carry a derived title, not a line
		}
		lines, ok := pinned[sc.File]
		if !ok {
			b, err := showAtPin(root, sc.File)
			if err != nil {
				add("coverage cites %s, which cannot be read at the pinned commit", sc.File)
				pinned[sc.File] = nil
				continue
			}
			lines = strings.Split(string(b), "\n")
			pinned[sc.File] = lines
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(sc.Locator, "L"))
		if n < 1 || n > len(lines) {
			add("coverage cites %s %s, which is out of range", sc.File, sc.Locator)
			continue
		}
		want, got := norm(lines[n-1]), norm(sc.Text)
		if got != "" && !strings.HasPrefix(want, got[:min(len(got), 60)]) && !strings.Contains(want, got[:min(len(got), 40)]) {
			add("coverage text for %s %s does not match the line at the pinned commit", sc.File, sc.Locator)
		}
	}

	problems = append(problems, checkTotals(string(md), entries)...)

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

// gitEnvVars are the variables that let git find a repository without looking
// at the working directory. GIT_DIR is the one that bites: it overrides
// discovery entirely, so a process that sets cmd.Dir and inherits the ambient
// environment reads whatever repository the environment names.
//
// **Git hooks set GIT_DIR.** Running `make check` from a pre-commit hook is
// enough to reach this, and the failure is silent: the tool succeeds against
// the wrong tree.
var gitEnvVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_COMMON_DIR",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_CEILING_DIRECTORIES",
	"GIT_DISCOVERY_ACROSS_FILESYSTEM",
	"GIT_NAMESPACE",
	"GIT_PREFIX",
}

// withoutGitEnv removes those variables from an environment.
func withoutGitEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		drop := false
		for _, v := range gitEnvVars {
			if strings.HasPrefix(kv, v+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// gitAt builds a git command rooted at dir, with the ambient repository
// environment removed so that dir is what decides which repository is read.
func gitAt(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = withoutGitEnv(os.Environ())
	return cmd
}
