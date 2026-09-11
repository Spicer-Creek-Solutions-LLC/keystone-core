// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// archivedReleaseTags are the Generation 1 releases that get an archived
// notice. They are named rather than discovered: "annotate every release"
// would silently annotate a Generation 2 release the day one is cut.
var archivedReleaseTags = []string{"v0.1.0", "v0.5.0"}

// wantPStageWorkstreams is how many P-stage workstreams the execution plan
// defines. Asserted rather than counted so a plan that silently loses one does
// not silently publish eleven epics.
const wantPStageWorkstreams = 12

// runPublishPlan composes the publication plan from the execution plan and the
// live forge, and writes it for review. It is read-only against the forge.
func runPublishPlan(c *client, o opts, out *strings.Builder) error {
	if o.planPath == "" {
		return fmt.Errorf("publish-plan: --plan <path> is required")
	}
	doc, err := os.ReadFile(o.source) // #nosec G304 G703 -- operator-supplied --source path
	if err != nil {
		return fmt.Errorf("read execution plan: %w", err)
	}

	var repo apiRepo
	if err := c.do("GET", "/repos/"+c.repo, nil, &repo); err != nil {
		return fmt.Errorf("read repository: %w", err)
	}
	owner, name, ok := strings.Cut(c.repo, "/")
	if !ok {
		return fmt.Errorf("repository %q is not owner/name", c.repo)
	}
	forge := Forge{Host: strings.TrimRight(c.base, "/"), Owner: owner, Name: name, RepoID: repo.ID}

	notes, err := resolveReleases(c, archivedReleaseTags)
	if err != nil {
		return err
	}

	p, err := buildPubPlan(string(doc), forge, notes, time.Now, wantPStageWorkstreams)
	if err != nil {
		return err
	}
	if err := checkBodyLinks(p, o.repoRoot); err != nil {
		return err
	}
	b, err := p.marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(o.planPath, b, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n\nplan written to %s\n", p.summary(), o.planPath)
	for _, is := range p.Issues {
		fmt.Fprintf(out, "  %-16s %-12s %s\n", is.TaskID, is.Kind, truncate(is.Title, 56))
	}
	return nil
}

// resolveReleases turns tag names into the release IDs and notices the plan
// records. Resolving at plan time rather than apply time means the reviewer
// approves a specific release, not a tag that could be re-pointed afterwards.
func resolveReleases(c *client, tags []string) ([]ReleaseNote, error) {
	var notes []ReleaseNote
	for _, tag := range tags {
		var rel apiRelease
		if err := c.do("GET", "/repos/"+c.repo+"/releases/tags/"+tag, nil, &rel); err != nil {
			return nil, fmt.Errorf("resolve release %s: %w", tag, err)
		}
		notes = append(notes, ReleaseNote{Tag: tag, ID: rel.ID, Notice: releaseNotice(tag)})
	}
	return notes, nil
}

func runPublish(c *client, o opts, out *strings.Builder) error {
	p, err := readPubPlan(o.planPath)
	if err != nil {
		return err
	}
	if err := p.verifySelf(); err != nil {
		return err
	}
	// The reviewer approves a digest, not a file path — the same rule apply
	// follows. A plan regenerated after review cannot be published by muscle
	// memory, and regeneration is likely here because the plan carries a
	// timestamp and the source document's hash.
	if o.expectPlan != "" && o.expectPlan != p.SHA256 {
		return fmt.Errorf("plan hashes to %s but --expect-plan-sha256 says %s", p.SHA256, o.expectPlan)
	}
	if o.apply && o.expectPlan == "" {
		return fmt.Errorf("--apply requires --expect-plan-sha256 naming the reviewed digest")
	}
	if err := checkForge(c, p.Forge); err != nil {
		return err
	}
	j, err := loadPubJournal(o.journal, p)
	if err != nil {
		return err
	}
	if !o.apply {
		fmt.Fprintf(out, "DRY RUN — no changes will be made\n%s\n\n", p.summary())
	}
	if err := Publish(c, p, j, !o.apply, out); err != nil {
		return err
	}
	created := 0
	for _, st := range j.Issues {
		if st.Number != 0 {
			created++
		}
	}
	if o.apply {
		fmt.Fprintf(out, "\npublished: %d issues exist, %d releases annotated\n", created, len(j.Releases))
	} else {
		fmt.Fprintf(out, "\n%d issues would be published; pass --apply --expect-plan-sha256 %s to perform it\n",
			len(p.Issues)-created, p.SHA256)
	}
	return nil
}

func runPublishVerify(c *client, o opts, out *strings.Builder) error {
	p, err := readPubPlan(o.planPath)
	if err != nil {
		return err
	}
	if err := p.verifySelf(); err != nil {
		return err
	}
	if err := checkForge(c, p.Forge); err != nil {
		return err
	}
	problems := VerifyPublication(c, p, out)
	if len(problems) > 0 {
		fmt.Fprintf(out, "\n%d problem(s):\n", len(problems))
		for _, pr := range problems {
			fmt.Fprintf(out, "  - %s\n", pr)
		}
		return fmt.Errorf("verification failed")
	}
	fmt.Fprint(out, "\npublish-verify: ok — every planned issue exists exactly once with its reviewed body,\n")
	fmt.Fprint(out, "  none was deduplicated against a Generation 1 issue, no replacement was created\n")
	fmt.Fprint(out, "  for archived issue #232, and both archived releases carry the notice.\n")
	return nil
}
