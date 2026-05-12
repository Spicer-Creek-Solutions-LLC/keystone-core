package main

import (
	"reflect"
	"testing"
)

func TestLoadLabelSpecs(t *testing.T) {
	specs, err := loadLabelSpecs()
	if err != nil {
		t.Fatalf("loadLabelSpecs: %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("no label specs loaded")
	}
	byName := map[string]labelSpec{}
	for _, s := range specs {
		if s.Name == "" || s.Color == "" {
			t.Errorf("spec with empty name/color: %+v", s)
		}
		byName[s.Name] = s
	}
	for _, want := range []string{"v1x-backlog", "source/v1x-backlog", "kind/feature", "kind/chore", "area/statemgmt", "v1.0-narrowing"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("missing expected label %q", want)
		}
	}
	if !byName["kind/feature"].Exclusive {
		t.Error("kind/feature should be exclusive")
	}
	if byName["area/statemgmt"].Exclusive {
		t.Error("area/statemgmt should not be exclusive")
	}
}

func TestLoadMilestoneSpecs(t *testing.T) {
	specs, err := loadMilestoneSpecs()
	if err != nil {
		t.Fatalf("loadMilestoneSpecs: %v", err)
	}
	var haveV11 bool
	for _, s := range specs {
		if s.Title == "v1.1" {
			haveV11 = true
		}
	}
	if !haveV11 {
		t.Error("expected a v1.1 milestone spec")
	}
}

func TestLabelDiffers(t *testing.T) {
	spec := labelSpec{Name: "kind/feature", Color: "5319e7", Description: "Feature work", Exclusive: true}
	tests := []struct {
		name string
		cur  label
		want bool
	}{
		{"identical", label{Name: "kind/feature", Color: "5319e7", Description: "Feature work", Exclusive: true}, false},
		{"hash prefix on server", label{Name: "kind/feature", Color: "#5319e7", Description: "Feature work", Exclusive: true}, false},
		{"uppercase color", label{Name: "kind/feature", Color: "#5319E7", Description: "Feature work", Exclusive: true}, false},
		{"different color", label{Name: "kind/feature", Color: "000000", Description: "Feature work", Exclusive: true}, true},
		{"different description", label{Name: "kind/feature", Color: "5319e7", Description: "old", Exclusive: true}, true},
		{"different exclusivity", label{Name: "kind/feature", Color: "5319e7", Description: "Feature work", Exclusive: false}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := labelDiffers(tt.cur, spec); got != tt.want {
				t.Errorf("labelDiffers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInferAreas(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"`service` stdlib module — OpenRC backends", []string{"area/statemgmt"}},
		{"Replay protection on agent commands over NATS", []string{"area/agent", "area/nats", "area/security"}},
		{"Schema versioning via golang-migrate", []string{"area/schema"}},
		{"Full RBAC role/permission CRUD", []string{"area/security"}},
		{"Something with no recognised keywords", nil},
	}
	for _, tt := range tests {
		got := inferAreas(tt.text)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("inferAreas(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestLabelNamesFor(t *testing.T) {
	feat := backlogEntry{Title: "Schema versioning via golang-migrate", Version: "v1.1"}
	got := labelNamesFor(feat)
	want := []string{"v1x-backlog", "source/v1x-backlog", "kind/feature", "area/schema"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("labelNamesFor(feature) = %v, want %v", got, want)
	}

	narrow := backlogEntry{Title: "Bootstrap auto-installs systemd unit", Narrowing: true}
	got = labelNamesFor(narrow)
	want = []string{"v1x-backlog", "source/v1x-backlog", "v1.0-narrowing", "kind/chore", "area/bootstrap"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("labelNamesFor(narrowing) = %v, want %v", got, want)
	}
}
