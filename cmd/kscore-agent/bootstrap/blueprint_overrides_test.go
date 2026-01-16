package bootstrap

import "testing"

func TestApplyBlueprintOverrides(t *testing.T) {
	opts := &Options{
		BlueprintParamArgs:      []string{"blueprints/demo:replicas=2"},
		BlueprintFeatureArgs:    []string{"blueprints/demo:monitoring=true"},
		BlueprintEntrypointArgs: []string{"blueprints/demo:states/primary.yaml"},
	}

	if err := applyBlueprintOverrides(opts); err != nil {
		t.Fatalf("applyBlueprintOverrides returned error: %v", err)
	}

	if len(opts.BlueprintParams) != 1 {
		t.Fatalf("expected 1 blueprint param, got %d", len(opts.BlueprintParams))
	}
	if len(opts.BlueprintFeatures) != 1 {
		t.Fatalf("expected 1 blueprint feature, got %d", len(opts.BlueprintFeatures))
	}
	if entry := opts.BlueprintEntrypoints["blueprints/demo"]; entry != "states/primary.yaml" {
		t.Fatalf("unexpected entrypoint: %s", entry)
	}
}
