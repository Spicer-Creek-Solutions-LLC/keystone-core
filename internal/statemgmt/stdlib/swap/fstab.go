package swap

import (
	"fmt"
	"sort"
	"strings"
)

// A swap fstab line has the form:
//
//	<source> none swap <opts> 0 0
//
// so swap entries are keyed by the *source* (field 1), not the mount
// point (always "none"). This file is the swap-specific fstab editor;
// it does not share the mount module's, which keys by mount point.

const (
	swapMountPoint = "none"
	swapFSType     = "swap"
)

// desiredOpts is the fstab options string a `swap` declaration wants:
// "defaults", plus "pri=<N>" when an explicit priority is set.
func desiredOpts(p *params) string {
	if p.Priority < 0 {
		return "defaults"
	}
	return fmt.Sprintf("defaults,pri=%d", p.Priority)
}

// desiredFstabLine renders the fstab line for the declaration.
func desiredFstabLine(p *params) string {
	return strings.Join([]string{p.Source, swapMountPoint, swapFSType, desiredOpts(p), "0", "0"}, " ")
}

// isSwapLine reports whether line is a swap fstab entry (fstype field
// == "swap"); if so it returns the source and opts fields.
func isSwapLine(line string) (source, opts string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return "", "", false
	}
	f := strings.Fields(t)
	if len(f) < 4 || f[2] != swapFSType {
		return "", "", false
	}
	return f[0], f[3], true
}

func contentLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func renderContent(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// findSwapEntry returns the opts field of the swap line for source
// and whether one was found.
func findSwapEntry(content, source string) (opts string, found bool) {
	for _, ln := range contentLines(content) {
		if s, o, ok := isSwapLine(ln); ok && s == source {
			return o, true
		}
	}
	return "", false
}

// upsertSwapEntry returns content with the swap line for p.Source set
// to desiredFstabLine(p) — replacing an existing one or appending.
func upsertSwapEntry(content string, p *params) string {
	lines := contentLines(content)
	want := desiredFstabLine(p)
	for i, ln := range lines {
		if s, _, ok := isSwapLine(ln); ok && s == p.Source {
			lines[i] = want
			return renderContent(lines)
		}
	}
	return renderContent(append(lines, want))
}

// removeSwapEntry returns content with every swap line for source
// removed.
func removeSwapEntry(content, source string) string {
	lines := contentLines(content)
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if s, _, ok := isSwapLine(ln); ok && s == source {
			continue
		}
		out = append(out, ln)
	}
	return renderContent(out)
}

func optsSetEqual(a, b string) bool {
	sa, sb := splitOpts(a), splitOpts(b)
	if len(sa) != len(sb) {
		return false
	}
	sort.Strings(sa)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

func splitOpts(s string) []string {
	var out []string
	for _, o := range strings.Split(s, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			out = append(out, o)
		}
	}
	return out
}
