// archlint enforces REQUIREMENTS-TRACEABILITY.md against the documents it
// claims to map, and against the work the epic says is done.
//
// ARCH-TEST-003 asks that every invariant be enforced by an automated test or
// an equivalent. ADR-0010 § 14 states plainly what this tool can and cannot do
// about that: a row naming a file that does not exist is a target, not
// enforcement. What archlint provides is that the gap is MEASURED -- every
// invariant has an owner, a gate, and a task that owes its evidence -- and that
// a task completing without paying what it owes is a failure rather than a
// silence.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type row struct {
	invariant, owner, evidence, gate, landsAt string
	line                                      int
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	var fails []string
	note := func(f string, a ...any) { fmt.Printf("  · "+f+"\n", a...) }

	invariants, err := readInvariants(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "archlint:", err)
		os.Exit(2)
	}
	rows, err := readRegister(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "archlint:", err)
		os.Exit(2)
	}
	if len(invariants) < 20 || len(rows) < 20 {
		fmt.Fprintf(os.Stderr, "archlint: %d invariants, %d rows — not reaching the documents\n",
			len(invariants), len(rows))
		os.Exit(2)
	}
	note("%d invariants, %d register rows", len(invariants), len(rows))

	fails = append(fails, checkFieldEncodings(*root)...)

	// Both directions. A register missing an invariant is a gap; a register row
	// naming an invariant that does not exist is a claim about nothing.
	byID := map[string]row{}
	for _, r := range rows {
		if _, dup := byID[r.invariant]; dup {
			fails = append(fails, fmt.Sprintf("%s has two register rows", r.invariant))
		}
		byID[r.invariant] = r
		if !invariants[r.invariant] {
			fails = append(fails, fmt.Sprintf(
				"register line %d maps %s, which ARCHITECTURE-INVARIANTS.md does not define",
				r.line, r.invariant))
		}
	}
	for id := range invariants {
		if _, ok := byID[id]; !ok {
			fails = append(fails, fmt.Sprintf("%s has no register row", id))
		}
	}

	gates := readGates(*root)
	if len(gates) < 3 {
		fails = append(fails, fmt.Sprintf("read %d gates from TESTING.md — the gate check is vacuous", len(gates)))
	}
	note("gates TESTING.md schedules: %s", strings.Join(sorted(gates), ", "))

	tasks, complete, err := readEpic(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "archlint:", err)
		os.Exit(2)
	}
	if len(tasks) < 20 {
		fails = append(fails, fmt.Sprintf("read %d tasks from the epic — the liveness check is vacuous", len(tasks)))
	}
	note("%d tasks in the epic, %d complete", len(tasks), len(complete))

	owed, paid := 0, 0
	var unvalidated, mixed []string
	for _, r := range rows {
		// Every owner must name a document that exists. This is the rule that
		// would have caught "Test architecture ADR" years before ADR-0010 did.
		if !ownerResolves(*root, r.owner) {
			fails = append(fails, fmt.Sprintf(
				"%s's design owner %q names no document in this repository", r.invariant, r.owner))
		}
		if r.gate != "" && !gates[normaliseGate(r.gate)] {
			fails = append(fails, fmt.Sprintf(
				"%s's gate %q is not one TESTING.md schedules", r.invariant, r.gate))
		}
		if r.landsAt == "" {
			fails = append(fails, fmt.Sprintf("%s names no task that owes its evidence", r.invariant))
			continue
		}
		if !tasks[r.landsAt] {
			fails = append(fails, fmt.Sprintf(
				"%s lands at %q, which is not a task in the epic", r.invariant, r.landsAt))
			continue
		}
		// The liveness rule. It is empty today and becomes live task by task,
		// with no dates to maintain: when the epic ticks a task, its rows must
		// have their evidence.
		if complete[r.landsAt] {
			owed++
			paths, prose, malformed := evidencePaths(r.evidence)
			missing := []string{}
			for _, path := range paths {
				if !exists(*root, path) {
					missing = append(missing, path)
				}
			}
			switch {
			case len(malformed) > 0:
				// A token that is shaped like a path and does not parse as one
				// is not prose. Letting it fall into that bucket is how a
				// reference stops being checked without anyone deciding to.
				fails = append(fails, fmt.Sprintf(
					"%s names evidence that looks like a path and cannot be checked as one: %s",
					r.invariant, strings.Join(malformed, ", ")))
			case len(missing) > 0:
				fails = append(fails, fmt.Sprintf(
					"%s lands at %s, which the epic shows complete, and its evidence does not exist: %s",
					r.invariant, r.landsAt, strings.Join(missing, ", ")))
			case prose && len(paths) > 0:
				// Mixed. The paths were checked and the prose was not, so this
				// is neither paid nor unvalidated -- and counting it as paid is
				// the false negative review of #334 found: one real path made
				// the rest of the cell invisible.
				mixed = append(mixed, r.invariant)
			case prose:
				// Not a failure and not a pass. Counting it as paid is how a
				// prose cell becomes a way to satisfy the gate by writing a
				// sentence.
				unvalidated = append(unvalidated, r.invariant)
			default:
				paid++
			}
		}
	}
	note("%d rows owed by a completed task, %d paid", owed, paid)
	if len(mixed) > 0 {
		// Reported separately from prose: these rows DO name paths that were
		// checked, and saying otherwise would understate what holds.
		note("%d owed rows name both a path and prose, so they are only partly machine-checked: %s",
			len(mixed), strings.Join(mixed, ", "))
	}
	if len(unvalidated) > 0 {
		// Reported, never counted. These are evidence a human verifies, and
		// leaving them invisible would let a sentence stand in for a test.
		note("%d owed rows describe their evidence in prose and are NOT machine-checked: %s",
			len(unvalidated), strings.Join(unvalidated, ", "))
	}
	if owed == 0 {
		note("the liveness rule has an empty population and cannot fail today — " +
			"ADR-0010 § 14: a row naming a file that does not exist is a target, not enforcement")
	}

	fmt.Println()
	if len(fails) > 0 {
		fmt.Println("FAIL")
		for _, f := range fails {
			fmt.Println(" -", f)
		}
		os.Exit(1)
	}
	fmt.Println("archlint: ok")
}

// G38: the envelope's nine field encodings are specified across nine sections of
// ADR-0005, and three tasks running closed one field's gap while leaving its
// neighbour -- each found by the next task rather than by the fix. The doclint
// sweeps added at G36 and G37 catch a document naming the wrong OWNER. Nothing
// caught a document saying NOTHING, which is what actually blocked C01-A once
// and C01-I twice.
//
// So section 1 carries an index of all nine fields, and this asserts it is
// complete: nine rows, numbered 1 to 9, none blank and none deferring. It does
// not check that an encoding is CORRECT -- that is review's -- only that one has
// been written.
var fieldRowRE = regexp.MustCompile(`(?m)^\| ([1-9]) \|([^|]*)\|([^|]*)\|\s*$`)

var unspecified = regexp.MustCompile(`(?i)\b(unspecified|undefined|tbd|to be decided|to be determined|not stated|n/?a)\b`)

// The index is ANCHORED to its own heading, and that is not fastidiousness.
// A first draft matched the row shape anywhere in the document and found
// section 1's ORIGINAL field table first -- same three columns, same leading
// digit, different meaning. Blanking a cell in the encoding index left archlint
// green; blanking one in the older table failed it. The check named one table
// and read another, which is the exact defect it exists to prevent, and only
// planting a blank in each table told them apart.
const fieldIndexHeading = "#### Every field's encoding, in one place"

func checkFieldEncodings(root string) []string {
	b, err := os.ReadFile(filepath.Join(root, "docs/adr/0005-versioned-encrypted-protocol.md"))
	if err != nil {
		return []string{fmt.Sprintf("field-encoding index: %v", err)}
	}
	doc := string(b)
	start := strings.Index(doc, fieldIndexHeading)
	if start < 0 {
		return []string{fmt.Sprintf("ADR-0005 has no %q section; the field-encoding index is gone", fieldIndexHeading)}
	}
	rest := doc[start+len(fieldIndexHeading):]
	if end := strings.Index(rest, "\n#### "); end >= 0 {
		rest = rest[:end]
	}
	if end := strings.Index(rest, "\n### "); end >= 0 {
		rest = rest[:end]
	}

	var fails []string
	seen := map[string]bool{}
	for _, m := range fieldRowRE.FindAllStringSubmatch(rest, -1) {
		num, name, enc := m[1], strings.TrimSpace(m[2]), strings.TrimSpace(m[3])
		if seen[num] {
			fails = append(fails, fmt.Sprintf("ADR-0005's field-encoding index has two rows for field %s", num))
			continue
		}
		seen[num] = true
		switch {
		case enc == "":
			fails = append(fails, fmt.Sprintf("ADR-0005 field %s (%s) has a blank encoding", num, name))
		case unspecified.MatchString(enc):
			fails = append(fails, fmt.Sprintf("ADR-0005 field %s (%s) defers its encoding: %q", num, name, enc))
		}
	}
	for i := 1; i <= 9; i++ {
		if !seen[fmt.Sprint(i)] {
			fails = append(fails, fmt.Sprintf("ADR-0005's field-encoding index has no row for field %d", i))
		}
	}
	return fails
}

var invRE = regexp.MustCompile(`(?m)^### (ARCH-[A-Z]+-\d+)`)

func readInvariants(root string) (map[string]bool, error) {
	b, err := os.ReadFile(filepath.Join(root, "docs/project/ARCHITECTURE-INVARIANTS.md"))
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, m := range invRE.FindAllStringSubmatch(string(b), -1) {
		out[m[1]] = true
	}
	return out, nil
}

var rowRE = regexp.MustCompile(`^\| (ARCH-[A-Z]+-\d+) \|([^|]*)\|([^|]*)\|([^|]*)\|([^|]*)\|`)

func readRegister(root string) ([]row, error) {
	b, err := os.ReadFile(filepath.Join(root, "docs/project/REQUIREMENTS-TRACEABILITY.md"))
	if err != nil {
		return nil, err
	}
	var out []row
	for i, line := range strings.Split(string(b), "\n") {
		m := rowRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out = append(out, row{
			invariant: m[1],
			owner:     strings.TrimSpace(m[2]),
			evidence:  strings.Trim(strings.TrimSpace(m[3]), "`"),
			gate:      strings.TrimSpace(m[4]),
			landsAt:   strings.Trim(strings.TrimSpace(m[5]), "`"),
			line:      i + 1,
		})
	}
	return out, nil
}

func readGates(root string) map[string]bool {
	b, _ := os.ReadFile(filepath.Join(root, "docs/project/TESTING.md"))
	sec := regexp.MustCompile(`(?ms)^## Gate schedule.*?(?:^## |\z)`).FindString(string(b))
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^### (.+)$`).FindAllStringSubmatch(sec, -1) {
		out[normaliseGate(m[1])] = true
	}
	return out
}

// normaliseGate maps a register cell onto a heading in TESTING.md's schedule.
// The register writes "PR" and "Main" where the schedule writes prose.
func normaliseGate(s string) string {
	// Strip emphasis everywhere, not at the ends: "Every merge to `main`" ends
	// with a backtick, so trimming left "every merge to `main" and no case
	// matched. A cutset trim is not a way to remove markup.
	s = strings.ToLower(strings.NewReplacer("`", "", "*", "").Replace(strings.TrimSpace(s)))
	switch s {
	case "pr", "every pull request", "pr + vm":
		return "pr"
	case "main", "every merge to `main`", "every merge to main":
		return "main"
	case "nightly":
		return "nightly"
	case "rc", "release candidate":
		return "rc"
	}
	return s
}

var (
	// Sub-items are written in bold -- "- [x] **P11a** —" -- so a pattern that
	// expects a bare identifier silently misses them, and a row landing at one
	// would be reported as naming a task the epic does not have.
	taskRE = regexp.MustCompile(`(?m)^\s*- \[([ x])\] \*{0,2}((?:[PCRG]\d\d[ab]?))\*{0,2}\b`)
)

func readEpic(root string) (all map[string]bool, done map[string]bool, err error) {
	b, err := os.ReadFile(filepath.Join(root, "epics/20-generation-2-reboot.md"))
	if err != nil {
		return nil, nil, err
	}
	all, done = map[string]bool{}, map[string]bool{}
	for _, m := range taskRE.FindAllStringSubmatch(string(b), -1) {
		all[m[2]] = true
		if m[1] == "x" {
			done[m[2]] = true
		}
	}
	return all, done, nil
}

// ownerResolves accepts an owner that names a document present in the tree. An
// owner that names nothing is how "Test architecture ADR" sat in this register
// pointing at a document nobody had written.
func ownerResolves(root, owner string) bool {
	owner = strings.Trim(strings.TrimSpace(owner), "`*")
	if owner == "" {
		return false
	}
	if m := regexp.MustCompile(`ADR-(\d{4})`).FindStringSubmatch(owner); m != nil {
		hits, _ := filepath.Glob(filepath.Join(root, "docs/adr", m[1]+"-*.md"))
		return len(hits) > 0
	}
	// A process rather than a document -- named explicitly, never by shape.
	switch owner {
	case "RFC/ADR process":
		return true
	}
	return false
}

// evidencePaths splits an evidence cell into the paths it names.
//
// A cell may list several ("a_test.go, b_test.go") or describe a rule in prose
// ("tools/archlint coverage rule"). An earlier version returned true for any
// cell containing a space, which made EVERY multi-path cell trivially paid --
// deleting either file named by ARCH-NATS-006 would have left this tool green.
// A check that cannot fail on the input it exists for is not a check.
func evidencePaths(cell string) (paths []string, prose bool, malformed []string) {
	for _, tok := range strings.Split(cell, ",") {
		tok = strings.Trim(strings.TrimSpace(tok), "`")
		if tok == "" {
			continue
		}
		// A path here is a slash-separated reference with a file extension.
		// Anything else is prose: it describes evidence a human verifies, and
		// it is reported rather than counted as paid.
		looksLikePath := strings.Contains(tok, "/") && filepath.Ext(tok) != ""
		switch {
		case looksLikePath && !strings.Contains(tok, " "):
			paths = append(paths, tok)
		case looksLikePath:
			// `a/b.go (generated later)` is not a description of evidence, it
			// is a path with something appended. Treating it as prose is a
			// silent demotion, so it is named and fails.
			malformed = append(malformed, tok)
		default:
			prose = true
		}
	}
	return paths, prose, malformed
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

func sorted(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func exitUnused() { _ = exec.Command }
