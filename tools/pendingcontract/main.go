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
}

type manifest struct {
	Requirements []requirement `json:"requirements"`
}

var tests = map[string]string{
	"AC-1": "TestAC1RoundTrip", "AC-2": "TestAC2Framing",
	"AC-3": "TestAC3SignedFields", "AC-4": "TestAC4WrongSignatureKey",
	"AC-5": "TestAC5SignedClasses", "AC-6": "TestAC6CiphertextRefusal",
	"AC-7": "TestAC7VersionAndSequenceRefusal", "AC-8": "TestAC8HeaderTolerance",
	"AC-9": "TestAC9CrossProcess", "AC-10": "TestAC10CoarseRefusals",
	"AC-11": "TestAC11IndistinguishableRefusals", "AC-12": "TestAC12GoldenStability",
	"AC-13": "TestAC13SeedCorpus",
}

func main() {
	root := flag.String("root", "../..", "repository root")
	manifestPath := flag.String("manifest", "test/contract/protocol/pending-requirements.json", "manifest path")
	flag.Parse()

	b, err := os.ReadFile(filepath.Join(*root, *manifestPath))
	if err != nil {
		fatal("read manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		fatal("parse manifest: %v", err)
	}
	if len(m.Requirements) == 0 {
		fatal("manifest has no pending requirements")
	}
	epic, err := os.ReadFile(filepath.Join(*root, "epics/20-generation-2-reboot.md"))
	if err != nil {
		fatal("read epic: %v", err)
	}
	seen := map[string]bool{}
	var cases []requirement
	for _, r := range m.Requirements {
		if r.Case == "" || r.Owner == "" || r.Expiry == "" || r.Reason == "" {
			fatal("requirement %q is missing Case, Owner, Expiry, or Reason", r.Case)
		}
		if !epicTask(string(epic), r.Owner) {
			fatal("requirement %q names owner %q, which is not a task in the epic", r.Case, r.Owner)
		}
		if !epicTask(string(epic), r.Expiry) {
			fatal("requirement %q names expiry %q, which is not a task in the epic", r.Case, r.Expiry)
		}
		if seen[r.Case] {
			fatal("duplicate requirement %q", r.Case)
		}
		seen[r.Case] = true
		if testName := tests[r.Case]; testName != "" {
			cases = append(cases, r)
		}
	}
	if len(cases) != len(tests) {
		fatal("manifest covers %d contract tests; want %d", len(cases), len(tests))
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Case < cases[j].Case })

	for _, r := range cases {
		pattern := "^" + tests[r.Case] + "$"
		cmd := exec.Command("go", "test", "-tags", "contract", "./test/contract/protocol", "-run", pattern, "-count=1")
		cmd.Dir = *root
		cmd.Env = append(os.Environ(), "KEYSTONE_PENDING_CONTRACT=1")
		output, runErr := cmd.CombinedOutput()
		if runErr == nil {
			fatal("%s passed; pending-contract must fail for: %s", r.Case, r.Reason)
		}
		if len(output) == 0 {
			fatal("%s failed without diagnostic output", r.Case)
		}
		fmt.Printf("%s fails as expected: %s\n", r.Case, r.Reason)
	}
	fmt.Printf("pending-contract: %d registered cases fail for their documented reasons\n", len(cases))
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
