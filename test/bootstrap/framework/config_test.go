package framework

import "testing"

func TestEnabledPlatformsFilter(t *testing.T) {
	platforms := []Platform{
		{Name: "ubuntu-22.04", Enabled: true},
		{Name: "debian-12", Enabled: true},
		{Name: "rhel-9", Enabled: false},
	}

	selected, err := filterPlatforms(platforms, "ubuntu-22.04")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 || selected[0].Name != "ubuntu-22.04" {
		t.Fatalf("unexpected selection: %+v", selected)
	}

	if _, err := filterPlatforms(platforms, "rhel-9"); err == nil {
		t.Fatal("expected disabled platform error")
	}
}

func TestEnabledScenariosFilter(t *testing.T) {
	scenarios := []Scenario{
		{Name: "demo", Enabled: true},
		{Name: "production-single", Enabled: true},
		{Name: "enterprise", Enabled: false},
	}

	selected, err := filterScenarios(scenarios, "demo,production-single")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("unexpected selection count: %d", len(selected))
	}

	if _, err := filterScenarios(scenarios, "enterprise"); err == nil {
		t.Fatal("expected disabled scenario error")
	}
}
