package semver

import "sort"

// Sort sorts a slice of versions in ascending order (oldest to newest).
func Sort(versions []Version) {
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Compare(versions[j]) < 0
	})
}

// SortDescending sorts a slice of versions in descending order (newest to oldest).
func SortDescending(versions []Version) {
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Compare(versions[j]) > 0
	})
}

// Latest returns the highest version from a slice.
// Returns zero value if the slice is empty.
func Latest(versions []Version) Version {
	if len(versions) == 0 {
		return Version{}
	}

	latest := versions[0]
	for _, v := range versions[1:] {
		if v.Compare(latest) > 0 {
			latest = v
		}
	}
	return latest
}

// Oldest returns the lowest version from a slice.
// Returns zero value if the slice is empty.
func Oldest(versions []Version) Version {
	if len(versions) == 0 {
		return Version{}
	}

	oldest := versions[0]
	for _, v := range versions[1:] {
		if v.Compare(oldest) < 0 {
			oldest = v
		}
	}
	return oldest
}

// Filter returns versions that satisfy the given constraint.
func Filter(versions []Version, constraint Constraint) []Version {
	var result []Version
	for _, v := range versions {
		if constraint.Check(v) {
			result = append(result, v)
		}
	}
	return result
}

// FilterStable returns only stable versions (no prerelease tags).
func FilterStable(versions []Version) []Version {
	var result []Version
	for _, v := range versions {
		if !v.IsPrerelease() {
			result = append(result, v)
		}
	}
	return result
}

// FilterPrerelease returns only prerelease versions.
func FilterPrerelease(versions []Version) []Version {
	var result []Version
	for _, v := range versions {
		if v.IsPrerelease() {
			result = append(result, v)
		}
	}
	return result
}

// LatestMatching returns the highest version that satisfies the constraint.
// Returns zero value if no versions match.
func LatestMatching(versions []Version, constraint Constraint) Version {
	matching := Filter(versions, constraint)
	return Latest(matching)
}

// Range returns all versions between min and max (inclusive).
func Range(versions []Version, min, max Version) []Version {
	var result []Version
	for _, v := range versions {
		if v.Compare(min) >= 0 && v.Compare(max) <= 0 {
			result = append(result, v)
		}
	}
	return result
}

// Unique removes duplicate versions from a slice.
// Versions are considered equal if Compare returns 0.
func Unique(versions []Version) []Version {
	if len(versions) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []Version

	for _, v := range versions {
		// Use core version (without build metadata) as key
		key := v.Core().String()
		if v.Prerelease != "" {
			key += "-" + v.Prerelease
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}

	return result
}

// ParseAll parses a slice of version strings, ignoring invalid versions.
func ParseAll(strings []string) []Version {
	var result []Version
	for _, s := range strings {
		v, err := Parse(s)
		if err == nil {
			result = append(result, v)
		}
	}
	return result
}

// ParseAllStrict parses a slice of version strings, returning an error for any invalid version.
func ParseAllStrict(strings []string) ([]Version, error) {
	result := make([]Version, len(strings))
	for i, s := range strings {
		v, err := Parse(s)
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}

// GroupByMajor groups versions by their major version number.
func GroupByMajor(versions []Version) map[int][]Version {
	result := make(map[int][]Version)
	for _, v := range versions {
		result[v.Major] = append(result[v.Major], v)
	}
	return result
}

// GroupByMinor groups versions by their major.minor version.
func GroupByMinor(versions []Version) map[string][]Version {
	result := make(map[string][]Version)
	for _, v := range versions {
		key := New(v.Major, v.Minor, 0).String()
		result[key] = append(result[key], v)
	}
	return result
}
