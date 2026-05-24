// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func featureManifest() *Manifest {
	return &Manifest{
		Metadata:    Metadata{Name: "demo", Version: "1.0.0"},
		Entrypoints: Entrypoints{Default: "x"},
		Features: map[string]Feature{
			"tls":     {Default: true, States: []string{"files:/etc/tls.conf"}},
			"metrics": {Default: false, States: []string{"pkg:prometheus"}},
		},
	}
}

func TestEvaluateFeatures(t *testing.T) {
	m := featureManifest()

	t.Run("defaults", func(t *testing.T) {
		got, err := EvaluateFeatures(m, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !got["tls"] || got["metrics"] {
			t.Fatalf("defaults wrong: %v", got)
		}
		if len(got) != 2 {
			t.Fatalf("want full map, got %v", got)
		}
	})

	t.Run("override on and off", func(t *testing.T) {
		got, err := EvaluateFeatures(m, []string{"metrics"}, []string{"tls"})
		if err != nil {
			t.Fatal(err)
		}
		if got["tls"] || !got["metrics"] {
			t.Fatalf("override wrong: %v", got)
		}
	})

	t.Run("unknown enable", func(t *testing.T) {
		_, err := EvaluateFeatures(m, []string{"ghost"}, nil)
		if !errors.Is(err, ErrUnknownFeature) {
			t.Fatalf("err=%v want ErrUnknownFeature", err)
		}
	})

	t.Run("unknown disable", func(t *testing.T) {
		_, err := EvaluateFeatures(m, nil, []string{"ghost"})
		if !errors.Is(err, ErrUnknownFeature) {
			t.Fatalf("err=%v want ErrUnknownFeature", err)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		_, err := EvaluateFeatures(m, []string{"tls"}, []string{"tls"})
		if !errors.Is(err, ErrFeatureConflict) {
			t.Fatalf("err=%v want ErrFeatureConflict", err)
		}
	})
}

func TestFilterStateFile(t *testing.T) {
	m := featureManifest()
	sf := &statemgmt.StateFile{
		Declarations: []*statemgmt.Declaration{
			{ID: "files:/etc/tls.conf", Module: "file"},
			{ID: "pkg:prometheus", Module: "pkg"},
			{ID: "files:/etc/app.conf", Module: "file"}, // ungated
			nil,
		},
	}

	t.Run("disabled feature drops its declarations", func(t *testing.T) {
		enabled, _ := EvaluateFeatures(m, nil, nil) // tls on, metrics off
		got, err := FilterStateFile(sf, m, enabled)
		if err != nil {
			t.Fatal(err)
		}
		ids := declIDs(got)
		if has(ids, "pkg:prometheus") {
			t.Errorf("metrics disabled but pkg:prometheus kept: %v", ids)
		}
		if !has(ids, "files:/etc/tls.conf") || !has(ids, "files:/etc/app.conf") {
			t.Errorf("kept set wrong: %v", ids)
		}
	})

	t.Run("enabling metrics keeps its declarations", func(t *testing.T) {
		enabled, _ := EvaluateFeatures(m, []string{"metrics"}, nil)
		got, _ := FilterStateFile(sf, m, enabled)
		if !has(declIDs(got), "pkg:prometheus") {
			t.Error("metrics enabled but pkg:prometheus dropped")
		}
	})

	t.Run("nil statefile", func(t *testing.T) {
		got, err := FilterStateFile(nil, m, nil)
		if got != nil || err != nil {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})

	t.Run("does not mutate input", func(t *testing.T) {
		enabled, _ := EvaluateFeatures(m, nil, nil)
		_, _ = FilterStateFile(sf, m, enabled)
		if len(sf.Declarations) != 4 {
			t.Fatalf("input mutated: %d decls", len(sf.Declarations))
		}
	})
}

func declIDs(sf *statemgmt.StateFile) []string {
	var out []string
	for _, d := range sf.Declarations {
		out = append(out, d.ID)
	}
	return out
}

func has(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
