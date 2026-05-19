package blueprint

import (
	"errors"
	"fmt"
	"regexp"

	"go.keystone-core.io/keystone-core/pkg/semver"
)

// ErrInvalidManifest wraps every structural validation failure so
// callers can errors.Is against it. Individual reasons are joined.
var ErrInvalidManifest = errors.New("blueprint: invalid manifest")

// nameRe is the permitted shape for blueprint and dependency names:
// lowercase DNS-label-ish.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

var validParamTypes = map[string]bool{
	TypeString: true, TypeInteger: true, TypeNumber: true,
	TypeBoolean: true, TypeArray: true, TypeObject: true,
}

// Validate checks the manifest for internal consistency. All
// problems are collected and returned joined under ErrInvalidManifest
// (callers see every error at once, not just the first).
func (m *Manifest) Validate() error {
	var errs []error
	add := func(format string, a ...any) {
		errs = append(errs, fmt.Errorf(format, a...))
	}

	if m.Metadata.Name == "" {
		add("metadata.name is required")
	} else if !nameRe.MatchString(m.Metadata.Name) {
		add("metadata.name %q must match %s", m.Metadata.Name, nameRe)
	}
	if m.Metadata.Version == "" {
		add("metadata.version is required")
	} else if _, err := semver.Parse(m.Metadata.Version); err != nil {
		add("metadata.version %q is not valid semver: %v", m.Metadata.Version, err)
	}
	if m.Compatibility.MinKeystoneVersion != "" {
		if _, err := semver.Parse(m.Compatibility.MinKeystoneVersion); err != nil {
			add("compatibility.min_keystone_version %q is not valid semver: %v",
				m.Compatibility.MinKeystoneVersion, err)
		}
	}
	if m.Entrypoints.Default == "" {
		add("entrypoints.default is required")
	}

	for name, p := range m.Parameters {
		if !validParamTypes[p.Type] {
			add("parameter %q: type %q is not one of string|integer|number|boolean|array|object", name, p.Type)
		}
		if p.Source != "" && p.Source != SourceSecret {
			add("parameter %q: source %q must be empty or %q", name, p.Source, SourceSecret)
		}
		if p.Source == SourceSecret && !p.Sensitive {
			add("parameter %q: source: secret requires sensitive: true", name)
		}
		if p.Min != nil && p.Max != nil && *p.Min > *p.Max {
			add("parameter %q: min (%g) > max (%g)", name, *p.Min, *p.Max)
		}
	}
	// Compiling the assembled schema surfaces malformed constraints
	// (e.g. an invalid regex pattern) at load time, not apply time.
	if _, err := m.compileParamSchema(); err != nil {
		add("parameters: schema does not compile: %v", err)
	}

	errs = append(errs, validateDeps(m.Metadata.Name, "requires", m.Dependencies.Requires)...)
	errs = append(errs, validateDeps(m.Metadata.Name, "requires_before", m.Dependencies.RequiresBefore)...)

	errs = append(errs, validateHookList("pre_apply", m.Hooks.PreApply)...)
	errs = append(errs, validateHookList("post_apply", m.Hooks.PostApply)...)
	errs = append(errs, validateHookList("pre_rollback", m.Hooks.PreRollback)...)
	errs = append(errs, validateHookList("post_rollback", m.Hooks.PostRollback)...)

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%w (%s): %w", ErrInvalidManifest, m.Metadata.Name, errors.Join(errs...))
}

func validateDeps(self, field string, deps []string) []error {
	var errs []error
	seen := make(map[string]bool, len(deps))
	for _, d := range deps {
		switch {
		case d == "":
			errs = append(errs, fmt.Errorf("dependencies.%s contains an empty name", field))
		case !nameRe.MatchString(d):
			errs = append(errs, fmt.Errorf("dependencies.%s: %q must match %s", field, d, nameRe))
		case d == self:
			errs = append(errs, fmt.Errorf("dependencies.%s: %q depends on itself", field, d))
		case seen[d]:
			errs = append(errs, fmt.Errorf("dependencies.%s: %q listed more than once", field, d))
		}
		seen[d] = true
	}
	return errs
}

func validateHookList(field string, hooks []string) []error {
	var errs []error
	for i, h := range hooks {
		if h == "" {
			errs = append(errs, fmt.Errorf("hooks.%s[%d] is an empty runbook name", field, i))
		}
	}
	return errs
}
