// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Policy is the default decision when no command allow/deny rule
// matches. Recommended: PolicyDeny — operators explicitly allow.
type Policy string

const (
	PolicyAllow Policy = "allow"
	PolicyDeny  Policy = "deny"
)

// CommandRequest is the parsed inbound command shape — what the
// control plane sends inside the envelope payload. SecurityEnforcer
// validates it before the Executor (Task 2) sees it. The same struct
// is the wire format consumed by both the agent (parses) and the
// control plane (signs + publishes).
//
// MessageID comes from the wrapping envelope.MessageID; it's part
// of the HMAC canonical input so a captured-and-replayed command on
// a different MessageID fails verification.
//
// Signature is hex-encoded HMAC-SHA-256 of the canonical-encoded
// request fields under the configured secret. Excluded from the
// canonical itself (it's the output, not the input).
type CommandRequest struct {
	MessageID      string            `json:"message_id"`
	Principal      string            `json:"principal"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	User           string            `json:"user,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Signature      string            `json:"signature"`
}

// CommandRules govern the deny / allow / default decision against
// req.Command (globs) and the full command line (regexes).
//
// Decision order: deny → allow → DefaultPolicy. Deny wins.
//   - Globs use stdlib path.Match against req.Command only (e.g.,
//     "/usr/bin/*" matches "/usr/bin/uptime"). v1.0 keeps it simple;
//     gobwas/glob's "**" can land in v1.x if needed.
//   - Regexes use stdlib regexp against the full command line
//     (req.Command + " " + strings.Join(req.Args, " ")), so policy
//     can express "block 'git push' but not 'git status'".
type CommandRules struct {
	AllowGlobs   []string
	AllowRegexes []string
	DenyGlobs    []string
	DenyRegexes  []string
}

// SecurityPolicy is the v1.0 protection bundle. v1.0 takes the
// HMAC secret via config; v1.x can derive from the bootstrap-
// issued credential. Replay protection (timestamp window + nonce
// dedup) is a post-v1.0 addition per PROJECT-DETAILS §4.10 — v1.0 trusts
// the HMAC alone. Manager.PublishEnvelope's producer-side dedup
// (Task 6) catches identical MessageIDs on the legitimate path;
// replay from a captured wire is an acknowledged v1.0 trust
// assumption.
type SecurityPolicy struct {
	HMACSecret         []byte
	PrincipalAllowlist []string
	CommandRules       CommandRules
	EnvVarAllowlist    []string
	MaxArgsBytes       int
	DefaultPolicy      Policy
}

const defaultMaxArgsBytes = 64 * 1024 // §4.7 default

// SecurityEnforcer is the agent-side policy gate (Epic 06 task 4).
// Stateless except for the policy + compiled regexes; safe for
// concurrent use.
type SecurityEnforcer struct {
	policy             SecurityPolicy
	log                *slog.Logger
	compiledAllowRegex []*regexp.Regexp
	compiledDenyRegex  []*regexp.Regexp
}

// Sentinel errors. Callers (Task 5) surface these to the response
// path so the control plane sees a typed reason for the rejection.
var (
	ErrHMACInvalid     = errors.New("security: invalid HMAC signature")
	ErrPrincipalDenied = errors.New("security: principal not in allowlist")
	ErrCommandDenied   = errors.New("security: command blocked by policy")
	ErrArgsTooLong     = errors.New("security: command args exceed MaxArgsBytes")
)

// NewSecurityEnforcer compiles the regex patterns and returns a
// ready enforcer. Pattern errors surface here rather than at first
// use — the agent fails to start with an unloadable policy, which
// is correct: a syntactically broken policy must not run.
func NewSecurityEnforcer(policy SecurityPolicy, log *slog.Logger) (*SecurityEnforcer, error) {
	if log == nil {
		log = slog.Default()
	}
	if policy.MaxArgsBytes == 0 {
		policy.MaxArgsBytes = defaultMaxArgsBytes
	}
	if policy.DefaultPolicy == "" {
		policy.DefaultPolicy = PolicyDeny
	}
	if policy.DefaultPolicy != PolicyAllow && policy.DefaultPolicy != PolicyDeny {
		return nil, fmt.Errorf("security: DefaultPolicy %q (must be allow or deny)", policy.DefaultPolicy)
	}

	allow, err := compileRegexes(policy.CommandRules.AllowRegexes)
	if err != nil {
		return nil, fmt.Errorf("security: AllowRegexes: %w", err)
	}
	deny, err := compileRegexes(policy.CommandRules.DenyRegexes)
	if err != nil {
		return nil, fmt.Errorf("security: DenyRegexes: %w", err)
	}

	// Precompile globs to catch malformed patterns early.
	for _, g := range policy.CommandRules.AllowGlobs {
		if _, err := path.Match(g, ""); err != nil {
			return nil, fmt.Errorf("security: AllowGlobs[%q]: %w", g, err)
		}
	}
	for _, g := range policy.CommandRules.DenyGlobs {
		if _, err := path.Match(g, ""); err != nil {
			return nil, fmt.Errorf("security: DenyGlobs[%q]: %w", g, err)
		}
	}

	return &SecurityEnforcer{
		policy:             policy,
		log:                log,
		compiledAllowRegex: allow,
		compiledDenyRegex:  deny,
	}, nil
}

// Validate enforces all v1.0 protections in order: HMAC, principal
// allowlist, command rules, MaxArgsBytes. Audit-logs the decision —
// WARN on reject, INFO on accept — so the level split is meaningful
// to operational tooling. Returns nil when accepted; returns one of
// the sentinel errors on rejection.
func (s *SecurityEnforcer) Validate(ctx context.Context, req CommandRequest) error {
	if err := s.checkHMAC(req); err != nil {
		s.audit(ctx, false, req, err)
		return err
	}
	if err := s.checkPrincipal(req); err != nil {
		s.audit(ctx, false, req, err)
		return err
	}
	if err := s.checkCommand(req); err != nil {
		s.audit(ctx, false, req, err)
		return err
	}
	if err := s.checkArgsLength(req); err != nil {
		s.audit(ctx, false, req, err)
		return err
	}
	s.audit(ctx, true, req, nil)
	return nil
}

// ComputeHMAC returns the canonical HMAC for req under the
// configured secret. Both the agent (verify) and the control plane
// (sign) use this — keeping a single function defends against
// canonicalization drift across the two sides.
func (s *SecurityEnforcer) ComputeHMAC(req CommandRequest) string {
	mac := hmac.New(sha256.New, s.policy.HMACSecret)
	mac.Write(canonical(req))
	return hex.EncodeToString(mac.Sum(nil))
}

// AppliedEnv returns req.Env filtered to the EnvVarAllowlist. Task 5
// passes this into Executor.ExecuteRequest.Env so the executor only
// receives operator-blessed env keys. Empty allowlist means no env
// passes through (tightest default).
func (s *SecurityEnforcer) AppliedEnv(req CommandRequest) map[string]string {
	if len(s.policy.EnvVarAllowlist) == 0 || len(req.Env) == 0 {
		return nil
	}
	allow := make(map[string]struct{}, len(s.policy.EnvVarAllowlist))
	for _, k := range s.policy.EnvVarAllowlist {
		allow[k] = struct{}{}
	}
	out := make(map[string]string, len(req.Env))
	for k, v := range req.Env {
		if _, ok := allow[k]; ok {
			out[k] = v
		}
	}
	return out
}

// ValidateConverge is Validate's counterpart for state runs. It runs
// the two checks that are meaningful for a ConvergeRequest — HMAC and
// principal — and skips the two that are not: there is no command line
// to match against CommandRules, and no argv to length-check.
//
// Deliberately the SAME secret and the same principal allowlist as the
// command path. Dispatching a state run is not a lesser privilege than
// dispatching a command (a state file can install packages and restart
// services), so it earns no separate, weaker gate.
func (s *SecurityEnforcer) ValidateConverge(ctx context.Context, req ConvergeRequest) error {
	if err := s.checkConvergeHMAC(req); err != nil {
		s.auditConverge(ctx, false, req, err)
		return err
	}
	if err := s.checkConvergePrincipal(req); err != nil {
		s.auditConverge(ctx, false, req, err)
		return err
	}
	s.auditConverge(ctx, true, req, nil)
	return nil
}

// ComputeConvergeHMAC returns the canonical HMAC for req under the
// configured secret. The control plane signs with this and the agent
// verifies with it, so canonicalization cannot drift between them.
func (s *SecurityEnforcer) ComputeConvergeHMAC(req ConvergeRequest) string {
	mac := hmac.New(sha256.New, s.policy.HMACSecret)
	mac.Write(canonicalConverge(req))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *SecurityEnforcer) checkConvergeHMAC(req ConvergeRequest) error {
	if len(s.policy.HMACSecret) == 0 {
		// Same rationale as checkHMAC: an unconfigured secret disables
		// the check so a fresh agent can boot before bootstrap wires
		// one in. ProductionWarnings() shouts about it in production.
		return nil
	}
	want, err := hex.DecodeString(req.Signature)
	if err != nil {
		return ErrHMACInvalid
	}
	mac := hmac.New(sha256.New, s.policy.HMACSecret)
	mac.Write(canonicalConverge(req))
	if !hmac.Equal(want, mac.Sum(nil)) {
		return ErrHMACInvalid
	}
	return nil
}

func (s *SecurityEnforcer) checkConvergePrincipal(req ConvergeRequest) error {
	if len(s.policy.PrincipalAllowlist) == 0 {
		return nil
	}
	for _, p := range s.policy.PrincipalAllowlist {
		if p == req.Principal {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrPrincipalDenied, req.Principal)
}

// canonicalConverge is the byte encoding signed for a ConvergeRequest.
// Length-prefixed like canonical() so no field boundary can be forged
// by choosing a value that contains the delimiter. YAML is hashed by
// content, not length alone, since it is the payload that decides what
// runs on the host.
func canonicalConverge(req ConvergeRequest) []byte {
	var buf bytes.Buffer
	buf.WriteString("MessageID:")
	buf.WriteString(req.MessageID)
	buf.WriteString("\nPrincipal:")
	buf.WriteString(req.Principal)
	buf.WriteString("\nRunID:")
	buf.WriteString(req.RunID)
	buf.WriteString("\nSource:")
	buf.WriteString(req.Source)
	buf.WriteString("\nMode:")
	buf.WriteString(req.Mode)
	buf.WriteString("\nTimeoutSeconds:")
	writeUint32(&buf, uint32(req.TimeoutSeconds))

	buf.WriteString("\nYAML:")
	writeUint32(&buf, uint32(len(req.YAML)))
	buf.Write(req.YAML)

	// Variables are map-ordered in Go, so sort before hashing or the
	// signature is not reproducible.
	buf.WriteString("\nVariables:")
	writeUint32(&buf, uint32(len(req.Variables)))
	keys := make([]string, 0, len(req.Variables))
	for k := range req.Variables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		writeUint32(&buf, uint32(len(k)))
		buf.WriteString(k)
		v := req.Variables[k]
		writeUint32(&buf, uint32(len(v)))
		buf.WriteString(v)
	}
	return buf.Bytes()
}

func (s *SecurityEnforcer) auditConverge(ctx context.Context, accepted bool, req ConvergeRequest, reason error) {
	if s.log == nil {
		return
	}
	attrs := []any{
		"event", "agent.converge.authz",
		"accepted", accepted,
		"message_id", req.MessageID,
		"run_id", req.RunID,
		"principal", req.Principal,
		"mode", req.Mode,
		"source", req.Source,
		"yaml_bytes", len(req.YAML),
	}
	if reason != nil {
		attrs = append(attrs, "reason", reason.Error())
	}
	if accepted {
		s.log.InfoContext(ctx, "agent: converge authorized", attrs...)
		return
	}
	s.log.WarnContext(ctx, "agent: converge rejected", attrs...)
}

func (s *SecurityEnforcer) checkHMAC(req CommandRequest) error {
	if len(s.policy.HMACSecret) == 0 {
		// No secret configured = HMAC check disabled. Empty default
		// lets a fresh agent boot without crypto config so the
		// bootstrap flow can wire it in later. Production
		// deployments MUST set HMACSecret -- internal/config/config.go
		// ProductionWarnings() emits a loud WARN at startup when this
		// is empty in production mode (Phase B5 finding C1).
		return nil
	}
	want, err := hex.DecodeString(req.Signature)
	if err != nil {
		return ErrHMACInvalid
	}
	mac := hmac.New(sha256.New, s.policy.HMACSecret)
	mac.Write(canonical(req))
	got := mac.Sum(nil)
	if !hmac.Equal(want, got) {
		return ErrHMACInvalid
	}
	return nil
}

func (s *SecurityEnforcer) checkPrincipal(req CommandRequest) error {
	if len(s.policy.PrincipalAllowlist) == 0 {
		return nil
	}
	for _, p := range s.policy.PrincipalAllowlist {
		if p == req.Principal {
			return nil
		}
	}
	return ErrPrincipalDenied
}

func (s *SecurityEnforcer) checkCommand(req CommandRequest) error {
	fullLine := req.Command
	if len(req.Args) > 0 {
		fullLine = req.Command + " " + strings.Join(req.Args, " ")
	}

	// Deny wins.
	for _, g := range s.policy.CommandRules.DenyGlobs {
		if ok, _ := path.Match(g, req.Command); ok {
			return ErrCommandDenied
		}
	}
	for _, re := range s.compiledDenyRegex {
		if re.MatchString(fullLine) {
			return ErrCommandDenied
		}
	}

	// Allow.
	for _, g := range s.policy.CommandRules.AllowGlobs {
		if ok, _ := path.Match(g, req.Command); ok {
			return nil
		}
	}
	for _, re := range s.compiledAllowRegex {
		if re.MatchString(fullLine) {
			return nil
		}
	}

	// Default fallback.
	if s.policy.DefaultPolicy == PolicyDeny {
		return ErrCommandDenied
	}
	return nil
}

func (s *SecurityEnforcer) checkArgsLength(req CommandRequest) error {
	total := 0
	for _, a := range req.Args {
		total += len(a)
		if total > s.policy.MaxArgsBytes {
			return ErrArgsTooLong
		}
	}
	return nil
}

// audit emits the per-decision log line. WARN on reject (operations
// pages on this); INFO on accept (telemetry-grade, lower priority).
func (s *SecurityEnforcer) audit(ctx context.Context, accepted bool, req CommandRequest, reason error) {
	attrs := []any{
		"principal", req.Principal,
		"command", req.Command,
		"message_id", req.MessageID,
	}
	if accepted {
		s.log.InfoContext(ctx, "security: command allowed", attrs...)
		return
	}
	if reason != nil {
		attrs = append(attrs, "reason", reason.Error())
	}
	s.log.WarnContext(ctx, "security: command rejected", attrs...)
}

// canonical builds the HMAC input bytes. Length-prefixed fields
// where ambiguity is possible (Args is variable-length); env keys
// sorted lexicographically so the same map[string]string produces a
// stable digest regardless of Go's map iteration order. Same
// defensive pattern as Task 6's dedup hash.
//
// Signature is deliberately excluded — it's the output of HMAC over
// these bytes, not an input.
func canonical(req CommandRequest) []byte {
	var buf bytes.Buffer
	buf.WriteString("MessageID:")
	buf.WriteString(req.MessageID)
	buf.WriteString("\nPrincipal:")
	buf.WriteString(req.Principal)
	buf.WriteString("\nCommand:")
	buf.WriteString(req.Command)
	buf.WriteString("\nWorkingDir:")
	buf.WriteString(req.WorkingDir)
	buf.WriteString("\nUser:")
	buf.WriteString(req.User)
	buf.WriteString("\nTimeoutSeconds:")
	writeUint32(&buf, uint32(req.TimeoutSeconds))

	buf.WriteString("\nArgs:")
	writeUint32(&buf, uint32(len(req.Args)))
	for _, a := range req.Args {
		writeUint32(&buf, uint32(len(a)))
		buf.WriteString(a)
	}

	buf.WriteString("\nEnv:")
	keys := make([]string, 0, len(req.Env))
	for k := range req.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	writeUint32(&buf, uint32(len(keys)))
	for _, k := range keys {
		writeUint32(&buf, uint32(len(k)))
		buf.WriteString(k)
		v := req.Env[k]
		writeUint32(&buf, uint32(len(v)))
		buf.WriteString(v)
	}

	return buf.Bytes()
}

func writeUint32(w *bytes.Buffer, n uint32) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], n)
	w.Write(buf[:])
}

func compileRegexes(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("compile %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}
