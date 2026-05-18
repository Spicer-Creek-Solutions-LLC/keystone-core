package resolver_test

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/module/cas"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/resolver"
	"go.keystone-core.io/keystone-core/pkg/semver"
)

// fakeSource is an in-memory module source.
type fakeSource struct {
	// module → list of "version" strings available
	versions map[string][]string
	// module → version → its dependency map
	deps map[string]map[string]map[string]string
}

func (f *fakeSource) ListVersions(_ context.Context, m string) ([]resolver.ModuleVersion, error) {
	vs, ok := f.versions[m]
	if !ok {
		return nil, errors.New("no such module")
	}
	out := make([]resolver.ModuleVersion, 0, len(vs))
	for _, v := range vs {
		out = append(out, resolver.ModuleVersion{
			Version: semver.MustParse(v),
			Hash:    cas.HashBytes([]byte(m + "@" + v)),
		})
	}
	return out, nil
}

func (f *fakeSource) GetManifest(_ context.Context, m string, v semver.Version) (*manifest.Manifest, error) {
	d := f.deps[m][v.String()]
	return &manifest.Manifest{
		Name: m, Version: v.String(), Type: manifest.TypeStarlark,
		Entrypoint: "main.star", Dependencies: d,
	}, nil
}

func root(deps map[string]string) *manifest.Manifest {
	return &manifest.Manifest{
		Name: "acme/app", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint: "main.star", Dependencies: deps,
	}
}

func TestResolve_MVS_PicksMinimumSatisfying(t *testing.T) {
	src := &fakeSource{
		versions: map[string][]string{
			"acme/lib": {"1.0.0", "1.2.0", "1.5.0", "2.0.0"},
		},
		deps: map[string]map[string]map[string]string{"acme/lib": {}},
	}
	res, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"acme/lib": ">=1.2.0 <2.0.0"}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	sel := res.Selected["acme/lib"]
	if sel.Version.String() != "1.2.0" { // MVS = lowest satisfying, NOT 1.5.0/2.0.0
		t.Fatalf("selected %s, want 1.2.0 (MVS minimum)", sel.Version)
	}
	if _, err := cas.ParseHash(sel.Hash); err != nil {
		t.Fatalf("hash not pinned: %q", sel.Hash)
	}
}

func TestResolve_TransitiveTighteningReselectsUpward(t *testing.T) {
	// root → a (>=1.0.0); a@1.0.0 → b (>=1.0.0); but root also → b (>=1.5.0).
	// b must end at the min satisfying BOTH (1.5.0), and adding a's
	// looser b-constraint must not lower it.
	src := &fakeSource{
		versions: map[string][]string{
			"x/a": {"1.0.0"},
			"x/b": {"1.0.0", "1.5.0", "1.8.0"},
		},
		deps: map[string]map[string]map[string]string{
			"x/a": {"1.0.0": {"x/b": ">=1.0.0"}},
			"x/b": {"1.0.0": {}, "1.5.0": {}, "1.8.0": {}},
		},
	}
	res, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.0.0", "x/b": ">=1.5.0"}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := res.Selected["x/b"].Version.String(); got != "1.5.0" {
		t.Fatalf("x/b = %s, want 1.5.0 (max of the minimums)", got)
	}
	if got := res.Selected["x/a"].Version.String(); got != "1.0.0" {
		t.Fatalf("x/a = %s, want 1.0.0", got)
	}
}

func TestResolve_NoSatisfyingVersion(t *testing.T) {
	src := &fakeSource{
		versions: map[string][]string{"x/a": {"1.0.0", "1.1.0"}},
		deps:     map[string]map[string]map[string]string{"x/a": {"1.0.0": {}, "1.1.0": {}}},
	}
	_, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=2.0.0"}))
	if !errors.Is(err, resolver.ErrNoSatisfyingVersion) {
		t.Fatalf("err = %v, want ErrNoSatisfyingVersion", err)
	}
}

func TestResolve_ConflictingRangesEmptyIntersection(t *testing.T) {
	// root → a (>=1.0.0); a → c (<1.0.0); root → c (>=2.0.0): no c satisfies both.
	src := &fakeSource{
		versions: map[string][]string{
			"x/a": {"1.0.0"},
			"x/c": {"0.9.0", "2.0.0"},
		},
		deps: map[string]map[string]map[string]string{
			"x/a": {"1.0.0": {"x/c": "<1.0.0"}},
			"x/c": {"0.9.0": {}, "2.0.0": {}},
		},
	}
	_, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.0.0", "x/c": ">=2.0.0"}))
	if !errors.Is(err, resolver.ErrNoSatisfyingVersion) {
		t.Fatalf("err = %v, want ErrNoSatisfyingVersion", err)
	}
}

func TestResolve_CycleDetected(t *testing.T) {
	src := &fakeSource{
		versions: map[string][]string{"x/a": {"1.0.0"}, "x/b": {"1.0.0"}},
		deps: map[string]map[string]map[string]string{
			"x/a": {"1.0.0": {"x/b": ">=1.0.0"}},
			"x/b": {"1.0.0": {"x/a": ">=1.0.0"}}, // a→b→a
		},
	}
	_, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.0.0"}))
	if !errors.Is(err, resolver.ErrCycle) {
		t.Fatalf("err = %v, want ErrCycle", err)
	}
}

func TestResolve_SelfLoopCycle(t *testing.T) {
	src := &fakeSource{
		versions: map[string][]string{"x/a": {"1.0.0"}},
		deps:     map[string]map[string]map[string]string{"x/a": {"1.0.0": {"x/a": ">=1.0.0"}}},
	}
	_, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.0.0"}))
	if !errors.Is(err, resolver.ErrCycle) {
		t.Fatalf("self-loop err = %v, want ErrCycle", err)
	}
}

func TestResolve_PrereleaseFilter(t *testing.T) {
	src := &fakeSource{
		versions: map[string][]string{"x/a": {"1.0.0", "1.1.0-beta.1", "1.1.0"}},
		deps: map[string]map[string]map[string]string{
			"x/a": {"1.0.0": {}, "1.1.0-beta.1": {}, "1.1.0": {}},
		},
	}
	// Default: prerelease excluded → min satisfying >=1.0.0 is 1.0.0.
	res, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.0.0"}))
	if err != nil || res.Selected["x/a"].Version.String() != "1.0.0" {
		t.Fatalf("default = %v / %v", res.Selected["x/a"].Version, err)
	}
	// AllowPrerelease + a constraint excluding the stable: beta is picked.
	res, err = resolver.New(src, resolver.Config{AllowPrerelease: true}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.1.0-0 <1.1.0"}))
	if err != nil {
		t.Fatalf("allow-prerelease: %v", err)
	}
	if got := res.Selected["x/a"].Version.String(); got != "1.1.0-beta.1" {
		t.Fatalf("prerelease selection = %s, want 1.1.0-beta.1", got)
	}
}

func TestResolve_UnknownModuleAndBadConstraint(t *testing.T) {
	src := &fakeSource{versions: map[string][]string{}, deps: map[string]map[string]map[string]string{}}
	_, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/missing": ">=1.0.0"}))
	if !errors.Is(err, resolver.ErrUnknownModule) {
		t.Fatalf("err = %v, want ErrUnknownModule", err)
	}
	_, err = resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": "not a constraint"}))
	if !errors.Is(err, resolver.ErrInvalidConstraint) {
		t.Fatalf("err = %v, want ErrInvalidConstraint", err)
	}
	if _, err := resolver.New(src, resolver.Config{}).Resolve(context.Background(), nil); err == nil {
		t.Fatal("nil root: want error")
	}
}

func TestResolution_LockFileAndOrder(t *testing.T) {
	// root → a → b ; root → a. Order: b before a.
	src := &fakeSource{
		versions: map[string][]string{"x/a": {"1.0.0"}, "x/b": {"2.0.0"}},
		deps: map[string]map[string]map[string]string{
			"x/a": {"1.0.0": {"x/b": ">=2.0.0"}},
			"x/b": {"2.0.0": {}},
		},
	}
	res, err := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.0.0"}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	lf, err := res.LockFile()
	if err != nil {
		t.Fatalf("LockFile: %v", err)
	}
	if err := lf.Validate(); err != nil {
		t.Fatalf("lockfile invalid: %v", err)
	}
	b1, _ := manifest.MarshalLockFile(lf)
	// Determinism: a fresh resolve → byte-identical lockfile.
	res2, _ := resolver.New(src, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.0.0"}))
	lf2, _ := res2.LockFile()
	b2, _ := manifest.MarshalLockFile(lf2)
	if string(b1) != string(b2) {
		t.Fatalf("lockfile not deterministic:\n%s\n---\n%s", b1, b2)
	}
	round, err := manifest.UnmarshalLockFile(b1)
	if err != nil || round.Modules["x/a"].Version != "1.0.0" || round.Modules["x/b"].Version != "2.0.0" {
		t.Fatalf("lockfile round-trip: %+v %v", round, err)
	}

	order := res.Order()
	ai, bi := indexOf(order, "x/a"), indexOf(order, "x/b")
	if ai < 0 || bi < 0 || bi > ai {
		t.Fatalf("Order = %v, want x/b before x/a", order)
	}
	if indexOf(order, "acme/app") != -1 {
		t.Fatalf("Order must omit the root: %v", order)
	}
}

func TestResolution_LockFileRejectsBadHash(t *testing.T) {
	// Source returns a malformed hash → LockFile must reject it.
	bad := &badHashSource{}
	res, err := resolver.New(bad, resolver.Config{}).
		Resolve(context.Background(), root(map[string]string{"x/a": ">=1.0.0"}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := res.LockFile(); err == nil {
		t.Fatal("LockFile with bad hash: want error")
	}
}

type badHashSource struct{}

func (badHashSource) ListVersions(context.Context, string) ([]resolver.ModuleVersion, error) {
	return []resolver.ModuleVersion{{Version: semver.MustParse("1.0.0"), Hash: "not-a-hash"}}, nil
}
func (badHashSource) GetManifest(_ context.Context, m string, v semver.Version) (*manifest.Manifest, error) {
	return &manifest.Manifest{Name: m, Version: v.String(), Type: manifest.TypeStarlark, Entrypoint: "main.star"}, nil
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
