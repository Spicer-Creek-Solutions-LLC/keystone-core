package bootstrap

import (
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/platform"
)

func TestBuildRepoPlanAPT(t *testing.T) {
	plan, err := BuildRepoPlan(platform.PackageManagerAPT, "stable")
	if err != nil {
		t.Fatalf("BuildRepoPlan returned error: %v", err)
	}
	content := plan.Files["/etc/apt/sources.list.d/kscore.list"]
	if !strings.Contains(content, "https://apt.keystonecore.io") {
		t.Fatalf("unexpected repo content: %s", content)
	}
	if plan.KeyPath == "" {
		t.Fatal("expected key path for apt repo plan")
	}
}

func TestBuildRepoPlanUnsupported(t *testing.T) {
	if _, err := BuildRepoPlan(platform.PackageManagerUnknown, "stable"); err == nil {
		t.Fatal("expected error for unsupported package manager")
	}
}
