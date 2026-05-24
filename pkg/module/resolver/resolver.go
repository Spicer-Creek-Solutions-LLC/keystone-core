// SPDX-License-Identifier: Apache-2.0

// Package resolver resolves a module's transitive dependency graph
// against semver constraints (Epic 14 task 6, PROJECT-DETAILS
// §4.18).
//
// Conflict resolution is Minimum Version Selection (the Go-modules
// pattern): for each module the *lowest* available version that
// satisfies every accumulated constraint is chosen — reproducible,
// no gratuitous upgrades. A tighter transitive constraint can only
// push a selection upward (monotone), so the fixpoint terminates.
//
// The output is a deterministic, fully-pinned manifest.LockFile.
// The module Source is a seam — the filesystem registry implements
// it (tasks 8/9); the cache policy is task 7; the loader composes
// resolve → CAS-store → verify (task 10).
//
// Reuses pkg/semver + pkg/module/manifest + the pkg/module/cas hash
// format; no new dependency.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"go.keystone-core.io/keystone-core/pkg/module/cas"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/semver"
)

var (
	// ErrNoSatisfyingVersion — no available version of a module
	// satisfies the intersection of all accumulated constraints.
	ErrNoSatisfyingVersion = errors.New("resolver: no version satisfies all constraints")
	// ErrCycle — the dependency graph contains a cycle.
	ErrCycle = errors.New("resolver: dependency cycle")
	// ErrUnknownModule — the Source has no such module.
	ErrUnknownModule = errors.New("resolver: unknown module")
	// ErrInvalidConstraint — a dependency constraint string is
	// not parseable.
	ErrInvalidConstraint = errors.New("resolver: invalid constraint")
)

// ModuleVersion is one published version of a module plus its
// content hash (sha256:<hex>), so resolution can emit a fully
// pinned lockfile.
type ModuleVersion struct {
	Version semver.Version
	Hash    string
}

// Source supplies module metadata. The registry (tasks 8/9)
// implements it; tests use an in-memory fake.
type Source interface {
	ListVersions(ctx context.Context, module string) ([]ModuleVersion, error)
	GetManifest(ctx context.Context, module string, v semver.Version) (*manifest.Manifest, error)
}

// Config tunes resolution.
type Config struct {
	// AllowPrerelease includes prerelease versions in selection.
	// Default false — prereleases are filtered out (the §4.18
	// "prerelease-filter configurable" knob).
	AllowPrerelease bool
}

// Resolver resolves dependency graphs against a Source.
type Resolver struct {
	src Source
	cfg Config
}

// New returns a Resolver.
func New(src Source, cfg Config) *Resolver {
	return &Resolver{src: src, cfg: cfg}
}

// Selected is a resolved module pin.
type Selected struct {
	Version semver.Version
	Hash    string
}

// Resolution is the resolved graph.
type Resolution struct {
	root     string
	Selected map[string]Selected
	edges    map[string][]string // requirer → deps (sorted)
}

// Resolve computes the pinned transitive closure of root's
// dependencies via MVS, then verifies the graph is acyclic.
func (r *Resolver) Resolve(ctx context.Context, root *manifest.Manifest) (*Resolution, error) {
	if root == nil {
		return nil, fmt.Errorf("resolver: nil root manifest")
	}
	cons := map[string][]semver.Constraint{}
	conStr := map[string][]string{} // for error messages
	edges := map[string][]string{}

	addDeps := func(requirer string, deps map[string]string) error {
		names := make([]string, 0, len(deps))
		for d := range deps {
			names = append(names, d)
		}
		sort.Strings(names)
		for _, dep := range names {
			c, err := semver.NewConstraint(deps[dep])
			if err != nil {
				return fmt.Errorf("%w: %q on %q: %v", ErrInvalidConstraint, deps[dep], dep, err)
			}
			cons[dep] = append(cons[dep], c)
			conStr[dep] = append(conStr[dep], deps[dep])
			edges[requirer] = appendSorted(edges[requirer], dep)
		}
		return nil
	}
	if err := addDeps(root.Name, root.Dependencies); err != nil {
		return nil, err
	}

	selected := map[string]semver.Version{}
	hashes := map[string]string{}

	// Fixpoint: selection is monotone-upward as constraints tighten,
	// versions are finite ⇒ this terminates.
	for {
		changed := false
		mods := sortedKeys(cons)
		for _, mod := range mods {
			pick, hash, err := r.selectVersion(ctx, mod, cons[mod], conStr[mod])
			if err != nil {
				return nil, err
			}
			if cur, ok := selected[mod]; ok && cur.Equal(pick) {
				continue
			}
			selected[mod] = pick
			hashes[mod] = hash
			changed = true
			m, err := r.src.GetManifest(ctx, mod, pick)
			if err != nil {
				return nil, fmt.Errorf("%w: %s@%s: %v", ErrUnknownModule, mod, pick, err)
			}
			if err := addDeps(mod, m.Dependencies); err != nil {
				return nil, err
			}
		}
		if !changed {
			break
		}
	}

	res := &Resolution{
		root:     root.Name,
		Selected: make(map[string]Selected, len(selected)),
		edges:    edges,
	}
	for mod, v := range selected {
		res.Selected[mod] = Selected{Version: v, Hash: hashes[mod]}
	}
	if path, cyc := detectCycle(root.Name, edges); cyc {
		return nil, fmt.Errorf("%w: %v", ErrCycle, path)
	}
	return res, nil
}

// selectVersion is the MVS core: the lowest available version
// satisfying every constraint (prerelease-filtered unless allowed).
func (r *Resolver) selectVersion(ctx context.Context, mod string, cs []semver.Constraint, csStr []string) (semver.Version, string, error) {
	avail, err := r.src.ListVersions(ctx, mod)
	if err != nil {
		return semver.Version{}, "", fmt.Errorf("%w: %s: %v", ErrUnknownModule, mod, err)
	}
	byVer := make(map[string]string, len(avail))
	vers := make([]semver.Version, 0, len(avail))
	for _, mv := range avail {
		byVer[mv.Version.String()] = mv.Hash
		vers = append(vers, mv.Version)
	}
	semver.Sort(vers) // ascending — MVS takes the lowest satisfying

	for _, v := range vers {
		if v.Prerelease() != "" && !r.cfg.AllowPrerelease {
			continue
		}
		ok := true
		for _, c := range cs {
			if !c.Check(v) {
				ok = false
				break
			}
		}
		if ok {
			return v, byVer[v.String()], nil
		}
	}
	return semver.Version{}, "", fmt.Errorf("%w: %s (constraints: %v)",
		ErrNoSatisfyingVersion, mod, csStr)
}

// LockFile renders the resolution as a deterministic, fully-pinned
// manifest.LockFile (Task-1 MarshalLockFile sorts keys → byte-
// identical re-resolution).
func (res *Resolution) LockFile() (*manifest.LockFile, error) {
	lf := &manifest.LockFile{
		SchemaVersion: manifest.LockFileSchemaVersion,
		Modules:       make(map[string]manifest.LockedModule, len(res.Selected)),
	}
	for mod, sel := range res.Selected {
		if _, err := cas.ParseHash(sel.Hash); err != nil {
			return nil, fmt.Errorf("resolver: %s hash: %w", mod, err)
		}
		lf.Modules[mod] = manifest.LockedModule{Version: sel.Version.String(), Hash: sel.Hash}
	}
	return lf, nil
}

// Order returns the modules in topological order (dependencies
// before dependents); deterministic. The root is omitted.
func (res *Resolution) Order() []string {
	visited := map[string]int{} // 0 unseen, 1 in-progress, 2 done
	out := make([]string, 0, len(res.Selected))
	var dfs func(string)
	dfs = func(n string) {
		if visited[n] == 2 {
			return
		}
		visited[n] = 1
		for _, dep := range res.edges[n] {
			if visited[dep] != 2 {
				dfs(dep)
			}
		}
		visited[n] = 2
		if n != res.root {
			out = append(out, n)
		}
	}
	dfs(res.root)
	// Any module not reachable from root via recorded edges (none in
	// practice) — append sorted for determinism.
	for _, m := range sortedSelected(res.Selected) {
		if visited[m] != 2 {
			out = append(out, m)
		}
	}
	return out
}

func appendSorted(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	s = append(s, v)
	sort.Strings(s)
	return s
}

func sortedKeys(m map[string][]semver.Constraint) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedSelected(m map[string]Selected) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// detectCycle returns a cycle path if the edge set (rooted at root)
// is not a DAG.
func detectCycle(root string, edges map[string][]string) ([]string, bool) {
	state := map[string]int{} // 0 unseen, 1 on-stack, 2 done
	var stack []string
	var dfs func(string) ([]string, bool)
	dfs = func(n string) ([]string, bool) {
		state[n] = 1
		stack = append(stack, n)
		for _, dep := range edges[n] {
			switch state[dep] {
			case 1: // back-edge → cycle
				return append(cyclePath(stack, dep), dep), true
			case 0:
				if p, ok := dfs(dep); ok {
					return p, true
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = 2
		return nil, false
	}
	// Start from root, then any unseen node (sorted, deterministic).
	if p, ok := dfs(root); ok {
		return p, true
	}
	nodes := make([]string, 0, len(edges))
	for n := range edges {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	for _, n := range nodes {
		if state[n] == 0 {
			if p, ok := dfs(n); ok {
				return p, true
			}
		}
	}
	return nil, false
}

func cyclePath(stack []string, from string) []string {
	for i, n := range stack {
		if n == from {
			return append([]string(nil), stack[i:]...)
		}
	}
	return append([]string(nil), stack...)
}
