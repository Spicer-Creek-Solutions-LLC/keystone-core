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
		write(t, filepath.Join(root, "test", "contract", pkg, "pending-requirements.json"),
			manifest{Requirements: []requirement{
				{Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "not built in " + pkg, Test: ""},
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
			[]requirement{{Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "r"}, {Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "r"}},
			epicWith("C02")},
		{"owner is not an epic task",
			[]requirement{{Case: "AC-1", Owner: "C99", Expiry: "C02", Reason: "r"}},
			epicWith("C02")},
		{"expiry is not an epic task",
			[]requirement{{Case: "AC-1", Owner: "C02", Expiry: "C99", Reason: "r"}},
			epicWith("C02")},
		{"missing reason",
			[]requirement{{Case: "AC-1", Owner: "C02", Expiry: "C02"}},
			epicWith("C02")},
		{"expiry task already complete",
			[]requirement{{Case: "AC-1", Owner: "C02", Expiry: "C02", Reason: "r"}},
			"# epic\n\n- [x] C02 — done.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "test", "contract", "x", "pending-requirements.json")
			write(t, path, manifest{Requirements: tc.reqs})
			_ = path
			if err := runTool(t, root, tc.epic); err == nil {
				t.Error("the tool accepted a manifest it must reject")
			}
		})
	}
}

// A well-formed deferred gate must pass, or the cases above would prove only
// that the tool rejects everything.
func TestAWellFormedDeferredGateIsAccepted(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test", "contract", "x", "pending-requirements.json")
	write(t, path, manifest{Requirements: []requirement{
		{Case: "nightly", Owner: "C15", Expiry: "C15", Reason: "no schedule exists", Test: ""},
	}})
	if err := runTool(t, root, epicWith("C15")); err != nil {
		t.Error("a well-formed deferred gate was rejected")
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
