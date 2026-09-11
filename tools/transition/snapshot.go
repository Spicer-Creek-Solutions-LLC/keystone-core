// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Snapshot is the reviewed record of tracker state at a cutoff instant. R07's
// allowlist is derived from it and never from a live "all open" query, so that
// what is closed is what a human reviewed rather than whatever happens to be
// open at apply time.
type Snapshot struct {
	Schema        string      `json:"schema"`
	SchemaVersion int         `json:"schema_version"`
	Cutoff        string      `json:"cutoff_utc"`
	Forge         Forge       `json:"forge"`
	Issues        []Issue     `json:"issues"`
	Milestones    []Milestone `json:"milestones"`

	// AfterCutoff records issues that appeared while the snapshot was being
	// taken. They are never allowlisted; recording them gives the reviewer an
	// explicit disposition rather than a silent omission.
	AfterCutoff []int `json:"opened_after_cutoff"`

	// ResponseHash covers the normalised issue and milestone records. Two
	// snapshots of the same tracker state hash identically regardless of
	// pagination order or field ordering.
	ResponseHash string `json:"response_hash"`
}

// Forge pins which repository on which host a snapshot describes. Apply fails
// closed unless every field matches, so a manifest reviewed against the test
// forge cannot be applied to the public one.
type Forge struct {
	Host   string `json:"host"`
	Owner  string `json:"owner"`
	Name   string `json:"name"`
	RepoID int64  `json:"repository_id"`
}

func (f Forge) repo() string { return f.Owner + "/" + f.Name }

func (f Forge) equal(o Forge) bool {
	return f.Host == o.Host && f.Owner == o.Owner && f.Name == o.Name && f.RepoID == o.RepoID
}

// Issue is one tracker issue as observed at the cutoff.
type Issue struct {
	Number      int      `json:"number"`
	State       string   `json:"state"`
	Title       string   `json:"title"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Labels      []string `json:"labels"`
	Milestone   string   `json:"milestone,omitempty"`
	MilestoneID int64    `json:"milestone_id,omitempty"`
	Comments    int      `json:"comments"`

	// CommentBodies preserves the discussion itself, not just its size. Closing
	// an issue does not delete its comments, so this is belt and braces — but
	// the archive's promise is that the original content survives, and a record
	// that only counts comments cannot demonstrate that.
	CommentBodies []Comment `json:"comment_bodies,omitempty"`

	// Identity is a fingerprint over the fields that cannot change without the
	// issue becoming a different issue. Generation 1 issues carry no task ID in
	// their bodies — they predate the convention — so identity is established
	// here rather than read from the issue. Generation 2 issues will carry a
	// task ID, and generation-aware matching compares against that.
	Identity string `json:"identity"`
}

// Comment is one comment on an issue, as it stood at the cutoff.
type Comment struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Body      string `json:"body"`
}

// Milestone records a milestone and its before-state, so R07 can close
// milestones without losing what they contained.
type Milestone struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	State        string `json:"state"`
	OpenIssues   int    `json:"open_issues"`
	ClosedIssues int    `json:"closed_issues"`
}

// fingerprint identifies an issue by the things that do not change: its
// number, its original title, and when it was created. Deliberately excludes
// labels, milestone, state and updated_at — all of which R07 itself changes.
func fingerprint(number int, title, createdAt string) string {
	sum := sha256.Sum256([]byte(strconv.Itoa(number) + "\x00" + title + "\x00" + createdAt))
	return hex.EncodeToString(sum[:])
}

// hashRecords produces a stable hash over normalised records. Sorting by number
// first makes the hash independent of the order pages came back in.
func hashRecords(issues []Issue, ms []Milestone) string {
	sorted := append([]Issue(nil), issues...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Number < sorted[j].Number })
	sortedMS := append([]Milestone(nil), ms...)
	sort.Slice(sortedMS, func(i, j int) bool { return sortedMS[i].ID < sortedMS[j].ID })

	h := sha256.New()
	for _, is := range sorted {
		labels := append([]string(nil), is.Labels...)
		sort.Strings(labels)
		fmt.Fprintf(h, "%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\n",
			is.Number, is.State, is.Title, is.CreatedAt, is.UpdatedAt, strings.Join(labels, ","), is.MilestoneID)
	}
	for _, m := range sortedMS {
		fmt.Fprintf(h, "M%d\x00%s\x00%s\x00%d\x00%d\n", m.ID, m.Title, m.State, m.OpenIssues, m.ClosedIssues)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// apiIssue is the subset of Forgejo's issue representation the snapshot needs.
type apiIssue struct {
	Number    int    `json:"number"`
	State     string `json:"state"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Comments  int    `json:"comments"`
	PullReq   *struct {
		Merged bool `json:"merged"`
	} `json:"pull_request"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Milestone *struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"milestone"`
}

type apiComment struct {
	ID   int64 `json:"id"`
	User *struct {
		Login string `json:"login"`
	} `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Body      string `json:"body"`
}

type apiMilestone struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	State        string `json:"state"`
	OpenIssues   int    `json:"open_issues"`
	ClosedIssues int    `json:"closed_issues"`
}

type apiRepo struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

// takeSnapshot reads the tracker at a cutoff. It is read-only: every request it
// makes is a GET.
func takeSnapshot(c *client, host string, now func() time.Time) (*Snapshot, error) {
	var repo apiRepo
	if err := c.do("GET", "/repos/"+c.repo, nil, &repo); err != nil {
		return nil, fmt.Errorf("read repository: %w", err)
	}
	owner, name, ok := strings.Cut(c.repo, "/")
	if !ok {
		return nil, fmt.Errorf("repository %q is not owner/name", c.repo)
	}

	cutoff := now().UTC().Truncate(time.Second)
	snap := &Snapshot{
		Schema:        "keystone-core/tracker-snapshot",
		SchemaVersion: 1,
		Cutoff:        cutoff.Format(time.RFC3339),
		Forge:         Forge{Host: strings.TrimRight(host, "/"), Owner: owner, Name: name, RepoID: repo.ID},
	}

	q := url.Values{"state": {"open"}, "type": {"issues"}}
	err := c.getPaged("/repos/"+c.repo+"/issues", q, func(page int) (int, error) {
		var batch []apiIssue
		if err := c.do("GET", "/repos/"+c.repo+"/issues"+pageQuery(q, page), nil, &batch); err != nil {
			return 0, err
		}
		for _, it := range batch {
			// type=issues should exclude pull requests; belt and braces, because
			// closing a pull request as a superseded issue would be wrong.
			if it.PullReq != nil {
				continue
			}
			created, cerr := time.Parse(time.RFC3339, it.CreatedAt)
			if cerr == nil && created.After(cutoff) {
				snap.AfterCutoff = append(snap.AfterCutoff, it.Number)
				continue
			}
			rec := Issue{
				Number: it.Number, State: it.State, Title: it.Title,
				CreatedAt: it.CreatedAt, UpdatedAt: it.UpdatedAt, Comments: it.Comments,
				Identity: fingerprint(it.Number, it.Title, it.CreatedAt),
			}
			for _, l := range it.Labels {
				rec.Labels = append(rec.Labels, l.Name)
			}
			if it.Milestone != nil {
				rec.Milestone = it.Milestone.Title
				rec.MilestoneID = it.Milestone.ID
			}
			snap.Issues = append(snap.Issues, rec)
		}
		return len(batch), nil
	})
	if err != nil {
		return nil, fmt.Errorf("read issues: %w", err)
	}

	// Comments are fetched separately, per the plan: the issue list does not
	// carry bodies. Only issues that report a non-zero count are queried, so a
	// tracker of generated issues with no discussion costs no extra requests.
	for i := range snap.Issues {
		if snap.Issues[i].Comments == 0 {
			continue
		}
		n := snap.Issues[i].Number
		cq := url.Values{}
		err := c.getPaged("/repos/"+c.repo+"/issues/"+strconv.Itoa(n)+"/comments", cq, func(page int) (int, error) {
			var batch []apiComment
			if err := c.do("GET", "/repos/"+c.repo+"/issues/"+strconv.Itoa(n)+"/comments"+pageQuery(cq, page), nil, &batch); err != nil {
				return 0, err
			}
			for _, cm := range batch {
				author := ""
				if cm.User != nil {
					author = cm.User.Login
				}
				snap.Issues[i].CommentBodies = append(snap.Issues[i].CommentBodies, Comment{
					ID: cm.ID, Author: author, CreatedAt: cm.CreatedAt, UpdatedAt: cm.UpdatedAt, Body: cm.Body,
				})
			}
			return len(batch), nil
		})
		if err != nil {
			return nil, fmt.Errorf("read comments for issue #%d: %w", n, err)
		}
	}

	mq := url.Values{"state": {"all"}}
	err = c.getPaged("/repos/"+c.repo+"/milestones", mq, func(page int) (int, error) {
		var batch []apiMilestone
		if err := c.do("GET", "/repos/"+c.repo+"/milestones"+pageQuery(mq, page), nil, &batch); err != nil {
			return 0, err
		}
		for _, m := range batch {
			snap.Milestones = append(snap.Milestones, Milestone(m))
		}
		return len(batch), nil
	})
	if err != nil {
		return nil, fmt.Errorf("read milestones: %w", err)
	}

	sort.Slice(snap.Issues, func(i, j int) bool { return snap.Issues[i].Number < snap.Issues[j].Number })
	sort.Ints(snap.AfterCutoff)
	snap.ResponseHash = hashRecords(snap.Issues, snap.Milestones)
	return snap, nil
}

func (s *Snapshot) marshal() ([]byte, error) {
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// hashOf is the SHA-256 of a file's exact bytes, used to bind an allowlist to
// the snapshot it came from.
func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
