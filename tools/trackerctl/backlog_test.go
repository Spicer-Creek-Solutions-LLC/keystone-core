// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

const sampleBacklog = `# Roadmap — Ranked v0.x Backlog

## How to use this file

- **When deferring scope mid-task**: add an entry.

#### Not an entry — under "How to use"
- should be ignored

---

## gate-v0.5 — blocks v0.5 external-tester milestone

#### Network persistence renderers
- **Priority**: gate-v0.5
- **What**: netplan + networkd writers for the network-base modules.

#### Cross-distro reboot detection
- **Priority**: gate-v0.5
- **What**: dnf needs-restarting on RHEL.

## gate-v1.0 — blocks v1.0 SemVer-stability commitment

#### Agent-side cancel propagation
- **Priority**: gate-v1.0
- **What**: signal agents over NATS.
- **Why deferred**: needs a new message type.

#### Schema versioning via golang-migrate
- **Priority**: gate-v1.0
- **What**: adopt golang-migrate.

### Done

#### Already landed thing
- landed in PR #1, must be skipped

## v0.x — desirable pre-v1.0 (no specific gate)

#### Glob matching: no double-star
- **Priority**: v0.x
- **What**: double-star support.

## v1.x — post-v1.0 feature additions

#### TUI monitor
- **Priority**: v1.x
- **What**: kscore-monitor.

#### Windows agent
- **Priority**: v1.x
- **What**: native service wrapper.

## v2.x+ — architectural post-v1.0

#### Federation / supercluster
- **Priority**: v2.x+
- **What**: NATS supercluster + leaf nodes.
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
		"Network persistence renderers",
		"Cross-distro reboot detection",
		"Agent-side cancel propagation",
		"Schema versioning via golang-migrate",
		"Glob matching: no double-star",
		"TUI monitor",
		"Windows agent",
		"Federation / supercluster",
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
	if e := byTitle["Network persistence renderers"]; e.Version != "gate-v0.5" {
		t.Errorf("network renderers: version=%q, want gate-v0.5", e.Version)
	}
	if e := byTitle["Agent-side cancel propagation"]; e.Version != "gate-v1.0" || e.Narrowing {
		t.Errorf("cancel propagation: version=%q narrowing=%v, want gate-v1.0/false", e.Version, e.Narrowing)
	}
	if !strings.Contains(byTitle["Agent-side cancel propagation"].Body, "signal agents over NATS") {
		t.Errorf("cancel propagation body missing expected text: %q", byTitle["Agent-side cancel propagation"].Body)
	}
	if e := byTitle["TUI monitor"]; e.Version != "v1.x" {
		t.Errorf("TUI monitor: version=%q, want v1.x", e.Version)
	}
	if e := byTitle["Federation / supercluster"]; e.Version != "v2.x+" {
		t.Errorf("federation: version=%q, want v2.x+", e.Version)
	}
	if _, ok := byTitle["Already landed thing"]; ok {
		t.Error("entry under ### Done should have been skipped")
	}
	if _, ok := byTitle[`Not an entry — under "How to use"`]; ok {
		t.Error("entry under non-version ## heading should have been skipped")
	}
}

func TestParseBacklogRealFile(t *testing.T) {
	// The committed roadmap must remain parseable and produce work in every
	// priority bucket.
	f, err := os.Open("../../docs/project/ROADMAP.md")
	if err != nil {
		t.Fatalf("open backlog: %v", err)
	}
	defer f.Close()
	entries, err := parseBacklog(f)
	if err != nil {
		t.Fatalf("parseBacklog(ROADMAP.md): %v", err)
	}
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Version]++
		if e.Title == "" {
			t.Error("parsed an entry with an empty title")
		}
	}
	for _, bucket := range []string{"gate-v0.5", "gate-v1.0", "v0.x", "v1.x", "v2.x+"} {
		if counts[bucket] == 0 {
			t.Errorf("expected at least one %s entry in ROADMAP.md", bucket)
		}
	}
}
