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
//
// Without --apply the tool reports what it would do and changes nothing.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	host := flag.String("host", "", "Forgejo base URL, e.g. http://192.168.10.21:3000 (required)")
	repo := flag.String("repo", "sbutts/keystone-core", "target repository, owner/name")
	apply := flag.Bool("apply", false, "perform changes; without it the tool only reports a plan")
	backlog := flag.String("backlog", "docs/project/V1X-BACKLOG.md", "path to V1X-BACKLOG.md (gen-issues)")
	flag.Parse()

	cmd := flag.Arg(0)
	if cmd == "" {
		fail("a command is required: sync-labels | sync-milestones | sync | gen-issues")
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
		err = genIssues(c, *backlog, *apply, os.Stdout)
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

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "trackerctl: "+format+"\n", args...)
	os.Exit(1)
}
