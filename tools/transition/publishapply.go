// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

// PubJournal records what a publish has already created. Creating an issue is
// not idempotent — a repeated run would produce a second public issue with the
// same text — so the journal is what makes an interrupted run resumable rather
// than merely restartable.
//
// R07 needed this because comment creation is limited to roughly 16 per five
// minutes. Issue creation is tighter still, and tiered, so a run of this size
// being interrupted is the expected case rather than the unlucky one.
type PubJournal struct {
	PlanSHA     string             `json:"plan_sha256"`
	Forge       Forge              `json:"forge"`
	MilestoneID int64              `json:"milestone_id,omitempty"`
	Issues      map[string]PubStep `json:"issues"`
	Releases    map[string]bool    `json:"release_annotations"`
	path        string
}

// PubStep is the per-issue record. Number is the forge's issue number, which is
// the only durable handle on something this tool created.
type PubStep struct {
	Number int  `json:"number"`
	Pinned bool `json:"pinned"`
}

func loadPubJournal(path string, p *PubPlan) (*PubJournal, error) {
	j := &PubJournal{
		PlanSHA: p.SHA256, Forge: p.Forge,
		Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: path,
	}
	b, err := os.ReadFile(path) // #nosec G304 G703 -- operator-supplied --journal path
	if os.IsNotExist(err) {
		return j, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read journal: %w", err)
	}
	var prev PubJournal
	if err := json.Unmarshal(b, &prev); err != nil {
		return nil, fmt.Errorf("parse journal: %w", err)
	}
	// A journal from a different plan describes a different publication.
	// Resuming across that boundary would create issues nobody reviewed while
	// believing the reviewed ones were already done.
	if prev.PlanSHA != p.SHA256 {
		return nil, fmt.Errorf("journal %s belongs to plan %s, not %s; start a fresh journal or re-review",
			path, short(prev.PlanSHA), short(p.SHA256))
	}
	if !prev.Forge.equal(p.Forge) {
		return nil, fmt.Errorf("journal %s was written against %s, not %s", path, prev.Forge.repo(), p.Forge.repo())
	}
	if prev.Issues == nil {
		prev.Issues = map[string]PubStep{}
	}
	if prev.Releases == nil {
		prev.Releases = map[string]bool{}
	}
	prev.path = path
	return &prev, nil
}

func (j *PubJournal) save() error {
	b, err := json.MarshalIndent(j, "", " ")
	if err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}

// --- reading what is already there --------------------------------------

// pubIssueView is an existing tracker issue, with the body this tool needs in
// order to read a task marker out of it.
type pubIssueView struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Milestone *struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"milestone"`
	PullReq *struct{} `json:"pull_request"`
}

// scanTracker reads every issue on the tracker, open and closed, and indexes
// them by the Generation 2 task marker in their bodies.
//
// Reading the whole tracker rather than searching for each ID in turn is the
// cheaper request by a wide margin at this size, and it is the only way to
// answer the question R09 actually has to answer: does any issue anywhere —
// including a closed Generation 1 one — already claim this task ID?
func scanTracker(c *client) (byTask map[string]pubIssueView, all []pubIssueView, err error) {
	byTask = map[string]pubIssueView{}
	q := url.Values{"state": {"all"}, "type": {"issues"}}
	err = c.getPaged("/repos/"+c.repo+"/issues", q, func(page int) (int, error) {
		var batch []pubIssueView
		if e := c.do("GET", "/repos/"+c.repo+"/issues"+pageQuery(q, page), nil, &batch); e != nil {
			return 0, e
		}
		for _, it := range batch {
			if it.PullReq != nil {
				continue
			}
			all = append(all, it)
			if id, ok := markerTaskID(it.Body); ok {
				byTask[id] = it
			}
		}
		return len(batch), nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read tracker: %w", err)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Number < all[j].Number })
	return byTask, all, nil
}

// markerTaskID pulls the task ID out of an issue body, if it carries one.
func markerTaskID(body string) (string, bool) {
	const open = "<!-- keystone-core-task: "
	i := strings.Index(body, open)
	if i < 0 {
		return "", false
	}
	rest := body[i+len(open):]
	j := strings.Index(rest, " -->")
	if j < 0 {
		return "", false
	}
	id := strings.TrimSpace(rest[:j])
	if id == "" {
		return "", false
	}
	return id, true
}

// --- apply --------------------------------------------------------------

type apiCreatedIssue struct {
	Number int `json:"number"`
}

// Publish creates the milestone, the labels and the issues, pins what the plan
// pins, and prepends the archived-release notices.
//
// Order matters and is fixed: labels and milestone first, because an issue
// cannot be created carrying a label or milestone that does not exist yet;
// then issues; then pins; then releases. Pinning is separated from creation so
// that a forge which refuses the pin does not also cost the issue.
func Publish(c *client, p *PubPlan, j *PubJournal, dryRun bool, out *strings.Builder) error {
	if err := p.verifySelf(); err != nil {
		return err
	}

	byTask, all, err := scanTracker(c)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "tracker holds %d issues; %d carry a Generation 2 task marker\n\n", len(all), len(byTask))

	// Anything already published — by an interrupted run, or by a previous
	// apply whose journal was lost — is adopted rather than duplicated. The
	// forge is the authority here, not the journal.
	for _, is := range p.Issues {
		if existing, ok := byTask[is.TaskID]; ok {
			if cur := j.Issues[is.TaskID]; cur.Number == 0 {
				j.Issues[is.TaskID] = PubStep{Number: existing.Number}
				if !dryRun {
					if err := j.save(); err != nil {
						return err
					}
				}
			}
			fmt.Fprintf(out, "  %-16s already exists as #%d, not recreating\n", is.TaskID, existing.Number)
		}
	}

	fmt.Fprint(out, "\nlabels:\n")
	if err := EnsureLabels(c, p.Labels, dryRun, out); err != nil {
		return err
	}

	fmt.Fprint(out, "\nmilestone:\n")
	msID, err := ensureMilestone(c, p.Milestone, j, dryRun, out)
	if err != nil {
		return err
	}

	fmt.Fprint(out, "\nissues:\n")
	for _, is := range p.Issues {
		if st := j.Issues[is.TaskID]; st.Number != 0 {
			continue
		}
		if dryRun {
			fmt.Fprintf(out, "  %-16s %-12s would be created — %s\n", is.TaskID, is.Kind, truncate(is.Title, 52))
			continue
		}
		body := map[string]any{
			"title":  is.Title,
			"body":   is.Body,
			"labels": []string{},
		}
		if len(is.Labels) > 0 {
			ids, lerr := labelIDs(c, is.Labels)
			if lerr != nil {
				return lerr
			}
			body["labels"] = ids
		}
		if is.Milestone && msID != 0 {
			body["milestone"] = msID
		}
		var made apiCreatedIssue
		if err := c.do("POST", "/repos/"+c.repo+"/issues", body, &made); err != nil {
			return fmt.Errorf("create %s: %w", is.TaskID, err)
		}
		j.Issues[is.TaskID] = PubStep{Number: made.Number}
		if err := j.save(); err != nil {
			return err
		}
		fmt.Fprintf(out, "  %-16s created as #%d\n", is.TaskID, made.Number)
	}

	fmt.Fprint(out, "\npins:\n")
	for _, is := range p.Issues {
		if !is.Pinned {
			continue
		}
		st := j.Issues[is.TaskID]
		if st.Pinned {
			fmt.Fprintf(out, "  %-16s already pinned\n", is.TaskID)
			continue
		}
		if dryRun {
			fmt.Fprintf(out, "  %-16s would be pinned\n", is.TaskID)
			continue
		}
		if st.Number == 0 {
			return fmt.Errorf("%s: cannot pin, no issue number recorded", is.TaskID)
		}
		// Pinning may need permissions this token does not have. That is
		// reported rather than worked around: an unpinned announcement is a
		// cosmetic loss, and silently swallowing a permission error would hide
		// a real fact about what the bot can do.
		if err := c.do("POST", "/repos/"+c.repo+"/issues/"+strconv.Itoa(st.Number)+"/pin", nil, nil); err != nil {
			fmt.Fprintf(out, "  %-16s PIN FAILED on #%d: %v\n", is.TaskID, st.Number, err)
			continue
		}
		st.Pinned = true
		j.Issues[is.TaskID] = st
		if err := j.save(); err != nil {
			return err
		}
		fmt.Fprintf(out, "  %-16s pinned (#%d)\n", is.TaskID, st.Number)
	}

	fmt.Fprint(out, "\nrelease annotations:\n")
	for _, r := range p.Releases {
		if err := annotateRelease(c, r, j, dryRun, out); err != nil {
			return err
		}
	}
	return nil
}

func ensureMilestone(c *client, spec MilestoneSpec, j *PubJournal, dryRun bool, out *strings.Builder) (int64, error) {
	var found *apiMilestone
	q := url.Values{"state": {"all"}}
	err := c.getPaged("/repos/"+c.repo+"/milestones", q, func(page int) (int, error) {
		var batch []apiMilestone
		if e := c.do("GET", "/repos/"+c.repo+"/milestones"+pageQuery(q, page), nil, &batch); e != nil {
			return 0, e
		}
		for i, m := range batch {
			if m.Title == spec.Title {
				found = &batch[i]
			}
		}
		return len(batch), nil
	})
	if err != nil {
		return 0, fmt.Errorf("read milestones: %w", err)
	}
	if found != nil {
		fmt.Fprintf(out, "  %-12s already exists (id %d, %s), left untouched\n", spec.Title, found.ID, found.State)
		j.MilestoneID = found.ID
		return found.ID, nil
	}
	if dryRun {
		fmt.Fprintf(out, "  %-12s would be created\n", spec.Title)
		return 0, nil
	}
	var made apiMilestone
	body := map[string]string{"title": spec.Title, "description": spec.Description}
	if err := c.do("POST", "/repos/"+c.repo+"/milestones", body, &made); err != nil {
		return 0, fmt.Errorf("create milestone %s: %w", spec.Title, err)
	}
	j.MilestoneID = made.ID
	if err := j.save(); err != nil {
		return 0, err
	}
	fmt.Fprintf(out, "  %-12s created (id %d)\n", spec.Title, made.ID)
	return made.ID, nil
}

// labelIDs resolves label names to the numeric IDs the issue-creation endpoint
// wants. Forgejo accepts names on some versions and IDs on others; IDs are
// accepted everywhere.
func labelIDs(c *client, names []string) ([]int64, error) {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var ids []int64
	q := url.Values{}
	err := c.getPaged("/repos/"+c.repo+"/labels", q, func(page int) (int, error) {
		var batch []apiLabel
		if e := c.do("GET", "/repos/"+c.repo+"/labels"+pageQuery(q, page), nil, &batch); e != nil {
			return 0, e
		}
		for _, l := range batch {
			if want[l.Name] {
				ids = append(ids, l.ID)
				delete(want, l.Name)
			}
		}
		return len(batch), nil
	})
	if err != nil {
		return nil, fmt.Errorf("resolve labels: %w", err)
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for n := range want {
			missing = append(missing, n)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("labels not found on the forge: %s", strings.Join(missing, ", "))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

type apiRelease struct {
	ID   int64  `json:"id"`
	Tag  string `json:"tag_name"`
	Body string `json:"body"`
}

// annotateRelease prepends the archived notice to a release body. It edits the
// body and nothing else: no asset is touched, no tag is moved, and the original
// notes are kept below the notice rather than replaced.
func annotateRelease(c *client, r ReleaseNote, j *PubJournal, dryRun bool, out *strings.Builder) error {
	var live apiRelease
	if err := c.do("GET", "/repos/"+c.repo+"/releases/"+strconv.FormatInt(r.ID, 10), nil, &live); err != nil {
		return fmt.Errorf("read release %s: %w", r.Tag, err)
	}
	if live.Tag != r.Tag {
		return fmt.Errorf("release id %d is tagged %s, but the plan says %s", r.ID, live.Tag, r.Tag)
	}
	if strings.Contains(live.Body, releaseNoticeMarker) {
		fmt.Fprintf(out, "  %-8s already annotated, not repeating\n", r.Tag)
		j.Releases[r.Tag] = true
		return nil
	}
	if dryRun {
		fmt.Fprintf(out, "  %-8s would be annotated (body %d chars -> %d, assets untouched)\n",
			r.Tag, len(live.Body), len(r.Notice)+len(live.Body)+6)
		return nil
	}
	body := map[string]string{"body": r.Notice + "\n\n---\n\n" + live.Body}
	if err := c.do("PATCH", "/repos/"+c.repo+"/releases/"+strconv.FormatInt(r.ID, 10), body, nil); err != nil {
		return fmt.Errorf("annotate release %s: %w", r.Tag, err)
	}
	j.Releases[r.Tag] = true
	if err := j.save(); err != nil {
		return err
	}
	fmt.Fprintf(out, "  %-8s annotated\n", r.Tag)
	return nil
}

// --- verify -------------------------------------------------------------

// VerifyPublication re-reads everything R09 created and checks the
// postconditions, including the two the acceptance criteria name explicitly:
// that every issue maps to a task, and that no issue was silently deduplicated
// against a closed Generation 1 one.
func VerifyPublication(c *client, p *PubPlan, out *strings.Builder) []string {
	var problems []string

	byTask, all, err := scanTracker(c)
	if err != nil {
		return []string{err.Error()}
	}

	// Every planned issue exists exactly once, with its body, labels and
	// milestone intact.
	for _, want := range p.Issues {
		got, ok := byTask[want.TaskID]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: no issue on the tracker carries this task marker", want.TaskID))
			continue
		}
		if got.State != "open" {
			problems = append(problems, fmt.Sprintf("%s (#%d): state is %q, expected open", want.TaskID, got.Number, got.State))
		}
		if got.Title != want.Title {
			problems = append(problems, fmt.Sprintf("%s (#%d): title is %q, expected %q", want.TaskID, got.Number, got.Title, want.Title))
		}
		if strings.TrimSpace(got.Body) != strings.TrimSpace(want.Body) {
			problems = append(problems, fmt.Sprintf("%s (#%d): body does not match the reviewed plan", want.TaskID, got.Number))
		}
		have := map[string]bool{}
		for _, l := range got.Labels {
			have[l.Name] = true
		}
		for _, l := range want.Labels {
			if !have[l] {
				problems = append(problems, fmt.Sprintf("%s (#%d): missing label %s", want.TaskID, got.Number, l))
			}
		}
		switch {
		case want.Milestone && got.Milestone == nil:
			problems = append(problems, fmt.Sprintf("%s (#%d): not assigned to milestone %s", want.TaskID, got.Number, p.Milestone.Title))
		case want.Milestone && got.Milestone.Title != p.Milestone.Title:
			problems = append(problems, fmt.Sprintf("%s (#%d): milestone is %q, expected %q", want.TaskID, got.Number, got.Milestone.Title, p.Milestone.Title))
		case !want.Milestone && got.Milestone != nil:
			problems = append(problems, fmt.Sprintf("%s (#%d): unexpectedly assigned to milestone %q", want.TaskID, got.Number, got.Milestone.Title))
		}
	}

	// No task marker exists that the plan did not put there. A stray marker
	// would mean something else is claiming a Generation 2 identity.
	planned := map[string]bool{}
	for _, is := range p.Issues {
		planned[is.TaskID] = true
	}
	for id, is := range byTask {
		if !planned[id] {
			problems = append(problems, fmt.Sprintf("#%d carries task marker %s, which is not in the plan", is.Number, id))
		}
	}

	// The deduplication check the acceptance criteria name. A forge that
	// merged a new issue into an old one, or refused it because a closed issue
	// held the title, would show up as a created number at or below the
	// highest pre-existing Generation 1 number.
	maxG1 := 0
	for _, is := range all {
		if _, isG2 := markerTaskID(is.Body); isG2 {
			continue
		}
		if is.Number > maxG1 {
			maxG1 = is.Number
		}
	}
	titleCollisions := 0
	for _, want := range p.Issues {
		got, ok := byTask[want.TaskID]
		if !ok {
			continue
		}
		if got.Number <= maxG1 {
			problems = append(problems, fmt.Sprintf(
				"%s is issue #%d, at or below the highest Generation 1 issue #%d; it was not created fresh",
				want.TaskID, got.Number, maxG1))
		}
		for _, other := range all {
			if other.Number == got.Number {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(other.Title), strings.TrimSpace(want.Title)) {
				titleCollisions++
				problems = append(problems, fmt.Sprintf(
					"%s (#%d) shares a title with #%d (%s); identity must come from the task marker, not the title",
					want.TaskID, got.Number, other.Number, other.State))
			}
		}
	}

	// The negative test for issue automation. R06 disabled the nightly job that
	// maintained #232 and R08 deleted the tool behind it, so the proof required
	// here is that closing #232 has produced no replacement: no other issue
	// carries its title, and no workflow can create one.
	problems = append(problems, verifyNoReplacement(all, 232)...)

	for _, r := range p.Releases {
		var live apiRelease
		if err := c.do("GET", "/repos/"+c.repo+"/releases/"+strconv.FormatInt(r.ID, 10), nil, &live); err != nil {
			problems = append(problems, fmt.Sprintf("release %s: re-read failed: %v", r.Tag, err))
			continue
		}
		if !strings.Contains(live.Body, releaseNoticeMarker) {
			problems = append(problems, fmt.Sprintf("release %s: body does not carry the archived notice", r.Tag))
		}
		if strings.Count(live.Body, releaseNoticeMarker) > 1 {
			problems = append(problems, fmt.Sprintf("release %s: notice appears %d times", r.Tag, strings.Count(live.Body, releaseNoticeMarker)))
		}
	}

	fmt.Fprintf(out, "  %d planned issues checked against %d tracker issues\n", len(p.Issues), len(all))
	fmt.Fprintf(out, "  highest Generation 1 issue: #%d; title collisions with planned issues: %d\n", maxG1, titleCollisions)
	sort.Strings(problems)
	return problems
}

// verifyNoReplacement proves that a closed issue has not been replaced by a
// look-alike. It is the negative test the execution plan requires for issue
// automation: automation that is generation-aware cannot recreate an archived
// issue, and this is what "cannot" looks like from the outside.
func verifyNoReplacement(all []pubIssueView, number int) []string {
	var target *pubIssueView
	for i, is := range all {
		if is.Number == number {
			target = &all[i]
			break
		}
	}
	if target == nil {
		return []string{fmt.Sprintf("negative test: issue #%d not found on the tracker", number)}
	}
	var problems []string
	if target.State != "closed" {
		problems = append(problems, fmt.Sprintf("negative test: issue #%d is %q, expected closed", number, target.State))
	}
	for _, is := range all {
		if is.Number == number {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(is.Title), strings.TrimSpace(target.Title)) {
			problems = append(problems, fmt.Sprintf(
				"negative test: #%d has the same title as archived issue #%d; a replacement was created",
				is.Number, number))
		}
	}
	return problems
}
