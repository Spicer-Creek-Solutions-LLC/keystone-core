// SPDX-License-Identifier: Apache-2.0

package verification

import "testing"

func TestRegisterAll_And_DefaultRegistry(t *testing.T) {
	t.Parallel()
	reg := NewDefaultRegistry(Deps{CommandRunner: &fakeRunner{}})
	for _, typ := range []string{"http", "grpc", "command"} {
		if _, ok := reg.Lookup(typ); !ok {
			t.Errorf("default registry missing verifier %q", typ)
		}
	}
}

func TestRegisterAll_Idempotent(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := RegisterAll(reg, Deps{}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	if err := RegisterAll(reg, Deps{}); err != nil {
		t.Fatalf("RegisterAll (re-run, overwrites): %v", err)
	}
	if _, ok := reg.Lookup("http"); !ok {
		t.Error("http verifier missing after re-register")
	}
}
