// SPDX-License-Identifier: Apache-2.0

// depsoutdated reports the direct go.mod dependencies that have a newer
// release available, split into:
//
//   - minor/patch updates (same major — low-risk, bump when convenient)
//   - new major versions (need the comparison-plan review per AGENTS.md
//     before bumping, since a major can change API/behavior)
//
// It drives `go list -m -u -json all` (which consults the module proxy
// for the latest release of each module) and filters to direct,
// non-main modules with an available Update.
//
// Informational only: it always exits 0. The nightly ci-full cron runs
// it so dependency drift surfaces proactively (the way nats-server
// v2.14.0→v2.14.2 would have, before it bit us in e2e) without waiting
// for someone to notice. It is NOT a gate and does NOT auto-bump —
// dependency changes still land as reviewed PRs.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// module is the subset of `go list -m -json` output we read.
type module struct {
	Path     string
	Version  string
	Indirect bool
	Main     bool
	Update   *struct {
		Version string
	}
}

func main() {
	syncIssueFlag := flag.Bool("issue", false,
		"after printing the report, create/update/close a single tracking issue "+
			"via the Forgejo API (best-effort; needs GITHUB_API_URL/REPOSITORY/TOKEN)")
	flag.Parse()

	cmd := exec.Command("go", "list", "-m", "-u", "-json", "all")
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fatal(err)
	}
	if err := cmd.Start(); err != nil {
		fatal(err)
	}

	var minor, major []string
	dec := json.NewDecoder(bufio.NewReader(stdout))
	for dec.More() {
		var m module
		if err := dec.Decode(&m); err != nil {
			fatal(err)
		}
		if m.Main || m.Indirect || m.Update == nil {
			continue
		}
		line := fmt.Sprintf("  %-52s %s => %s", m.Path, m.Version, m.Update.Version)
		if sameMajor(m.Version, m.Update.Version) {
			minor = append(minor, line)
		} else {
			major = append(major, line)
		}
	}
	// A non-zero exit from `go list -u` (e.g. an unreachable module) is
	// reported but does not fail this informational tool — print what we
	// gathered and note the warning.
	warn := cmd.Wait()

	sort.Strings(minor)
	sort.Strings(major)
	text := report(minor, major)
	fmt.Print(text)
	if warn != nil {
		fmt.Fprintln(os.Stderr, "depsoutdated: go list reported:", warn)
	}

	if *syncIssueFlag {
		// Best-effort: API errors are logged but never fail this
		// informational tool (the report already printed above, and a
		// red nightly over a token hiccup would just be noise).
		syncIssue(len(minor)+len(major) > 0, text)
	}
}

// report renders the freshness summary as a string (so the same text
// feeds both stdout and the --issue tracking-issue body).
func report(minor, major []string) string {
	var b strings.Builder
	if len(minor) == 0 && len(major) == 0 {
		fmt.Fprintln(&b, "deps-outdated: all direct dependencies are on their latest release.")
		return b.String()
	}
	if len(minor) > 0 {
		fmt.Fprintf(&b, "deps-outdated: %d minor/patch update(s) available (same major — low-risk):\n", len(minor))
		for _, l := range minor {
			fmt.Fprintln(&b, l)
		}
	}
	if len(major) > 0 {
		fmt.Fprintf(&b, "deps-outdated: %d new MAJOR version(s) available (review per the comparison-plan rule before bumping):\n", len(major))
		for _, l := range major {
			fmt.Fprintln(&b, l)
		}
	}
	return b.String()
}

// sameMajor reports whether two module versions share a major version.
// A change of major (v1.x => v2.x) is a review-worthy upgrade; same
// major is a routine bump. Unparseable versions (pseudo-versions with
// no clean major) are treated conservatively as a major change so they
// surface for a human look rather than being silently grouped as safe.
func sameMajor(cur, upd string) bool {
	cm, ok1 := majorOf(cur)
	um, ok2 := majorOf(upd)
	if !ok1 || !ok2 {
		return false
	}
	return cm == um
}

// majorOf extracts the integer major version from a module version
// string ("v1.2.3" → 1, "v2.0.0-rc.1" → 2, "v3.1.0+incompatible" → 3).
// Returns ok=false when there is no clean leading vN.
func majorOf(v string) (int, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, ".-+"); i >= 0 {
		v = v[:i]
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "depsoutdated:", err)
	os.Exit(1)
}
