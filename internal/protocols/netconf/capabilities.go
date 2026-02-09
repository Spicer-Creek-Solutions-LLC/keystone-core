package netconf

import (
	"net/url"
	"strings"
)

// ParseCapabilities extracts YANG model information from capability URIs.
// YANG capability URIs follow the pattern:
// http://example.com/ns/module?module=name&revision=2024-01-01&features=f1,f2&deviations=d1
func ParseCapabilities(caps []Capability) []YANGModel {
	var models []YANGModel
	for _, cap := range caps {
		m, ok := parseYANGCapability(string(cap))
		if ok {
			models = append(models, m)
		}
	}
	return models
}

func parseYANGCapability(uri string) (YANGModel, bool) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return YANGModel{}, false
	}

	q := parsed.Query()
	module := q.Get("module")
	if module == "" {
		return YANGModel{}, false
	}

	m := YANGModel{
		Module:    module,
		Revision:  q.Get("revision"),
		Namespace: parsed.Scheme + "://" + parsed.Host + parsed.Path,
	}

	if features := q.Get("features"); features != "" {
		m.Features = strings.Split(features, ",")
	}
	if deviations := q.Get("deviations"); deviations != "" {
		m.Deviations = strings.Split(deviations, ",")
	}

	return m, true
}

// HasCapability checks if a specific capability is present in the list.
func HasCapability(caps []Capability, target Capability) bool {
	t := string(target)
	for _, c := range caps {
		s := string(c)
		if s == t || strings.HasPrefix(s, t+"?") {
			return true
		}
	}
	return false
}

// SupportsCandidate returns true if the capability list includes candidate datastore support.
func SupportsCandidate(caps []Capability) bool {
	return HasCapability(caps, CandidateCapability)
}

// SupportsWritableRunning returns true if writable-running is advertised.
func SupportsWritableRunning(caps []Capability) bool {
	return HasCapability(caps, WritableRunning)
}

// SupportsValidate returns true if the validate capability is present.
func SupportsValidate(caps []Capability) bool {
	return HasCapability(caps, Validate10) || HasCapability(caps, Validate11)
}

// SupportsRollbackOnError returns true if rollback-on-error is supported.
func SupportsRollbackOnError(caps []Capability) bool {
	return HasCapability(caps, RollbackOnError)
}

// SupportsConfirmedCommit returns true if confirmed-commit is supported.
func SupportsConfirmedCommit(caps []Capability) bool {
	return HasCapability(caps, ConfirmedCommit10) || HasCapability(caps, ConfirmedCommit11)
}

// SupportsStartup returns true if the startup datastore is supported.
func SupportsStartup(caps []Capability) bool {
	return HasCapability(caps, StartupCapability)
}

// SupportsXPath returns true if XPath filtering is supported.
func SupportsXPath(caps []Capability) bool {
	return HasCapability(caps, XPathCapability)
}

// SupportsURL returns true if URL-based operations are supported.
func SupportsURL(caps []Capability) bool {
	return HasCapability(caps, URLCapability)
}

// SupportsBase11 returns true if NETCONF 1.1 (chunked framing) is supported.
func SupportsBase11(caps []Capability) bool {
	return HasCapability(caps, BaseCapability11)
}
