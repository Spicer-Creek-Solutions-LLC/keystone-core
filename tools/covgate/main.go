// SPDX-License-Identifier: Apache-2.0

// covgate enforces the per-package coverage gates documented in
// docs/project/COVERAGE-GATES.md and called out in PROJECT-DETAILS
// §5.3 / AGENTS.md §5: the state engine ≥85%, state stdlib modules
// ≥80% (the v0.5 gate bars), critical packages ≥70%, CLI packages
// ≥40%.
//
// Reads a coverage.out profile (mode=set), aggregates statement
// coverage per package, classifies each package via the rules in
// config.go (excluded / engine / module / critical / cli / unmatched),
// and asserts the per-category threshold. An optional allowList carries
// per-package exceptions that are below threshold today but tracked
// for graduation; the gate fails if an allowList entry's actual
// coverage rises above its category threshold (forces removal —
// graduating into the regular gate).
//
// Usage:
//
//	go run ./tools/covgate --profile=coverage.out
//
// Exit codes: 0 on all-pass, 1 on any FAIL or unmatched-WARN that's
// promoted to error via --strict-unmatched.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

const modulePrefix = "go.keystone-core.io/keystone-core/"

// pkgStats accumulates the numerator (covered statements) and the
// denominator (total statements) per package across the profile.
type pkgStats struct {
	covered int
	total   int
}

func (p pkgStats) percent() float64 {
	if p.total == 0 {
		return 0
	}
	return 100 * float64(p.covered) / float64(p.total)
}

// classifyPackage returns the gate category for a relative package
// path (e.g. "internal/agent" or "cmd/kscore-server"). Returns
// ("excluded", 0) when no gate applies.
//
// Resolution order: excludedPrefixes → enginePackages exact →
// modulePrefixes → criticalPackages exact → criticalPrefixes → cli
// (cmd/* and internal/cli) → unmatched. Engine/module precede the
// critical rules so the state engine and stdlib modules get the
// higher v0.5 gate bars rather than the critical floor.
func classifyPackage(relPath string) (category string, threshold float64) {
	for _, prefix := range excludedPrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return "excluded", 0
		}
	}
	for _, p := range enginePackages {
		if relPath == p {
			return "engine", engineThreshold
		}
	}
	for _, prefix := range modulePrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return "module", moduleThreshold
		}
	}
	for _, p := range criticalPackages {
		if relPath == p {
			return "critical", criticalThreshold
		}
	}
	for _, prefix := range criticalPrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return "critical", criticalThreshold
		}
	}
	if relPath == "internal/cli" || strings.HasPrefix(relPath, "cmd/") || strings.HasPrefix(relPath, "internal/cli/") {
		return "cli", cliThreshold
	}
	return "unmatched", 0
}

// parseProfile reads a go-cover profile and returns per-package
// pkgStats keyed by relative package path. The profile format is:
//
//	mode: <mode>
//	<module>/<pkg>/<file>:start.col,end.col stmts hits
//	...
//
// We aggregate by package, accepting any mode (set/count/atomic).
// Hits > 0 counts the block as covered; hits == 0 counts it as
// uncovered. Both contribute `stmts` to the denominator.
func parseProfile(path string) (map[string]*pkgStats, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied profile path
	if err != nil {
		return nil, fmt.Errorf("open profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	out := map[string]*pkgStats{}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 64<<10), 1<<20)
	first := true
	for scan.Scan() {
		line := scan.Text()
		if first {
			first = false
			if !strings.HasPrefix(line, "mode:") {
				return nil, fmt.Errorf("profile missing leading `mode:` line")
			}
			continue
		}
		if line == "" {
			continue
		}
		// `<module>/<pkg>/<file.go>:start.col,end.col stmts hits`
		colonIdx := strings.IndexByte(line, ':')
		if colonIdx < 0 {
			return nil, fmt.Errorf("malformed line: %q", line)
		}
		fileLoc := line[:colonIdx]
		rest := line[colonIdx+1:]
		// rest = "start.col,end.col stmts hits"
		fields := strings.Fields(rest)
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed counts on line: %q", line)
		}
		stmts, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("stmts parse: %w", err)
		}
		hits, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("hits parse: %w", err)
		}

		// fileLoc = "<module>/<pkg>/<file.go>"; package = dir part.
		if !strings.HasPrefix(fileLoc, modulePrefix) {
			return nil, fmt.Errorf("unexpected module prefix on %q", fileLoc)
		}
		rel := fileLoc[len(modulePrefix):]
		// strip "/<file.go>"
		slash := strings.LastIndexByte(rel, '/')
		if slash < 0 {
			return nil, fmt.Errorf("no path separator in %q", rel)
		}
		pkg := rel[:slash]

		ps, ok := out[pkg]
		if !ok {
			ps = &pkgStats{}
			out[pkg] = ps
		}
		ps.total += stmts
		if hits > 0 {
			ps.covered += stmts
		}
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// result is one package's verdict.
type result struct {
	pkg       string
	category  string
	pct       float64
	threshold float64
	verdict   string // PASS | FAIL | WARN | SKIP
	notes     string
}

func main() {
	var (
		profile         string
		strictUnmatched bool
	)
	flag.StringVar(&profile, "profile", "coverage.out", "path to go-cover profile")
	flag.BoolVar(&strictUnmatched, "strict-unmatched", false, "fail if any package is unmatched by the classification rules")
	flag.Parse()

	stats, err := parseProfile(profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "covgate:", err)
		os.Exit(2)
	}

	pkgs := make([]string, 0, len(stats))
	for p := range stats {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	results := make([]result, 0, len(pkgs))
	var fails, warns int
	for _, pkg := range pkgs {
		ps := stats[pkg]
		cat, threshold := classifyPackage(pkg)
		r := result{pkg: pkg, category: cat, pct: ps.percent(), threshold: threshold}

		switch cat {
		case "excluded":
			r.verdict = "SKIP"
		case "unmatched":
			r.verdict = "WARN"
			r.notes = "no classification rule — add to criticalPackages, excludedPrefixes, or accept the cli default"
			if strictUnmatched {
				r.verdict = "FAIL"
				fails++
			} else {
				warns++
			}
		case "engine", "module", "critical", "cli":
			r.verdict = "PASS"
			if r.pct < threshold {
				// Check the allowList.
				if entry, ok := allowList[pkg]; ok {
					if r.pct > entry+entry*0.01 { // 1% headroom
						r.verdict = "FAIL"
						r.notes = fmt.Sprintf("coverage %.1f%% now exceeds allowList entry %.1f%% — remove the entry and graduate this package into the regular gate", r.pct, entry)
						fails++
					} else {
						r.verdict = "ALLOWED"
						r.notes = fmt.Sprintf("below %s threshold (%.0f%%) but listed in allowList @ %.1f%%", cat, threshold, entry)
					}
				} else {
					r.verdict = "FAIL"
					r.notes = fmt.Sprintf("below %s threshold (%.0f%%)", cat, threshold)
					fails++
				}
			}
		}
		results = append(results, r)
	}

	// Verify every allowList entry actually appeared in the profile —
	// stale entries hide regressions.
	seen := map[string]bool{}
	for _, r := range results {
		seen[r.pkg] = true
	}
	for entry := range allowList {
		if !seen[entry] {
			fmt.Fprintf(os.Stderr, "covgate: allowList entry %q has no coverage data in profile — remove the entry\n", entry)
			fails++
		}
	}

	// Pretty-print.
	fmt.Println("covgate: per-package coverage gate")
	fmt.Println("==================================")
	fmt.Printf("%-10s %-9s %-7s %-7s %s\n", "VERDICT", "CATEGORY", "PCT", "BAR", "PACKAGE")
	for _, r := range results {
		bar := "-"
		if r.threshold > 0 {
			bar = fmt.Sprintf("%.0f%%", r.threshold)
		}
		line := fmt.Sprintf("%-10s %-9s %5.1f%%  %-7s %s",
			r.verdict, r.category, r.pct, bar, r.pkg)
		if r.notes != "" {
			line += "  // " + r.notes
		}
		fmt.Println(line)
	}
	fmt.Println("==================================")
	fmt.Printf("packages: %d   fails: %d   warns: %d\n", len(results), fails, warns)

	if fails > 0 {
		os.Exit(1)
	}
}
