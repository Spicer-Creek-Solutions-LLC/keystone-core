package bootstrap

import (
	"testing"

	"github.com/shawnbutts/keystone-core/internal/platform"
)

func TestBuildPackagePlanAPT(t *testing.T) {
	packages := []string{"kscore-server", "kscore-agent"}
	plan, err := BuildPackagePlan(platform.PackageManagerAPT, "stable", packages, "")
	if err != nil {
		t.Fatalf("BuildPackagePlan returned error: %v", err)
	}
	if plan.Manager != platform.PackageManagerAPT {
		t.Fatalf("expected manager apt, got %s", plan.Manager)
	}
	if len(plan.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(plan.Commands))
	}
	if plan.Commands[0].Name != "apt-get" || plan.Commands[0].Args[0] != "update" {
		t.Fatalf("unexpected first command: %s %v", plan.Commands[0].Name, plan.Commands[0].Args)
	}
	if plan.Commands[1].Name != "apt-get" {
		t.Fatalf("unexpected install command name: %s", plan.Commands[1].Name)
	}
	if len(plan.Packages) != len(packages) {
		t.Fatalf("expected %d packages, got %d", len(packages), len(plan.Packages))
	}
}

func TestBuildPackagePlanUnsupported(t *testing.T) {
	if _, err := BuildPackagePlan(platform.PackageManagerPacman, "stable", []string{"kscore-agent"}, ""); err == nil {
		t.Fatal("expected error for unsupported package manager")
	}
}

func TestBuildPackagePlanVersionPin(t *testing.T) {
	packages := []string{"kscore-agent"}
	plan, err := BuildPackagePlan(platform.PackageManagerAPT, "stable", packages, "1.2.3")
	if err != nil {
		t.Fatalf("BuildPackagePlan returned error: %v", err)
	}
	if plan.Packages[0] != "kscore-agent=1.2.3" {
		t.Fatalf("expected pinned package, got %s", plan.Packages[0])
	}
}

func TestPackagesForRole(t *testing.T) {
	tests := []struct {
		role     string
		packages []string
	}{
		{role: "control-plane", packages: []string{"kscore-server"}},
		{role: "agent", packages: []string{"kscore-agent"}},
		{role: "both", packages: []string{"kscore-server", "kscore-agent"}},
		{role: "unknown", packages: nil},
	}

	for _, tt := range tests {
		pkgs := packagesForRole(tt.role)
		if len(pkgs) != len(tt.packages) {
			t.Fatalf("expected %d packages for role %s, got %d", len(tt.packages), tt.role, len(pkgs))
		}
		for i, pkg := range tt.packages {
			if pkgs[i] != pkg {
				t.Fatalf("expected package %s at index %d, got %s", pkg, i, pkgs[i])
			}
		}
	}
}
