package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// areaKeywords maps a lowercase substring to an area/* label. Matching is
// deliberately conservative: an area label is only attached on a confident hit,
// otherwise it is left off for a human to add. Order is irrelevant; all matches
// apply.
var areaKeywords = []struct{ kw, area string }{
	{"stdlib", "area/statemgmt"},
	{"statemgmt", "area/statemgmt"},
	{"state.apply", "area/statemgmt"},
	{"saga", "area/statemgmt"},
	{"resolver", "area/statemgmt"},
	{"agent", "area/agent"},
	{"nats", "area/nats"},
	{"golang-migrate", "area/schema"},
	{"migration journal", "area/schema"},
	{"schema versioning", "area/schema"},
	{"foreign key", "area/schema"},
	{"bootstrap", "area/bootstrap"},
	{"boostrap", "area/bootstrap"}, // matches the typo'd heading in V1X-BACKLOG.md
	{"policy", "area/policy"},
	{"grpc", "area/server"},
	{"dispatch", "area/server"},
	{"batch job", "area/server"},
	{"rbac", "area/security"},
	{"trust federation", "area/security"},
	{"replay protection", "area/security"},
	{"encryption at rest", "area/security"},
	{"creds", "area/security"},
	{"telemetry", "area/observability"},
	{"observability", "area/observability"},
}

func inferAreas(text string) []string {
	low := strings.ToLower(text)
	seen := map[string]bool{}
	var areas []string
	for _, k := range areaKeywords {
		if strings.Contains(low, k.kw) && !seen[k.area] {
			seen[k.area] = true
			areas = append(areas, k.area)
		}
	}
	sort.Strings(areas)
	return areas
}

// labelNamesFor returns the label names that issue for e should carry:
// the umbrella + source labels, the `v1.0-narrowing` marker for narrowings,
// a `kind/*`, and any confidently-inferred `area/*`.
func labelNamesFor(e backlogEntry) []string {
	names := []string{"v1x-backlog", "source/v1x-backlog"}
	if e.Narrowing {
		names = append(names, "v1.0-narrowing")
	}
	if e.Version != "" {
		names = append(names, "kind/feature")
	} else {
		names = append(names, "kind/chore")
	}
	return append(names, inferAreas(e.Title+"\n"+e.Body)...)
}

// labelsFor is labelNamesFor restricted to labels that exist in the repo.
func labelsFor(e backlogEntry, have map[string]int64) []int64 {
	var ids []int64
	for _, n := range labelNamesFor(e) {
		if id, ok := have[n]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func issueBody(e backlogEntry) string {
	var b strings.Builder
	b.WriteString(e.Body)
	b.WriteString("\n\n---\n")
	switch {
	case e.Narrowing && e.Version != "":
		fmt.Fprintf(&b, "Source: `docs/project/V1X-BACKLOG.md` — *Implementation-time narrowings* → `### Targeted: %s`. ", e.Version)
	case e.Narrowing:
		b.WriteString("Source: `docs/project/V1X-BACKLOG.md` — *Implementation-time narrowings* (unscheduled). ")
	default:
		fmt.Fprintf(&b, "Source: `docs/project/V1X-BACKLOG.md` — section `## %s`. ", e.Version)
	}
	b.WriteString("That file holds the authoritative entry; on completion update it per `docs/project/ISSUE-TRACKING.md` §6.\n")
	b.WriteString("\n_Filed by `tools/trackerctl gen-issues`._")
	return b.String()
}

// narrowingsVersionTag is the synthetic selector that matches every entry in
// the "Implementation-time narrowings" section regardless of its target.
const narrowingsVersionTag = "v1.0-narrowing"

// selectEntries restricts entries to the requested selectors. An empty slice
// means "all". A selector of the form vX.Y matches any entry whose target
// version is vX.Y (including narrowings targeted there); narrowingsVersionTag
// matches every narrowing.
func selectEntries(entries []backlogEntry, versions []string) []backlogEntry {
	if len(versions) == 0 {
		return entries
	}
	want := make(map[string]bool, len(versions))
	for _, v := range versions {
		want[v] = true
	}
	var out []backlogEntry
	for _, e := range entries {
		switch {
		case e.Version != "" && want[e.Version]:
			out = append(out, e)
		case e.Narrowing && want[narrowingsVersionTag]:
			out = append(out, e)
		}
	}
	return out
}

func genIssues(c *client, backlogPath string, versions []string, apply bool, out io.Writer) error {
	// #nosec G304 -- backlogPath is the operator-supplied --backlog flag (defaults to docs/project/V1X-BACKLOG.md); this is a CLI admin tool.
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
	if len(entries) == 0 {
		fmt.Fprintf(out, "no backlog entries match the requested versions %v\n", versions)
		return nil
	}

	issues, err := c.listIssues()
	if err != nil {
		return err
	}
	haveTitle := make(map[string]bool, len(issues))
	for _, is := range issues {
		haveTitle[is.Title] = true
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

	var created, skipped, missingMilestone int
	for _, e := range entries {
		if haveTitle[e.Title] {
			skipped++
			continue
		}
		var mid int64
		if e.Version != "" {
			id, ok := mileID[e.Version]
			if !ok {
				fmt.Fprintf(out, "! skip %q: milestone %q does not exist (run sync-milestones)\n", e.Title, e.Version)
				missingMilestone++
				continue
			}
			mid = id
		}
		lids := labelsFor(e, labelID)
		fmt.Fprintf(out, "+ issue %q [milestone=%s labels=%s]\n", e.Title, milestoneName(e), strings.Join(labelNamesFor(e), ","))
		created++
		if apply {
			if _, err := c.createIssue(issuePayload{
				Title:     e.Title,
				Body:      issueBody(e),
				Labels:    lids,
				Milestone: mid,
			}); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(out, "issues: %d to create, %d already present, %d skipped (missing milestone)\n", created, skipped, missingMilestone)
	return nil
}

func milestoneName(e backlogEntry) string {
	if e.Version == "" {
		return "(none)"
	}
	return e.Version
}
