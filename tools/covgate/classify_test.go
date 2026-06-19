// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestClassifyPackage(t *testing.T) {
	tests := []struct {
		name          string
		relPath       string
		wantCategory  string
		wantThreshold float64
	}{
		{"state engine exact", "internal/statemgmt", "engine", engineThreshold},
		{"stdlib registry is module", "internal/statemgmt/stdlib", "module", moduleThreshold},
		{"stdlib module by prefix", "internal/statemgmt/stdlib/pkg", "module", moduleThreshold},
		{"nested stdlib module", "internal/statemgmt/stdlib/firewalld", "module", moduleThreshold},
		{"critical exact", "internal/identity", "critical", criticalThreshold},
		{"critical by prefix (ratelimit)", "internal/ratelimit/extract", "critical", criticalThreshold},
		{"cli binary", "cmd/kscore-server", "cli", cliThreshold},
		{"cli internal", "internal/cli/identity", "cli", cliThreshold},
		{"excluded generated proto", "pkg/api/v1", "excluded", 0},
		{"excluded tooling", "tools/covgate", "excluded", 0},
		{"unmatched", "internal/somethingnew", "unmatched", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat, th := classifyPackage(tt.relPath)
			if cat != tt.wantCategory {
				t.Errorf("category = %q, want %q", cat, tt.wantCategory)
			}
			if th != tt.wantThreshold {
				t.Errorf("threshold = %.1f, want %.1f", th, tt.wantThreshold)
			}
		})
	}
}

// TestEngineModulePrecedeCritical guards the resolution order: the
// state engine and stdlib modules must classify as engine/module
// (the higher v0.5 bars), never fall through to the critical floor.
func TestEngineModulePrecedeCritical(t *testing.T) {
	if cat, th := classifyPackage("internal/statemgmt"); cat != "engine" || th != engineThreshold {
		t.Errorf("internal/statemgmt = (%q, %.1f), want engine @ %.1f", cat, th, engineThreshold)
	}
	if cat, th := classifyPackage("internal/statemgmt/stdlib/disk"); cat != "module" || th != moduleThreshold {
		t.Errorf("stdlib/disk = (%q, %.1f), want module @ %.1f", cat, th, moduleThreshold)
	}
	if engineThreshold <= criticalThreshold || moduleThreshold <= criticalThreshold {
		t.Errorf("engine (%.0f) and module (%.0f) bars must exceed the critical floor (%.0f)",
			engineThreshold, moduleThreshold, criticalThreshold)
	}
}
