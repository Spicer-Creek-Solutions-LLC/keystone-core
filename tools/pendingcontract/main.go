package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type requirement struct {
	Case   string `json:"case"`
	Owner  string `json:"owner"`
	Expiry string `json:"expiry"`
	Reason string `json:"reason"`

	// Test names the Go test function this case is registered against, and is
	// empty for a deferred gate that has no test at all.
	//
	// It is DATA rather than a table in this tool because the tool cannot
	// import a contract package -- tools/ are separate modules -- so a
	// hardcoded map was the only way to know the mapping, and a hardcoded map
	// is single-package by construction. That is what G39 removed.
	//
	// A wrong name is self-reporting: `go test -run ^Nonexistent$` exits zero,
	// and a case that PASSES is already fatal here, so a typo surfaces as
	// "passed; pending-contract must fail" rather than as a case that quietly
	// checks nothing.
	//
	// A MISSING name is a different matter, and Deferred is why.
	Test string `json:"test"`

	// Deferred marks an entry that legitimately has no test -- an owed gate
	// rather than an unimplemented case.
	//
	// It exists because G39's first draft inferred that from an empty Test,
	// and review found what that costs: a manifest entry that simply omitted
	// the field was reported as a deferred gate and the case disappeared from
	// the runner. A typo in one field could retire a real acceptance case
	// silently.
	//
	// The old tool held an allowlist of case names that were allowed to have no
	// test, which is the same statement made in a place the manifest could not
	// reach. Classification is now DECLARED rather than inferred from absence,
	// and every contradiction is fatal: a case cannot be both, and a case can
	// be neither only by saying so.
	Deferred bool `json:"deferred"`
}

type manifest struct {
	Requirements []requirement `json:"requirements"`
}

// contractGlob is where a contract package's manifest lives. The set of
// packages is ENUMERATED from the filesystem rather than listed here, for the
// reason docs-links enumerates from git: a list beside the thing it lists is a
// second copy, and the copy is what goes stale.
const contractGlob = "test/contract/*/pending-requirements.json"

func main() {
	root := flag.String("root", "../..", "repository root")
	flag.Parse()

	manifests, err := filepath.Glob(filepath.Join(*root, contractGlob))
	if err != nil {
		fatal("enumerate contract packages: %v", err)
	}
	sort.Strings(manifests)
	if len(manifests) == 0 {
		fatal("no contract package carries a manifest; %s matched nothing", contractGlob)
	}
	epic, err := os.ReadFile(filepath.Join(*root, "epics/20-generation-2-reboot.md"))
	if err != nil {
		fatal("read epic: %v", err)
	}

	total := 0
	for _, path := range manifests {
		total += checkPackage(*root, path, string(epic))
	}
	fmt.Printf("pending-contract: %d registered case(s) across %d package(s) fail for their documented reasons\n",
		total, len(manifests))
}

// checkPackage runs one contract package's registered cases, and returns how
// many it ran.
//
// Case identifiers are scoped to their own manifest. Two packages may both
// register AC-1 and neither is lost, which is why they need no
// package-qualified form: the directory already qualifies them.
func checkPackage(root, manifestPath, epic string) int {
	pkgDir := filepath.Dir(manifestPath)
	rel, err := filepath.Rel(root, pkgDir)
	if err != nil {
		fatal("locate %s: %v", pkgDir, err)
	}
	pkg := "./" + filepath.ToSlash(rel)

	b, err := os.ReadFile(manifestPath)
	if err != nil {
		fatal("read %s: %v", rel, err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		fatal("parse %s: %v", rel, err)
	}
	if len(m.Requirements) == 0 {
		fatal("%s has no pending requirements", rel)
	}

	surface := surfaceCases(pkgDir, rel)

	seen := map[string]bool{}
	var cases []requirement
	for _, r := range m.Requirements {
		if r.Case == "" || r.Owner == "" || r.Expiry == "" || r.Reason == "" {
			fatal("%s: requirement %q is missing Case, Owner, Expiry, or Reason", rel, r.Case)
		}
		if !epicTask(epic, r.Owner) {
			fatal("%s: requirement %q names owner %q, which is not a task in the epic", rel, r.Case, r.Owner)
		}
		if !epicTask(epic, r.Expiry) {
			fatal("%s: requirement %q names expiry %q, which is not a task in the epic", rel, r.Case, r.Expiry)
		}
		if complete, known := epicTaskStatus(epic, r.Expiry); known && complete {
			fatal("%s: requirement %q expired at completed task %q", rel, r.Case, r.Expiry)
		}
		if seen[r.Case] {
			fatal("%s: duplicate requirement %q", rel, r.Case)
		}
		seen[r.Case] = true

		switch {
		case r.Test != "" && r.Deferred:
			fatal("%s: requirement %q names a test and is marked deferred; it is one or the other", rel, r.Case)
		case r.Deferred && surface[r.Case]:
			fatal("%s: requirement %q is marked deferred, but %s registers it as an acceptance case. "+
				"A deferred entry is an owed gate with no test; a registered case has one, and marking it "+
				"deferred reports a case the contract runs and passes as work nobody has done",
				rel, r.Case, filepath.Join(rel, surfaceFile))
		case r.Test == "" && !r.Deferred:
			fatal("%s: requirement %q names no test and is not marked deferred. "+
				"A pending case needs \"test\"; an owed gate needs \"deferred\": true", rel, r.Case)
		case r.Test != "":
			cases = append(cases, r)
		}
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Case < cases[j].Case })

	for _, r := range cases {
		pattern := "^" + regexp.QuoteMeta(r.Test) + "$"
		cmd := exec.Command("go", "test", "-tags", "contract", pkg, "-run", pattern, "-count=1")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "KEYSTONE_PENDING_CONTRACT=1")
		output, runErr := cmd.CombinedOutput()
		if runErr == nil {
			fatal("%s: %s passed; pending-contract must fail for: %s", rel, r.Case, r.Reason)
		}
		if len(output) == 0 {
			fatal("%s: %s failed without diagnostic output", rel, r.Case)
		}
		fmt.Printf("%s %s fails as expected: %s\n", rel, r.Case, r.Reason)
	}
	for _, r := range m.Requirements {
		if r.Deferred {
			fmt.Printf("%s %s remains registered as a deferred gate: %s\n", rel, r.Case, r.Reason)
		}
	}
	return len(cases)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "pending-contract: "+format+"\n", args...)
	os.Exit(1)
}

var taskRE = regexp.MustCompile(`(?m)^\s*- \[[ x]\] \*{0,2}([PCRG][0-9][0-9][ab]?)\*{0,2}\b`)

func epicTask(epic, name string) bool {
	for _, match := range taskRE.FindAllStringSubmatch(epic, -1) {
		if match[1] == name {
			return true
		}
	}
	// C01-A and C01-I are the two task halves of the epic's C01 workstream.
	// The epic deliberately ticks the shared workstream once, so their liveness
	// resolves to that parent task rather than inventing a second lifecycle.
	if strings.HasSuffix(name, "-A") || strings.HasSuffix(name, "-I") {
		parent := strings.TrimSuffix(strings.TrimSuffix(name, "-A"), "-I")
		return epicTask(epic, parent)
	}
	return false
}

func epicTaskStatus(epic, name string) (complete, known bool) {
	for _, line := range strings.Split(epic, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [") {
			continue
		}
		match := taskRE.FindStringSubmatch(line)
		if len(match) == 0 || match[1] != name {
			continue
		}
		return strings.HasPrefix(trimmed, "- [x]"), true
	}
	if strings.HasSuffix(name, "-A") || strings.HasSuffix(name, "-I") {
		parent := strings.TrimSuffix(strings.TrimSuffix(name, "-A"), "-I")
		return epicTaskStatus(epic, parent)
	}
	return false, false
}

// surfaceFile is the frozen acceptance surface, beside the manifest.
const surfaceFile = "contract_surface.go"

// surfaceMapStart opens the map whose keys are the package's case identifiers.
// The second map in that file is keyed by the same identifiers but keyed to
// function values, so reading either would do; this one carries the requirement
// text and is the one a reader checks a manifest against.
const surfaceMapStart = "acceptanceContractSurface = map[string]acceptanceCase{"

var surfaceCaseRE = regexp.MustCompile(`(?m)^\s*"([^"]+)":`)

// surfaceCases reads the case identifiers a contract package registers.
//
// It parses Go source with a regular expression because tools/ are separate
// modules and cannot import a contract package -- the same constraint that put
// the test names in the manifest as DATA. The cost is bounded: the map is
// generated by nobody and edited by hand under an immutability gate, and an id
// this misses can only make the check MISS a violation, never invent one.
//
// A missing surface is fatal. Treating it as "no cases" would let a package
// with no readable surface accept any deferred entry at all, which is inference
// from absence: the defect G39 removed from this tool's classification.
func surfaceCases(pkgDir, rel string) map[string]bool {
	b, err := os.ReadFile(filepath.Join(pkgDir, surfaceFile))
	if err != nil {
		fatal("read %s: %v", filepath.Join(rel, surfaceFile), err)
	}
	_, after, found := strings.Cut(string(b), surfaceMapStart)
	if !found {
		fatal("%s does not declare %s", filepath.Join(rel, surfaceFile), surfaceMapStart)
	}
	body, _, found := strings.Cut(after, "\n}")
	if !found {
		fatal("%s does not close %s", filepath.Join(rel, surfaceFile), surfaceMapStart)
	}
	cases := map[string]bool{}
	for _, m := range surfaceCaseRE.FindAllStringSubmatch(body, -1) {
		cases[m[1]] = true
	}
	if len(cases) == 0 {
		fatal("%s registers no acceptance case", filepath.Join(rel, surfaceFile))
	}
	return cases
}
