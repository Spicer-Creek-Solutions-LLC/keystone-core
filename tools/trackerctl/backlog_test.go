package main

import (
	"os"
	"strings"
	"testing"
)

const sampleBacklog = `# v1.x Backlog

## How to use this file

- **When deferring scope mid-task**: add an entry.

#### Not an entry — under "How to use"
- should be ignored

---

## v1.1

#### Agent-side cancel propagation
- **What**: signal agents over NATS.
- **Why deferred**: needs a new message type.

#### Schema versioning via golang-migrate
- **What**: adopt golang-migrate.

### Done

#### Already landed thing
- landed in PR #1, must be skipped

## v1.2

#### TUI monitor
- **What**: kscore-monitor.

## Implementation-time narrowings inside delivered v1.0 features

These are not version-tagged.

#### Cluster-wide HMAC secret (vs per-agent)
- **What**: one secret for the cluster.
`

func TestParseBacklog(t *testing.T) {
	entries, err := parseBacklog(strings.NewReader(sampleBacklog))
	if err != nil {
		t.Fatalf("parseBacklog: %v", err)
	}
	var titles []string
	for _, e := range entries {
		titles = append(titles, e.Title)
	}
	want := []string{
		"Agent-side cancel propagation",
		"Schema versioning via golang-migrate",
		"TUI monitor",
		"Cluster-wide HMAC secret (vs per-agent)",
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries %v, want %d %v", len(entries), titles, len(want), want)
	}
	for i, w := range want {
		if entries[i].Title != w {
			t.Errorf("entry %d: got %q, want %q", i, entries[i].Title, w)
		}
	}

	byTitle := map[string]backlogEntry{}
	for _, e := range entries {
		byTitle[e.Title] = e
	}
	if e := byTitle["Agent-side cancel propagation"]; e.Version != "v1.1" || e.Narrowing {
		t.Errorf("cancel propagation: version=%q narrowing=%v, want v1.1/false", e.Version, e.Narrowing)
	}
	if !strings.Contains(byTitle["Agent-side cancel propagation"].Body, "signal agents over NATS") {
		t.Errorf("cancel propagation body missing expected text: %q", byTitle["Agent-side cancel propagation"].Body)
	}
	if e := byTitle["TUI monitor"]; e.Version != "v1.2" {
		t.Errorf("TUI monitor: version=%q, want v1.2", e.Version)
	}
	if e := byTitle["Cluster-wide HMAC secret (vs per-agent)"]; !e.Narrowing || e.Version != "" {
		t.Errorf("HMAC secret: narrowing=%v version=%q, want true/empty", e.Narrowing, e.Version)
	}
	if _, ok := byTitle["Already landed thing"]; ok {
		t.Error("entry under ### Done should have been skipped")
	}
	if _, ok := byTitle[`Not an entry — under "How to use"`]; ok {
		t.Error("entry under non-version ## heading should have been skipped")
	}
}

func TestParseBacklogRealFile(t *testing.T) {
	// The committed backlog must remain parseable and produce v1.1 work.
	f, err := os.Open("../../docs/project/V1X-BACKLOG.md")
	if err != nil {
		t.Fatalf("open backlog: %v", err)
	}
	defer f.Close()
	entries, err := parseBacklog(f)
	if err != nil {
		t.Fatalf("parseBacklog(V1X-BACKLOG.md): %v", err)
	}
	var v11, narrowings int
	for _, e := range entries {
		switch {
		case e.Narrowing:
			narrowings++
		case e.Version == "v1.1":
			v11++
		}
		if e.Title == "" {
			t.Error("parsed an entry with an empty title")
		}
	}
	if v11 == 0 {
		t.Error("expected at least one v1.1 entry in V1X-BACKLOG.md")
	}
	if narrowings == 0 {
		t.Error("expected at least one narrowings entry in V1X-BACKLOG.md")
	}
}
