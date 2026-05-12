package main

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

// backlogEntry is one `####` heading parsed out of docs/project/V1X-BACKLOG.md.
type backlogEntry struct {
	Title     string // the heading text
	Version   string // target version, e.g. "v1.2"; empty if unscheduled
	Narrowing bool   // true if from "Implementation-time narrowings inside delivered v1.0 features"
	Body      string // markdown lines under the heading (trimmed)
}

var (
	versionHeadingRe   = regexp.MustCompile(`^##\s+(v\d+\.\d+)\s*$`)
	narrowingHeadingRe = regexp.MustCompile(`(?i)^##\s+Implementation-time narrowings`)
	doneHeadingRe      = regexp.MustCompile(`(?i)^###\s+Done`)
	targetedHeadingRe  = regexp.MustCompile(`(?i)^###\s+Targeted:\s*(v\d+\.\d+)\s*$`)
)

// parseBacklog extracts every actionable `####` entry. Entries under
// "## How to use this file", under a "### Done" subsection, or under any other
// non-version `##` heading are skipped. Inside the narrowings section the entry
// version comes from the enclosing "### Targeted: vX.Y" subsection (empty under
// "### Unscheduled" or no subsection).
func parseBacklog(r io.Reader) ([]backlogEntry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		entries    []backlogEntry
		curVersion string
		narrowing  bool
		narrowTgt  string
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
			inDone, narrowTgt = false, ""
			if m := versionHeadingRe.FindStringSubmatch(line); m != nil {
				curVersion, narrowing = m[1], false
			} else if narrowingHeadingRe.MatchString(line) {
				curVersion, narrowing = "", true
			} else {
				curVersion, narrowing = "", false // e.g. "## How to use this file"
			}
		case strings.HasPrefix(line, "### "):
			flush()
			switch {
			case doneHeadingRe.MatchString(line):
				inDone = true
			case narrowing:
				inDone = false
				if m := targetedHeadingRe.FindStringSubmatch(line); m != nil {
					narrowTgt = m[1]
				} else {
					narrowTgt = "" // e.g. "### Unscheduled"
				}
			default:
				inDone = false
			}
		case strings.HasPrefix(line, "#### "):
			flush()
			if inDone || (curVersion == "" && !narrowing) {
				continue
			}
			version := curVersion
			if narrowing {
				version = narrowTgt
			}
			title := strings.TrimSpace(strings.TrimPrefix(line, "####"))
			cur = &backlogEntry{Title: title, Version: version, Narrowing: narrowing}
		default:
			if cur != nil {
				body = append(body, line)
			}
		}
	}
	flush()
	return entries, sc.Err()
}
