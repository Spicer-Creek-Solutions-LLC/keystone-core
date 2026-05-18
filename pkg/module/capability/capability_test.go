package capability_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	maudit "go.keystone-core.io/keystone-core/pkg/module/audit"
	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

type fakeAuditor struct {
	mu sync.Mutex
	es []maudit.Entry
}

func (f *fakeAuditor) Emit(_ context.Context, e maudit.Entry) {
	f.mu.Lock()
	f.es = append(f.es, e)
	f.mu.Unlock()
}

func (f *fakeAuditor) entries() []maudit.Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]maudit.Entry(nil), f.es...)
}

func grantedManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0",
		Type: manifest.TypeStarlark, Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{
			manifest.CapFSRead: {Paths: []string{"/etc/**"}},
			manifest.CapLog:    {RateLimit: "10/s"},
		},
	}
}

func TestNewRegistryFromManifest(t *testing.T) {
	r, err := capability.NewRegistryFromManifest(grantedManifest())
	if err != nil {
		t.Fatalf("NewRegistryFromManifest: %v", err)
	}
	if !r.Has(manifest.CapFSRead) || !r.Has(manifest.CapLog) {
		t.Fatalf("granted set wrong: %v", r.List())
	}
	if r.Has(manifest.CapExec) {
		t.Fatal("exec must NOT be granted")
	}
	if got := r.List(); len(got) != 2 || got[0] != "fs.read" || got[1] != "log" {
		t.Fatalf("List not sorted/complete: %v", got)
	}

	// Unknown capability rejected defensively (even pre-Validate).
	bad := grantedManifest()
	bad.Capabilities["fs.delete"] = manifest.CapabilityConfig{}
	if _, err := capability.NewRegistryFromManifest(bad); !errors.Is(err, capability.ErrUnknownCapability) {
		t.Fatalf("unknown cap: err = %v, want ErrUnknownCapability", err)
	}
	if _, err := capability.NewRegistryFromManifest(nil); err == nil {
		t.Fatal("nil manifest: want error")
	}
}

func TestNilRegistryDeniesAll(t *testing.T) {
	var r *capability.Registry
	if r.Has(manifest.CapLog) || r.List() != nil {
		t.Fatal("nil registry must deny + return nil list")
	}
}

func TestInvoker_GrantedSuccess(t *testing.T) {
	r, _ := capability.NewRegistryFromManifest(grantedManifest())
	fa := &fakeAuditor{}
	inv := capability.NewInvoker(r, fa)

	ran := false
	err := inv.Invoke(context.Background(), manifest.CapFSRead, "read", func(context.Context) error {
		ran = true
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("granted call: err=%v ran=%v", err, ran)
	}
	es := fa.entries()
	if len(es) != 1 || es[0].Capability != "fs.read" || es[0].Operation != "read" ||
		!es[0].Success || es[0].Module != "acme/widget" {
		t.Fatalf("audit entry = %+v", es)
	}
}

func TestInvoker_GrantedErrorPropagates(t *testing.T) {
	r, _ := capability.NewRegistryFromManifest(grantedManifest())
	fa := &fakeAuditor{}
	inv := capability.NewInvoker(r, fa)

	want := errors.New("backend boom")
	err := inv.Invoke(context.Background(), manifest.CapLog, "write", func(context.Context) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want propagated %v", err, want)
	}
	if es := fa.entries(); len(es) != 1 || es[0].Success {
		t.Fatalf("failed call must audit Success=false: %+v", es)
	}
}

func TestInvoker_NotGrantedDeniedAndAudited(t *testing.T) {
	r, _ := capability.NewRegistryFromManifest(grantedManifest())
	fa := &fakeAuditor{}
	inv := capability.NewInvoker(r, fa)

	ran := false
	err := inv.Invoke(context.Background(), manifest.CapExec, "run", func(context.Context) error {
		ran = true
		return nil
	})
	if ran {
		t.Fatal("fn must NOT run for a non-granted capability")
	}
	if !errors.Is(err, capability.ErrCapabilityNotGranted) {
		t.Fatalf("err = %v, want ErrCapabilityNotGranted", err)
	}
	es := fa.entries()
	if len(es) != 1 || es[0].Success || es[0].Capability != "exec" ||
		es[0].Operation != "denied" || es[0].Details["requested_operation"] != "run" {
		t.Fatalf("denied audit entry = %+v", es)
	}
}

func TestInvoker_NilRegistryDeniesWithAudit(t *testing.T) {
	fa := &fakeAuditor{}
	inv := capability.NewInvoker(nil, fa)
	if err := inv.Invoke(context.Background(), manifest.CapKV, "get", func(context.Context) error {
		return nil
	}); !errors.Is(err, capability.ErrCapabilityNotGranted) {
		t.Fatalf("nil registry: err = %v", err)
	}
	if len(fa.entries()) != 1 {
		t.Fatal("denial must still be audited with a nil registry")
	}
}

func TestInvoker_NilAuditorSafe(t *testing.T) {
	r, _ := capability.NewRegistryFromManifest(grantedManifest())
	inv := capability.NewInvoker(r, nil) // → noop
	if err := inv.Invoke(context.Background(), manifest.CapFSRead, "read", func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatalf("nil auditor must be safe: %v", err)
	}
}
