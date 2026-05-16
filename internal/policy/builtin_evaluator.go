package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/glob"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// BuiltinEvaluator implements [Evaluator] for audit.PolicyTypeBuiltin
// via 13 hardcoded rules per §4.12. A builtin policy's Code is a
// single JSON object {"rule":"<name>", ...params} — one policy =
// one rule; compose rules with a PolicySet (task 5).
//
// Two error classes, deliberately distinct:
//
//   - Config errors (malformed JSON, unknown rule, bad time format,
//     bad glob) are the policy author's fault → evaluator error
//     (ErrInvalidPolicy family), mirroring OPA/CEL compile errors.
//   - Data-shape mismatches at eval (a resource field missing or the
//     wrong type) are NOT the author's fault — the data comes from
//     the operation, not the policy — so they fail closed: deny +
//     audit.Violation, nil error.
//
// Config is parsed + the rule "compiled" (globs/time-window
// pre-built) once and cached keyed policyID + sha256(Code) under a
// mutex. Code is immutable per registration (no Deregister in
// v1.0); a v1.8 re-register with changed Code gets a fresh key.
type BuiltinEvaluator struct {
	mu    sync.Mutex
	cache map[string]compiledRule
}

// compiledRule is a rule with its config bound, ready to evaluate an
// input. Returns the violations (empty = pass).
type compiledRule func(in EvaluationInput, sev audit.Severity) []audit.Violation

// NewBuiltinEvaluator returns a ready BuiltinEvaluator.
func NewBuiltinEvaluator() *BuiltinEvaluator {
	return &BuiltinEvaluator{cache: make(map[string]compiledRule)}
}

// Evaluate parses (cached) policy.Code and runs the named builtin
// rule. Pass → Allowed=true, no violations. Fail → Allowed=false +
// violations (severity = policy's declared severity). Config errors
// → ErrInvalidPolicy.
func (e *BuiltinEvaluator) Evaluate(ctx context.Context, policy *Policy, input EvaluationInput) (result EvaluationResult, err error) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: builtin evaluate panic: %v", ErrInvalidPolicy, r)
		}
	}()

	rule, err := e.compiled(policy)
	if err != nil {
		return EvaluationResult{}, err
	}

	violations := rule(input, policy.Severity)
	res := EvaluationResult{
		PolicyID:    policy.ID,
		PolicyName:  policy.Name,
		Allowed:     len(violations) == 0,
		Violations:  violations,
		EvaluatedAt: start.UTC(),
		Duration:    time.Since(start),
	}
	return res, nil
}

// compiled returns the cached compiledRule for policy, parsing +
// building on first use. Key = policyID + sha256(Code).
func (e *BuiltinEvaluator) compiled(policy *Policy) (compiledRule, error) {
	sum := sha256.Sum256([]byte(policy.Code))
	key := policy.ID + ":" + hex.EncodeToString(sum[:])

	e.mu.Lock()
	defer e.mu.Unlock()
	if r, ok := e.cache[key]; ok {
		return r, nil
	}

	var env struct {
		Rule string `json:"rule"`
	}
	if err := json.Unmarshal([]byte(policy.Code), &env); err != nil {
		return nil, fmt.Errorf("%w: builtin %q: malformed JSON config: %v", ErrInvalidPolicy, policy.ID, err)
	}
	builder, ok := ruleBuilders[env.Rule]
	if !ok {
		return nil, fmt.Errorf("%w: builtin %q: unknown rule %q", ErrInvalidPolicy, policy.ID, env.Rule)
	}
	rule, err := builder([]byte(policy.Code))
	if err != nil {
		return nil, fmt.Errorf("%w: builtin %q rule %q: %v", ErrInvalidPolicy, policy.ID, env.Rule, err)
	}
	e.cache[key] = rule
	return rule, nil
}

// ruleBuilders maps a rule name to its config-parsing builder. Each
// builder validates config at compile time and returns a
// compiledRule. Adding a rule = one entry here + one builder func.
var ruleBuilders = map[string]func(code []byte) (compiledRule, error){
	"require-labels":       buildRequireLabels,
	"require-owner":        buildRequireOwner,
	"allowed-environments": buildAllowedEnvironments,
	"allowed-actions":      buildAllowedActions,
	"deny-privileged":      buildDenyPrivileged,
	"allowed-users":        buildAllowedUsers,
	"time-window":          buildTimeWindow,
	"no-root-execution":    buildNoRootExecution,
	"require-approval":     buildRequireApproval,
	"max-concurrent":       buildMaxConcurrent,
	"resource-quota":       buildResourceQuota,
	"pattern-allow":        buildPatternAllow,
	"pattern-deny":         buildPatternDeny,
}

// ---- shared helpers -------------------------------------------------------

// strictTruthy: true | "true" | "yes" | "1" | 1 are truthy
// (strings case-insensitive). Everything else, including a missing
// value (nil), is falsy.
func strictTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "yes", "1":
			return true
		}
		return false
	case float64:
		return x == 1
	case int:
		return x == 1
	default:
		return false
	}
}

// sourceMap returns Resource (default) or Context per the optional
// "source" config key.
func sourceMap(in EvaluationInput, source string) map[string]any {
	if strings.EqualFold(source, "context") {
		if in.Context == nil {
			return map[string]any{}
		}
		return in.Context
	}
	if in.Resource == nil {
		return map[string]any{}
	}
	return in.Resource
}

// labelsOf returns resource.labels as a string→any map (empty when
// absent or wrong-typed — a require-labels miss is then a clean
// deny, not a panic).
func labelsOf(in EvaluationInput) map[string]any {
	if in.Resource == nil {
		return map[string]any{}
	}
	l, ok := in.Resource["labels"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return l
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// asNumber accepts JSON float64 / int. Strings are NOT coerced —
// resource-quota on a string field is a data-shape mismatch (deny).
func asNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func inSet(s string, set []string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}

func vio(rule, msg, path, expected, actual string, sev audit.Severity) audit.Violation {
	return audit.Violation{
		Rule: rule, Message: msg, Path: path,
		Expected: expected, Actual: actual, Severity: sev,
	}
}

// ---- rule builders --------------------------------------------------------

func buildRequireLabels(code []byte) (compiledRule, error) {
	var c struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	if len(c.Keys) == 0 {
		return nil, fmt.Errorf("require-labels needs a non-empty \"keys\"")
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		labels := labelsOf(in)
		var vs []audit.Violation
		for _, k := range c.Keys {
			val, ok := labels[k]
			s, _ := asString(val)
			if !ok || strings.TrimSpace(s) == "" {
				vs = append(vs, vio("require-labels",
					fmt.Sprintf("required label %q is missing or empty", k),
					"resource.labels."+k, "non-empty", "", sev))
			}
		}
		return vs
	}, nil
}

func buildRequireOwner(code []byte) (compiledRule, error) {
	var c struct {
		Field string `json:"field"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	field := c.Field
	if field == "" {
		field = "owner"
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		labels := labelsOf(in)
		s, _ := asString(labels[field])
		if strings.TrimSpace(s) == "" {
			return []audit.Violation{vio("require-owner",
				fmt.Sprintf("required owner label %q is missing or empty", field),
				"resource.labels."+field, "non-empty", "", sev)}
		}
		return nil
	}, nil
}

func buildAllowedEnvironments(code []byte) (compiledRule, error) {
	var c struct {
		Field   string   `json:"field"`
		Allowed []string `json:"allowed"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	if len(c.Allowed) == 0 {
		return nil, fmt.Errorf("allowed-environments needs a non-empty \"allowed\"")
	}
	field := c.Field
	if field == "" {
		field = "env"
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		s, _ := asString(labelsOf(in)[field])
		if !inSet(s, c.Allowed) {
			return []audit.Violation{vio("allowed-environments",
				fmt.Sprintf("environment %q is not allowed", s),
				"resource.labels."+field, strings.Join(c.Allowed, "|"), s, sev)}
		}
		return nil
	}, nil
}

func buildAllowedActions(code []byte) (compiledRule, error) {
	var c struct {
		Allowed []string `json:"allowed"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	if len(c.Allowed) == 0 {
		return nil, fmt.Errorf("allowed-actions needs a non-empty \"allowed\"")
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		if !inSet(in.Action, c.Allowed) {
			return []audit.Violation{vio("allowed-actions",
				fmt.Sprintf("action %q is not allowed", in.Action),
				"action", strings.Join(c.Allowed, "|"), in.Action, sev)}
		}
		return nil
	}, nil
}

func buildDenyPrivileged(code []byte) (compiledRule, error) {
	var c struct {
		Field  string `json:"field"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	field := c.Field
	if field == "" {
		field = "privileged"
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		if strictTruthy(sourceMap(in, c.Source)[field]) {
			return []audit.Violation{vio("deny-privileged",
				fmt.Sprintf("privileged flag %q is set", field),
				field, "false", "true", sev)}
		}
		return nil
	}, nil
}

func buildAllowedUsers(code []byte) (compiledRule, error) {
	var c struct {
		Allowed []string `json:"allowed"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	if len(c.Allowed) == 0 {
		return nil, fmt.Errorf("allowed-users needs a non-empty \"allowed\"")
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		if !inSet(in.User, c.Allowed) {
			return []audit.Violation{vio("allowed-users",
				fmt.Sprintf("user %q is not allowed", in.User),
				"user", strings.Join(c.Allowed, "|"), in.User, sev)}
		}
		return nil
	}, nil
}

func buildTimeWindow(code []byte) (compiledRule, error) {
	var c struct {
		Days  []string `json:"days"`
		Start string   `json:"start"`
		End   string   `json:"end"`
		TZ    string   `json:"tz"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	loc := time.UTC
	if c.TZ != "" {
		l, err := time.LoadLocation(c.TZ)
		if err != nil {
			return nil, fmt.Errorf("time-window: bad tz %q: %v", c.TZ, err)
		}
		loc = l
	}
	var start, end time.Time
	const hm = "15:04"
	var err error
	if c.Start != "" {
		if start, err = time.Parse(hm, c.Start); err != nil {
			return nil, fmt.Errorf("time-window: bad start %q (want HH:MM)", c.Start)
		}
	}
	if c.End != "" {
		if end, err = time.Parse(hm, c.End); err != nil {
			return nil, fmt.Errorf("time-window: bad end %q (want HH:MM)", c.End)
		}
	}
	dayset := map[time.Weekday]bool{}
	for _, d := range c.Days {
		wd, ok := parseWeekday(d)
		if !ok {
			return nil, fmt.Errorf("time-window: bad day %q", d)
		}
		dayset[wd] = true
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		ts := in.Timestamp.In(loc)
		if len(dayset) > 0 && !dayset[ts.Weekday()] {
			return []audit.Violation{vio("time-window",
				fmt.Sprintf("%s is not an allowed day", ts.Weekday()),
				"input.timestamp", strings.Join(c.Days, ","), ts.Weekday().String(), sev)}
		}
		if c.Start != "" || c.End != "" {
			mins := ts.Hour()*60 + ts.Minute()
			lo := start.Hour()*60 + start.Minute()
			hi := end.Hour()*60 + end.Minute()
			if (c.Start != "" && mins < lo) || (c.End != "" && mins > hi) {
				return []audit.Violation{vio("time-window",
					"timestamp is outside the allowed time-of-day window",
					"input.timestamp", c.Start+"-"+c.End, ts.Format(hm), sev)}
			}
		}
		return nil
	}, nil
}

func buildNoRootExecution(code []byte) (compiledRule, error) {
	var c struct {
		Field  string `json:"field"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		user := in.User
		if c.Field != "" {
			if s, ok := asString(sourceMap(in, c.Source)[c.Field]); ok {
				user = s
			}
		}
		if user == "root" || user == "0" {
			return []audit.Violation{vio("no-root-execution",
				"execution as root is not permitted",
				"user", "non-root", user, sev)}
		}
		return nil
	}, nil
}

func buildRequireApproval(code []byte) (compiledRule, error) {
	var c struct {
		Field  string `json:"field"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	field := c.Field
	if field == "" {
		field = "approved"
	}
	source := c.Source
	if source == "" {
		source = "context"
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		if !strictTruthy(sourceMap(in, source)[field]) {
			return []audit.Violation{vio("require-approval",
				fmt.Sprintf("approval flag %q is not set", field),
				source+"."+field, "true", "", sev)}
		}
		return nil
	}, nil
}

func buildMaxConcurrent(code []byte) (compiledRule, error) {
	var c struct {
		Field  string  `json:"field"`
		Max    float64 `json:"max"`
		Source string  `json:"source"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	if c.Field == "" {
		c.Field = "concurrent"
	}
	source := c.Source
	if source == "" {
		source = "context"
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		n, ok := asNumber(sourceMap(in, source)[c.Field])
		if !ok {
			return []audit.Violation{vio("max-concurrent",
				fmt.Sprintf("field %q is missing or not numeric", c.Field),
				source+"."+c.Field, "numeric", "", sev)}
		}
		if n > c.Max {
			return []audit.Violation{vio("max-concurrent",
				fmt.Sprintf("concurrency %g exceeds max %g", n, c.Max),
				source+"."+c.Field, fmt.Sprintf("<=%g", c.Max), fmt.Sprintf("%g", n), sev)}
		}
		return nil
	}, nil
}

func buildResourceQuota(code []byte) (compiledRule, error) {
	var c struct {
		Field string  `json:"field"`
		Max   float64 `json:"max"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return nil, err
	}
	if c.Field == "" {
		return nil, fmt.Errorf("resource-quota needs a \"field\"")
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		n, ok := asNumber(sourceMap(in, "resource")[c.Field])
		if !ok {
			return []audit.Violation{vio("resource-quota",
				fmt.Sprintf("resource field %q is missing or not numeric", c.Field),
				"resource."+c.Field, "numeric", "", sev)}
		}
		if n > c.Max {
			return []audit.Violation{vio("resource-quota",
				fmt.Sprintf("%s %g exceeds quota %g", c.Field, n, c.Max),
				"resource."+c.Field, fmt.Sprintf("<=%g", c.Max), fmt.Sprintf("%g", n), sev)}
		}
		return nil
	}, nil
}

func buildPatternAllow(code []byte) (compiledRule, error) {
	field, globs, err := parsePatternConfig(code, "pattern-allow")
	if err != nil {
		return nil, err
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		s, _ := asString(sourceMap(in, "resource")[field])
		for _, g := range globs {
			if g.Match(s) {
				return nil
			}
		}
		return []audit.Violation{vio("pattern-allow",
			fmt.Sprintf("%q matches no allowed pattern", s),
			"resource."+field, "match an allowed pattern", s, sev)}
	}, nil
}

func buildPatternDeny(code []byte) (compiledRule, error) {
	field, globs, err := parsePatternConfig(code, "pattern-deny")
	if err != nil {
		return nil, err
	}
	return func(in EvaluationInput, sev audit.Severity) []audit.Violation {
		s, _ := asString(sourceMap(in, "resource")[field])
		for _, g := range globs {
			if g.Match(s) {
				return []audit.Violation{vio("pattern-deny",
					fmt.Sprintf("%q matches a denied pattern", s),
					"resource."+field, "match no denied pattern", s, sev)}
			}
		}
		return nil
	}, nil
}

// parsePatternConfig is shared by pattern-allow / pattern-deny: it
// validates the field + compiles every glob at build time so a bad
// pattern is a config error, not a per-eval surprise.
func parsePatternConfig(code []byte, rule string) (string, []glob.Glob, error) {
	var c struct {
		Field    string   `json:"field"`
		Patterns []string `json:"patterns"`
	}
	if err := json.Unmarshal(code, &c); err != nil {
		return "", nil, err
	}
	if c.Field == "" {
		return "", nil, fmt.Errorf("%s needs a \"field\"", rule)
	}
	if len(c.Patterns) == 0 {
		return "", nil, fmt.Errorf("%s needs a non-empty \"patterns\"", rule)
	}
	globs := make([]glob.Glob, 0, len(c.Patterns))
	for _, p := range c.Patterns {
		g, err := glob.Compile(p)
		if err != nil {
			return "", nil, fmt.Errorf("%s: bad glob %q: %v", rule, p, err)
		}
		globs = append(globs, g)
	}
	return c.Field, globs, nil
}

func parseWeekday(s string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sun", "sunday":
		return time.Sunday, true
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tues", "tuesday":
		return time.Tuesday, true
	case "wed", "weds", "wednesday":
		return time.Wednesday, true
	case "thu", "thur", "thurs", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	}
	return 0, false
}

// Compile-time assertion that *BuiltinEvaluator satisfies [Evaluator].
var _ Evaluator = (*BuiltinEvaluator)(nil)
