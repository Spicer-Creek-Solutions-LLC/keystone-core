// Command trackerctl provisions the keystone-core Forgejo issue tracker from
// the configuration checked into this directory: label set, milestones, and
// (for the active release) leaf issues generated from docs/project/V1X-BACKLOG.md.
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
//	gen-issues       create leaf issues from docs/project/V1X-BACKLOG.md (skips ones that already exist)
//	reconcile-issues update existing issues' milestone + managed labels to match the backlog
//	gen-tracker      create/update the "vX.Y — release tracker" issue (--version required)
//
// gen-issues and reconcile-issues default to every backlog entry; pass
// --versions to restrict them, e.g. --versions v1.1 (the recommended way to add
// tickets one release at a time) or --versions v1.1,v1.0-narrowing.
// reconcile-issues is the counterpart to gen-issues: gen-issues only creates,
// reconcile-issues only updates (milestone + source/kind/umbrella labels;
// area/* labels are left alone). Run it after editing V1X-BACKLOG.md.
//
// gen-tracker orders the release's leaf issues per config/release-order.yaml
// (falling back to backlog file order) and preserves ticked checkboxes on
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
	host := flag.String("host", "", "Forgejo base URL, e.g. http://192.168.10.21:3000 (required)")
	repo := flag.String("repo", "sbutts/keystone-core", "target repository, owner/name")
	apply := flag.Bool("apply", false, "perform changes; without it the tool only reports a plan")
	backlog := flag.String("backlog", "docs/project/V1X-BACKLOG.md", "path to V1X-BACKLOG.md (gen-issues)")
	versions := flag.String("versions", "", "gen-issues / reconcile-issues: comma-separated version tags to limit to, e.g. v1.1 or v1.1,v1.0-narrowing (empty = all)")
	version := flag.String("version", "", "gen-tracker: the release whose tracker issue to create/update, e.g. v1.1 (required)")
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

	c := newClient(*host, *repo, token)

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
