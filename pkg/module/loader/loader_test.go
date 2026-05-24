// SPDX-License-Identifier: Apache-2.0

package loader_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/cas"
	"go.keystone-core.io/keystone-core/pkg/module/loader"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

const mainStar = "def main():\n  return 1\n"

func moduleZip(t *testing.T, m *manifest.Manifest) []byte {
	t.Helper()
	manBytes, err := manifest.MarshalManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name string, data []byte) {
		w, werr := zw.Create(name)
		if werr != nil {
			t.Fatal(werr)
		}
		if _, werr := w.Write(data); werr != nil {
			t.Fatal(werr)
		}
	}
	write("manifest.yaml", manBytes)
	write(m.Entrypoint, []byte(mainStar))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeZip(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mod.zip")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func sampleManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{
			manifest.CapLog: {RateLimit: "100/s"},
			manifest.CapKV:  {},
		},
	}
}

// --- fakes ---------------------------------------------------------------

type fakeInstance struct {
	caps   map[string]any
	closed bool
}

func (f *fakeInstance) Execute(_ context.Context, in map[string]any) (*loader.ExecuteResult, error) {
	return &loader.ExecuteResult{Output: map[string]any{"echo": in["x"], "ncaps": len(f.caps)}}, nil
}
func (f *fakeInstance) Close() error { f.closed = true; return nil }

type fakeRuntime struct {
	last *fakeInstance
}

func (r *fakeRuntime) Init(_ context.Context, _ *manifest.Manifest, entry []byte, caps map[string]any) (loader.Instance, error) {
	if len(entry) == 0 {
		return nil, errors.New("empty entrypoint")
	}
	r.last = &fakeInstance{caps: caps}
	return r.last, nil
}

type denyPolicy struct {
	denyManifest bool
	denyCap      string
}

func (p denyPolicy) CheckManifest(_ context.Context, _ *manifest.Manifest) (loader.PolicyResult, error) {
	if p.denyManifest {
		return loader.PolicyResult{Allowed: false, Detail: "nope"}, nil
	}
	return loader.PolicyResult{Allowed: true}, nil
}
func (p denyPolicy) CheckCapability(_ context.Context, _, name string, _ manifest.CapabilityConfig) (loader.CapabilityDecision, error) {
	if name == p.denyCap {
		return loader.CapabilityDecision{Allowed: false, Reason: "blocked"}, nil
	}
	return loader.CapabilityDecision{Allowed: true}, nil
}

type recordObs struct {
	mu     sync.Mutex
	phases []string
}

func (o *recordObs) OnLoadEvent(e loader.LoadEvent) {
	if e.Err != nil {
		return
	}
	o.mu.Lock()
	o.phases = append(o.phases, e.Phase)
	o.mu.Unlock()
}

type countVerifier struct {
	inner *verify.Verifier
	n     int
}

func (c *countVerifier) Verify(b []byte, s verify.Signature) error {
	c.n++
	return c.inner.Verify(b, s)
}

// signFor returns a trust policy + a signer over the given blob.
func signFor(t *testing.T, blob []byte) (*verify.TrustPolicy, verify.Signature) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tp := verify.NewTrustPolicy()
	if _, err := tp.AddKey(k.Public()); err != nil {
		t.Fatal(err)
	}
	sig, err := verify.Sign(blob, k)
	if err != nil {
		t.Fatal(err)
	}
	return tp, sig
}

func runtimes(rt loader.Runtime) *loader.RuntimeRegistry {
	rr := loader.NewRuntimeRegistry()
	rr.Register(manifest.TypeStarlark, rt)
	return rr
}

// --- tests ---------------------------------------------------------------

func TestLoad_HappyPath(t *testing.T) {
	z := moduleZip(t, sampleManifest())
	path := writeZip(t, z)
	tp, sig := signFor(t, z)
	rt := &fakeRuntime{}
	obs := &recordObs{}

	l := loader.New(loader.Config{
		Verifier: verify.NewVerifier(tp),
		Runtimes: runtimes(rt),
		Observer: obs,
	})
	res, err := l.Load(context.Background(), path, loader.LoadOptions{Signature: &sig})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if res.Manifest.Name != "acme/widget" || res.Runtime == nil {
		t.Fatalf("result = %+v", res)
	}
	if !res.VerificationResult.Verified || res.VerificationResult.Hash != cas.HashBytes(z) {
		t.Fatalf("verification = %+v", res.VerificationResult)
	}
	if len(res.RegisteredCapabilities) != 2 || len(res.DeniedCapabilities) != 0 {
		t.Fatalf("caps reg=%v den=%v", res.RegisteredCapabilities, res.DeniedCapabilities)
	}
	// Runtime received exactly the granted capability backends.
	if len(rt.last.caps) != 2 {
		t.Fatalf("runtime caps = %d, want 2", len(rt.last.caps))
	}
	if _, ok := rt.last.caps[manifest.CapLog].(*capability.Log); !ok {
		t.Fatalf("log cap type = %T", rt.last.caps[manifest.CapLog])
	}
	want := []string{"parse", "verify", "policy", "capability_policy", "capability_lock", "runtime_init", "register"}
	if len(obs.phases) != len(want) {
		t.Fatalf("phases = %v", obs.phases)
	}
	for i, p := range want {
		if obs.phases[i] != p {
			t.Fatalf("phase[%d] = %q, want %q (%v)", i, obs.phases[i], p, obs.phases)
		}
	}

	out, err := l.Execute(context.Background(), res, map[string]any{"x": 42})
	if err != nil || out.Output["echo"] != 42 {
		t.Fatalf("Execute = %+v, %v", out, err)
	}
}

func TestLoad_SkipVerificationAndPolicy(t *testing.T) {
	z := moduleZip(t, sampleManifest())
	path := writeZip(t, z)
	l := loader.New(loader.Config{Runtimes: runtimes(&fakeRuntime{})}) // no verifier/policy
	res, err := l.Load(context.Background(), path, loader.LoadOptions{
		SkipVerification: true, SkipPolicyValidation: true,
	})
	if err != nil {
		t.Fatalf("Load (skips): %v", err)
	}
	if res.VerificationResult.Verified {
		t.Fatal("Verified should be false when skipped")
	}
	if !res.PolicyResult.Allowed {
		t.Fatal("policy should default-allow when skipped")
	}
}

func TestLoad_VerificationFailures(t *testing.T) {
	z := moduleZip(t, sampleManifest())
	path := writeZip(t, z)

	// No verifier / no signature.
	l := loader.New(loader.Config{Runtimes: runtimes(&fakeRuntime{})})
	if _, err := l.Load(context.Background(), path, loader.LoadOptions{}); !errors.Is(err, loader.ErrVerification) {
		t.Fatalf("no verifier = %v, want ErrVerification", err)
	}

	// Signature over different content → mismatch.
	tp, badSig := signFor(t, []byte("other content"))
	l = loader.New(loader.Config{Verifier: verify.NewVerifier(tp), Runtimes: runtimes(&fakeRuntime{})})
	if _, err := l.Load(context.Background(), path, loader.LoadOptions{Signature: &badSig}); !errors.Is(err, loader.ErrVerification) {
		t.Fatalf("tampered = %v, want ErrVerification", err)
	}
}

func TestLoad_PolicyDeniesManifestAndCapability(t *testing.T) {
	z := moduleZip(t, sampleManifest())
	path := writeZip(t, z)

	// Manifest-level deny.
	l := loader.New(loader.Config{
		Policy: denyPolicy{denyManifest: true}, Runtimes: runtimes(&fakeRuntime{}),
	})
	if _, err := l.Load(context.Background(), path, loader.LoadOptions{SkipVerification: true}); !errors.Is(err, loader.ErrPolicyDenied) {
		t.Fatalf("manifest deny = %v, want ErrPolicyDenied", err)
	}

	// Capability-level deny: kv blocked → denied, not registered,
	// runtime never sees it.
	rt := &fakeRuntime{}
	l = loader.New(loader.Config{
		Policy: denyPolicy{denyCap: manifest.CapKV}, Runtimes: runtimes(rt),
	})
	res, err := l.Load(context.Background(), path, loader.LoadOptions{SkipVerification: true})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.DeniedCapabilities) != 1 || res.DeniedCapabilities[0] != manifest.CapKV {
		t.Fatalf("denied = %v", res.DeniedCapabilities)
	}
	if _, ok := rt.last.caps[manifest.CapKV]; ok {
		t.Fatal("runtime must not receive a policy-denied capability")
	}
	if _, ok := rt.last.caps[manifest.CapLog]; !ok {
		t.Fatal("granted capability missing from runtime")
	}
}

func TestLoad_CapabilityLock(t *testing.T) {
	z := moduleZip(t, sampleManifest())
	path := writeZip(t, z)
	l := loader.New(loader.Config{
		Policy: denyPolicy{denyCap: manifest.CapKV}, Runtimes: runtimes(&fakeRuntime{}),
	})
	// kv was previously granted but is now denied → revoked.
	_, err := l.Load(context.Background(), path, loader.LoadOptions{
		SkipVerification:     true,
		PreviousCapabilities: []string{manifest.CapKV},
	})
	if !errors.Is(err, loader.ErrCapabilityRevoked) {
		t.Fatalf("err = %v, want ErrCapabilityRevoked", err)
	}
}

func TestLoad_NoRuntimeAndBadBundle(t *testing.T) {
	z := moduleZip(t, sampleManifest())
	path := writeZip(t, z)

	// No runtime registered.
	l := loader.New(loader.Config{})
	if _, err := l.Load(context.Background(), path, loader.LoadOptions{SkipVerification: true}); !errors.Is(err, loader.ErrNoRuntime) {
		t.Fatalf("no runtime = %v, want ErrNoRuntime", err)
	}

	// Missing file.
	if _, err := l.Load(context.Background(), "/no/such.zip", loader.LoadOptions{SkipVerification: true}); !errors.Is(err, loader.ErrManifest) {
		t.Fatalf("missing path = %v, want ErrManifest", err)
	}
	// Not a zip.
	bad := writeZip(t, []byte("not a zip"))
	if _, err := l.Load(context.Background(), bad, loader.LoadOptions{SkipVerification: true}); !errors.Is(err, loader.ErrManifest) {
		t.Fatalf("bad zip = %v, want ErrManifest", err)
	}
	// Zip without manifest.yaml.
	var nb bytes.Buffer
	zw := zip.NewWriter(&nb)
	w, _ := zw.Create("readme.txt")
	_, _ = w.Write([]byte("hi"))
	_ = zw.Close()
	if _, err := l.Load(context.Background(), writeZip(t, nb.Bytes()), loader.LoadOptions{SkipVerification: true}); !errors.Is(err, loader.ErrManifest) {
		t.Fatalf("no manifest = %v, want ErrManifest", err)
	}
}

func TestLoad_CacheShortCircuitsVerify(t *testing.T) {
	z := moduleZip(t, sampleManifest())
	path := writeZip(t, z)
	tp, sig := signFor(t, z)
	cv := &countVerifier{inner: verify.NewVerifier(tp)}
	store, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l := loader.New(loader.Config{Verifier: cv, Runtimes: runtimes(&fakeRuntime{}), Cache: store})

	r1, err := l.Load(context.Background(), path, loader.LoadOptions{Signature: &sig})
	if err != nil {
		t.Fatalf("Load 1: %v", err)
	}
	r2, err := l.Load(context.Background(), path, loader.LoadOptions{Signature: &sig})
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	if cv.n != 1 {
		t.Fatalf("verifier called %d times, want 1 (2nd is cache-memoised)", cv.n)
	}
	if r1.VerificationResult.Cached || !r2.VerificationResult.Cached {
		t.Fatalf("cached flags: r1=%v r2=%v", r1.VerificationResult.Cached, r2.VerificationResult.Cached)
	}
}

func TestLoadAndExecute(t *testing.T) {
	z := moduleZip(t, sampleManifest())
	path := writeZip(t, z)
	rt := &fakeRuntime{}
	l := loader.New(loader.Config{Runtimes: runtimes(rt), Cache: nil})
	out, err := l.LoadAndExecute(context.Background(), path, loader.LoadOptions{SkipVerification: true}, map[string]any{"x": 7})
	if err != nil {
		t.Fatalf("LoadAndExecute: %v", err)
	}
	if out.Output["echo"] != 7 {
		t.Fatalf("output = %+v", out.Output)
	}
	if !rt.last.closed {
		t.Fatal("instance must be closed after LoadAndExecute")
	}
}
