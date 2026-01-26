// Package semver provides semantic versioning parsing, comparison, and constraint checking.
// NOTE: API/ABI is not finalized and may change without notice.
//
// This package implements the Semantic Versioning 2.0.0 specification (https://semver.org)
// with additional features for rich version comparison and constraint matching.
//
// # Key Features
//
//   - Version parsing with support for prerelease and build metadata
//   - Rich diff output that classifies changes as major/minor/patch
//   - Semantic helpers: IsBreaking(), IsFeature(), IsBugFix()
//   - Constraint matching: ^1.2.3, ~1.2.3, >=1.0.0 <2.0.0, 1.x, 1.2.x
//   - Sorting, filtering, and batch operations
//   - Version bumping: NextMajor(), NextMinor(), NextPatch()
//
// # Basic Usage
//
//	v1, _ := semver.Parse("1.2.3")
//	v2, _ := semver.Parse("2.0.0")
//
//	diff := semver.Compare(v1, v2)
//	fmt.Println(diff.Type)       // major
//	fmt.Println(diff.Direction)  // upgrade
//	fmt.Println(diff.IsBreaking()) // true
//	fmt.Println(diff.String())   // "major upgrade (breaking change)"
//
// # Constraints
//
//	constraint, _ := semver.ParseConstraint("^1.2.0")
//	constraint.Check(semver.MustParse("1.5.0"))  // true
//	constraint.Check(semver.MustParse("2.0.0"))  // false
//
// # Constraint Syntax
//
// The following constraint formats are supported:
//
//	Exact:    "1.2.3", "=1.2.3"
//	Range:    ">1.0.0", ">=1.0.0", "<2.0.0", "<=2.0.0", "!=1.5.0"
//	Caret:    "^1.2.3" (compatible with 1.x.x where x >= 2.3)
//	Tilde:    "~1.2.3" (compatible with 1.2.x where x >= 3)
//	Wildcard: "1.x", "1.2.x", "1.*", "1.2.*"
//	Compound: ">=1.0.0 <2.0.0", ">=1.0.0, <2.0.0"
//	OR:       "^1.0.0 || ^2.0.0"
//
// # Version Bumping
//
//	v := semver.MustParse("1.2.3")
//	v.NextPatch()  // 1.2.4
//	v.NextMinor()  // 1.3.0
//	v.NextMajor()  // 2.0.0
//
// # Sorting and Filtering
//
//	versions := semver.ParseAll([]string{"1.0.0", "2.0.0", "1.5.0"})
//	semver.Sort(versions)         // [1.0.0, 1.5.0, 2.0.0]
//	semver.Latest(versions)       // 2.0.0
//	semver.FilterStable(versions) // versions without prerelease tags
package semver
