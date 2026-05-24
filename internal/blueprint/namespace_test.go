// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func collectionFixture() *statemgmt.StateFile {
	return &statemgmt.StateFile{
		Declarations: []*statemgmt.Declaration{
			{ID: "pkg:nginx", Module: "pkg", State: "installed", Name: "nginx"},
			{
				ID:     "files:/etc/nginx.conf",
				Module: "file",
				State:  "managed",
				Name:   "/etc/nginx.conf",
				Params: map[string]any{
					"mode": "0644",
					// intra-collection ref + an external ref.
					"require": []any{
						map[string]any{"pkg": "nginx"},
						map[string]any{"pkg": "other-blueprint-thing"},
					},
				},
			},
			nil,
		},
	}
}

func TestNamespace(t *testing.T) {
	src := collectionFixture()
	got, err := Namespace(src, "inst1")
	if err != nil {
		t.Fatalf("Namespace: %v", err)
	}
	if len(got.Declarations) != 2 {
		t.Fatalf("decl count = %d", len(got.Declarations))
	}

	ids := declIDs(got)
	if !has(ids, "pkg:inst1/nginx") || !has(ids, "files:inst1//etc/nginx.conf") {
		t.Fatalf("namespaced ids wrong: %v", ids)
	}

	conf := got.Declarations[1]
	if conf.Module != "file" || conf.State != "managed" || conf.Name != "/etc/nginx.conf" {
		t.Errorf("module/state/name must be untouched: %+v", conf)
	}
	if conf.Params["mode"] != "0644" {
		t.Errorf("non-requisite param lost: %v", conf.Params)
	}
	req := conf.Params["require"].([]any)
	in := req[0].(map[string]any)
	ext := req[1].(map[string]any)
	if in["pkg"] != "inst1/nginx" {
		t.Errorf("intra-collection ref not rewritten: %v", in)
	}
	if ext["pkg"] != "other-blueprint-thing" {
		t.Errorf("external ref must be untouched: %v", ext)
	}

	// Input not mutated.
	if src.Declarations[0].ID != "pkg:nginx" {
		t.Errorf("source mutated: %s", src.Declarations[0].ID)
	}
	if src.Declarations[1].Params["require"].([]any)[0].(map[string]any)["pkg"] != "nginx" {
		t.Error("source requisite mutated")
	}
}

func TestNamespace_Errors(t *testing.T) {
	if _, err := Namespace(nil, "x"); err != nil {
		t.Fatalf("nil sf: %v", err)
	}
	if _, err := Namespace(collectionFixture(), "Bad Name"); !errors.Is(err, ErrInvalidNamespace) {
		t.Fatalf("err=%v want ErrInvalidNamespace", err)
	}
	if _, err := Namespace(collectionFixture(), ""); !errors.Is(err, ErrInvalidNamespace) {
		t.Fatalf("err=%v want ErrInvalidNamespace", err)
	}
}

func TestNamespace_IDWithoutColon(t *testing.T) {
	sf := &statemgmt.StateFile{Declarations: []*statemgmt.Declaration{{ID: "bareid", Module: "m"}}}
	got, err := Namespace(sf, "ns")
	if err != nil {
		t.Fatal(err)
	}
	if got.Declarations[0].ID != "ns/bareid" {
		t.Fatalf("id=%q want ns/bareid", got.Declarations[0].ID)
	}
}

func TestDetectCollisions(t *testing.T) {
	base := collectionFixture()

	t.Run("no collision across distinct namespaces", func(t *testing.T) {
		a, _ := Namespace(base, "inst1")
		b, _ := Namespace(base, "inst2")
		if err := DetectCollisions(a, b); err != nil {
			t.Fatalf("unexpected collision: %v", err)
		}
	})

	t.Run("namespaced vs unnamespaced is fine (disjoint ids)", func(t *testing.T) {
		ns, _ := Namespace(base, "inst1")
		if err := DetectCollisions(base, ns, nil); err != nil {
			t.Fatalf("unexpected collision: %v", err)
		}
	})

	t.Run("same namespace twice collides", func(t *testing.T) {
		a, _ := Namespace(base, "inst1")
		b, _ := Namespace(base, "inst1")
		err := DetectCollisions(a, b)
		if !errors.Is(err, ErrStateNameCollision) {
			t.Fatalf("err=%v want ErrStateNameCollision", err)
		}
		if !strings.Contains(err.Error(), "pkg:inst1/nginx") {
			t.Errorf("collision detail missing: %v", err)
		}
	})

	t.Run("unnamespaced duplicate collides", func(t *testing.T) {
		if err := DetectCollisions(base, base); !errors.Is(err, ErrStateNameCollision) {
			t.Fatalf("err=%v want ErrStateNameCollision", err)
		}
	})
}
