// SPDX-License-Identifier: Apache-2.0

// Command trackerctl provisions the keystone-core Forgejo issue tracker from
// the configuration checked into this directory: label set, milestones, and
// (for the active bucket) leaf issues generated from docs/project/ROADMAP.md.
//
// It is idempotent and host-parameterized: the same invocation that set up the
// internal test server is what sets up the production server at announcement
// time (cutover model b — prod is a clean regeneration, not a repo migration).
// See docs/project/ISSUE-TRACKING.md.
//
// Usage:
//
//	FORGEJO_TOKEN=… trackerctl --host http://host:3000 [--repo owner/name] [--apply] <command>
//
// Commands:
//
//	sync-labels      create/update labels from config/labels.yaml
//	sync-milestones  create/update milestones from config/milestones.yaml
//	sync             sync-labels then sync-milestones
//	gen-issues       create leaf issues from docs/project/ROADMAP.md (skips ones that already exist)
//	reconcile-issues update existing issues' milestone + managed labels to match the backlog
//	gen-tracker      create/update the "<bucket> — tracker" issue (--version required)
//
// All flags must precede the subcommand (Go's flag package stops parsing at
// the first positional argument); a flag placed after the subcommand is
// silently ignored.
//
// gen-issues and reconcile-issues default to every backlog entry; pass
// --versions to restrict them, e.g. --versions gate-v0.5 gen-issues (the
// recommended way to add tickets one bucket at a time) or
// --versions gate-v0.5,gate-v1.0 gen-issues.
// reconcile-issues is the counterpart to gen-issues: gen-issues only creates,
// reconcile-issues only updates (milestone + source/kind/umbrella labels;
// area/* labels are left alone). Run it after editing ROADMAP.md.
//
// gen-tracker orders the bucket's leaf issues per config/release-order.yaml
// (falling back to ROADMAP.md file order) and preserves ticked checkboxes on
// re-run.
//
// Without --apply the tool reports what it would do and changes nothing.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	host := flag.String("host", "", "Forgejo base URL, e.g. https://codeberg.org (required)")
	repo := flag.String("repo", "Spicer-Creek-Solutions-LLC/keystone-core", "target repository, owner/name (default is the public Codeberg canonical; pass `--repo sbutts/keystone-core` for the self-hosted test server)")
	apply := flag.Bool("apply", false, "perform changes; without it the tool only reports a plan")
	backlog := flag.String("backlog", "docs/project/ROADMAP.md", "path to ROADMAP.md (gen-issues)")
	versions := flag.String("versions", "", "gen-issues / reconcile-issues: comma-separated priority buckets to limit to, e.g. gate-v0.5 or gate-v0.5,gate-v1.0 (empty = all)")
	version := flag.String("version", "", "gen-tracker: the priority bucket whose tracker issue to create/update, e.g. gate-v0.5 (required)")
	throttle := flag.Duration("throttle", 0, "pause before each create/update request, e.g. 250ms — useful for rate-limited hosts during bulk gen-issues (rate-limit responses are retried with backoff regardless)")
	flag.Parse()

	cmd := flag.Arg(0)
	if cmd == "" {
		fail("a command is required: sync-labels | sync-milestones | sync | gen-issues | reconcile-issues | gen-tracker")
	}
	if *host == "" {
		fail("--host is required")
	}
	token := os.Getenv("FORGEJO_TOKEN")
	if token == "" {
		fail("FORGEJO_TOKEN environment variable is required")
	}
	if *throttle < 0 {
		fail("--throttle must not be negative")
	}

	c := newClient(*host, *repo, token, *throttle)

	var err error
	switch cmd {
	case "sync-labels":
		err = syncLabels(c, *apply, os.Stdout)
	case "sync-milestones":
		err = syncMilestones(c, *apply, os.Stdout)
	case "sync":
		if err = syncLabels(c, *apply, os.Stdout); err == nil {
			err = syncMilestones(c, *apply, os.Stdout)
		}
	case "gen-issues":
		err = genIssues(c, *backlog, splitCSV(*versions), *apply, os.Stdout)
	case "reconcile-issues":
		err = reconcileIssues(c, *backlog, splitCSV(*versions), *apply, os.Stdout)
	case "gen-tracker":
		err = genTracker(c, *backlog, strings.TrimSpace(*version), *apply, os.Stdout)
	default:
		fail("unknown command %q", cmd)
	}
	if err != nil {
		fail("%v", err)
	}
	if !*apply {
		fmt.Fprintln(os.Stderr, "(dry-run — re-run with --apply to perform these changes)")
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "trackerctl: "+format+"\n", args...)
	os.Exit(1)
}
