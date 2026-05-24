// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

// backlogEntry is one `####` heading parsed out of docs/project/ROADMAP.md.
type backlogEntry struct {
	Title     string // the heading text
	Version   string // priority bucket: "gate-v0.5", "gate-v1.0", "v0.x", "v1.x", "v2.x+"; empty if outside a recognised section
	Narrowing bool   // legacy field — always false post-versioning-rename; kept so callers don't break
	Body      string // markdown lines under the heading (trimmed)
}

var (
	// versionHeadingRe matches a priority-section heading. Captures the
	// bucket name (`gate-v0.5`, `gate-v1.0`, `v0.x`, `v1.x`, `v2.x+`); the
	// rest of the heading text (`— blocks …`) is allowed but ignored.
	versionHeadingRe = regexp.MustCompile(`^##\s+(gate-v\d+\.\d+|v\d+\.x\+?|v\d+\.\d+)(?:\s+—.*)?\s*$`)
	doneHeadingRe    = regexp.MustCompile(`(?i)^###\s+Done`)
)

// parseBacklog extracts every actionable `####` entry. Entries under
// "## How to use this file", under a "### Done" subsection, or under any other
// non-priority `##` heading are skipped. The pre-rename "## Implementation-time
// narrowings" section was retired in the v0.x scheme — narrowings are now
// mixed into the priority buckets directly.
func parseBacklog(r io.Reader) ([]backlogEntry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		entries    []backlogEntry
		curVersion string
		inDone     bool
		cur        *backlogEntry
		body       []string
	)
	flush := func() {
		if cur != nil {
			cur.Body = strings.TrimSpace(strings.Join(body, "\n"))
			entries = append(entries, *cur)
		}
		cur, body = nil, nil
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "## "):
			flush()
			inDone = false
			if m := versionHeadingRe.FindStringSubmatch(line); m != nil {
				curVersion = m[1]
			} else {
				curVersion = "" // e.g. "## How to use this file"
			}
		case strings.HasPrefix(line, "### "):
			flush()
			inDone = doneHeadingRe.MatchString(line)
		case strings.HasPrefix(line, "#### "):
			flush()
			if inDone || curVersion == "" {
				continue
			}
			title := strings.TrimSpace(strings.TrimPrefix(line, "####"))
			cur = &backlogEntry{Title: title, Version: curVersion}
		default:
			if cur != nil {
				body = append(body, line)
			}
		}
	}
	flush()
	return entries, sc.Err()
}
