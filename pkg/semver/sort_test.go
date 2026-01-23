package semver

import (
	"reflect"
	"testing"
)

func TestSort(t *testing.T) {
	versions := []Version{
		MustParse("2.0.0"),
		MustParse("1.0.0"),
		MustParse("1.5.0"),
		MustParse("1.0.1"),
	}

	Sort(versions)

	want := []string{"1.0.0", "1.0.1", "1.5.0", "2.0.0"}
	for i, v := range versions {
		if v.String() != want[i] {
			t.Errorf("Sort() index %d = %s, want %s", i, v, want[i])
		}
	}
}

func TestSortDescending(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("2.0.0"),
		MustParse("1.5.0"),
	}

	SortDescending(versions)

	want := []string{"2.0.0", "1.5.0", "1.0.0"}
	for i, v := range versions {
		if v.String() != want[i] {
			t.Errorf("SortDescending() index %d = %s, want %s", i, v, want[i])
		}
	}
}

func TestLatest(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("2.0.0"),
		MustParse("1.5.0"),
	}

	if got := Latest(versions); got.String() != "2.0.0" {
		t.Errorf("Latest() = %s, want 2.0.0", got)
	}

	// Empty slice
	if got := Latest(nil); !got.IsZero() {
		t.Errorf("Latest(nil) = %s, want zero", got)
	}
}

func TestOldest(t *testing.T) {
	versions := []Version{
		MustParse("2.0.0"),
		MustParse("1.0.0"),
		MustParse("1.5.0"),
	}

	if got := Oldest(versions); got.String() != "1.0.0" {
		t.Errorf("Oldest() = %s, want 1.0.0", got)
	}

	// Empty slice
	if got := Oldest(nil); !got.IsZero() {
		t.Errorf("Oldest(nil) = %s, want zero", got)
	}
}

func TestFilter(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.5.0"),
		MustParse("2.0.0"),
		MustParse("2.5.0"),
	}

	constraint := MustParseConstraint("^1.0.0")
	filtered := Filter(versions, constraint)

	if len(filtered) != 2 {
		t.Fatalf("Filter() returned %d versions, want 2", len(filtered))
	}
	if filtered[0].String() != "1.0.0" || filtered[1].String() != "1.5.0" {
		t.Errorf("Filter() = %v, want [1.0.0, 1.5.0]", filtered)
	}
}

func TestFilterStable(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.0.0-alpha"),
		MustParse("2.0.0"),
		MustParse("2.0.0-beta"),
	}

	stable := FilterStable(versions)

	if len(stable) != 2 {
		t.Fatalf("FilterStable() returned %d versions, want 2", len(stable))
	}
	for _, v := range stable {
		if v.IsPrerelease() {
			t.Errorf("FilterStable() contains prerelease: %s", v)
		}
	}
}

func TestFilterPrerelease(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.0.0-alpha"),
		MustParse("2.0.0"),
		MustParse("2.0.0-beta"),
	}

	prereleases := FilterPrerelease(versions)

	if len(prereleases) != 2 {
		t.Fatalf("FilterPrerelease() returned %d versions, want 2", len(prereleases))
	}
	for _, v := range prereleases {
		if !v.IsPrerelease() {
			t.Errorf("FilterPrerelease() contains stable: %s", v)
		}
	}
}

func TestLatestMatching(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.5.0"),
		MustParse("2.0.0"),
		MustParse("2.5.0"),
	}

	constraint := MustParseConstraint("^1.0.0")
	if got := LatestMatching(versions, constraint); got.String() != "1.5.0" {
		t.Errorf("LatestMatching() = %s, want 1.5.0", got)
	}

	// No match
	constraint2 := MustParseConstraint("^3.0.0")
	if got := LatestMatching(versions, constraint2); !got.IsZero() {
		t.Errorf("LatestMatching() with no match = %s, want zero", got)
	}
}

func TestRange(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.5.0"),
		MustParse("2.0.0"),
		MustParse("2.5.0"),
		MustParse("3.0.0"),
	}

	ranged := Range(versions, MustParse("1.5.0"), MustParse("2.5.0"))

	if len(ranged) != 3 {
		t.Fatalf("Range() returned %d versions, want 3", len(ranged))
	}
	want := []string{"1.5.0", "2.0.0", "2.5.0"}
	for i, v := range ranged {
		if v.String() != want[i] {
			t.Errorf("Range() index %d = %s, want %s", i, v, want[i])
		}
	}
}

func TestUnique(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.0.0"),
		MustParse("2.0.0"),
		MustParse("1.0.0+build"),
		MustParse("2.0.0"),
	}

	unique := Unique(versions)

	// Per semver spec, build metadata doesn't affect version identity/precedence
	// So 1.0.0 and 1.0.0+build are considered the same version
	// Result should be: 1.0.0, 2.0.0
	if len(unique) != 2 {
		t.Fatalf("Unique() returned %d versions, want 2", len(unique))
	}

	// Empty slice
	if got := Unique(nil); got != nil {
		t.Errorf("Unique(nil) = %v, want nil", got)
	}
}

func TestParseAll(t *testing.T) {
	strings := []string{"1.0.0", "invalid", "2.0.0", "also-invalid", "3.0.0"}

	versions := ParseAll(strings)

	if len(versions) != 3 {
		t.Fatalf("ParseAll() returned %d versions, want 3", len(versions))
	}
	want := []string{"1.0.0", "2.0.0", "3.0.0"}
	for i, v := range versions {
		if v.String() != want[i] {
			t.Errorf("ParseAll() index %d = %s, want %s", i, v, want[i])
		}
	}
}

func TestParseAllStrict(t *testing.T) {
	// Valid versions
	strings := []string{"1.0.0", "2.0.0", "3.0.0"}
	versions, err := ParseAllStrict(strings)
	if err != nil {
		t.Fatalf("ParseAllStrict() error = %v", err)
	}
	if len(versions) != 3 {
		t.Errorf("ParseAllStrict() returned %d versions, want 3", len(versions))
	}

	// Invalid version
	strings2 := []string{"1.0.0", "invalid"}
	_, err = ParseAllStrict(strings2)
	if err == nil {
		t.Error("ParseAllStrict() with invalid version should return error")
	}
}

func TestGroupByMajor(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.5.0"),
		MustParse("2.0.0"),
		MustParse("2.1.0"),
		MustParse("3.0.0"),
	}

	groups := GroupByMajor(versions)

	if len(groups) != 3 {
		t.Fatalf("GroupByMajor() returned %d groups, want 3", len(groups))
	}
	if len(groups[1]) != 2 {
		t.Errorf("GroupByMajor()[1] has %d versions, want 2", len(groups[1]))
	}
	if len(groups[2]) != 2 {
		t.Errorf("GroupByMajor()[2] has %d versions, want 2", len(groups[2]))
	}
	if len(groups[3]) != 1 {
		t.Errorf("GroupByMajor()[3] has %d versions, want 1", len(groups[3]))
	}
}

func TestGroupByMinor(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.0.1"),
		MustParse("1.1.0"),
		MustParse("2.0.0"),
	}

	groups := GroupByMinor(versions)

	if len(groups) != 3 {
		t.Fatalf("GroupByMinor() returned %d groups, want 3", len(groups))
	}
	if len(groups["1.0.0"]) != 2 {
		t.Errorf("GroupByMinor()[1.0.0] has %d versions, want 2", len(groups["1.0.0"]))
	}
}

func TestSortWithPrerelease(t *testing.T) {
	versions := []Version{
		MustParse("1.0.0"),
		MustParse("1.0.0-alpha"),
		MustParse("1.0.0-beta"),
		MustParse("1.0.0-rc.1"),
	}

	Sort(versions)

	want := []string{"1.0.0-alpha", "1.0.0-beta", "1.0.0-rc.1", "1.0.0"}
	for i, v := range versions {
		if v.String() != want[i] {
			t.Errorf("Sort() index %d = %s, want %s", i, v, want[i])
		}
	}
}

func TestSortNumericCorrectness(t *testing.T) {
	// This tests that version sorting is numeric, not lexicographic
	versions := []Version{
		MustParse("1.9.0"),
		MustParse("1.10.0"),
		MustParse("1.2.0"),
		MustParse("2.0.0"),
		MustParse("10.0.0"),
	}

	Sort(versions)

	want := []string{"1.2.0", "1.9.0", "1.10.0", "2.0.0", "10.0.0"}
	got := make([]string, len(versions))
	for i, v := range versions {
		got[i] = v.String()
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Sort() = %v, want %v", got, want)
	}
}
