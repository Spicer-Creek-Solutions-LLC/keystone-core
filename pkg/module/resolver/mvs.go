package resolver

import (
	"fmt"
	"sort"
)

// MVSConflictResolver implements Minimum Version Selection (MVS)
// as used by Go modules. It selects the minimum version that satisfies
// all constraints for a module.
type MVSConflictResolver struct {
	parser   ConstraintParser
	selector VersionSelector
}

// NewMVSConflictResolver creates a new MVS conflict resolver
func NewMVSConflictResolver() *MVSConflictResolver {
	return &MVSConflictResolver{
		parser:   &DefaultConstraintParser{},
		selector: &DefaultVersionSelector{},
	}
}

// Resolve resolves version conflicts using Minimum Version Selection
// Given multiple constraints, it finds the minimum version that satisfies all
func (r *MVSConflictResolver) Resolve(moduleName string, constraints []VersionConstraint) (string, error) {
	if len(constraints) == 0 {
		return "", fmt.Errorf("%w: no constraints provided for %s", ErrConstraintUnsatisfiable, moduleName)
	}

	// If there's only one constraint, try to find any version satisfying it
	if len(constraints) == 1 {
		// For a single constraint, we don't have available versions here
		// This should be handled by the caller with actual version data
		// For now, if it's an exact constraint, return that version
		if constraints[0].IsExact() {
			return constraints[0].String()[1:], nil // Remove operator prefix
		}
		return "", fmt.Errorf("%w: cannot resolve single non-exact constraint without available versions", ErrConstraintUnsatisfiable)
	}

	// For multiple constraints, we need to find overlapping ranges
	// This is a simplified implementation - real MVS would query available versions
	return r.resolveMultiple(moduleName, constraints)
}

// resolveMultiple resolves multiple constraints
func (r *MVSConflictResolver) resolveMultiple(moduleName string, constraints []VersionConstraint) (string, error) {
	// Extract exact versions if any
	var exactVersions []string
	var rangeConstraints []VersionConstraint

	for _, c := range constraints {
		if c.IsExact() {
			// Extract version from exact constraint
			versionStr := c.String()
			// Remove operator prefix (= or ==)
			if versionStr[0] == '=' {
				versionStr = versionStr[1:]
			}
			if versionStr != "" && versionStr[0] == '=' {
				versionStr = versionStr[1:]
			}
			exactVersions = append(exactVersions, versionStr)
		} else {
			rangeConstraints = append(rangeConstraints, c)
		}
	}

	// If we have exact versions, they must all be the same
	if len(exactVersions) > 0 {
		firstVersion := exactVersions[0]
		for _, v := range exactVersions[1:] {
			if v != firstVersion {
				return "", &ConflictError{
					Module:      moduleName,
					Constraints: toStrings(constraints),
				}
			}
		}

		// Check if exact version satisfies all range constraints
		for _, rc := range rangeConstraints {
			if !rc.Matches(firstVersion) {
				return "", &ConflictError{
					Module:      moduleName,
					Constraints: toStrings(constraints),
				}
			}
		}

		return firstVersion, nil
	}

	// No exact versions - need to find overlapping range
	// This is simplified - in practice, we'd need available versions
	return "", fmt.Errorf("%w: cannot resolve non-exact constraints without available versions", ErrConstraintUnsatisfiable)
}

// ResolveWithVersions resolves conflicts given available versions
func (r *MVSConflictResolver) ResolveWithVersions(moduleName string, constraints []VersionConstraint, available []string) (string, error) {
	if len(available) == 0 {
		return "", fmt.Errorf("%w: %s", ErrNoVersionsAvailable, moduleName)
	}

	if len(constraints) == 0 {
		// No constraints - return highest available
		return r.selector.SelectHighest(available)
	}

	// Filter versions that satisfy ALL constraints
	matching := make([]string, 0)
	for _, version := range available {
		satisfiesAll := true
		for _, constraint := range constraints {
			if !constraint.Matches(version) {
				satisfiesAll = false
				break
			}
		}
		if satisfiesAll {
			matching = append(matching, version)
		}
	}

	if len(matching) == 0 {
		return "", &ConflictError{
			Module:      moduleName,
			Constraints: toStrings(constraints),
		}
	}

	// MVS: Select the HIGHEST version that satisfies all constraints
	// (Note: Go's MVS selects the minimum required version across the graph,
	// but for a single module with multiple constraints, we select the highest
	// matching version to ensure all constraints are satisfied)
	return r.selector.SelectHighest(matching)
}

// Strategy returns the conflict resolution strategy name
func (r *MVSConflictResolver) Strategy() string {
	return "MVS"
}

// toStrings converts version constraints to strings
func toStrings(constraints []VersionConstraint) []string {
	result := make([]string, len(constraints))
	for i, c := range constraints {
		result[i] = c.String()
	}
	return result
}

// BuildRequirementList builds the module requirement list for MVS
// This is used to determine the minimum required version for each module
type BuildRequirementList struct {
	requirements map[string]*Version // module -> minimum required version
	parser       ConstraintParser
}

// NewBuildRequirementList creates a new build requirement list
func NewBuildRequirementList() *BuildRequirementList {
	return &BuildRequirementList{
		requirements: make(map[string]*Version),
		parser:       &DefaultConstraintParser{},
	}
}

// AddRequirement adds a requirement for a module
func (b *BuildRequirementList) AddRequirement(moduleName, versionConstraint string) error {
	constraint, err := b.parser.Parse(versionConstraint)
	if err != nil {
		return err
	}

	// For MVS, we need to track the minimum version that satisfies the constraint
	// For exact constraints, use that version
	// For range constraints, we'd need to query available versions (simplified here)

	if constraint.IsExact() {
		versionStr := constraint.String()
		// Remove operator prefix
		if versionStr != "" && versionStr[0] == '=' {
			versionStr = versionStr[1:]
		}
		if versionStr != "" && versionStr[0] == '=' {
			versionStr = versionStr[1:]
		}

		version, err := ParseVersion(versionStr)
		if err != nil {
			return err
		}

		// If we already have a requirement, take the higher one
		if existing, exists := b.requirements[moduleName]; exists {
			if version.Greater(existing) {
				b.requirements[moduleName] = version
			}
		} else {
			b.requirements[moduleName] = version
		}
	}

	return nil
}

// GetRequirement returns the minimum required version for a module
func (b *BuildRequirementList) GetRequirement(moduleName string) (*Version, bool) {
	version, exists := b.requirements[moduleName]
	return version, exists
}

// GetAllRequirements returns all module requirements
func (b *BuildRequirementList) GetAllRequirements() map[string]string {
	result := make(map[string]string)
	for moduleName, version := range b.requirements {
		result[moduleName] = version.String()
	}
	return result
}

// Merge merges another requirement list into this one
// For conflicting requirements, takes the higher version
func (b *BuildRequirementList) Merge(other *BuildRequirementList) {
	for moduleName, otherVersion := range other.requirements {
		if existing, exists := b.requirements[moduleName]; exists {
			if otherVersion.Greater(existing) {
				b.requirements[moduleName] = otherVersion
			}
		} else {
			b.requirements[moduleName] = otherVersion
		}
	}
}

// Sort returns requirements sorted by module name
func (b *BuildRequirementList) Sort() []struct {
	Module  string
	Version string
} {
	names := make([]string, 0, len(b.requirements))
	for name := range b.requirements {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]struct {
		Module  string
		Version string
	}, len(names))

	for i, name := range names {
		result[i].Module = name
		result[i].Version = b.requirements[name].String()
	}

	return result
}
