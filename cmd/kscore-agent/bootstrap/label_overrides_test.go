package bootstrap

import "testing"

func TestApplyNodeLabelOverrides(t *testing.T) {
	opts := &Options{
		NodeLabelArgs: []string{"env=prod", "role=agent"},
	}
	if err := applyNodeLabelOverrides(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts.NodeLabels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(opts.NodeLabels))
	}
	if opts.NodeLabels["env"] != "prod" {
		t.Fatalf("expected env=prod, got %s", opts.NodeLabels["env"])
	}
}

func TestApplyNodeLabelOverridesErrors(t *testing.T) {
	opts := &Options{
		NodeLabelArgs: []string{"=missing"},
	}
	if err := applyNodeLabelOverrides(opts); err == nil {
		t.Fatal("expected error for invalid label")
	}
}
