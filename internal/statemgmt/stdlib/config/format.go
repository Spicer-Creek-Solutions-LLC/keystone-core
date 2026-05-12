package config

import "strings"

// This file holds the pure config-file parsing/editing logic for the
// two v1.0 formats:
//
//	keyvalue — flat "key=value" lines (whitespace around '=' allowed);
//	           full-line comments only ('#' or ';' as the first
//	           non-whitespace char); '[...]' lines and any line
//	           without an '=' are opaque (preserved, never
//	           interpreted).
//	ini      — the same "key=value" lines plus "[section]" headers; a
//	           key belongs to the section whose header most recently
//	           preceded it (the implicit "" section before any
//	           header).
//
// All editing is line-oriented: the line that defines the target key
// is the only one rewritten/removed, so comments, blank lines, and
// every other key are preserved exactly.

// parsedKV holds the pieces of a "key=value" line so an in-place
// value replacement can keep the operator's spacing:
//
//	<lead><key><sep><value>
//
// lead  — leading whitespace.
// key   — the key, leading/trailing whitespace stripped.
// sep   — the literal substring between the end of key and the start
//
//	of value (includes the '=' and any whitespace around it).
//
// value — the value, trailing whitespace stripped.
type parsedKV struct {
	lead  string
	key   string
	sep   string
	value string
}

func isBlank(line string) bool { return strings.TrimSpace(line) == "" }

func isComment(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";")
}

// sectionName reports whether line is a "[section]" header (ini) and
// returns the section name (whitespace-trimmed).
func sectionName(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if len(t) >= 2 && t[0] == '[' && t[len(t)-1] == ']' {
		return strings.TrimSpace(t[1 : len(t)-1]), true
	}
	return "", false
}

// parseKV parses a "key=value" line. ok is false for blank lines,
// comment lines, "[section]" headers, lines with no '=', and lines
// whose key part is empty.
func parseKV(line string) (parsedKV, bool) {
	if isBlank(line) || isComment(line) {
		return parsedKV{}, false
	}
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return parsedKV{}, false
	}
	before, after := line[:eq], line[eq+1:]

	keyStart := 0
	for keyStart < len(before) && (before[keyStart] == ' ' || before[keyStart] == '\t') {
		keyStart++
	}
	lead := before[:keyStart]
	rawKey := before[keyStart:]
	key := strings.TrimRight(rawKey, " \t")
	if key == "" {
		return parsedKV{}, false
	}
	afterKeyWS := rawKey[len(key):]

	valStart := 0
	for valStart < len(after) && (after[valStart] == ' ' || after[valStart] == '\t') {
		valStart++
	}
	return parsedKV{
		lead:  lead,
		key:   key,
		sep:   afterKeyWS + "=" + after[:valStart],
		value: strings.TrimRight(after[valStart:], " \t"),
	}, true
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func insertLine(lines []string, idx int, val string) []string {
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, val)
	out = append(out, lines[idx:]...)
	return out
}

// locate scans lines for the target (section, key). For the keyvalue
// format section is ignored. It returns the indices of every line
// that defines key (within the section), the index at which a fresh
// line for key should be inserted, and — ini only — whether the
// target section's header is missing.
func locate(lines []string, ini bool, section, key string) (idxs []int, insertAt int, needHeader bool) {
	if !ini {
		for i, ln := range lines {
			if kv, ok := parseKV(ln); ok && kv.key == key {
				idxs = append(idxs, i)
			}
		}
		return idxs, len(lines), false
	}

	// ini: find the body range of `section` (between its header and
	// the next header / EOF; for the implicit "" section, line 0 to
	// the first header). When a section header appears more than
	// once, the last occurrence's body wins.
	bodyStart, bodyEnd := -1, -1
	headerSeen := section == ""
	if section == "" {
		bodyStart = 0
	}
	for i, ln := range lines {
		name, ok := sectionName(ln)
		if !ok {
			continue
		}
		if bodyStart >= 0 && bodyEnd < 0 {
			bodyEnd = i
		}
		if name == section {
			headerSeen = true
			bodyStart = i + 1
			bodyEnd = -1
		}
	}
	if bodyStart >= 0 && bodyEnd < 0 {
		bodyEnd = len(lines)
	}
	if !headerSeen {
		return nil, len(lines), true
	}
	for i := bodyStart; i < bodyEnd; i++ {
		if kv, ok := parseKV(lines[i]); ok && kv.key == key {
			idxs = append(idxs, i)
		}
	}
	ins := bodyEnd
	for ins > bodyStart && isBlank(lines[ins-1]) {
		ins--
	}
	return idxs, ins, false
}

// get returns the current value of (section, key) and whether it is
// present. The first occurrence wins.
func get(content string, ini bool, section, key string) (string, bool) {
	lines := splitLines(content)
	idxs, _, _ := locate(lines, ini, section, key)
	if len(idxs) == 0 {
		return "", false
	}
	kv, _ := parseKV(lines[idxs[0]])
	return kv.value, true
}

// set returns content with (section, key)=value. When the key already
// exists its first occurrence's value is replaced in place (lead/key/
// separator preserved); otherwise a new line is inserted at the end of
// the section (ini) / end of file (keyvalue). For ini, a missing
// section header is created at EOF. changed reports whether anything
// changed.
func set(content string, ini bool, section, key, value string, spaceAround bool) (newContent string, changed bool) {
	lines := splitLines(content)
	idxs, insertAt, needHeader := locate(lines, ini, section, key)
	if len(idxs) > 0 {
		kv, _ := parseKV(lines[idxs[0]])
		if kv.value == value {
			return content, false
		}
		lines[idxs[0]] = kv.lead + kv.key + kv.sep + value
		return joinLines(lines), true
	}
	sep := "="
	if spaceAround {
		sep = " = "
	}
	newLine := key + sep + value
	if needHeader {
		var add []string
		if len(lines) > 0 && !isBlank(lines[len(lines)-1]) {
			add = append(add, "")
		}
		add = append(add, "["+section+"]", newLine)
		return joinLines(append(lines, add...)), true
	}
	return joinLines(insertLine(lines, insertAt, newLine)), true
}

// del returns content with every line defining (section, key)
// removed. The section header is left in place even if it becomes
// empty. changed reports whether anything was removed.
func del(content string, ini bool, section, key string) (newContent string, changed bool) {
	lines := splitLines(content)
	idxs, _, needHeader := locate(lines, ini, section, key)
	if needHeader || len(idxs) == 0 {
		return content, false
	}
	skip := make(map[int]struct{}, len(idxs))
	for _, i := range idxs {
		skip[i] = struct{}{}
	}
	out := make([]string, 0, len(lines)-len(idxs))
	for i, ln := range lines {
		if _, ok := skip[i]; ok {
			continue
		}
		out = append(out, ln)
	}
	return joinLines(out), true
}
