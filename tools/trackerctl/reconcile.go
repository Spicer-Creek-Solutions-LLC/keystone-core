// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// isManagedLabel reports whether reconcile-issues is authoritative over a
// label. The `source/*`, `kind/*`, and umbrella labels are derived from the
// backlog and reconciled to match it; `area/*` and anything else are left
// alone so human curation (and the heuristic's near-misses) survive.
func isManagedLabel(name string) bool {
	return strings.HasPrefix(name, "source/") ||
		strings.HasPrefix(name, "kind/") ||
		name == "roadmap-backlog"
}

// managedLabelDelta returns the managed labels to add to / remove from an issue
// so its managed-label set matches wantNames. Non-managed labels in either list
// are ignored.
func managedLabelDelta(wantNames, currentNames []string) (add, remove []string) {
	want := map[string]bool{}
	for _, n := range wantNames {
		if isManagedLabel(n) {
			want[n] = true
		}
	}
	have := map[string]bool{}
	for _, n := range currentNames {
		if isManagedLabel(n) {
			have[n] = true
		}
	}
	for n := range want {
		if !have[n] {
			add = append(add, n)
		}
	}
	for n := range have {
		if !want[n] {
			remove = append(remove, n)
		}
	}
	sort.Strings(add)
	sort.Strings(remove)
	return add, remove
}

// reconcileIssues brings existing issues' milestone and managed labels in line
// with what docs/project/ROADMAP.md now says. Use it after editing the
// roadmap (e.g. moving an entry between priority buckets); gen-issues only
// ever creates, never updates.
func reconcileIssues(c *client, backlogPath string, versions []string, apply bool, out io.Writer) error {
	// #nosec G304 -- backlogPath is the operator-supplied --backlog flag (see gen-issues); this is a CLI admin tool.
	f, err := os.Open(backlogPath)
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := parseBacklog(f)
	if err != nil {
		return err
	}
	entries = selectEntries(entries, versions)

	issues, err := c.listIssues()
	if err != nil {
		return err
	}
	byTitle := make(map[string]issue, len(issues))
	for _, is := range issues {
		byTitle[is.Title] = is
	}

	labels, err := c.listLabels()
	if err != nil {
		return err
	}
	labelID := make(map[string]int64, len(labels))
	for _, l := range labels {
		labelID[l.Name] = l.ID
	}

	miles, err := c.listMilestones()
	if err != nil {
		return err
	}
	mileID := make(map[string]int64, len(miles))
	for _, m := range miles {
		mileID[m.Title] = m.ID
	}

	var changed, inSync, missing int
	for _, e := range entries {
		is, ok := byTitle[e.Title]
		if !ok {
			fmt.Fprintf(out, "! no issue titled %q yet — run gen-issues\n", e.Title)
			missing++
			continue
		}

		var currentNames []string
		for _, l := range is.Labels {
			currentNames = append(currentNames, l.Name)
		}
		wantToAdd, removeNames := managedLabelDelta(labelNamesFor(e), currentNames)
		var addNames []string
		for _, n := range wantToAdd {
			if _, exists := labelID[n]; exists {
				addNames = append(addNames, n)
			} else {
				fmt.Fprintf(out, "! #%d wants label %q which does not exist (run sync-labels)\n", is.Number, n)
			}
		}

		var wantMid int64
		milestoneKnown := true
		if e.Version != "" {
			id, ok := mileID[e.Version]
			if !ok {
				fmt.Fprintf(out, "! #%d wants milestone %q which does not exist (run sync-milestones)\n", is.Number, e.Version)
				milestoneKnown = false
			} else {
				wantMid = id
			}
		}
		var curMid int64
		curMileName := "(none)"
		if is.Milestone != nil {
			curMid = is.Milestone.ID
			curMileName = is.Milestone.Title
		}
		milestoneNeeds := milestoneKnown && wantMid != curMid

		if len(addNames) == 0 && len(removeNames) == 0 && !milestoneNeeds {
			inSync++
			continue
		}
		changed++
		var parts []string
		if len(addNames) > 0 {
			parts = append(parts, "+["+strings.Join(addNames, ",")+"]")
		}
		if len(removeNames) > 0 {
			parts = append(parts, "-["+strings.Join(removeNames, ",")+"]")
		}
		if milestoneNeeds {
			to := "(none)"
			if wantMid != 0 {
				to = e.Version
			}
			parts = append(parts, fmt.Sprintf("milestone %s→%s", curMileName, to))
		}
		fmt.Fprintf(out, "~ #%d %q: %s\n", is.Number, is.Title, strings.Join(parts, "  "))
		if apply {
			if len(addNames) > 0 {
				ids := make([]int64, 0, len(addNames))
				for _, n := range addNames {
					ids = append(ids, labelID[n])
				}
				if err := c.addIssueLabels(is.Number, ids); err != nil {
					return err
				}
			}
			for _, n := range removeNames {
				if err := c.removeIssueLabel(is.Number, labelID[n]); err != nil {
					return err
				}
			}
			if milestoneNeeds {
				if err := c.editIssueMilestone(is.Number, wantMid); err != nil {
					return err
				}
			}
		}
	}
	fmt.Fprintf(out, "reconcile: %d changed, %d in sync, %d backlog entries without an issue\n", changed, inSync, missing)
	return nil
}
