// SPDX-License-Identifier: Apache-2.0

// goleakgate enforces docs/project/TEST-POLICY.md: every package
// that contains a `//go:build integration` test file must also
// contain a TestMain that wraps the project's
// test/goleak.VerifyTestMain.
//
// The lint walks the source tree, identifies directories with at
// least one //go:build integration file, then checks each for a
// matching TestMain. A package is treated as conformant when:
//
//   - it has a TestMain function in any *_test.go file in the same
//     directory, AND
//   - the TestMain body calls `goleakhelper.VerifyTestMain` (the
//     shared wrapper from test/goleak/).
//
// Packages that don't match either rule fail the lint, unless they
// appear on `allowList` with a GRADUATE-BY comment. The lint also
// fails if an allowList entry no longer needs the exception
// (i.e. its TestMain now passes the conformance check) — same
// posture as covgate / racegate.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// allowList carries packages that ship an //go:build integration
// test file but don't yet have a goleak-wired TestMain. Each entry
// must carry a GRADUATE-BY comment naming the follow-up.
//
// As of the task-6 PR this list is empty — every integration
// package has goleak wired. The mechanism is here so a future
// integration-test addition without goleak fails CI loudly until
// the operator either fixes the package or files an exception.
var allowList = map[string]string{}

// Root-relative roots to scan. Keeps the lint fast (no .cache walk).
var scanRoots = []string{
	"cmd",
	"internal",
	"pkg",
	"test",
}

func main() {
	var repoRoot string
	flag.StringVar(&repoRoot, "repo", ".", "repo root to scan")
	flag.Parse()

	integrationDirs, err := collectIntegrationDirs(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goleakgate:", err)
		os.Exit(2)
	}

	var fails int
	type result struct {
		pkg     string
		verdict string
		reason  string
	}
	results := make([]result, 0, len(integrationDirs))
	conformant := map[string]bool{}

	for _, dir := range integrationDirs {
		ok, err := packageWiresGoleak(repoRoot, dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goleakgate: scan %s: %v\n", dir, err)
			os.Exit(2)
		}
		r := result{pkg: dir}
		switch {
		case ok:
			r.verdict = "PASS"
			conformant[dir] = true
		default:
			if reason, allowed := allowList[dir]; allowed {
				r.verdict = "ALLOWED"
				r.reason = reason
			} else {
				r.verdict = "FAIL"
				r.reason = "no TestMain wrapping goleakhelper.VerifyTestMain — add one in a `//go:build integration` test file or document an exception in tools/goleakgate/main.go's allowList"
				fails++
			}
		}
		results = append(results, r)
	}

	// Stale allowList entries: listed but actually conformant now.
	for entry := range allowList {
		if conformant[entry] {
			fmt.Fprintf(os.Stderr, "goleakgate: allowList entry %q now wires goleak — remove the entry\n", entry)
			fails++
		}
	}
	// Stale allowList entries: listed but no longer have any
	// integration test files.
	seen := map[string]bool{}
	for _, dir := range integrationDirs {
		seen[dir] = true
	}
	for entry := range allowList {
		if !seen[entry] {
			fmt.Fprintf(os.Stderr, "goleakgate: allowList entry %q has no //go:build integration files — remove the entry\n", entry)
			fails++
		}
	}

	fmt.Println("goleakgate: TestMain-with-goleak gate")
	fmt.Println("=====================================")
	fmt.Printf("%-10s %s\n", "VERDICT", "PACKAGE")
	sort.Slice(results, func(i, j int) bool { return results[i].pkg < results[j].pkg })
	for _, r := range results {
		line := fmt.Sprintf("%-10s %s", r.verdict, r.pkg)
		if r.reason != "" {
			line += "  // " + r.reason
		}
		fmt.Println(line)
	}
	fmt.Println("=====================================")
	fmt.Printf("integration packages: %d   fails: %d\n", len(results), fails)
	if fails > 0 {
		os.Exit(1)
	}
}

// collectIntegrationDirs returns repo-relative package directories
// that contain at least one //go:build integration test file.
func collectIntegrationDirs(root string) ([]string, error) {
	seen := map[string]struct{}{}
	for _, sub := range scanRoots {
		walkRoot := filepath.Join(root, sub)
		err := filepath.WalkDir(walkRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			has, err := hasIntegrationBuildTag(path)
			if err != nil {
				return err
			}
			if has {
				rel, err := filepath.Rel(root, filepath.Dir(path))
				if err != nil {
					return err
				}
				seen[filepath.ToSlash(rel)] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, nil
}

// hasIntegrationBuildTag returns true if path begins with a
// `//go:build integration` build constraint (possibly combined
// with || / && / other tags).
func hasIntegrationBuildTag(path string) (bool, error) {
	f, err := os.Open(path) //nolint:gosec // walking our own repo
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//go:build") {
			return strings.Contains(line, "integration"), nil
		}
		if strings.HasPrefix(line, "//") {
			continue
		}
		// Bail at the first real line — build tags only sit at file head.
		return false, nil
	}
	return false, scan.Err()
}

// packageWiresGoleak returns true if any *_test.go file in dir
// contains a TestMain that calls goleakhelper.VerifyTestMain. We
// scan textually rather than parse the AST — the policy is purely
// pattern-based, and a stdlib walker keeps the dep surface flat.
func packageWiresGoleak(root, relDir string) (bool, error) {
	abs := filepath.Join(root, relDir)
	entries, err := os.ReadDir(abs)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(abs, e.Name())
		body, err := os.ReadFile(path) //nolint:gosec // walking our own repo
		if err != nil {
			return false, err
		}
		text := string(body)
		// Heuristic: both signatures must appear. A TestMain that
		// doesn't call goleak is the failure case the gate exists
		// to catch.
		if strings.Contains(text, "func TestMain(") && strings.Contains(text, "goleakhelper.VerifyTestMain") {
			return true, nil
		}
	}
	return false, nil
}
