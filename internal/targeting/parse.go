package targeting

import (
	"fmt"
	"strings"
	"unicode"
)

// builtinFields are the top-level identifiers that map directly onto
// the env schema. Any other field is sugar for labels.<name>.
var builtinFields = map[string]bool{
	"id":       true,
	"hostname": true,
	"os":       true,
	"arch":     true,
	"status":   true,
	"ip":       true,
}

// translate walks the shorthand and emits expr-lang source. Each
// `field:value` term becomes a `match(field, "value")` call; operators
// (AND/OR/NOT and the symbolic forms) pass through with case
// normalization; whitespace and parens are preserved.
func translate(in string) (string, error) {
	runes := []rune(in)
	var out strings.Builder
	i := 0
	for i < len(runes) {
		c := runes[i]
		if unicode.IsSpace(c) {
			out.WriteRune(c)
			i++
			continue
		}
		if c == '(' || c == ')' {
			out.WriteRune(c)
			i++
			continue
		}
		if op, n := matchOp(runes, i); op != "" {
			if needsSpaceBefore(out.String()) {
				out.WriteByte(' ')
			}
			out.WriteString(op)
			out.WriteByte(' ')
			i += n
			continue
		}
		field, n := readIdent(runes, i)
		if field == "" {
			return "", fmt.Errorf("unexpected character %q at position %d", string(c), i)
		}
		i += n
		if i >= len(runes) || runes[i] != ':' {
			return "", fmt.Errorf("expected ':' after field %q at position %d", field, i)
		}
		i++ // consume ':'
		value, n, err := readValue(runes, i)
		if err != nil {
			return "", err
		}
		i += n
		out.WriteString(emitTerm(field, value))
	}
	return strings.TrimSpace(out.String()), nil
}

// matchOp returns the expr-side operator (`and`/`or`/`not`) and the
// rune count consumed when input at i begins one. Returns "", 0
// otherwise. Word forms are case-insensitive; symbolic forms (&&, ||,
// !) are also accepted.
func matchOp(r []rune, i int) (string, int) {
	if i+1 < len(r) {
		switch string(r[i : i+2]) {
		case "&&":
			return "and", 2
		case "||":
			return "or", 2
		}
	}
	if r[i] == '!' {
		return "not", 1
	}
	word, n := readIdent(r, i)
	if word == "" {
		return "", 0
	}
	switch strings.ToLower(word) {
	case "and", "or", "not":
		return strings.ToLower(word), n
	}
	return "", 0
}

// readIdent reads an identifier: ASCII letter or underscore start,
// followed by letters/digits/underscores/dots. Hyphens stop the ident
// so `id:web-*` parses with `web-*` as the value, not the field.
func readIdent(r []rune, i int) (string, int) {
	if i >= len(r) {
		return "", 0
	}
	c := r[i]
	if c != '_' && !unicode.IsLetter(c) {
		return "", 0
	}
	j := i + 1
	for j < len(r) {
		c := r[j]
		if c == '_' || c == '.' || unicode.IsLetter(c) || unicode.IsDigit(c) {
			j++
			continue
		}
		break
	}
	return string(r[i:j]), j - i
}

// readValue reads a term value. A leading `"` or `'` switches to quoted
// mode (with `\` escapes); otherwise the value runs until whitespace or
// a closing paren.
func readValue(r []rune, i int) (string, int, error) {
	if i >= len(r) {
		return "", 0, fmt.Errorf("unexpected end after ':'")
	}
	if r[i] == '"' || r[i] == '\'' {
		quote := r[i]
		var b strings.Builder
		j := i + 1
		for j < len(r) {
			if r[j] == '\\' && j+1 < len(r) {
				b.WriteRune(r[j+1])
				j += 2
				continue
			}
			if r[j] == quote {
				return b.String(), j - i + 1, nil
			}
			b.WriteRune(r[j])
			j++
		}
		return "", 0, fmt.Errorf("unterminated quoted value at position %d", i)
	}
	j := i
	for j < len(r) {
		c := r[j]
		if unicode.IsSpace(c) || c == '(' || c == ')' {
			break
		}
		j++
	}
	if j == i {
		return "", 0, fmt.Errorf("empty value at position %d", i)
	}
	return string(r[i:j]), j - i, nil
}

// emitTerm produces a `match(<accessor>, "value")` call. Built-in
// fields and the explicit `labels.` prefix pass through; anything else
// is sugar for a label lookup.
func emitTerm(field, value string) string {
	head := field
	if dot := strings.IndexByte(field, '.'); dot >= 0 {
		head = field[:dot]
	}
	accessor := field
	if !builtinFields[head] && head != "labels" {
		accessor = "labels." + field
	}
	return "match(" + accessor + ", " + quoteString(value) + ")"
}

// quoteString produces a Go-syntax double-quoted string suitable for
// embedding in expr-lang source.
func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// needsSpaceBefore reports whether the previously-emitted source ends
// in a non-space, non-open-paren rune so an operator should be padded
// to keep tokens distinct.
func needsSpaceBefore(s string) bool {
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	return last != ' ' && last != '\t' && last != '\n' && last != '('
}
