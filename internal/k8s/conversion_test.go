package k8s

import (
	"testing"
)

func TestStateConfigToConvertedStateFile(t *testing.T) {
	t.Run("basic_conversion", func(t *testing.T) {
		sc := &StateConfig{
			Spec: StateConfigSpec{
				States: []StateDeclaration{
					{
						Name:       "install_nginx",
						Module:     "package",
						Parameters: map[string]interface{}{"name": "nginx"},
					},
					{
						Name:       "nginx_config",
						Module:     "file",
						Parameters: map[string]interface{}{"path": "/etc/nginx/nginx.conf", "content": "server {}"},
						Requisites: map[string][]string{"require": {"install_nginx"}},
					},
					{
						Name:       "nginx_running",
						Module:     "service",
						Parameters: map[string]interface{}{"name": "nginx"},
						Requisites: map[string][]string{"require": {"install_nginx"}, "watch": {"nginx_config"}},
					},
				},
				Vars: map[string]interface{}{"env": "production", "port": float64(80)},
			},
		}

		sf := StateConfigToConvertedStateFile(sc)

		// Check variables
		if sf.Variables["env"] != "production" {
			t.Errorf("Expected env=production, got %v", sf.Variables["env"])
		}

		// Check states are grouped by module
		if len(sf.States["package"]) != 1 {
			t.Errorf("Expected 1 package state, got %d", len(sf.States["package"]))
		}
		if len(sf.States["file"]) != 1 {
			t.Errorf("Expected 1 file state, got %d", len(sf.States["file"]))
		}
		if len(sf.States["service"]) != 1 {
			t.Errorf("Expected 1 service state, got %d", len(sf.States["service"]))
		}

		// Check state details
		pkg := sf.States["package"][0]
		if pkg.ID != "install_nginx" {
			t.Errorf("Expected ID install_nginx, got %s", pkg.ID)
		}
		if pkg.Module != "package" {
			t.Errorf("Expected Module package, got %s", pkg.Module)
		}

		// Check requisites conversion
		file := sf.States["file"][0]
		if len(file.Requisites.Require) != 1 {
			t.Fatalf("Expected 1 require requisite, got %d", len(file.Requisites.Require))
		}
		if file.Requisites.Require[0] != "install_nginx" {
			t.Errorf("Expected require install_nginx, got %s", file.Requisites.Require[0])
		}

		svc := sf.States["service"][0]
		if len(svc.Requisites.Require) != 1 {
			t.Errorf("Expected 1 require for service, got %d", len(svc.Requisites.Require))
		}
		if len(svc.Requisites.Watch) != 1 {
			t.Errorf("Expected 1 watch for service, got %d", len(svc.Requisites.Watch))
		}
	})

	t.Run("empty_states", func(t *testing.T) {
		sc := &StateConfig{
			Spec: StateConfigSpec{},
		}

		sf := StateConfigToConvertedStateFile(sc)
		if len(sf.States) != 0 {
			t.Errorf("Expected empty states map, got %d entries", len(sf.States))
		}
	})

	t.Run("nil_vars", func(t *testing.T) {
		sc := &StateConfig{
			Spec: StateConfigSpec{
				States: []StateDeclaration{
					{Name: "test", Module: "file"},
				},
			},
		}

		sf := StateConfigToConvertedStateFile(sc)
		if sf.Variables != nil {
			t.Errorf("Expected nil variables, got %v", sf.Variables)
		}
		if len(sf.States["file"]) != 1 {
			t.Errorf("Expected 1 file state, got %d", len(sf.States["file"]))
		}
	})

	t.Run("multiple_same_module", func(t *testing.T) {
		sc := &StateConfig{
			Spec: StateConfigSpec{
				States: []StateDeclaration{
					{Name: "config_a", Module: "file", Parameters: map[string]interface{}{"path": "/a"}},
					{Name: "config_b", Module: "file", Parameters: map[string]interface{}{"path": "/b"}},
					{Name: "config_c", Module: "file", Parameters: map[string]interface{}{"path": "/c"}},
				},
			},
		}

		sf := StateConfigToConvertedStateFile(sc)
		if len(sf.States["file"]) != 3 {
			t.Errorf("Expected 3 file states, got %d", len(sf.States["file"]))
		}
	})

	t.Run("all_requisite_types", func(t *testing.T) {
		sc := &StateConfig{
			Spec: StateConfigSpec{
				States: []StateDeclaration{
					{
						Name:   "full_reqs",
						Module: "file",
						Requisites: map[string][]string{
							"require":      {"a"},
							"require_in":   {"b"},
							"watch":        {"c"},
							"watch_in":     {"d"},
							"prereq":       {"e"},
							"prereq_in":    {"f"},
							"onchanges":    {"g"},
							"onchanges_in": {"h"},
						},
					},
				},
			},
		}

		sf := StateConfigToConvertedStateFile(sc)
		reqs := sf.States["file"][0].Requisites

		if len(reqs.Require) != 1 || reqs.Require[0] != "a" {
			t.Errorf("Require: got %v", reqs.Require)
		}
		if len(reqs.RequireIn) != 1 || reqs.RequireIn[0] != "b" {
			t.Errorf("RequireIn: got %v", reqs.RequireIn)
		}
		if len(reqs.Watch) != 1 || reqs.Watch[0] != "c" {
			t.Errorf("Watch: got %v", reqs.Watch)
		}
		if len(reqs.WatchIn) != 1 || reqs.WatchIn[0] != "d" {
			t.Errorf("WatchIn: got %v", reqs.WatchIn)
		}
		if len(reqs.Prereq) != 1 || reqs.Prereq[0] != "e" {
			t.Errorf("Prereq: got %v", reqs.Prereq)
		}
		if len(reqs.PrereqIn) != 1 || reqs.PrereqIn[0] != "f" {
			t.Errorf("PrereqIn: got %v", reqs.PrereqIn)
		}
		if len(reqs.Onchanges) != 1 || reqs.Onchanges[0] != "g" {
			t.Errorf("Onchanges: got %v", reqs.Onchanges)
		}
		if len(reqs.OnchangesIn) != 1 || reqs.OnchangesIn[0] != "h" {
			t.Errorf("OnchangesIn: got %v", reqs.OnchangesIn)
		}
	})

	t.Run("no_requisites", func(t *testing.T) {
		sc := &StateConfig{
			Spec: StateConfigSpec{
				States: []StateDeclaration{
					{Name: "no_reqs", Module: "package"},
				},
			},
		}

		sf := StateConfigToConvertedStateFile(sc)
		reqs := sf.States["package"][0].Requisites
		if len(reqs.Require) != 0 || len(reqs.Watch) != 0 {
			t.Errorf("Expected empty requisites, got require=%d watch=%d", len(reqs.Require), len(reqs.Watch))
		}
	})
}
