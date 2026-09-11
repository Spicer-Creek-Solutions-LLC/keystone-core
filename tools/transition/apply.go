// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// transitionLabels are added to every retired issue. They are added, never
// substituted: an issue's existing labels are preserved so the archive keeps
// the classification the original work carried.
var transitionLabels = []string{"status/superseded", "generation/1"}

// Journal records what an apply has already done, so an interrupted run resumes
// instead of repeating. Each step is recorded after the call that performed it
// returns, so a crash between call and record costs at most one idempotent
// repeat rather than a lost step.
type Journal struct {
	AllowlistSHA string          `json:"allowlist_sha256"`
	SnapshotSHA  string          `json:"snapshot_sha256"`
	Forge        Forge           `json:"forge"`
	Steps        map[string]Step `json:"steps"`
	path         string
}

// Step is the per-issue progress record. Commenting is not idempotent on a
// forge — a repeated apply would post a second comment — so the journal is what
// makes resumption safe rather than merely convenient.
type Step struct {
	Commented bool `json:"commented"`
	Labelled  bool `json:"labelled"`
	Closed    bool `json:"closed"`
}

func loadJournal(path string, al *Allowlist) (*Journal, error) {
	j := &Journal{
		AllowlistSHA: al.SHA256, SnapshotSHA: al.SnapshotHash,
		Forge: al.Forge, Steps: map[string]Step{}, path: path,
	}
	b, err := os.ReadFile(path) // #nosec G304 G703 -- operator-supplied --journal path
	if os.IsNotExist(err) {
		return j, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read journal: %w", err)
	}
	var prev Journal
	if err := json.Unmarshal(b, &prev); err != nil {
		return nil, fmt.Errorf("parse journal: %w", err)
	}
	// A journal from a different allowlist describes a different operation.
	// Resuming across that boundary would apply reviewed decisions to unreviewed
	// issues, so it fails closed rather than starting over.
	if prev.AllowlistSHA != al.SHA256 {
		return nil, fmt.Errorf("journal %s belongs to allowlist %s, not %s; start a fresh journal or re-review",
			path, short(prev.AllowlistSHA), short(al.SHA256))
	}
	if !prev.Forge.equal(al.Forge) {
		return nil, fmt.Errorf("journal %s was written against %s, not %s", path, prev.Forge.repo(), al.Forge.repo())
	}
	if prev.Steps == nil {
		prev.Steps = map[string]Step{}
	}
	prev.path = path
	return &prev, nil
}

func (j *Journal) get(n int) Step { return j.Steps[strconv.Itoa(n)] }
func (j *Journal) set(n int, s Step) error {
	j.Steps[strconv.Itoa(n)] = s
	return j.save()
}

func (j *Journal) save() error {
	b, err := json.MarshalIndent(j, "", " ")
	if err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	// Rename so an interrupted write cannot leave a half-written journal, which
	// would strand the operation between "resume" and "start over".
	return os.Rename(tmp, j.path)
}

func (j *Journal) done() (commented, labelled, closed int) {
	for _, s := range j.Steps {
		if s.Commented {
			commented++
		}
		if s.Labelled {
			labelled++
		}
		if s.Closed {
			closed++
		}
	}
	return
}

// checkPreconditions re-reads one issue and refuses to proceed on drift from
// what was reviewed. It runs before the first mutation and again before each
// close, because an issue can move between the comment and the close of a long
// run.
//
// identityOnly narrows the check to the fingerprint. It is set once this run
// (or an interrupted earlier one) has already commented or labelled the issue,
// because those are our own writes and they bump updated_at. Comparing against
// the reviewed timestamp then would make the tool fail its own precondition on
// resume — which it did, until the interrupted-run test caught it.
func checkPreconditions(c *client, e AllowEntry, identityOnly bool) error {
	var live apiIssue
	if err := c.do("GET", "/repos/"+c.repo+"/issues/"+strconv.Itoa(e.Number), nil, &live); err != nil {
		return fmt.Errorf("issue #%d: re-read failed: %w", e.Number, err)
	}
	if got := fingerprint(live.Number, live.Title, live.CreatedAt); got != e.Identity {
		return fmt.Errorf("issue #%d: identity changed since review (title or creation time differs)", e.Number)
	}
	if !identityOnly && live.State != e.State {
		return fmt.Errorf("issue #%d: state is %q, was %q when reviewed", e.Number, live.State, e.State)
	}
	if !identityOnly && live.UpdatedAt != e.UpdatedAt {
		return fmt.Errorf("issue #%d: updated_at is %s, was %s when reviewed; someone touched it since",
			e.Number, live.UpdatedAt, e.UpdatedAt)
	}
	return nil
}

// Apply performs the retirement. Order is fixed: comment, then labels, then
// close. The comment must land before the close so the explanation is visible
// on an issue nobody is watching any more, and the labels before the close so a
// reader filtering by status/superseded finds it.
func Apply(c *client, al *Allowlist, j *Journal, comment func(AllowEntry) string, dryRun bool, out *strings.Builder) error {
	if err := al.verifySelf(); err != nil {
		return err
	}
	entries := al.order()

	// Preconditions for the whole set before the first mutation, so a drifted
	// tracker is discovered before anything has been changed rather than
	// halfway through.
	if !dryRun {
		for _, e := range entries {
			st := j.get(e.Number)
			if st.Closed {
				continue // already retired by an earlier run
			}
			// An issue this operation has already started carries our own
			// writes; only its identity can still be meaningfully compared.
			started := st.Commented || st.Labelled
			if err := checkPreconditions(c, e, started); err != nil {
				return fmt.Errorf("precondition failed, nothing applied: %w", err)
			}
		}
	}

	for _, e := range entries {
		st := j.get(e.Number)
		if st.Commented && st.Labelled && st.Closed {
			continue
		}
		if dryRun {
			fmt.Fprintf(out, "  #%-5d %-7s comment+labels+close  %s\n", e.Number, e.Kind, truncate(e.Title, 58))
			continue
		}

		if !st.Commented {
			// A 500 on a comment POST is ambiguous: the comment may have been
			// created before the response failed, and the journal would not know.
			// Resuming blindly would leave two identical retirement notices on a
			// public issue, so check first. Cheap, and only on resume.
			already, err := hasRetirementComment(c, e.Number)
			if err != nil {
				return fmt.Errorf("issue #%d: checking for an existing retirement comment: %w", e.Number, err)
			}
			if already {
				fmt.Fprintf(out, "  #%-5d retirement comment already present, not repeating\n", e.Number)
				st.Commented = true
				if err := j.set(e.Number, st); err != nil {
					return err
				}
			}
		}
		if !st.Commented {
			body := map[string]string{"body": comment(e)}
			if err := c.do("POST", "/repos/"+c.repo+"/issues/"+strconv.Itoa(e.Number)+"/comments", body, nil); err != nil {
				return fmt.Errorf("issue #%d: comment: %w", e.Number, err)
			}
			st.Commented = true
			if err := j.set(e.Number, st); err != nil {
				return err
			}
		}
		if !st.Labelled {
			body := map[string][]string{"labels": transitionLabels}
			if err := c.do("POST", "/repos/"+c.repo+"/issues/"+strconv.Itoa(e.Number)+"/labels", body, nil); err != nil {
				return fmt.Errorf("issue #%d: labels: %w", e.Number, err)
			}
			st.Labelled = true
			if err := j.set(e.Number, st); err != nil {
				return err
			}
		}
		if !st.Closed {
			// Re-check immediately before the close. The comment and label this
			// run just posted change updated_at, so only identity is compared.
			if err := checkPreconditions(c, e, true); err != nil {
				return fmt.Errorf("issue #%d: precondition failed before close: %w", e.Number, err)
			}
			body := map[string]string{"state": "closed"}
			if err := c.do("PATCH", "/repos/"+c.repo+"/issues/"+strconv.Itoa(e.Number), body, nil); err != nil {
				return fmt.Errorf("issue #%d: close: %w", e.Number, err)
			}
			st.Closed = true
			if err := j.set(e.Number, st); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "  #%-5d retired\n", e.Number)
	}
	return nil
}

// Verify re-reads every allowlisted issue and confirms the postconditions, and
// separately confirms that nothing outside the allowlist changed since the
// snapshot.
func Verify(c *client, al *Allowlist, snap *Snapshot, out *strings.Builder) []string {
	var problems []string
	for _, e := range al.order() {
		var live apiIssue
		if err := c.do("GET", "/repos/"+c.repo+"/issues/"+strconv.Itoa(e.Number), nil, &live); err != nil {
			problems = append(problems, fmt.Sprintf("#%d: re-read failed: %v", e.Number, err))
			continue
		}
		if live.State != "closed" {
			problems = append(problems, fmt.Sprintf("#%d: state is %q, expected closed", e.Number, live.State))
		}
		have := map[string]bool{}
		for _, l := range live.Labels {
			have[l.Name] = true
		}
		for _, want := range transitionLabels {
			if !have[want] {
				problems = append(problems, fmt.Sprintf("#%d: missing label %s", e.Number, want))
			}
		}
	}

	// Nothing outside the allowlist may have moved. This is the check that
	// proves the blast radius was what was reviewed.
	for _, is := range snap.Issues {
		if al.contains(is.Number) {
			continue
		}
		var live apiIssue
		if err := c.do("GET", "/repos/"+c.repo+"/issues/"+strconv.Itoa(is.Number), nil, &live); err != nil {
			problems = append(problems, fmt.Sprintf("#%d (not allowlisted): re-read failed: %v", is.Number, err))
			continue
		}
		if live.State != is.State || live.UpdatedAt != is.UpdatedAt {
			problems = append(problems, fmt.Sprintf("#%d is outside the allowlist but changed since the snapshot", is.Number))
		}
	}
	sort.Strings(problems)
	fmt.Fprintf(out, "  checked %d allowlisted and %d non-allowlisted issues\n",
		len(al.Entries), len(snap.Issues)-len(al.Entries))
	return problems
}

// retirementMarker is the first line of the retirement comment. It identifies
// a comment this tool posted, so a resume can tell "already done" from "not yet".
const retirementMarker = "**Superseded, not completed.**"

// hasRetirementComment reports whether this tool has already commented on an
// issue, regardless of what the journal believes.
func hasRetirementComment(c *client, number int) (bool, error) {
	var batch []struct {
		Body string `json:"body"`
	}
	if err := c.do("GET", "/repos/"+c.repo+"/issues/"+strconv.Itoa(number)+"/comments", nil, &batch); err != nil {
		return false, err
	}
	for _, cm := range batch {
		if strings.Contains(cm.Body, retirementMarker) {
			return true, nil
		}
	}
	return false, nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
