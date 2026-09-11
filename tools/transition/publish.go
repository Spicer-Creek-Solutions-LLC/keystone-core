// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PubPlan is the reviewed, fully composed description of everything R09
// publishes: the milestone, the labels, every issue body verbatim, and the
// notice prepended to each archived release.
//
// Bodies are composed here rather than at apply time, and that is the point.
// R07's retirement comment was generated during the apply, so the dry run
// showed which issues would be touched but not the exact words that would
// appear on them. Issue bodies are the most public artifact this reboot
// produces; the reviewer approves the text, not a promise about the text.
type PubPlan struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	Task          string `json:"task"`
	GeneratedAt   string `json:"generated_at"`

	// Source binds the plan to the execution-plan document its epic bodies were
	// extracted from. "Every issue maps to a task below" is the acceptance
	// criterion; extracting rather than retyping is what makes that mechanical
	// instead of a claim.
	Source       string `json:"source_file"`
	SourceSHA256 string `json:"source_sha256"`

	Forge     Forge         `json:"forge"`
	Labels    []LabelSpec   `json:"labels"`
	Milestone MilestoneSpec `json:"milestone"`
	Issues    []PubIssue    `json:"issues"`
	Releases  []ReleaseNote `json:"release_annotations"`

	SHA256 string `json:"plan_sha256"`
}

// MilestoneSpec is the single Generation 2 milestone. There is exactly one:
// a milestone for work nobody has committed to is a promise the project cannot
// keep, which is the lesson of the five Generation 1 milestones that closed
// holding 116 issues between them.
type MilestoneSpec struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// PubIssue is one issue to create, with its body exactly as it will be posted.
type PubIssue struct {
	TaskID    string   `json:"task_id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Labels    []string `json:"labels"`
	Milestone bool     `json:"milestone"`
	Pinned    bool     `json:"pinned"`
	DependsOn string   `json:"depends_on,omitempty"`
}

// ReleaseNote is the notice prepended to an archived release's body. The
// release ID is recorded so apply targets a specific release rather than
// resolving a tag that could be re-pointed.
type ReleaseNote struct {
	Tag    string `json:"tag"`
	ID     int64  `json:"release_id"`
	Notice string `json:"notice"`
}

const (
	pubPlanSchema = "keystone-core/publication-plan"

	// taskMarkerFmt is the machine-readable identity every Generation 2 issue
	// carries in its body. ISSUE-TRACKING.md § Principles makes identity
	// explicit and matching by task ID rather than by title; this is that ID.
	//
	// An HTML comment because it must survive rendering without being visible,
	// and because a title is not identity: two workstreams can be renamed, and
	// a closed Generation 1 issue must never be able to suppress a Generation 2
	// one by resembling it.
	taskMarkerFmt = "<!-- keystone-core-task: %s -->"

	// releaseNoticeMarker identifies a notice this tool already prepended, so a
	// re-run annotates once rather than stacking banners on a public page.
	releaseNoticeMarker = "**Archived and unsupported.**"

	repoMainBase = repoWebBase + "/src/branch/main"
)

func taskMarker(id string) string { return fmt.Sprintf(taskMarkerFmt, id) }

// pubLabelSpecs are the labels R09 needs that R07 did not create. As in R07,
// existing labels are never rewritten: a label already in use carries meaning
// this task did not assign.
var pubLabelSpecs = []LabelSpec{
	{
		Name:        "generation/2",
		Color:       "16a34a",
		Description: "Belongs to the Generation 2 line, which starts from RFC 0001.",
	},
	{
		Name:        "kind/epic",
		Color:       "4338ca",
		Description: "A workstream tracker, not a unit of work. Carries no acceptance criteria of its own.",
	},
	{
		Name:        "kind/tracker",
		Color:       "b45309",
		Description: "Roll-up issue for a release or milestone.",
	},
	{
		Name:        "kind/announcement",
		Color:       "0f766e",
		Description: "A statement of record, not work to be done.",
	},
}

// --- extraction from the execution plan ---------------------------------

var (
	sectionRe = regexp.MustCompile(`(?m)^### (P[01][0-9]) — (.+)$`)
	gateRowRe = regexp.MustCompile(`(?m)^\| (P[01][0-9]) \| (.+?) \|$`)
)

// planSection is one P-stage workstream as the execution plan states it.
type planSection struct {
	ID       string
	Name     string
	Prose    string
	Requires string
}

// extractSections reads the P-stage workstreams out of the execution plan: the
// heading, the prose beneath it up to the next heading, and the "Must follow"
// column of the dependency-gate table.
//
// It is deliberately strict. A section it cannot parse is an error rather than
// an issue created with an empty body, because the failure mode this guards
// against is publishing twelve public issues that describe nothing.
func extractSections(doc string) ([]planSection, error) {
	gates := map[string]string{}
	for _, m := range gateRowRe.FindAllStringSubmatch(doc, -1) {
		gates[m[1]] = strings.TrimSpace(m[2])
	}

	locs := sectionRe.FindAllStringSubmatchIndex(doc, -1)
	if len(locs) == 0 {
		return nil, fmt.Errorf("no P-stage sections found in the execution plan")
	}
	var out []planSection
	for i, loc := range locs {
		id := doc[loc[2]:loc[3]]
		name := strings.TrimSpace(doc[loc[4]:loc[5]])

		end := len(doc)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		// The dependency-gate table follows the last workstream under its own
		// heading; stop at any heading so it is never swallowed into P11.
		body := doc[loc[1]:end]
		if cut := strings.Index(body, "\n### "); cut >= 0 {
			body = body[:cut]
		}
		if cut := strings.Index(body, "\n## "); cut >= 0 {
			body = body[:cut]
		}
		prose := strings.TrimSpace(body)
		if prose == "" {
			return nil, fmt.Errorf("%s has no description in the execution plan", id)
		}
		req, ok := gates[id]
		if !ok {
			return nil, fmt.Errorf("%s has no row in the P-stage dependency-gate table", id)
		}
		out = append(out, planSection{ID: id, Name: name, Prose: prose, Requires: req})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// --- body composition ---------------------------------------------------

func epicBody(s planSection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", taskMarker(s.ID))
	fmt.Fprintf(&b, "**Task `%s` — Generation 2 workstream epic.**\n\n", s.ID)
	fmt.Fprintf(&b, "%s\n\n", s.Prose)
	fmt.Fprintf(&b, "**Must follow:** %s\n\n", s.Requires)

	b.WriteString("### This epic carries no leaf issues yet\n\n")
	b.WriteString("An epic is a workstream, not a unit of work, and it has no acceptance criteria of its own. ")
	b.WriteString("Leaf issues are created only once this workstream has a checked-in dossier ")
	b.WriteString("and its plan has been approved — never in advance. ")
	b.WriteString("The dossier must define depends-on task IDs and commit SHAs, allowed paths, exact outputs, ")
	b.WriteString("applicable requirement IDs, acceptance cases with N/A rationale, validation commands, ")
	b.WriteString("documentation changes, security reviewer, rollback, and handoff artifacts.\n\n")

	fmt.Fprintf(&b, "- Full text of this workstream: [REBOOT-EXECUTION-PLAN.md](%s/docs/project/REBOOT-EXECUTION-PLAN.md) § Stage P\n", repoMainBase)
	fmt.Fprintf(&b, "- Why the project rebooted: [RFC 0001](%s/docs/rfcs/0001-generation-2-reboot.md)\n", repoMainBase)
	fmt.Fprintf(&b, "- What \"shipped\" means here: [TESTING.md](%s/docs/project/TESTING.md)\n", repoMainBase)
	fmt.Fprintf(&b, "- Workflow: [AGENTS.md](%s/AGENTS.md) § 3\n\n", repoMainBase)

	b.WriteString(pubFooter)
	return b.String()
}

const pubFooter = "_Created by `tools/transition publish` (reboot task R09)._"

func trackerBody(sections []planSection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", taskMarker("TRACKER-v0.6.0"))
	b.WriteString("**Release tracker for `v0.6.0`** — the first completed Generation 2 release.\n\n")

	b.WriteString("`v0.6.0` ships when one promise is demonstrably true:\n\n")
	b.WriteString("> An operator can securely enroll a Linux agent, target it, execute a bounded\n")
	b.WriteString("> command over NATS, observe its durable lifecycle, cancel it, and retrieve an\n")
	b.WriteString("> auditable result.\n\n")
	b.WriteString("Demonstrably true means a black-box test drives the production CLI or API, ")
	b.WriteString("crosses the production transport, executes in the intended agent process, ")
	b.WriteString("and verifies the externally observable effect. Package tests support development; ")
	b.WriteString("they do not satisfy acceptance.\n\n")
	b.WriteString("Nothing else is in scope. Everything Generation 1 had is catalogued as a Future ")
	b.WriteString("candidate rather than a commitment, and reaches this milestone only through the ")
	b.WriteString("promotion gate.\n\n")

	b.WriteString("### Foundation workstreams\n\n")
	b.WriteString("No implementation starts until all twelve are accepted. Each is a separate approval and pull request.\n\n")
	b.WriteString("| Workstream | Must follow |\n|---|---|\n")
	for _, s := range sections {
		fmt.Fprintf(&b, "| `%s` — %s | %s |\n", s.ID, s.Name, s.Requires)
	}
	b.WriteString("\n")

	b.WriteString("### Then the behaviour stages\n\n")
	b.WriteString("Stage C (`C01`–`C16`) turns the accepted design into the release. ")
	b.WriteString("It has no issues yet and will not get them until Stage P is accepted: ")
	b.WriteString("each behaviour workstream splits into an acceptance-contract task and an ")
	b.WriteString("implementation task, worked by different agents.\n\n")

	fmt.Fprintf(&b, "- Every task, in order: [REBOOT-EXECUTION-PLAN.md](%s/docs/project/REBOOT-EXECUTION-PLAN.md)\n", repoMainBase)
	fmt.Fprintf(&b, "- Version policy: [VERSIONING.md](%s/docs/project/VERSIONING.md)\n", repoMainBase)
	fmt.Fprintf(&b, "- Roadmap: [ROADMAP.md](%s/docs/project/ROADMAP.md)\n\n", repoMainBase)

	b.WriteString(pubFooter)
	return b.String()
}

func announcementBody() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", taskMarker("ANNOUNCE-reboot"))

	b.WriteString("**Keystone Core has restarted from a narrower base.**\n\n")
	b.WriteString("In September 2026 the project accepted a reboot. The Generation 1 implementation — ")
	b.WriteString("around 1,900 files, two public releases, a large feature surface — was archived ")
	b.WriteString("rather than continued, and the repository now holds planning, governance and ")
	b.WriteString("transition evidence. **There is nothing to install.**\n\n")

	b.WriteString("### Why\n\n")
	b.WriteString("Not because it failed to work. Because its breadth outran the evidence that any ")
	b.WriteString("single operator journey worked end to end. Late fixes had found enrollment ")
	b.WriteString("credentials that were not retained, and state and blueprint applies that did not ")
	b.WriteString("reliably reach remote agents — cases where package-level test coverage sat ")
	b.WriteString("alongside missing production wiring. Adding features would have preserved that ")
	b.WriteString("risk rather than resolved it.\n\n")

	b.WriteString("### Nothing was deleted\n\n")
	b.WriteString("| | |\n|---|---|\n")
	fmt.Fprintf(&b, "| Archive branch | `%s` |\n", archiveRef)
	b.WriteString("| Signed tag | `archive-2026-09-pre-v0.6-reboot` |\n")
	b.WriteString("| Offline bundle | held by the maintainer, SHA-256 in the transition manifest |\n\n")
	b.WriteString("Both refs are protected against update and deletion. The `v0.1.0` and `v0.5.0` ")
	b.WriteString("releases remain published, with their artifacts intact, and are unsupported: ")
	b.WriteString("they receive no updates, including security updates.\n\n")
	b.WriteString("The 106 open Generation 1 issues were closed as **superseded, not completed** — ")
	b.WriteString("closed because the project changed direction, not because the work was finished ")
	b.WriteString("or judged unwanted. Their titles, bodies, comments, labels and milestones are retained.\n\n")

	b.WriteString("### What Generation 2 is\n\n")
	b.WriteString("One promise, deliberately narrow:\n\n")
	b.WriteString("> An operator can securely enroll a Linux agent, target it, execute a bounded\n")
	b.WriteString("> command over NATS, observe its durable lifecycle, cancel it, and retrieve an\n")
	b.WriteString("> auditable result.\n\n")
	b.WriteString("Nothing else ships until that is demonstrably true. State management, blueprints, ")
	b.WriteString("runbooks, plugins, secrets, GitOps, webhooks, policy and clustering are Future ")
	b.WriteString("candidates rather than commitments. Every capability Generation 1 had is ")
	b.WriteString("catalogued with its status, known gaps and archived source, so the reboot discards ")
	b.WriteString("the code without discarding what was learned.\n\n")

	b.WriteString("### Where to read more\n\n")
	fmt.Fprintf(&b, "- [RFC 0001](%s/docs/rfcs/0001-generation-2-reboot.md) — the decision, its reasoning, and the alternatives rejected\n", repoMainBase)
	fmt.Fprintf(&b, "- [REBOOT-EXECUTION-PLAN.md](%s/docs/project/REBOOT-EXECUTION-PLAN.md) — every task, in order\n", repoMainBase)
	fmt.Fprintf(&b, "- [FUTURE-CAPABILITIES.md](%s/docs/project/FUTURE-CAPABILITIES.md) — the archive capability catalog\n", repoMainBase)
	fmt.Fprintf(&b, "- [VERSIONING.md](%s/docs/project/VERSIONING.md) — what the version numbers now mean\n", repoMainBase)
	fmt.Fprintf(&b, "- [docs/transition/](%s/docs/transition/) — the archive manifest and the tracker-retirement record\n\n", repoMainBase)

	b.WriteString("This issue is a statement of record and is not work to be done. ")
	b.WriteString("Questions and objections are welcome as comments.\n\n")

	b.WriteString(pubFooter)
	return b.String()
}

func releaseNotice(tag string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "> %s\n>\n", releaseNoticeMarker)
	fmt.Fprintf(&b, "> `%s` belongs to Generation 1, which the project archived in September 2026\n", tag)
	b.WriteString("> rather than continuing. It receives no updates, including security updates,\n")
	b.WriteString("> and there is no supported release of Keystone Core at this time.\n>\n")
	b.WriteString("> The artifacts below are left in place deliberately — nothing about this\n")
	b.WriteString("> release has been deleted or rewritten. The source it was built from is\n")
	fmt.Fprintf(&b, "> preserved at the protected tag `archive-2026-09-pre-v0.6-reboot`.\n>\n")
	fmt.Fprintf(&b, "> The decision and its reasoning: [RFC 0001](%s/docs/rfcs/0001-generation-2-reboot.md).\n", repoMainBase)
	b.WriteString("> Generation 2 starts from a deliberately narrow promise and has not yet shipped.\n")
	return b.String()
}

// bodyLinkRe matches the repository paths the issue bodies link to.
var bodyLinkRe = regexp.MustCompile(regexp.QuoteMeta(repoMainBase) + `/([A-Za-z0-9._/-]+)`)

// checkBodyLinks resolves every in-repository link an issue body carries.
//
// These links are absolute forge URLs, which the CI link gate cannot help with:
// lychee runs offline and skips external hosts entirely. Nothing else would
// notice a path that moved, and the cost of finding out later is fourteen
// public issues pointing at 404s.
func checkBodyLinks(p *PubPlan, root string) error {
	var bad []string
	for _, is := range p.Issues {
		for _, m := range bodyLinkRe.FindAllStringSubmatch(is.Body, -1) {
			rel := strings.TrimSuffix(m[1], "/")
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
				bad = append(bad, fmt.Sprintf("%s links to %s, which does not exist", is.TaskID, rel))
			}
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("%d broken in-repository link(s):\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
	return nil
}

// --- plan assembly ------------------------------------------------------

// buildPubPlan composes the plan. wantSections is the number of P-stage
// workstreams the caller expects: the count is asserted rather than accepted,
// because a regex that quietly matches eleven of twelve sections would publish
// a tracker whose table is missing a workstream and nothing would say so.
func buildPubPlan(doc string, forge Forge, releases []ReleaseNote, now func() time.Time, wantSections int) (*PubPlan, error) {
	sections, err := extractSections(doc)
	if err != nil {
		return nil, err
	}
	if len(sections) != wantSections {
		return nil, fmt.Errorf("expected %d P-stage workstreams, found %d", wantSections, len(sections))
	}

	p := &PubPlan{
		Schema:        pubPlanSchema,
		SchemaVersion: 1,
		Task:          "R09",
		GeneratedAt:   now().UTC().Truncate(time.Second).Format(time.RFC3339),
		Source:        "docs/project/REBOOT-EXECUTION-PLAN.md",
		SourceSHA256:  hashOf([]byte(doc)),
		Forge:         forge,
		Labels:        pubLabelSpecs,
		Milestone: MilestoneSpec{
			Title: "v0.6.0",
			Description: "The first completed Generation 2 release: secure enrollment, targeted " +
				"bounded command execution over NATS, durable lifecycle, cancellation, and an " +
				"auditable result — each proven by a black-box test against production binaries. " +
				"The single Generation 2 milestone. Later versions get one when the work is accepted.",
		},
		Releases: releases,
	}

	p.Issues = append(p.Issues, PubIssue{
		TaskID: "ANNOUNCE-reboot", Kind: "announcement",
		Title:  "Keystone Core has restarted: Generation 1 is archived, Generation 2 begins",
		Body:   announcementBody(),
		Labels: []string{"generation/2", "kind/announcement"},
		Pinned: true,
	})
	p.Issues = append(p.Issues, PubIssue{
		TaskID: "TRACKER-v0.6.0", Kind: "tracker",
		Title:     "v0.6.0 — release tracker",
		Body:      trackerBody(sections),
		Labels:    []string{"generation/2", "kind/tracker"},
		Milestone: true,
	})
	for _, s := range sections {
		p.Issues = append(p.Issues, PubIssue{
			TaskID: s.ID, Kind: "epic",
			Title:     fmt.Sprintf("%s — %s", s.ID, s.Name),
			Body:      epicBody(s),
			Labels:    []string{"generation/2", "kind/epic"},
			Milestone: true,
			DependsOn: s.Requires,
		})
	}

	if err := p.checkSelfConsistent(); err != nil {
		return nil, err
	}
	p.SHA256 = p.planHash()
	return p, nil
}

// checkSelfConsistent catches the mistakes that would only become visible once
// the issues were public: a duplicate task ID, a body that lost its marker, a
// label the plan never creates.
func (p *PubPlan) checkSelfConsistent() error {
	known := map[string]bool{}
	for _, l := range p.Labels {
		known[l.Name] = true
	}
	seen := map[string]bool{}
	for _, is := range p.Issues {
		if is.TaskID == "" {
			return fmt.Errorf("issue %q has no task ID", is.Title)
		}
		if seen[is.TaskID] {
			return fmt.Errorf("task ID %s appears twice", is.TaskID)
		}
		seen[is.TaskID] = true
		if !strings.Contains(is.Body, taskMarker(is.TaskID)) {
			return fmt.Errorf("%s: body does not carry its task marker", is.TaskID)
		}
		if strings.TrimSpace(is.Title) == "" {
			return fmt.Errorf("%s: empty title", is.TaskID)
		}
		for _, l := range is.Labels {
			if !known[l] {
				return fmt.Errorf("%s: label %s is not in the plan's label list", is.TaskID, l)
			}
		}
	}
	return nil
}

// planHash covers everything a reviewer reads: identity, titles, bodies,
// labels, milestone text, and the release notices. Changing a single word of a
// body changes the digest, so a plan edited after review cannot be applied
// against the approved one.
func (p *PubPlan) planHash() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\n", p.Task, p.Milestone.Title, p.Milestone.Description)
	labels := append([]LabelSpec(nil), p.Labels...)
	sort.Slice(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })
	for _, l := range labels {
		fmt.Fprintf(h, "L\x00%s\x00%s\x00%s\n", l.Name, l.Color, l.Description)
	}
	issues := append([]PubIssue(nil), p.Issues...)
	sort.Slice(issues, func(i, j int) bool { return issues[i].TaskID < issues[j].TaskID })
	for _, is := range issues {
		fmt.Fprintf(h, "I\x00%s\x00%s\x00%s\x00%s\x00%t\x00%t\x00%s\n",
			is.TaskID, is.Kind, is.Title, strings.Join(is.Labels, ","), is.Milestone, is.Pinned, is.Body)
	}
	rels := append([]ReleaseNote(nil), p.Releases...)
	sort.Slice(rels, func(i, j int) bool { return rels[i].Tag < rels[j].Tag })
	for _, r := range rels {
		fmt.Fprintf(h, "R\x00%s\x00%d\x00%s\n", r.Tag, r.ID, r.Notice)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (p *PubPlan) verifySelf() error {
	if err := p.checkSelfConsistent(); err != nil {
		return err
	}
	if got := p.planHash(); got != p.SHA256 {
		return fmt.Errorf("plan hashes to %s but the file records %s; it was edited after review", short(got), short(p.SHA256))
	}
	return nil
}

func (p *PubPlan) marshal() ([]byte, error) {
	b, err := json.MarshalIndent(p, "", " ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (p *PubPlan) summary() string {
	kinds := map[string]int{}
	pinned := 0
	for _, is := range p.Issues {
		kinds[is.Kind]++
		if is.Pinned {
			pinned++
		}
	}
	return "publishing: " + strconv.Itoa(len(p.Issues)) + " issues (" +
		strconv.Itoa(kinds["epic"]) + " epic, " + strconv.Itoa(kinds["tracker"]) + " tracker, " +
		strconv.Itoa(kinds["announcement"]) + " announcement, " + strconv.Itoa(pinned) + " pinned)" +
		"\nmilestone:        " + p.Milestone.Title +
		"\nlabels:           " + strconv.Itoa(len(p.Labels)) +
		"\nrelease notices:  " + strconv.Itoa(len(p.Releases)) +
		"\nplan sha256:      " + p.SHA256 +
		"\nsource sha256:    " + p.SourceSHA256 + " (" + p.Source + ")" +
		"\nforge:            " + p.Forge.Host + " " + p.Forge.repo() + " (id " + strconv.FormatInt(p.Forge.RepoID, 10) + ")"
}

func readPubPlan(path string) (*PubPlan, error) {
	if path == "" {
		return nil, fmt.Errorf("--plan <path> is required")
	}
	b, err := os.ReadFile(path) // #nosec G304 G703 -- operator-supplied --plan path
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	var p PubPlan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	if p.Schema != pubPlanSchema {
		return nil, fmt.Errorf("%s is not a publication plan (schema %q)", path, p.Schema)
	}
	return &p, nil
}
