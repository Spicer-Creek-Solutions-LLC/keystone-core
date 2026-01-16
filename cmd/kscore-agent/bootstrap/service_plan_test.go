package bootstrap

import "testing"

func TestBuildServicePlanSystemdBoth(t *testing.T) {
	cfg := &BootstrapConfig{
		NodeRole: "both",
		NATSMode: "embedded",
		Storage:  "sqlite",
	}

	plan, err := BuildServicePlan(cfg, "systemd")
	if err != nil {
		t.Fatalf("BuildServicePlan returned error: %v", err)
	}
	if len(plan.Files) != 4 {
		t.Fatalf("expected 4 files, got %d", len(plan.Files))
	}
	if len(plan.Commands) < 5 {
		t.Fatalf("expected at least 5 commands, got %d", len(plan.Commands))
	}
}
