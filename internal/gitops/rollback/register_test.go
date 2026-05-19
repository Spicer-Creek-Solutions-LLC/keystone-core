package rollback

import "testing"

func TestRegisterAll_And_DefaultRegistry(t *testing.T) {
	t.Parallel()
	// K8s nil is valid (client-go adapter deferred to boot).
	reg := NewDefaultRegistry(Deps{Git: &fakeGit{}, Argo: &fakeArgo{}})
	for _, typ := range []string{"git", "argocd", "k8s"} {
		if _, ok := reg.Lookup(typ); !ok {
			t.Errorf("default registry missing executor %q", typ)
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
		t.Fatalf("RegisterAll (re-run): %v", err)
	}
	if _, ok := reg.Lookup("git"); !ok {
		t.Error("git executor missing after re-register")
	}
}
