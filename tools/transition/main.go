// SPDX-License-Identifier: Apache-2.0

// Command transition retires Generation 1 tracker state as part of the
// Generation 2 reboot (execution plan tasks R04 and R07).
//
// It is deliberately narrow. It comments on, labels, and closes an exact,
// reviewed set of issues, and it never deletes anything — not an issue, not a
// comment, not a label, not a milestone, not a project card. Generation 1
// issues are being marked superseded, not completed, and their content is the
// archive's record of what the project once intended.
//
// Four commands, in the order they are used:
//
//	snapshot   read the tracker at a cutoff instant (GET only)
//	plan       derive an allowlist from a reviewed snapshot and print its digest
//	apply      comment, label, close — dry run unless --apply is passed
//	verify     confirm postconditions, and that nothing else moved
//
// Dry run is the default everywhere. `apply` without `--apply` reports what it
// would do and touches nothing.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	var (
		host       = flag.String("host", "https://codeberg.org", "Forgejo base URL")
		repo       = flag.String("repo", "Spicer-Creek-Solutions-LLC/keystone-core", "target repository, owner/name")
		apply      = flag.Bool("apply", false, "perform changes; without it every command is a dry run")
		snapPath   = flag.String("snapshot", "", "path to the snapshot file (read by plan/verify, written by snapshot)")
		allowPath  = flag.String("allowlist", "", "path to the allowlist file (written by plan, read by apply/verify)")
		journal    = flag.String("journal", "transition-journal.json", "apply progress journal, for resuming an interrupted run")
		expectSHA  = flag.String("expect-allowlist-sha256", "", "refuse to apply unless the allowlist hashes to this reviewed value")
		generation = flag.String("generation", "1", "which generation's issues this allowlist retires")
		exclude    = flag.String("exclude", "", "comma-separated issue numbers to leave alone, e.g. issues an automation still writes")
		throttle   = flag.Duration("throttle", 0, "pause before each mutating request")
		maxWait    = flag.Duration("max-wait", 0, "budget for waiting out a server-stated rate-limit window; 0 = fail fast")
	)
	flag.Parse()

	if err := run(flag.Arg(0), opts{
		host: *host, repo: *repo, apply: *apply, snapPath: *snapPath,
		allowPath: *allowPath, journal: *journal, expectSHA: *expectSHA,
		generation: *generation, exclude: *exclude, throttle: *throttle, maxWait: *maxWait,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "transition:", err)
		os.Exit(1)
	}
}

type opts struct {
	host, repo          string
	apply               bool
	snapPath, allowPath string
	journal, expectSHA  string
	generation, exclude string
	throttle, maxWait   time.Duration
}

func run(cmd string, o opts) error {
	token := os.Getenv("FORGEJO_TOKEN")
	if token == "" && cmd != "plan" {
		return fmt.Errorf("FORGEJO_TOKEN is not set")
	}
	c := newClient(o.host, o.repo, token, o.throttle, o.maxWait)
	var out strings.Builder
	defer func() { fmt.Print(out.String()) }()

	switch cmd {
	case "snapshot":
		if o.snapPath == "" {
			return fmt.Errorf("snapshot: --snapshot <path> is required")
		}
		snap, err := takeSnapshot(c, o.host, time.Now)
		if err != nil {
			return err
		}
		b, err := snap.marshal()
		if err != nil {
			return err
		}
		// #nosec G304 G703 -- operator-supplied --snapshot output path.
		if err := os.WriteFile(o.snapPath, b, 0o600); err != nil {
			return err
		}
		fmt.Fprintf(&out, "snapshot written to %s\n  cutoff: %s\n  issues: %d open, %d opened after cutoff\n  milestones: %d\n  response hash: %s\n",
			o.snapPath, snap.Cutoff, len(snap.Issues), len(snap.AfterCutoff), len(snap.Milestones), snap.ResponseHash)
		return nil

	case "plan":
		snap, raw, err := readSnapshot(o.snapPath)
		if err != nil {
			return err
		}
		skip, err := parseExclude(o.exclude)
		if err != nil {
			return err
		}
		al, err := buildAllowlist(snap, raw, o.generation, skip)
		if err != nil {
			return err
		}
		fmt.Fprintf(&out, "%s\n", al.summary())
		if len(snap.AfterCutoff) > 0 {
			fmt.Fprintf(&out, "excluded, opened after cutoff: %v\n", snap.AfterCutoff)
		}
		if len(skip) > 0 {
			fmt.Fprintf(&out, "excluded by --exclude: %v\n", sortedKeys(skip))
		}
		if o.allowPath == "" {
			fmt.Fprint(&out, "\n(no --allowlist given; nothing written)\n")
			return nil
		}
		b, err := al.marshal()
		if err != nil {
			return err
		}
		if err := os.WriteFile(o.allowPath, b, 0o600); err != nil {
			return err
		}
		fmt.Fprintf(&out, "\nallowlist written to %s\n", o.allowPath)
		return nil

	case "apply":
		al, err := readAllowlist(o.allowPath)
		if err != nil {
			return err
		}
		if err := al.verifySelf(); err != nil {
			return err
		}
		// The reviewer approves a digest, not a file path. Requiring it on the
		// command line means an allowlist regenerated after review cannot be
		// applied by muscle memory.
		if o.expectSHA != "" && o.expectSHA != al.SHA256 {
			return fmt.Errorf("allowlist hashes to %s but --expect-allowlist-sha256 says %s", al.SHA256, o.expectSHA)
		}
		if o.apply && o.expectSHA == "" {
			return fmt.Errorf("--apply requires --expect-allowlist-sha256 naming the reviewed digest")
		}
		if err := checkForge(c, al.Forge); err != nil {
			return err
		}
		j, err := loadJournal(o.journal, al)
		if err != nil {
			return err
		}
		if !o.apply {
			fmt.Fprintf(&out, "DRY RUN — no changes will be made\n%s\n\n", al.summary())
		}
		if err := Apply(c, al, j, supersededComment, !o.apply, &out); err != nil {
			return err
		}
		cm, lb, cl := j.done()
		if o.apply {
			fmt.Fprintf(&out, "\napplied: %d commented, %d labelled, %d closed\n", cm, lb, cl)
		} else {
			fmt.Fprintf(&out, "\n%d issues would be retired; pass --apply --expect-allowlist-sha256 %s to perform it\n",
				len(al.Entries), al.SHA256)
		}
		return nil

	case "verify":
		al, err := readAllowlist(o.allowPath)
		if err != nil {
			return err
		}
		snap, _, err := readSnapshot(o.snapPath)
		if err != nil {
			return err
		}
		if err := checkForge(c, al.Forge); err != nil {
			return err
		}
		problems := Verify(c, al, snap, &out)
		if len(problems) > 0 {
			fmt.Fprintf(&out, "\n%d problem(s):\n", len(problems))
			for _, p := range problems {
				fmt.Fprintf(&out, "  - %s\n", p)
			}
			return fmt.Errorf("verification failed")
		}
		fmt.Fprint(&out, "\nverify: ok — every allowlisted issue is closed and labelled, and nothing else moved\n")
		return nil

	default:
		return fmt.Errorf("usage: transition [flags] snapshot|plan|apply|verify (flags precede the subcommand)")
	}
}

// checkForge refuses to operate unless the live repository is exactly the one
// the manifest was reviewed against — host, owner, name and numeric ID. The ID
// matters because owner/name can be reassigned.
func checkForge(c *client, want Forge) error {
	var repo apiRepo
	if err := c.do("GET", "/repos/"+c.repo, nil, &repo); err != nil {
		return fmt.Errorf("read repository: %w", err)
	}
	owner, name, _ := strings.Cut(c.repo, "/")
	got := Forge{Host: strings.TrimRight(c.base, "/"), Owner: owner, Name: name, RepoID: repo.ID}
	if !got.equal(want) {
		return fmt.Errorf("refusing to act: manifest is for %s %s (id %d), this is %s %s (id %d)",
			want.Host, want.repo(), want.RepoID, got.Host, got.repo(), got.RepoID)
	}
	return nil
}

func parseExclude(s string) (map[int]string, error) {
	out := map[int]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(part, "#"))
		if err != nil {
			return nil, fmt.Errorf("--exclude: %q is not an issue number", part)
		}
		out[n] = "excluded by operator"
	}
	return out, nil
}

func sortedKeys(m map[int]string) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
