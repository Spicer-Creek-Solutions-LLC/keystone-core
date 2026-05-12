package ssh

import (
	"regexp"
	"strings"
)

// An authorized_keys line is:
//
//	[options ]<keytype> <blob>[ comment]
//
// The entry's identity is the key material — <keytype> <blob> — so
// the comment and the options prefix can change without it being a
// different key. This file is the pure line-oriented editor for that
// file.

// keytypeRE matches the algorithm token at the start of the
// keytype/blob pair: "ssh-rsa", "ssh-ed25519", "ssh-dss",
// "ssh-rsa-cert-v01@openssh.com", "ecdsa-sha2-nistp256",
// "ecdsa-sha2-nistp256-cert-v01@openssh.com",
// "sk-ssh-ed25519@openssh.com", "sk-ecdsa-sha2-nistp256@openssh.com".
// Every current OpenSSH public-key type starts with "ssh-", "ecdsa-"
// or "sk-"; requiring that prefix is what keeps an ordinary
// whitespace-separated line from being mistaken for a key.
var keytypeRE = regexp.MustCompile(`^(ssh-|ecdsa-|sk-)[a-z0-9.-]*(@[a-z0-9.-]+)?$`)

// blobRE matches a base64 key blob (the standard alphabet, with up to
// two padding chars).
var blobRE = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)

// authKey holds the parsed pieces of an authorized_keys line.
type authKey struct {
	Options string // the options prefix, "" if none
	KeyType string
	Blob    string
	Comment string // "" if none
}

// identity is the key material that uniquely identifies the entry.
func (k authKey) identity() string { return k.KeyType + " " + k.Blob }

// render serialises the entry back to a single authorized_keys line.
func (k authKey) render() string {
	var b strings.Builder
	if k.Options != "" {
		b.WriteString(k.Options)
		b.WriteByte(' ')
	}
	b.WriteString(k.KeyType)
	b.WriteByte(' ')
	b.WriteString(k.Blob)
	if k.Comment != "" {
		b.WriteByte(' ')
		b.WriteString(k.Comment)
	}
	return b.String()
}

// parseAuthLine parses one authorized_keys line. ok is false for
// blank / comment lines and lines with no recognisable keytype/blob
// pair. Note: an options field containing runs of consecutive spaces
// is normalised to single spaces (authorized_keys lines don't use
// those in practice).
func parseAuthLine(line string) (authKey, bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return authKey{}, false
	}
	tokens := strings.Fields(t)
	idx := -1
	for i, tok := range tokens {
		if i+1 < len(tokens) && keytypeRE.MatchString(tok) && blobRE.MatchString(tokens[i+1]) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return authKey{}, false
	}
	k := authKey{
		KeyType: tokens[idx],
		Blob:    tokens[idx+1],
	}
	if idx > 0 {
		k.Options = strings.Join(tokens[:idx], " ")
	}
	if idx+2 < len(tokens) {
		k.Comment = strings.Join(tokens[idx+2:], " ")
	}
	return k, true
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

// findLine returns the parsed entry whose key material equals
// (keyType, blob) and whether one was found.
func findLine(content, keyType, blob string) (authKey, bool) {
	want := keyType + " " + blob
	for _, ln := range contentLines(content) {
		if k, ok := parseAuthLine(ln); ok && k.identity() == want {
			return k, true
		}
	}
	return authKey{}, false
}

// upsertLine returns content with the entry whose key material equals
// want's set to want.render() — replacing the first matching line or
// appending a new one.
func upsertLine(content string, want authKey) string {
	lines := contentLines(content)
	for i, ln := range lines {
		if k, ok := parseAuthLine(ln); ok && k.identity() == want.identity() {
			lines[i] = want.render()
			return renderContent(lines)
		}
	}
	return renderContent(append(lines, want.render()))
}

// removeLines returns content with every entry whose key material
// equals (keyType, blob) removed.
func removeLines(content, keyType, blob string) string {
	want := keyType + " " + blob
	lines := contentLines(content)
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if k, ok := parseAuthLine(ln); ok && k.identity() == want {
			continue
		}
		out = append(out, ln)
	}
	return renderContent(out)
}
