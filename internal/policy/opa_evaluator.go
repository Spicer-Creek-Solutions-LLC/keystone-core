package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// OPAPackage is the fixed Rego package every policy module must
// declare. The evaluator queries data.keystone.policy.{allow,
// violations,warnings} — a predictable convention (mirrors how
// Gatekeeper / Conftest standardize entrypoints) so operators don't
// have to wire package names and the evaluator never has to parse
// the module to discover them.
const OPAPackage = "keystone.policy"

// opaQuery is the single query the evaluator prepares. Querying the
// whole package object (rather than three separate allow/violations/
// warnings queries) means one PrepareForEval + one Eval per policy
// and lets us distinguish "allow rule undefined" (key absent) from
// "allow == false" (key present, false).
const opaQuery = "data." + OPAPackage

// DefaultOPAEvalTimeout caps a single Rego evaluation. A pathological
// or maliciously-complex policy must not hang the control plane;
// callers override with WithOPAEvalTimeout.
const DefaultOPAEvalTimeout = 5 * time.Second

// deniedBuiltinPrefixes are the Rego builtin name prefixes stripped
// from the evaluator's capability set. Operator-supplied policies
// evaluate inside the control plane; they must be pure decision
// logic. http.send / net.* would let a policy SSRF or exfiltrate;
// opa.runtime leaks process/runtime detail. Removing the builtins
// from Capabilities makes a policy that references them fail to
// COMPILE (surfaced as an evaluator error at registration-time use),
// not silently no-op.
var deniedBuiltinPrefixes = []string{
	"http.",
	"net.",
	"opa.runtime",
}

// OPAEvaluator implements [Evaluator] for audit.PolicyTypeOPA by
// embedding the open-policy-agent/opa Rego engine (v1 syntax).
//
// Compilation is expensive and Policy.Code is immutable per
// registration (no Deregister in v1.0). The evaluator lazily
// compiles a [rego.PreparedEvalQuery] on first use and caches it
// keyed by policyID + sha256(Code); a re-registered policy whose
// Code changed (post-v1.0 CRUD) gets a fresh cache entry naturally
// because the hash is part of the key, so no explicit invalidation
// is needed.
type OPAEvaluator struct {
	evalTimeout time.Duration
	caps        *ast.Capabilities

	mu    sync.Mutex
	cache map[string]rego.PreparedEvalQuery
}

// OPAOption configures an OPAEvaluator.
type OPAOption func(*OPAEvaluator)

// WithOPAEvalTimeout overrides the per-evaluation timeout. A
// non-positive value falls back to [DefaultOPAEvalTimeout].
func WithOPAEvalTimeout(d time.Duration) OPAOption {
	return func(e *OPAEvaluator) {
		if d > 0 {
			e.evalTimeout = d
		}
	}
}

// WithOPACapabilities overrides the restricted capability set. The
// default denies the network / runtime builtins (see
// deniedBuiltinPrefixes); pass a custom set only for a trusted
// single-tenant deploy that genuinely needs e.g. http.send in
// policy. A nil argument is ignored (keeps the safe default).
func WithOPACapabilities(c *ast.Capabilities) OPAOption {
	return func(e *OPAEvaluator) {
		if c != nil {
			e.caps = c
		}
	}
}

// NewOPAEvaluator returns an OPAEvaluator with the restricted
// capability set and the default eval timeout unless overridden.
func NewOPAEvaluator(opts ...OPAOption) *OPAEvaluator {
	e := &OPAEvaluator{
		evalTimeout: DefaultOPAEvalTimeout,
		caps:        restrictedCapabilities(),
		cache:       make(map[string]rego.PreparedEvalQuery),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// restrictedCapabilities returns this OPA version's capabilities
// with the network / runtime builtins removed and remote-ref
// fetching disabled (AllowNet = empty slice → NO host reachable
// during type-checking JSON schema refs).
func restrictedCapabilities() *ast.Capabilities {
	caps := ast.CapabilitiesForThisVersion()
	kept := caps.Builtins[:0:0]
	for _, b := range caps.Builtins {
		if b == nil || isDeniedBuiltin(b.Name) {
			continue
		}
		kept = append(kept, b)
	}
	caps.Builtins = kept
	// Non-nil empty slice = explicitly "no host may be connected to"
	// (nil would mean "ANY host"); see ast.Capabilities.AllowNet.
	caps.AllowNet = []string{}
	return caps
}

func isDeniedBuiltin(name string) bool {
	for _, p := range deniedBuiltinPrefixes {
		if name == p || strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// Evaluate compiles (cached) and runs policy.Code against input,
// then maps the Rego decision into an EvaluationResult.
//
//   - Compile / prepare / eval failures → non-nil error (evaluator
//     -internal failure per the Evaluator contract).
//   - allow rule undefined → Allowed=false + one synthetic
//     Violation noting the policy produced no decision; nil error
//     (a misconfigured policy is an audit signal, not an engine bug).
//   - allow present → Allowed = its bool value; violations /
//     warnings extracted best-effort.
func (e *OPAEvaluator) Evaluate(ctx context.Context, policy *Policy, input EvaluationInput) (result EvaluationResult, err error) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: opa evaluate panic: %v", ErrInvalidPolicy, r)
		}
	}()

	pq, err := e.prepared(ctx, policy)
	if err != nil {
		return EvaluationResult{}, err
	}

	evalCtx := ctx
	if e.evalTimeout > 0 {
		var cancel context.CancelFunc
		evalCtx, cancel = context.WithTimeout(ctx, e.evalTimeout)
		defer cancel()
	}

	rs, err := pq.Eval(evalCtx, rego.EvalInput(opaInput(input)))
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("%w: opa eval %q: %w", ErrInvalidPolicy, policy.ID, err)
	}

	res := EvaluationResult{
		PolicyID:    policy.ID,
		PolicyName:  policy.Name,
		EvaluatedAt: start.UTC(),
		Duration:    time.Since(start),
	}

	pkg := packageObject(rs)
	allowVal, hasAllow := pkg["allow"]
	if !hasAllow {
		res.Allowed = false
		res.Violations = []audit.Violation{{
			Rule:     "opa.no-decision",
			Message:  fmt.Sprintf("policy %q did not define data.%s.allow; treated as deny", policy.ID, OPAPackage),
			Severity: policy.Severity,
		}}
		return res, nil
	}

	allowed, ok := allowVal.(bool)
	if !ok {
		// allow defined but not a boolean — that is a policy authoring
		// error, but per the fail-closed convention we deny + record
		// rather than error the engine.
		res.Allowed = false
		res.Violations = []audit.Violation{{
			Rule:     "opa.non-bool-allow",
			Message:  fmt.Sprintf("policy %q data.%s.allow is %T, want bool; treated as deny", policy.ID, OPAPackage, allowVal),
			Severity: policy.Severity,
		}}
		return res, nil
	}
	res.Allowed = allowed
	res.Violations = extractViolations(pkg["violations"], policy.Severity)
	res.Warnings = extractWarnings(pkg["warnings"])
	return res, nil
}

// prepared returns the cached PreparedEvalQuery for policy, compiling
// + caching on first use. Key = policyID + sha256(Code) so a changed
// Code (post-v1.0 re-register) transparently re-compiles.
func (e *OPAEvaluator) prepared(ctx context.Context, policy *Policy) (rego.PreparedEvalQuery, error) {
	sum := sha256.Sum256([]byte(policy.Code))
	key := policy.ID + ":" + hex.EncodeToString(sum[:])

	e.mu.Lock()
	defer e.mu.Unlock()
	if pq, ok := e.cache[key]; ok {
		return pq, nil
	}

	r := rego.New(
		rego.Query(opaQuery),
		rego.Module(policy.ID+".rego", policy.Code),
		rego.Capabilities(e.caps),
		rego.SetRegoVersion(ast.RegoV1),
	)
	pq, err := r.PrepareForEval(ctx)
	if err != nil {
		return rego.PreparedEvalQuery{}, fmt.Errorf("%w: opa compile %q: %w", ErrInvalidPolicy, policy.ID, err)
	}
	e.cache[key] = pq
	return pq, nil
}

// opaInput maps EvaluationInput into the Rego `input` document. The
// caller's maps are referenced, not copied — the Evaluator contract
// forbids mutating them and Rego treats input as read-only.
func opaInput(in EvaluationInput) map[string]any {
	return map[string]any{
		"resource":  in.Resource,
		"action":    in.Action,
		"user":      in.User,
		"context":   in.Context,
		"timestamp": in.Timestamp.UTC().Format(time.RFC3339Nano),
	}
}

// packageObject pulls the data.keystone.policy object out of the
// result set. Empty / shaped-unexpectedly → empty map (treated as
// "allow undefined" by the caller).
func packageObject(rs rego.ResultSet) map[string]any {
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return map[string]any{}
	}
	obj, ok := rs[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return obj
}

// extractViolations best-effort-maps the `violations` binding. Each
// element may be a plain string (→ Violation{Message}) or an object
// with rule/message/severity/path/expected/actual/remediation keys.
// Unknown severity strings fall back to the policy's declared
// severity so a violation always carries a usable level.
func extractViolations(raw any, fallback audit.Severity) []audit.Violation {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]audit.Violation, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case string:
			out = append(out, audit.Violation{Message: v, Severity: fallback})
		case map[string]any:
			vi := audit.Violation{
				Rule:        stringField(v, "rule"),
				Message:     stringField(v, "message"),
				Path:        stringField(v, "path"),
				Expected:    stringField(v, "expected"),
				Actual:      stringField(v, "actual"),
				Remediation: stringField(v, "remediation"),
				Severity:    fallback,
			}
			if s := stringField(v, "severity"); s != "" {
				if parsed, perr := audit.ParseSeverity(s); perr == nil {
					vi.Severity = parsed
				}
			}
			out = append(out, vi)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// extractWarnings maps the `warnings` binding to []string. Non-string
// elements are skipped.
func extractWarnings(raw any) []string {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// Compile-time assertion that *OPAEvaluator satisfies [Evaluator].
var _ Evaluator = (*OPAEvaluator)(nil)
