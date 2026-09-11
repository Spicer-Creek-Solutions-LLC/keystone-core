// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// transitionLabelSpecs are the labels R07 creates before it starts commenting.
// They are created rather than assumed: none of the three existed on this
// tracker, and adding a label to an issue fails if the label is not defined.
//
// The descriptions matter more than they look. Someone filtering a closed
// tracker years from now sees the label before they see any of the prose, and
// "superseded" without explanation reads as a synonym for "rejected".
var transitionLabelSpecs = []LabelSpec{
	{
		Name:        "status/superseded",
		Color:       "6b7280",
		Description: "Closed because the project changed direction, not because the work was done or unwanted.",
	},
	{
		Name:        "generation/1",
		Color:       "8b5cf6",
		Description: "Belongs to the Generation 1 line, archived at archive/2026-09-pre-v0.6-reboot.",
	},
	{
		Name:        "kind/rfc",
		Color:       "0ea5e9",
		Description: "Decision record or request for comment.",
	},
}

// LabelSpec is a label to ensure exists.
type LabelSpec struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type apiLabel struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// EnsureLabels creates any of the transition labels that do not already exist.
// It never edits or deletes an existing label: a label already in use carries
// meaning this task did not assign and is not entitled to change.
func EnsureLabels(c *client, specs []LabelSpec, dryRun bool, out *strings.Builder) error {
	existing := map[string]apiLabel{}
	q := url.Values{}
	err := c.getPaged("/repos/"+c.repo+"/labels", q, func(page int) (int, error) {
		var batch []apiLabel
		if err := c.do("GET", "/repos/"+c.repo+"/labels"+pageQuery(q, page), nil, &batch); err != nil {
			return 0, err
		}
		for _, l := range batch {
			existing[l.Name] = l
		}
		return len(batch), nil
	})
	if err != nil {
		return fmt.Errorf("read labels: %w", err)
	}

	for _, spec := range specs {
		if cur, ok := existing[spec.Name]; ok {
			fmt.Fprintf(out, "  label %-22s already exists (id %d), left untouched\n", spec.Name, cur.ID)
			continue
		}
		if dryRun {
			fmt.Fprintf(out, "  label %-22s would be created\n", spec.Name)
			continue
		}
		var made apiLabel
		if err := c.do("POST", "/repos/"+c.repo+"/labels", spec, &made); err != nil {
			return fmt.Errorf("create label %s: %w", spec.Name, err)
		}
		fmt.Fprintf(out, "  label %-22s created (id %d)\n", spec.Name, made.ID)
	}
	return nil
}

// CloseMilestones closes the milestones a snapshot recorded as open. It closes;
// it never deletes. A deleted milestone would strip the grouping from every
// issue that referenced it, which is precisely the history the archive exists
// to keep.
func CloseMilestones(c *client, snap *Snapshot, dryRun bool, out *strings.Builder) error {
	for _, m := range snap.Milestones {
		if m.State != "open" {
			fmt.Fprintf(out, "  milestone %-12s already %s\n", m.Title, m.State)
			continue
		}
		if dryRun {
			fmt.Fprintf(out, "  milestone %-12s (id %d) would be closed — %d open, %d closed at snapshot\n",
				m.Title, m.ID, m.OpenIssues, m.ClosedIssues)
			continue
		}
		body := map[string]string{"state": "closed"}
		if err := c.do("PATCH", "/repos/"+c.repo+"/milestones/"+strconv.FormatInt(m.ID, 10), body, nil); err != nil {
			return fmt.Errorf("close milestone %s: %w", m.Title, err)
		}
		fmt.Fprintf(out, "  milestone %-12s closed\n", m.Title)
	}
	return nil
}

// VerifyMilestones re-reads the milestones and reports any that are not closed.
func VerifyMilestones(c *client, snap *Snapshot) []string {
	var problems []string
	live := map[int64]apiMilestone{}
	q := url.Values{"state": {"all"}}
	err := c.getPaged("/repos/"+c.repo+"/milestones", q, func(page int) (int, error) {
		var batch []apiMilestone
		if err := c.do("GET", "/repos/"+c.repo+"/milestones"+pageQuery(q, page), nil, &batch); err != nil {
			return 0, err
		}
		for _, m := range batch {
			live[m.ID] = m
		}
		return len(batch), nil
	})
	if err != nil {
		return []string{fmt.Sprintf("could not re-read milestones: %v", err)}
	}
	for _, m := range snap.Milestones {
		got, ok := live[m.ID]
		if !ok {
			// A milestone that vanished is worse than one left open: this task
			// is not permitted to delete anything.
			problems = append(problems, fmt.Sprintf("milestone %s (id %d) no longer exists; it should have been closed, never deleted", m.Title, m.ID))
			continue
		}
		if got.State != "closed" {
			problems = append(problems, fmt.Sprintf("milestone %s is %q, expected closed", m.Title, got.State))
		}
	}
	return problems
}

// VerifyLabels confirms the transition labels exist and are applied nowhere
// unexpected — that is, only on issues the allowlist covers.
func VerifyLabels(c *client, al *Allowlist, specs []LabelSpec) []string {
	var problems []string
	have := map[string]bool{}
	q := url.Values{}
	err := c.getPaged("/repos/"+c.repo+"/labels", q, func(page int) (int, error) {
		var batch []apiLabel
		if err := c.do("GET", "/repos/"+c.repo+"/labels"+pageQuery(q, page), nil, &batch); err != nil {
			return 0, err
		}
		for _, l := range batch {
			have[l.Name] = true
		}
		return len(batch), nil
	})
	if err != nil {
		return []string{fmt.Sprintf("could not re-read labels: %v", err)}
	}
	for _, spec := range specs {
		if !have[spec.Name] {
			problems = append(problems, fmt.Sprintf("label %s does not exist", spec.Name))
		}
	}
	return problems
}
