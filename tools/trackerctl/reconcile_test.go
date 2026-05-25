// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"testing"
)

func TestIsManagedLabel(t *testing.T) {
	managed := []string{"source/v1x-backlog", "kind/feature", "kind/release-tracker", "roadmap-backlog"}
	for _, n := range managed {
		if !isManagedLabel(n) {
			t.Errorf("%q should be managed", n)
		}
	}
	for _, n := range []string{"area/agent", "area/statemgmt", "wontfix", "v1x-backlog", "v1.0-narrowing", ""} {
		if isManagedLabel(n) {
			t.Errorf("%q should not be managed", n)
		}
	}
}

func TestManagedLabelDelta(t *testing.T) {
	tests := []struct {
		name          string
		want, current []string
		expectAdd     []string
		expectRemove  []string
	}{
		{
			name:    "no change",
			want:    []string{"roadmap-backlog", "source/v1x-backlog", "kind/feature", "area/agent"},
			current: []string{"area/agent", "kind/feature", "source/v1x-backlog", "roadmap-backlog"},
		},
		{
			name:      "add missing umbrella",
			want:      []string{"roadmap-backlog", "source/v1x-backlog", "kind/feature"},
			current:   []string{"kind/feature"},
			expectAdd: []string{"roadmap-backlog", "source/v1x-backlog"},
		},
		{
			name:         "remove stray managed label",
			want:         []string{"roadmap-backlog", "source/v1x-backlog", "kind/feature"},
			current:      []string{"roadmap-backlog", "source/v1x-backlog", "kind/feature", "source/triage"},
			expectRemove: []string{"source/triage"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add, remove := managedLabelDelta(tt.want, tt.current)
			if !reflect.DeepEqual(add, tt.expectAdd) {
				t.Errorf("add = %v, want %v", add, tt.expectAdd)
			}
			if !reflect.DeepEqual(remove, tt.expectRemove) {
				t.Errorf("remove = %v, want %v", remove, tt.expectRemove)
			}
		})
	}
}
