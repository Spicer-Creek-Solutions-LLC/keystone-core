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
	Version   string // "v1.1", "v1.2", … — empty for the v1.0-narrowings section
	Narrowing bool   // true if from "Implementation-time narrowings inside delivered v1.0 features"
	Body      string // markdown lines under the heading (trimmed)
}

var (
	versionHeadingRe   = regexp.MustCompile(`^##\s+(v\d+\.\d+)\s*$`)
	narrowingHeadingRe = regexp.MustCompile(`(?i)^##\s+Implementation-time narrowings`)
	doneHeadingRe      = regexp.MustCompile(`(?i)^###\s+Done`)
)

// parseBacklog extracts every actionable `####` entry. Entries under
// "## How to use this file", under a "### Done" subsection, or under any other
// non-version `##` heading are skipped.
func parseBacklog(r io.Reader) ([]backlogEntry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		entries    []backlogEntry
		curVersion string
		narrowing  bool
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
				curVersion, narrowing = m[1], false
			} else if narrowingHeadingRe.MatchString(line) {
				curVersion, narrowing = "", true
			} else {
				curVersion, narrowing = "", false // e.g. "## How to use this file"
			}
		case strings.HasPrefix(line, "### "):
			flush()
			inDone = doneHeadingRe.MatchString(line)
		case strings.HasPrefix(line, "#### "):
			flush()
			if inDone || (curVersion == "" && !narrowing) {
				continue
			}
			title := strings.TrimSpace(strings.TrimPrefix(line, "####"))
			cur = &backlogEntry{Title: title, Version: curVersion, Narrowing: narrowing}
		default:
			if cur != nil {
				body = append(body, line)
			}
		}
	}
	flush()
	return entries, sc.Err()
}
