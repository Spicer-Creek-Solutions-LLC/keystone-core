package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// G39 generalised this tool from one contract package to any number. The
// single-package coupling was in three places -- a hardcoded case-to-test map,
// a default manifest path, and the package literal passed to `go test` -- and
// D-C02-4 had named only the first.
//
// G38's review left the other half of the lesson: archlint shipped a check with
// no test file, so its demonstrations lived in a shell and guarded nothing after
// the session. This tool had none either. These cases are the demonstration, in
// the tree.

func write(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// surface writes the frozen acceptance surface a contract package must carry.
//
// Every fixture needs one, because G42 made a missing surface fatal: a package
// whose surface cannot be read would otherwise accept any deferred entry at
// all, which is inference from absence.
//
// The identifiers here are deliberately ones the manifests below do not name,
// so each case still fails for its own reason rather than for G42's rule.
func surface(t *testing.T, dir string, cases ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("package x_test\n\nvar acceptanceContractSurface = map[string]acceptanceCase{\n")
	for _, c := range cases {
		b.WriteString("\t\"" + c + "\": {\"Test" + c + "\", \"a requirement\"},\n")
	}
	b.WriteString("}\n")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "contract_surface.go"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func epicWith(tasks ...string) string {
	var b strings.Builder
	b.WriteString("# epic\n\n")
	for _, t := range tasks {
		b.WriteString("- [ ] " + t + " — a task.\n")
	}
	return b.String()
}

// The property D-C02-4 was written about: two packages may both register AC-1
// and neither is lost. Identifiers are scoped to their own manifest, so the
// directory qualifies them and no naming scheme is needed.
func TestTwoPackagesMayBothRegisterTheSameCaseIdentifier(t *testing.T) {
	root := t.TempDir()
	for _, pkg := range []string{"protocol", "persistence"} {
		surface(t, filepath.Join(root, "test", "contract", pkg), "AC-99")
		write(t, filepath.Join(root, "test", "contract", pkg, "pending-requirements.json"),
			manifest{Requirements: []requirement{
				{Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "not built in " + pkg, Deferred: true},
			}})
	}

	// Run the REAL entry point, not checkPackage directly. A first draft called
	// checkPackage once per package from the test itself, which asserted that
	// `seen` is scoped per manifest -- true, and not the property the name
	// claims. It passed unchanged against a glob hardcoded to one package,
	// because the test was doing the enumerating.
	out, err := runToolOutput(t, root, epicWith("C02"))
	if err != nil {
		t.Fatalf("two well-formed packages were rejected: %v\n%s", err, out)
	}
	for _, pkg := range []string{"protocol", "persistence"} {
		if !strings.Contains(out, filepath.Join("test", "contract", pkg)) {
			t.Errorf("%s was not reported; the enumeration missed a package\n%s", pkg, out)
		}
	}
	if !strings.Contains(out, "2 package(s)") {
		t.Errorf("the tool did not report two packages\n%s", out)
	}
}

// Validation the tool must still do, per manifest.
func TestManifestValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		reqs []requirement
		epic string
	}{
		{"duplicate case within one manifest",
			[]requirement{{Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "r", Deferred: true}, {Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "r", Deferred: true}},
			epicWith("C02")},
		{"owner is not an epic task",
			[]requirement{{Case: "AC-1", Owner: "C99", Expiry: "C02", Reason: "r", Deferred: true}},
			epicWith("C02")},
		{"expiry is not an epic task",
			[]requirement{{Case: "AC-1", Owner: "C02", Expiry: "C99", Reason: "r", Deferred: true}},
			epicWith("C02")},
		{"missing reason",
			[]requirement{{Case: "AC-1", Owner: "C02", Expiry: "C02", Deferred: true}},
			epicWith("C02")},
		{"expiry task already complete",
			[]requirement{{Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "r", Deferred: true}},
			"# epic\n\n- [x] C02 — done.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			surface(t, filepath.Join(root, "test", "contract", "x"), "AC-99")
			path := filepath.Join(root, "test", "contract", "x", "pending-requirements.json")
			write(t, path, manifest{Requirements: tc.reqs})
			_ = path
			if err := runTool(t, root, tc.epic); err == nil {
				t.Error("the tool accepted a manifest it must reject")
			}
		})
	}
}

// Review of #356: G39's first draft inferred "deferred gate" from an empty test
// field, so a manifest entry that merely OMITTED the field was reported as
// deferred and its case vanished from the runner. A typo could retire a real
// acceptance case silently, which is what pending-contract exists to prevent.
//
// Classification is declared now, and every combination is checked here --
// including the two that must be rejected, because a rule that only ever sees
// well-formed input is not a rule.
func TestDeferralIsDeclaredAndNotInferred(t *testing.T) {
	for _, tc := range []struct {
		name   string
		req    requirement
		reject bool
	}{
		{"a real case with no test and no deferred flag",
			requirement{Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "not built"}, true},
		{"a case that is both a test and deferred",
			requirement{Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "r", Test: "TestAC1", Deferred: true}, true},
		{"an owed gate, declared",
			requirement{Case: "nightly", Owner: "C02", Expiry: "C02", Reason: "no schedule", Deferred: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			surface(t, filepath.Join(root, "test", "contract", "x"), "AC-99")
			write(t, filepath.Join(root, "test", "contract", "x", "pending-requirements.json"),
				manifest{Requirements: []requirement{tc.req}})
			out, err := runToolOutput(t, root, epicWith("C02"))
			switch {
			case tc.reject && err == nil:
				t.Errorf("accepted a manifest it must reject\n%s", out)
			case !tc.reject && err != nil:
				t.Errorf("rejected a well-formed manifest: %v\n%s", err, out)
			}
			// And the case must never be reported as deferred unless it said so.
			if tc.reject && strings.Contains(out, "remains registered as a deferred gate") {
				t.Errorf("an unclassified case was reported as a deferred gate\n%s", out)
			}
		})
	}
}

// A well-formed deferred gate must pass, or the cases above would prove only
// that the tool rejects everything.
func TestAWellFormedDeferredGateIsAccepted(t *testing.T) {
	root := t.TempDir()
	surface(t, filepath.Join(root, "test", "contract", "x"), "AC-99")
	path := filepath.Join(root, "test", "contract", "x", "pending-requirements.json")
	write(t, path, manifest{Requirements: []requirement{
		{Case: "nightly", Owner: "C15", Expiry: "C15", Reason: "no schedule exists", Deferred: true},
	}})
	if err := runTool(t, root, epicWith("C15")); err != nil {
		t.Error("a well-formed deferred gate was rejected")
	}
}

// G42. C02-I marked AC-11 -- a case the frozen surface registers and the
// contract runs -- as a deferred gate, and this tool accepted it: `make
// contract` reported the case green while the manifest reported it as work
// nobody had done. Neither could fail, and the two records disagreed.
//
// A deferred entry is an owed gate with no test. Declaring one for a case that
// HAS a test retires it from the runner while it still passes, which is the
// same silent retirement G39 removed -- reached by a different route.
func TestADeferredEntryMayNotNameARegisteredCase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		id     string
		reject bool
	}{
		{"a registered case declared a deferred gate", "AC-11", true},
		{"an owed gate the surface does not register", "nightly-fuzz-search", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			surface(t, filepath.Join(root, "test", "contract", "x"), "AC-11")
			write(t, filepath.Join(root, "test", "contract", "x", "pending-requirements.json"),
				manifest{Requirements: []requirement{
					{Case: tc.id, Owner: "C02", Expiry: "C02", Reason: "owed", Deferred: true},
				}})
			out, err := runToolOutput(t, root, epicWith("C02"))
			switch {
			case tc.reject && err == nil:
				t.Errorf("accepted a deferred entry naming a registered case\n%s", out)
			case !tc.reject && err != nil:
				t.Errorf("rejected a well-formed owed gate: %v\n%s", err, out)
			}
		})
	}
}

// An unreadable surface must be fatal. Reading it as "no cases" would let a
// package with no surface accept any deferred entry, which is the inference
// from absence G39 took out of this tool's classification.
func TestAMissingSurfaceIsFatal(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "test", "contract", "x", "pending-requirements.json"),
		manifest{Requirements: []requirement{
			{Case: "nightly", Owner: "C02", Expiry: "C02", Reason: "owed", Deferred: true},
		}})
	if err := runTool(t, root, epicWith("C02")); err == nil {
		t.Error("a contract package with no acceptance surface was accepted")
	}
}

// runTool runs the real entry point against a temporary root, because fatal
// calls os.Exit and an exit code needs a process to exit. It is the binary
// under test, not a reimplementation of it.
func runToolOutput(t *testing.T, root, epic string) (string, error) {
	t.Helper()
	epicPath := filepath.Join(root, "epics", "20-generation-2-reboot.md")
	if err := os.MkdirAll(filepath.Dir(epicPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(epicPath, []byte(epic), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".", "-root", root)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		t.Fatalf("the tool produced no diagnostic output: %v", err)
	}
	return string(out), err
}

func runTool(t *testing.T, root, epic string) error {
	t.Helper()
	_, err := runToolOutput(t, root, epic)
	return err
}
