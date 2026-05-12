package mount

import (
	"sort"
	"strconv"
	"strings"
)

// fstabEntry is the parsed shape of an /etc/fstab line keyed by the
// module: <device> <mountpoint> <fstype> <opts> <dump> <pass>.
// Missing dump/pass fields default to 0.
type fstabEntry struct {
	Device     string
	MountPoint string
	FSType     string
	Opts       string
	Dump       int
	Pass       int
}

// desiredEntry builds the fstab entry a `mounted` / `present`
// declaration wants.
func desiredEntry(p *params) fstabEntry {
	return fstabEntry{
		Device:     p.Device,
		MountPoint: p.MountPoint,
		FSType:     p.FSType,
		Opts:       p.Opts,
		Dump:       p.Dump,
		Pass:       p.Pass,
	}
}

// parseFstabLine parses one fstab line into an entry. ok is false for
// blank / comment lines or lines with fewer than 4 fields.
func parseFstabLine(line string) (fstabEntry, bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return fstabEntry{}, false
	}
	f := strings.Fields(t)
	if len(f) < 4 {
		return fstabEntry{}, false
	}
	e := fstabEntry{Device: f[0], MountPoint: f[1], FSType: f[2], Opts: f[3]}
	if len(f) >= 5 {
		e.Dump, _ = strconv.Atoi(f[4])
	}
	if len(f) >= 6 {
		e.Pass, _ = strconv.Atoi(f[5])
	}
	return e, true
}

// render serialises an entry to a single fstab line (single-space
// separated; no column alignment).
func (e fstabEntry) render() string {
	opts := e.Opts
	if strings.TrimSpace(opts) == "" {
		opts = defaultOpts
	}
	return strings.Join([]string{e.Device, e.MountPoint, e.FSType, opts, strconv.Itoa(e.Dump), strconv.Itoa(e.Pass)}, " ")
}

// matchesDesired reports whether the on-disk entry already satisfies
// the desired one. Options are compared as a set (order-insensitive,
// whitespace-trimmed).
func (e fstabEntry) matchesDesired(want fstabEntry) bool {
	return e.Device == want.Device &&
		e.MountPoint == want.MountPoint &&
		e.FSType == want.FSType &&
		e.Dump == want.Dump &&
		e.Pass == want.Pass &&
		optsSetEqual(e.Opts, want.Opts)
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

// --- line-oriented editing -------------------------------------------

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

// findEntry returns the parsed fstab entry for mountPoint and whether
// one was found.
func findEntry(content, mountPoint string) (fstabEntry, bool) {
	for _, ln := range contentLines(content) {
		if e, ok := parseFstabLine(ln); ok && e.MountPoint == mountPoint {
			return e, true
		}
	}
	return fstabEntry{}, false
}

// upsertEntry returns content with the line for want.MountPoint set to
// want.render() — replacing an existing line or appending a new one.
func upsertEntry(content string, want fstabEntry) string {
	lines := contentLines(content)
	for i, ln := range lines {
		if e, ok := parseFstabLine(ln); ok && e.MountPoint == want.MountPoint {
			lines[i] = want.render()
			return renderContent(lines)
		}
	}
	return renderContent(append(lines, want.render()))
}

// removeEntry returns content with every line for mountPoint deleted.
func removeEntry(content, mountPoint string) string {
	lines := contentLines(content)
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if e, ok := parseFstabLine(ln); ok && e.MountPoint == mountPoint {
			continue
		}
		out = append(out, ln)
	}
	return renderContent(out)
}
