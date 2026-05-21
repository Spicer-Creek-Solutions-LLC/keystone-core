// racegate enforces docs/project/TEST-POLICY.md: every `go test`
// invocation under tracked source files must include -race unless
// the enclosing target is on an explicit allowlist (currently:
// `slo`, where race instrumentation inflates wall-clock 2-10x and
// would make the asserted SLO numbers meaningless).
//
// Scope of the lint:
//
//   - Makefile (project root)
//   - scripts/smoke-test.sh
//
// Both are scanned line-by-line. The lint identifies `go test`
// invocations (allowing common flag variations like leading
// "CGO_ENABLED=1") and asserts each is either:
//
//   - race-instrumented (contains "-race" somewhere on the same
//     command line, possibly continued via trailing "\"); or
//   - inside a section whose target name appears on the allowList.
//
// Allowlist entries are tracked here, not in a separate config
// file — racegate is small and grep-able. Adding an exception
// requires updating allowList + a comment naming the reason and
// the policy entry in docs/project/TEST-POLICY.md.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// allowList carries Make-target names whose `go test` invocations
// deliberately omit -race. Each entry needs a comment naming the
// docs/project/TEST-POLICY.md section that documents the
// exception.
var allowList = map[string]string{
	// `make slo` runs the wall-clock SLO tests. Race adds 2-10x
	// overhead to wall-clock measurements, which would make the
	// asserted SLO numbers meaningless. See
	// docs/project/TEST-POLICY.md "Documented exceptions".
	"slo": "wall-clock SLO measurement",
}

// goTestPattern matches a `go test` command on a line. Allows for
// a leading env-var assignment (e.g. "CGO_ENABLED=1") and tab/space
// indentation. Doesn't try to be a shell parser — line-oriented is
// enough because the Makefile (and smoke-test.sh) keep each go
// test on one line (continuations use trailing "\").
var goTestPattern = regexp.MustCompile(`(^|\s|;)go\s+test\b`)

// targetPattern matches a Make target declaration: `name:` at
// start-of-line, where name is letters/digits/_-/. The optional
// trailing " ##" docstring is irrelevant.
var targetPattern = regexp.MustCompile(`^([A-Za-z0-9_./-]+)\s*:\s*(?:[^=]|$)`)

// finding is one violation.
type finding struct {
	file    string
	lineNum int
	target  string
	line    string
}

func main() {
	var (
		makefile  string
		smokeFile string
	)
	flag.StringVar(&makefile, "makefile", "Makefile", "Makefile path")
	flag.StringVar(&smokeFile, "smoke", "scripts/smoke-test.sh", "smoke script path")
	flag.Parse()

	var findings []finding

	mfFindings, err := scanMakefile(makefile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "racegate:", err)
		os.Exit(2)
	}
	findings = append(findings, mfFindings...)

	smFindings, err := scanScript(smokeFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "racegate:", err)
		os.Exit(2)
	}
	findings = append(findings, smFindings...)

	if len(findings) == 0 {
		fmt.Println("racegate: ok (every `go test` is either -race or on the allowlist)")
		return
	}

	fmt.Fprintln(os.Stderr, "racegate: violations")
	fmt.Fprintln(os.Stderr, "====================")
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "%s:%d (target %q):\n  %s\n",
			f.file, f.lineNum, f.target, strings.TrimSpace(f.line))
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Fix by adding -race to the command, or add the target to the allowList in tools/racegate/main.go with a docs/project/TEST-POLICY.md reference.")
	os.Exit(1)
}

// scanMakefile walks the Makefile, tracking the current target and
// flagging any `go test` line that doesn't contain `-race` unless
// the current target is allowlisted.
func scanMakefile(path string) ([]finding, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied Makefile path
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var (
		findings []finding
		target   string
		scanner  = bufio.NewScanner(f)
		lineNum  int
	)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		// Track Make target boundaries. A target line starts at
		// column 0 with name:. Tab-indented lines are recipe lines
		// of the current target.
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") {
			if m := targetPattern.FindStringSubmatch(line); m != nil {
				target = m[1]
			}
			// .PHONY blocks, variable assignments, comments, and
			// blank lines all drop through here and keep the
			// previously-set target (or clear it if we just matched
			// a new one above).
			continue
		}
		// Recipe line.
		if !goTestPattern.MatchString(line) {
			continue
		}
		if strings.Contains(line, "-race") {
			continue
		}
		if _, ok := allowList[target]; ok {
			continue
		}
		findings = append(findings, finding{
			file:    path,
			lineNum: lineNum,
			target:  target,
			line:    line,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %q: %w", path, err)
	}
	return findings, nil
}

// scanScript treats shell scripts as having a single implicit
// "target" matching the script's basename. Every `go test` line in
// the script must include `-race` (no per-line allowlist —
// shell scripts that need an exception should call out to a
// dedicated Make target that IS allowlisted).
func scanScript(path string) ([]finding, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied script path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var (
		findings []finding
		scanner  = bufio.NewScanner(f)
		lineNum  int
	)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if !goTestPattern.MatchString(line) {
			continue
		}
		if strings.Contains(line, "-race") {
			continue
		}
		findings = append(findings, finding{
			file:    path,
			lineNum: lineNum,
			target:  "<script>",
			line:    line,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %q: %w", path, err)
	}
	return findings, nil
}
