// SPDX-License-Identifier: Apache-2.0

package firewalld

import (
	"sort"
	"strings"
)

// canonicalizeRichRule returns a normalised form of a firewalld rich
// rule so that two rules differing only in whitespace, attribute
// quoting, or the order of attributes within an element compare equal.
//
// It is deliberately *syntactic*: it does not normalise value semantics
// (e.g. it won't lowercase a MAC or canonicalise a CIDR), so a value
// that firewalld itself rewrites won't match — operators should write
// values as firewalld stores them. The point of the function is not to
// reproduce firewalld's exact canonical string but to be a *consistent*
// normaliser: the module runs both the declared rule and each stored
// rule (`--list-rich-rules`) through it and compares, so equivalent
// rules map to the same string regardless of firewalld's own format.
//
// Algorithm: tokenise the rule (quote-aware), then walk the tokens.
// Each bare token (no `=`) is a keyword and is emitted in place; each
// attribute token (`key=value`) attaches to the most recent keyword.
// Attributes within a keyword's group are normalised to `key="value"`
// and sorted, so intra-element attribute order is irrelevant. Keyword
// order — which firewalld's grammar fixes — is preserved.
func canonicalizeRichRule(rule string) string {
	tokens := tokenizeRichRule(rule)
	var out []string
	var attrs []string
	flush := func() {
		if len(attrs) == 0 {
			return
		}
		sort.Strings(attrs)
		out = append(out, attrs...)
		attrs = nil
	}
	for _, tok := range tokens {
		if k, v, ok := splitAttr(tok); ok {
			attrs = append(attrs, k+"=\""+v+"\"")
			continue
		}
		// A keyword: flush the previous group's sorted attributes, then
		// emit the keyword.
		flush()
		out = append(out, tok)
	}
	flush()
	return strings.Join(out, " ")
}

// tokenizeRichRule splits a rich rule on whitespace, keeping a
// double-quoted span (which may itself contain spaces, e.g. a log
// prefix) as a single token. Quotes are retained in the token so
// splitAttr can strip them uniformly.
func tokenizeRichRule(rule string) []string {
	var tokens []string
	var b strings.Builder
	inQuote := false
	for _, r := range rule {
		switch {
		case r == '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case (r == ' ' || r == '\t') && !inQuote:
			if b.Len() > 0 {
				tokens = append(tokens, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		tokens = append(tokens, b.String())
	}
	return tokens
}

// splitAttr reports whether tok is a `key=value` attribute and, if so,
// returns the key and the unquoted value. A token is an attribute when
// it contains `=` and the part before the first `=` is a non-empty
// attribute name (letters, digits, `-`); this distinguishes
// `port="80"` from a bare keyword.
func splitAttr(tok string) (key, value string, ok bool) {
	i := strings.IndexByte(tok, '=')
	if i <= 0 {
		return "", "", false
	}
	key = tok[:i]
	if !isAttrName(key) {
		return "", "", false
	}
	value = strings.Trim(tok[i+1:], `"`)
	return key, value, true
}

func isAttrName(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return s != ""
}
