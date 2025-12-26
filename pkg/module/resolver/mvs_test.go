package resolver

import (
	"testing"
)

func TestMVS_ResolveWithVersions_SingleConstraint(t *testing.T) {
	mvs := NewMVSConflictResolver()
	parser := &DefaultConstraintParser{}

	available := []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0"}

	constraint, _ := parser.Parse("^1.0.0")
	result, err := mvs.ResolveWithVersions("test/module", []VersionConstraint{constraint}, available)
	if err != nil {
		t.Fatalf("ResolveWithVersions() error = %v", err)
	}

	// Should select highest matching version
	if result != "1.2.0" {
		t.Errorf("ResolveWithVersions() = %v, want 1.2.0", result)
	}
}

func TestMVS_ResolveWithVersions_MultipleConstraints(t *testing.T) {
	mvs := NewMVSConflictResolver()
	parser := &DefaultConstraintParser{}

	available := []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0", "2.0.0"}

	// Constraints: >=1.1.0 and <1.3.0
	c1, _ := parser.Parse(">=1.1.0")
	c2, _ := parser.Parse("<1.3.0")

	result, err := mvs.ResolveWithVersions("test/module",
		[]VersionConstraint{c1, c2}, available)
	if err != nil {
		t.Fatalf("ResolveWithVersions() error = %v", err)
	}

	// Should select highest version satisfying both: 1.2.0
	if result != "1.2.0" {
		t.Errorf("ResolveWithVersions() = %v, want 1.2.0", result)
	}
}

func TestMVS_ResolveWithVersions_NoMatchingVersions(t *testing.T) {
	mvs := NewMVSConflictResolver()
	parser := &DefaultConstraintParser{}

	available := []string{"1.0.0", "1.1.0"}

	// Constraint that no available version satisfies
	constraint, _ := parser.Parse(">=2.0.0")

	_, err := mvs.ResolveWithVersions("test/module", []VersionConstraint{constraint}, available)
	if err == nil {
		t.Error("ResolveWithVersions() should error when no versions match")
	}
}

func TestMVS_ResolveWithVersions_ConflictingConstraints(t *testing.T) {
	mvs := NewMVSConflictResolver()
	parser := &DefaultConstraintParser{}

	available := []string{"1.0.0", "1.5.0", "2.0.0"}

	// Conflicting: <1.2.0 and >1.8.0 (no overlap)
	c1, _ := parser.Parse("<1.2.0")
	c2, _ := parser.Parse(">1.8.0")

	_, err := mvs.ResolveWithVersions("test/module",
		[]VersionConstraint{c1, c2}, available)
	if err == nil {
		t.Error("ResolveWithVersions() should error on conflicting constraints")
	}
}

func TestMVS_Strategy(t *testing.T) {
	mvs := NewMVSConflictResolver()

	if mvs.Strategy() != "MVS" {
		t.Errorf("Strategy() = %v, want MVS", mvs.Strategy())
	}
}

func TestBuildRequirementList_AddRequirement(t *testing.T) {
	list := NewBuildRequirementList()

	err := list.AddRequirement("test/module", "=1.2.3")
	if err != nil {
		t.Fatalf("AddRequirement() error = %v", err)
	}

	version, exists := list.GetRequirement("test/module")
	if !exists {
		t.Fatal("GetRequirement() should return existing requirement")
	}

	if version.String() != "1.2.3" {
		t.Errorf("GetRequirement() = %v, want 1.2.3", version.String())
	}
}

func TestBuildRequirementList_HigherVersionWins(t *testing.T) {
	list := NewBuildRequirementList()

	// Add lower version first
	list.AddRequirement("test/module", "=1.0.0")

	// Add higher version
	list.AddRequirement("test/module", "=1.5.0")

	version, _ := list.GetRequirement("test/module")

	// Should keep the higher version
	if version.String() != "1.5.0" {
		t.Errorf("GetRequirement() = %v, want 1.5.0 (higher version)", version.String())
	}
}

func TestBuildRequirementList_LowerVersionDoesNotReplace(t *testing.T) {
	list := NewBuildRequirementList()

	// Add higher version first
	list.AddRequirement("test/module", "=2.0.0")

	// Try to add lower version
	list.AddRequirement("test/module", "=1.0.0")

	version, _ := list.GetRequirement("test/module")

	// Should keep the higher version
	if version.String() != "2.0.0" {
		t.Errorf("GetRequirement() = %v, want 2.0.0 (existing higher version)", version.String())
	}
}

func TestBuildRequirementList_Merge(t *testing.T) {
	list1 := NewBuildRequirementList()
	list1.AddRequirement("module-a", "=1.0.0")
	list1.AddRequirement("module-b", "=2.0.0")

	list2 := NewBuildRequirementList()
	list2.AddRequirement("module-b", "=2.5.0") // Higher version
	list2.AddRequirement("module-c", "=3.0.0") // New module

	list1.Merge(list2)

	// module-a should be unchanged
	if v, _ := list1.GetRequirement("module-a"); v.String() != "1.0.0" {
		t.Errorf("module-a = %v, want 1.0.0", v.String())
	}

	// module-b should be updated to higher version
	if v, _ := list1.GetRequirement("module-b"); v.String() != "2.5.0" {
		t.Errorf("module-b = %v, want 2.5.0", v.String())
	}

	// module-c should be added
	if v, exists := list1.GetRequirement("module-c"); !exists || v.String() != "3.0.0" {
		t.Errorf("module-c = %v (exists=%v), want 3.0.0", v, exists)
	}
}

func TestBuildRequirementList_GetAllRequirements(t *testing.T) {
	list := NewBuildRequirementList()
	list.AddRequirement("module-a", "=1.0.0")
	list.AddRequirement("module-b", "=2.0.0")

	all := list.GetAllRequirements()

	if len(all) != 2 {
		t.Errorf("GetAllRequirements() = %v modules, want 2", len(all))
	}

	if all["module-a"] != "1.0.0" {
		t.Errorf("module-a version = %v, want 1.0.0", all["module-a"])
	}

	if all["module-b"] != "2.0.0" {
		t.Errorf("module-b version = %v, want 2.0.0", all["module-b"])
	}
}

func TestBuildRequirementList_Sort(t *testing.T) {
	list := NewBuildRequirementList()
	list.AddRequirement("zebra", "=1.0.0")
	list.AddRequirement("alpha", "=2.0.0")
	list.AddRequirement("beta", "=3.0.0")

	sorted := list.Sort()

	if len(sorted) != 3 {
		t.Fatalf("Sort() = %v items, want 3", len(sorted))
	}

	// Should be alphabetically sorted
	if sorted[0].Module != "alpha" || sorted[1].Module != "beta" || sorted[2].Module != "zebra" {
		t.Errorf("Sort() order = %v, %v, %v; want alpha, beta, zebra",
			sorted[0].Module, sorted[1].Module, sorted[2].Module)
	}
}
