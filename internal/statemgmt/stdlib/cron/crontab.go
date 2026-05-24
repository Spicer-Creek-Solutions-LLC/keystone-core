// SPDX-License-Identifier: Apache-2.0

package cron

import "strings"

// markerPrefix introduces the comment line that tags a
// keystone-managed crontab entry. The full marker is
// "# keystone-cron: <id>" and it sits on the line immediately above
// the entry it owns. This lets the module find, update, or remove
// exactly its own entry even when the schedule or command changes —
// the entry line itself carries nothing identifying.
const markerPrefix = "# keystone-cron: "

func markerLine(id string) string { return markerPrefix + id }

// contentLines splits a crontab into its lines, dropping a single
// trailing newline so the slice has no spurious empty last element.
// An empty (or newline-only) crontab yields nil.
func contentLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// renderContent serialises lines back to crontab text — newline-
// separated with a trailing newline, or "" when there are no lines.
func renderContent(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// findJob locates the entry owned by id. found reports whether the
// marker line exists; jobLine is the line directly below it (empty
// if the marker is the file's last line — a degenerate state the
// reconciler heals).
func findJob(content, id string) (jobLine string, found bool) {
	lines := contentLines(content)
	m := markerLine(id)
	for i, ln := range lines {
		if strings.TrimRight(ln, " \t") == m {
			if i+1 < len(lines) {
				return lines[i+1], true
			}
			return "", true
		}
	}
	return "", false
}

// upsertJob returns content with id's entry set to jobLine. If the
// marker exists, its following line is replaced (or appended when the
// marker was the last line). Otherwise a fresh "<marker>\n<jobLine>"
// block is appended.
func upsertJob(content, id, jobLine string) string {
	lines := contentLines(content)
	m := markerLine(id)
	for i, ln := range lines {
		if strings.TrimRight(ln, " \t") == m {
			if i+1 < len(lines) {
				lines[i+1] = jobLine
			} else {
				lines = append(lines, jobLine)
			}
			return renderContent(lines)
		}
	}
	lines = append(lines, m, jobLine)
	return renderContent(lines)
}

// removeJob returns content with id's marker line and the entry line
// directly below it deleted. A marker with no following line drops
// just the marker. Content without id's marker is returned unchanged.
func removeJob(content, id string) string {
	lines := contentLines(content)
	m := markerLine(id)
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == m {
			i++ // also skip the entry line below the marker
			continue
		}
		out = append(out, lines[i])
	}
	return renderContent(out)
}
