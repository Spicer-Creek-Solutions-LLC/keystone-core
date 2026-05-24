// SPDX-License-Identifier: Apache-2.0

// Package loader is the runtime-agnostic 7-step module load
// pipeline (Epic 14 task 10, PROJECT-DETAILS §4.18):
//
//	parse → verify → policy → capability-policy → capability-lock
//	→ runtime-init → register-granted-only
//
// Every external concern is an injected seam so pkg/module stays
// dependency-light: the Runtime (Starlark impl is task 11), the
// PolicyChecker (an internal/policy adapter wired at boot), the
// capability Hosts (real internal/secrets/exec/http wired at boot),
// the signature Verifier + trust policy, and the optional load-time
// cache. A module artifact is a ZIP containing manifest.yaml + the
// entrypoint; the install/resolve flow (kscore-module install) is
// task 14.
package loader

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	maudit "go.keystone-core.io/keystone-core/pkg/module/audit"
	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/cas"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

var (
	// ErrManifest — the bundle has no / an invalid manifest.
	ErrManifest = errors.New("loader: invalid module manifest")
	// ErrVerification — hash/signature verification failed (or no
	// signature/verifier and verification was not skipped).
	ErrVerification = errors.New("loader: verification failed")
	// ErrPolicyDenied — a manifest-level policy check denied load.
	ErrPolicyDenied = errors.New("loader: denied by policy")
	// ErrCapabilityRevoked — a previously-granted capability is now
	// denied (capability-lock check).
	ErrCapabilityRevoked = errors.New("loader: capability revoked since previous load")
	// ErrNoRuntime — no runtime registered for the manifest type.
	ErrNoRuntime = errors.New("loader: no runtime for module type")
)

// ---- runtime seam (Starlark impl = task 11) -----------------------------

// Instance is a loaded, capability-bound module ready to execute.
type Instance interface {
	Execute(ctx context.Context, input map[string]any) (*ExecuteResult, error)
	Close() error
}

// Runtime constructs an Instance from a manifest, its entrypoint
// source, and the granted capability backends.
type Runtime interface {
	Init(ctx context.Context, m *manifest.Manifest, entrypoint []byte, caps map[string]any) (Instance, error)
}

// RuntimeRegistry maps a module type to its Runtime.
type RuntimeRegistry struct {
	mu sync.RWMutex
	rt map[manifest.ModuleType]Runtime
}

// NewRuntimeRegistry returns an empty registry.
func NewRuntimeRegistry() *RuntimeRegistry {
	return &RuntimeRegistry{rt: map[manifest.ModuleType]Runtime{}}
}

// Register binds a Runtime to a module type.
func (r *RuntimeRegistry) Register(t manifest.ModuleType, rt Runtime) {
	r.mu.Lock()
	r.rt[t] = rt
	r.mu.Unlock()
}

func (r *RuntimeRegistry) for_(t manifest.ModuleType) (Runtime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.rt[t]
	return rt, ok
}

// ---- policy seam (internal/policy adapter wired at boot) ----------------

// PolicyResult is the manifest-level policy verdict.
type PolicyResult struct {
	Allowed bool
	Detail  string
}

// CapabilityDecision is the per-capability policy verdict.
type CapabilityDecision struct {
	Allowed bool
	Reason  string
}

// PolicyChecker evaluates module + capability policy. A nil checker
// means "allow all" (equivalent to SkipPolicyValidation).
type PolicyChecker interface {
	CheckManifest(ctx context.Context, m *manifest.Manifest) (PolicyResult, error)
	CheckCapability(ctx context.Context, module, capName string, cfg manifest.CapabilityConfig) (CapabilityDecision, error)
}

// signatureVerifier is the slice of verify.Verifier the loader
// needs (a local interface for testability / nil-handling).
type signatureVerifier interface {
	Verify(blob []byte, sig verify.Signature) error
}

var _ signatureVerifier = (*verify.Verifier)(nil)

// ---- load events --------------------------------------------------------

// LoadEvent is emitted on completion of each pipeline phase (Err
// non-nil ⇒ the phase failed and load aborts). The §4.18 telemetry
// hook.
type LoadEvent struct {
	Phase string
	At    time.Time
	Err   error
}

// LoadObserver receives LoadEvents.
type LoadObserver interface {
	OnLoadEvent(LoadEvent)
}

// ---- results ------------------------------------------------------------

// VerificationResult records step 2's outcome.
type VerificationResult struct {
	Hash     string // sha256:<hex> of the artifact
	Verified bool   // signature checked (false if skipped)
	KeyID    string
	Cached   bool // short-circuited via the load-time cache memo
}

// LoadResult is the §4.18 loader output.
type LoadResult struct {
	Manifest                  *manifest.Manifest
	Runtime                   Instance
	VerificationResult        VerificationResult
	PolicyResult              PolicyResult
	CapabilityPolicyDecisions map[string]CapabilityDecision
	RegisteredCapabilities    []string
	DeniedCapabilities        []string
	LoadDuration              time.Duration
}

// ExecuteResult is the runtime execution output.
type ExecuteResult struct {
	Output map[string]any
	Logs   []string
}

// LoadOptions tunes a Load.
type LoadOptions struct {
	Signature            *verify.Signature
	SkipVerification     bool
	SkipPolicyValidation bool
	// PreviousCapabilities, if non-nil, enables the capability-lock
	// check: a capability in this set that is now denied aborts load.
	PreviousCapabilities []string
}

// ---- loader -------------------------------------------------------------

// Config wires the loader's seams. All are optional; a nil Verifier
// with verification not skipped fails closed.
type Config struct {
	Verifier signatureVerifier
	Policy   PolicyChecker
	Hosts    capability.Hosts
	Runtimes *RuntimeRegistry
	Auditor  maudit.Auditor
	Observer LoadObserver
	Cache    *cas.Store // optional verified-hash memo source-of-truth
}

// ModuleLoader runs the load pipeline.
type ModuleLoader struct {
	cfg      Config
	mu       sync.Mutex
	verified map[string]struct{} // content hashes verified this process
}

// New returns a ModuleLoader.
func New(cfg Config) *ModuleLoader {
	if cfg.Auditor == nil {
		cfg.Auditor = maudit.NoopAuditor{}
	}
	return &ModuleLoader{cfg: cfg, verified: map[string]struct{}{}}
}

func (l *ModuleLoader) emit(phase string, err error) error {
	if l.cfg.Observer != nil {
		l.cfg.Observer.OnLoadEvent(LoadEvent{Phase: phase, At: time.Now(), Err: err})
	}
	return err
}

// Load runs the 7-step pipeline on the module ZIP at path.
func (l *ModuleLoader) Load(ctx context.Context, path string, opts LoadOptions) (*LoadResult, error) {
	start := time.Now()
	blob, err := os.ReadFile(path) //nolint:gosec // operator-supplied module path
	if err != nil {
		return nil, l.emit("parse", fmt.Errorf("%w: read %s: %v", ErrManifest, path, err))
	}

	// 1. Parse manifest (+ entrypoint) from the ZIP.
	m, entry, err := readBundle(blob)
	if err != nil {
		return nil, l.emit("parse", err)
	}
	_ = l.emit("parse", nil)
	res := &LoadResult{Manifest: m, CapabilityPolicyDecisions: map[string]CapabilityDecision{}}

	// 2. Verify (hash + signature) unless skipped.
	hash := cas.HashBytes(blob)
	res.VerificationResult.Hash = hash
	if !opts.SkipVerification {
		if l.cached(hash) {
			res.VerificationResult.Verified = true
			res.VerificationResult.Cached = true
		} else {
			if l.cfg.Verifier == nil || opts.Signature == nil {
				return nil, l.emit("verify", fmt.Errorf("%w: missing verifier or signature", ErrVerification))
			}
			if err := l.cfg.Verifier.Verify(blob, *opts.Signature); err != nil {
				return nil, l.emit("verify", fmt.Errorf("%w: %v", ErrVerification, err))
			}
			res.VerificationResult.Verified = true
			res.VerificationResult.KeyID = opts.Signature.KeyID
			l.markVerified(hash)
		}
	}
	_ = l.emit("verify", nil)

	// 3. Policy check (manifest vs policy).
	if !opts.SkipPolicyValidation && l.cfg.Policy != nil {
		pr, perr := l.cfg.Policy.CheckManifest(ctx, m)
		if perr != nil {
			return nil, l.emit("policy", fmt.Errorf("loader: policy check: %w", perr))
		}
		res.PolicyResult = pr
		if !pr.Allowed {
			return nil, l.emit("policy", fmt.Errorf("%w: %s", ErrPolicyDenied, pr.Detail))
		}
	} else {
		res.PolicyResult = PolicyResult{Allowed: true, Detail: "skipped"}
	}
	_ = l.emit("policy", nil)

	// 4. Capability policy evaluation (per requested capability).
	granted := make(map[string]manifest.CapabilityConfig)
	for name, cfg := range m.Capabilities {
		dec := CapabilityDecision{Allowed: true}
		if l.cfg.Policy != nil {
			d, derr := l.cfg.Policy.CheckCapability(ctx, m.Name, name, cfg)
			if derr != nil {
				return nil, l.emit("capability_policy", fmt.Errorf("loader: capability policy %q: %w", name, derr))
			}
			dec = d
		}
		res.CapabilityPolicyDecisions[name] = dec
		if dec.Allowed {
			granted[name] = cfg
		} else {
			res.DeniedCapabilities = append(res.DeniedCapabilities, name)
		}
	}
	for name := range granted {
		res.RegisteredCapabilities = append(res.RegisteredCapabilities, name)
	}
	sort.Strings(res.RegisteredCapabilities)
	sort.Strings(res.DeniedCapabilities)
	_ = l.emit("capability_policy", nil)

	// 5. Capability lock — a previously-granted cap now denied aborts.
	if opts.PreviousCapabilities != nil {
		for _, prev := range opts.PreviousCapabilities {
			if _, ok := granted[prev]; !ok {
				return nil, l.emit("capability_lock",
					fmt.Errorf("%w: %q", ErrCapabilityRevoked, prev))
			}
		}
	}
	_ = l.emit("capability_lock", nil)

	// 6. Runtime init with the granted capabilities only.
	if l.cfg.Runtimes == nil {
		return nil, l.emit("runtime_init", fmt.Errorf("%w: %s", ErrNoRuntime, m.Type))
	}
	rt, ok := l.cfg.Runtimes.for_(m.Type)
	if !ok {
		return nil, l.emit("runtime_init", fmt.Errorf("%w: %s", ErrNoRuntime, m.Type))
	}
	grantedManifest := *m
	grantedManifest.Capabilities = granted
	caps, err := capability.BuildCapabilities(&grantedManifest, l.cfg.Hosts)
	if err != nil {
		return nil, l.emit("runtime_init", fmt.Errorf("loader: build capabilities: %w", err))
	}
	inst, err := rt.Init(ctx, m, entry, caps)
	if err != nil {
		return nil, l.emit("runtime_init", fmt.Errorf("loader: runtime init: %w", err))
	}
	res.Runtime = inst
	_ = l.emit("runtime_init", nil)

	// 7. Granted capabilities are now registered with the runtime.
	_ = l.emit("register", nil)

	res.LoadDuration = time.Since(start)
	return res, nil
}

// Execute runs the loaded instance.
func (l *ModuleLoader) Execute(ctx context.Context, res *LoadResult, input map[string]any) (*ExecuteResult, error) {
	if res == nil || res.Runtime == nil {
		return nil, fmt.Errorf("loader: nil load result / runtime")
	}
	return res.Runtime.Execute(ctx, input)
}

// LoadAndExecute composes Load + Execute, closing the instance after.
func (l *ModuleLoader) LoadAndExecute(ctx context.Context, path string, opts LoadOptions, input map[string]any) (*ExecuteResult, error) {
	res, err := l.Load(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Runtime.Close() }()
	return l.Execute(ctx, res, input)
}

func (l *ModuleLoader) cached(hash string) bool {
	if l.cfg.Cache == nil {
		return false
	}
	l.mu.Lock()
	_, ok := l.verified[hash]
	l.mu.Unlock()
	return ok
}

func (l *ModuleLoader) markVerified(hash string) {
	if l.cfg.Cache == nil {
		return
	}
	l.mu.Lock()
	l.verified[hash] = struct{}{}
	l.mu.Unlock()
}

// readBundle reads manifest.yaml (validated) and the entrypoint
// file from a module ZIP.
func readBundle(blob []byte) (*manifest.Manifest, []byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: open zip: %v", ErrManifest, err)
	}
	manBytes, err := readZipFile(zr, "manifest.yaml")
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	m, err := manifest.UnmarshalManifest(manBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	if err := m.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	entry, err := readZipFile(zr, m.Entrypoint)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: entrypoint %q: %v", ErrManifest, m.Entrypoint, err)
	}
	return m, entry, nil
}

func readZipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%q not found in module zip", name)
}
