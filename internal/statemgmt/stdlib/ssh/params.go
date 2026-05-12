package ssh

import (
	"fmt"
	"regexp"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const (
	paramKey      = "key"
	paramUser     = "user"
	paramOptions  = "options"
	paramComment  = "comment"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramKey:      {},
	paramUser:     {},
	paramOptions:  {},
	paramComment:  {},
	paramSeverity: {},
}

// userRE is a plausible-username gate (POSIX-ish, with an optional
// trailing '$' for machine accounts). It rejects whitespace and shell
// metacharacters.
var userRE = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]*\$?$`)

type params struct {
	Label   string // Declaration.Name — a human label (decl ID; not used for matching)
	State   string
	User    string
	KeyType string // parsed from `key`
	Blob    string // parsed from `key`
	Options string // the options prefix; "" if not given
	Comment string // the comment for the managed line; "" if not given anywhere

	seen map[string]struct{}
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	seen := make(map[string]struct{}, len(decl.Params))
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: key, user, options, comment, severity)", k)
		}
		seen[k] = struct{}{}
	}
	p := &params{Label: decl.Name, State: decl.State, seen: seen}
	if raw, ok := decl.Params[paramUser]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("user: expected string, got %T", raw)
		}
		p.User = s
	}
	commentFromKey := ""
	if raw, ok := decl.Params[paramKey]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("key: expected string, got %T", raw)
		}
		kt, blob, comment, err := parseKeyParam(s)
		if err != nil {
			return nil, fmt.Errorf("key: %w", err)
		}
		p.KeyType, p.Blob, commentFromKey = kt, blob, comment
	}
	if raw, ok := decl.Params[paramOptions]; ok {
		opt, err := parseOptions(raw)
		if err != nil {
			return nil, fmt.Errorf("options: %w", err)
		}
		p.Options = opt
	}
	// The comment param overrides any comment carried in `key`.
	if raw, ok := decl.Params[paramComment]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("comment: expected string, got %T", raw)
		}
		p.Comment = s
	} else {
		p.Comment = commentFromKey
	}
	return p, nil
}

// parseKeyParam splits a "<keytype> <blob>[ comment]" string. It does
// not accept an options prefix (use the `options` param). keytype and
// blob are charset-validated; the comment is everything after the
// blob (may be empty).
func parseKeyParam(s string) (keyType, blob, comment string, err error) {
	if strings.ContainsAny(s, "\r\n") {
		return "", "", "", fmt.Errorf("must be a single line")
	}
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) < 2 {
		return "", "", "", fmt.Errorf("expected at least \"<keytype> <blob>\", got %q", s)
	}
	if !keytypeRE.MatchString(f[0]) {
		return "", "", "", fmt.Errorf("unrecognised key type %q", f[0])
	}
	if !blobRE.MatchString(f[1]) {
		return "", "", "", fmt.Errorf("key blob is not valid base64")
	}
	keyType, blob = f[0], f[1]
	if len(f) > 2 {
		comment = strings.Join(f[2:], " ")
	}
	return keyType, blob, comment, nil
}

// parseOptions accepts the options prefix as a string (verbatim) or a
// list of option strings (joined with ","). The result must be a
// single line and non-empty.
func parseOptions(raw any) (string, error) {
	var s string
	switch v := raw.(type) {
	case string:
		s = strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for i, e := range v {
			es, ok := e.(string)
			if !ok {
				return "", fmt.Errorf("element %d: expected string, got %T", i, e)
			}
			es = strings.TrimSpace(es)
			if es == "" {
				return "", fmt.Errorf("element %d: empty", i)
			}
			parts = append(parts, es)
		}
		s = strings.Join(parts, ",")
	default:
		return "", fmt.Errorf("expected a string or a list of strings, got %T", raw)
	}
	if s == "" {
		return "", fmt.Errorf("empty")
	}
	if strings.ContainsAny(s, "\r\n") {
		return "", fmt.Errorf("must be a single line")
	}
	if strings.ContainsAny(s, " \t") {
		// authorized_keys options are comma-separated with no spaces
		// (spaces only inside quotes, which v1.0 doesn't special-
		// case); a bare space almost always means a mistake.
		return "", fmt.Errorf("must not contain spaces (use commas to separate options)")
	}
	return s, nil
}

// desiredKey is the authorized_keys entry the declaration wants.
func (p *params) desiredKey() authKey {
	return authKey{Options: p.Options, KeyType: p.KeyType, Blob: p.Blob, Comment: p.Comment}
}

func (p *params) validate() error {
	if strings.TrimSpace(p.User) == "" {
		return fmt.Errorf("user is required")
	}
	if !userRE.MatchString(p.User) {
		return fmt.Errorf("invalid user %q", p.User)
	}
	if p.KeyType == "" || p.Blob == "" {
		return fmt.Errorf("key is required (a \"<keytype> <blob>\" public key)")
	}
	if strings.ContainsAny(p.Comment, "\r\n") {
		return fmt.Errorf("comment must be a single line")
	}
	switch p.State {
	case StatePresent:
		// key, user, options, comment all allowed.
	case StateAbsent:
		var leaked []string
		if _, ok := p.seen[paramOptions]; ok {
			leaked = append(leaked, paramOptions)
		}
		if _, ok := p.seen[paramComment]; ok {
			leaked = append(leaked, paramComment)
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry these params (only the key material identifies the entry): %v", leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
