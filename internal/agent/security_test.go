// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// signRequest is a test helper: computes a valid HMAC for req under
// secret and returns a copy of req with the Signature field set.
// Mirrors what the control plane's command-signer code does in
// production.
func signRequest(secret []byte, req CommandRequest) CommandRequest {
	tmp := req
	tmp.Signature = ""
	enf, err := NewSecurityEnforcer(SecurityPolicy{
		HMACSecret: secret,
	}, testLogger())
	if err != nil {
		panic(err)
	}
	tmp.Signature = enf.ComputeHMAC(tmp)
	return tmp
}

func newTestEnforcer(t *testing.T, mut func(*SecurityPolicy)) *SecurityEnforcer {
	t.Helper()
	p := SecurityPolicy{
		HMACSecret:    []byte("test-secret"),
		DefaultPolicy: PolicyAllow, // friendly default for tests not exercising deny
		MaxArgsBytes:  64 * 1024,
	}
	if mut != nil {
		mut(&p)
	}
	enf, err := NewSecurityEnforcer(p, testLogger())
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	return enf
}

// goodRequest returns a request that passes a default-allow
// enforcer with HMACSecret = "test-secret".
func goodRequest(t *testing.T) CommandRequest {
	t.Helper()
	req := CommandRequest{
		MessageID: "msg-1",
		Principal: "admin",
		Command:   "/usr/bin/uptime",
		Args:      []string{},
		Env:       map[string]string{"PATH": "/usr/bin"},
		User:      "kscore-agent",
	}
	return signRequest([]byte("test-secret"), req)
}

func TestNewSecurityEnforcer_DefaultsApplied(t *testing.T) {
	enf, err := NewSecurityEnforcer(SecurityPolicy{}, testLogger())
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	if enf.policy.MaxArgsBytes != defaultMaxArgsBytes {
		t.Errorf("MaxArgsBytes = %d, want default %d", enf.policy.MaxArgsBytes, defaultMaxArgsBytes)
	}
	if enf.policy.DefaultPolicy != PolicyDeny {
		t.Errorf("DefaultPolicy = %q, want default deny", enf.policy.DefaultPolicy)
	}
}

func TestNewSecurityEnforcer_RejectsBadDefaultPolicy(t *testing.T) {
	if _, err := NewSecurityEnforcer(SecurityPolicy{DefaultPolicy: "perhaps"}, testLogger()); err == nil {
		t.Error("expected error for invalid DefaultPolicy")
	}
}

func TestNewSecurityEnforcer_RejectsBadRegex(t *testing.T) {
	_, err := NewSecurityEnforcer(SecurityPolicy{
		CommandRules: CommandRules{DenyRegexes: []string{"["}},
	}, testLogger())
	if err == nil {
		t.Error("expected error for malformed regex")
	}
}

func TestNewSecurityEnforcer_RejectsBadGlob(t *testing.T) {
	_, err := NewSecurityEnforcer(SecurityPolicy{
		CommandRules: CommandRules{AllowGlobs: []string{"["}},
	}, testLogger())
	if err == nil {
		t.Error("expected error for malformed glob")
	}
}

func TestSecurityEnforcer_HMACAccept(t *testing.T) {
	enf := newTestEnforcer(t, nil)
	if err := enf.Validate(context.Background(), goodRequest(t)); err != nil {
		t.Errorf("Validate good request: %v", err)
	}
}

func TestSecurityEnforcer_HMACRejectsTamperedField(t *testing.T) {
	enf := newTestEnforcer(t, nil)
	req := goodRequest(t)
	req.Command = "/bin/rm" // signature is no longer valid
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrHMACInvalid) {
		t.Errorf("err = %v, want ErrHMACInvalid", err)
	}
}

func TestSecurityEnforcer_HMACRejectsWrongSecret(t *testing.T) {
	enf := newTestEnforcer(t, nil)
	// Sign with a different secret than the enforcer holds.
	req := signRequest([]byte("different-secret"), CommandRequest{
		MessageID: "msg-1",
		Principal: "admin",
		Command:   "/usr/bin/uptime",
	})
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrHMACInvalid) {
		t.Errorf("err = %v, want ErrHMACInvalid", err)
	}
}

func TestSecurityEnforcer_HMACRejectsMissingSignature(t *testing.T) {
	enf := newTestEnforcer(t, nil)
	req := goodRequest(t)
	req.Signature = ""
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrHMACInvalid) {
		t.Errorf("err = %v, want ErrHMACInvalid", err)
	}
}

func TestSecurityEnforcer_HMACRejectsBadHex(t *testing.T) {
	enf := newTestEnforcer(t, nil)
	req := goodRequest(t)
	req.Signature = "not-hex-zzz"
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrHMACInvalid) {
		t.Errorf("err = %v, want ErrHMACInvalid", err)
	}
}

func TestSecurityEnforcer_HMACDisabledWhenSecretEmpty(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.HMACSecret = nil
	})
	// Empty signature still works; HMAC check skipped.
	req := CommandRequest{
		MessageID: "msg-1",
		Principal: "admin",
		Command:   "/usr/bin/uptime",
	}
	if err := enf.Validate(context.Background(), req); err != nil {
		t.Errorf("Validate with HMAC disabled: %v", err)
	}
}

func TestSecurityEnforcer_PrincipalAllowlistEmptyAcceptsAll(t *testing.T) {
	enf := newTestEnforcer(t, nil) // empty PrincipalAllowlist
	req := goodRequest(t)
	if err := enf.Validate(context.Background(), req); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestSecurityEnforcer_PrincipalAllowlistRejectsUnlisted(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.PrincipalAllowlist = []string{"only-this-user"}
	})
	req := goodRequest(t) // Principal: "admin"
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrPrincipalDenied) {
		t.Errorf("err = %v, want ErrPrincipalDenied", err)
	}
}

func TestSecurityEnforcer_PrincipalAllowlistAcceptsListed(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.PrincipalAllowlist = []string{"admin", "operator"}
	})
	req := goodRequest(t)
	if err := enf.Validate(context.Background(), req); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestSecurityEnforcer_CommandAllowGlobAccepts(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.DefaultPolicy = PolicyDeny
		p.CommandRules.AllowGlobs = []string{"/usr/bin/*"}
	})
	req := goodRequest(t)
	if err := enf.Validate(context.Background(), req); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestSecurityEnforcer_CommandAllowGlobMismatchDenies(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.DefaultPolicy = PolicyDeny
		p.CommandRules.AllowGlobs = []string{"/sbin/*"}
	})
	req := goodRequest(t) // /usr/bin/uptime, not /sbin/...
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrCommandDenied) {
		t.Errorf("err = %v, want ErrCommandDenied", err)
	}
}

func TestSecurityEnforcer_DenyWinsOverAllow(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.CommandRules.AllowGlobs = []string{"/usr/bin/*"}
		p.CommandRules.DenyGlobs = []string{"/usr/bin/uptime"}
	})
	req := goodRequest(t)
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrCommandDenied) {
		t.Errorf("err = %v, want ErrCommandDenied (deny wins)", err)
	}
}

func TestSecurityEnforcer_DenyRegexBlocksFullCommandLine(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.CommandRules.AllowGlobs = []string{"/usr/bin/git"}
		p.CommandRules.DenyRegexes = []string{`\bpush\b`}
	})
	req := signRequest([]byte("test-secret"), CommandRequest{
		MessageID: "m",
		Principal: "admin",
		Command:   "/usr/bin/git",
		Args:      []string{"push"},
	})
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrCommandDenied) {
		t.Errorf("err = %v, want ErrCommandDenied", err)
	}
}

func TestSecurityEnforcer_AllowRegexAccepts(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.DefaultPolicy = PolicyDeny
		p.CommandRules.AllowRegexes = []string{`^/usr/bin/git status$`}
	})
	req := signRequest([]byte("test-secret"), CommandRequest{
		MessageID: "m",
		Principal: "admin",
		Command:   "/usr/bin/git",
		Args:      []string{"status"},
	})
	if err := enf.Validate(context.Background(), req); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestSecurityEnforcer_DefaultDenyOnNoMatch(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.DefaultPolicy = PolicyDeny
		// No allow / deny rules → everything falls through to default.
	})
	req := goodRequest(t)
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrCommandDenied) {
		t.Errorf("err = %v, want ErrCommandDenied (default deny)", err)
	}
}

func TestSecurityEnforcer_DefaultAllowOnNoMatch(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.DefaultPolicy = PolicyAllow
	})
	req := goodRequest(t)
	if err := enf.Validate(context.Background(), req); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestSecurityEnforcer_MaxArgsBytesEnforced(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.MaxArgsBytes = 10
	})
	long := strings.Repeat("a", 50)
	req := signRequest([]byte("test-secret"), CommandRequest{
		MessageID: "m",
		Principal: "admin",
		Command:   "/usr/bin/uptime",
		Args:      []string{long},
	})
	err := enf.Validate(context.Background(), req)
	if !errors.Is(err, ErrArgsTooLong) {
		t.Errorf("err = %v, want ErrArgsTooLong", err)
	}
}

func TestSecurityEnforcer_AppliedEnvFiltersToAllowlist(t *testing.T) {
	enf := newTestEnforcer(t, func(p *SecurityPolicy) {
		p.EnvVarAllowlist = []string{"PATH", "USER"}
	})
	req := CommandRequest{
		Env: map[string]string{
			"PATH":         "/usr/bin",
			"USER":         "agent",
			"SECRET_TOKEN": "should-not-pass-through",
		},
	}
	got := enf.AppliedEnv(req)
	if got["PATH"] != "/usr/bin" {
		t.Errorf("PATH = %q, want /usr/bin", got["PATH"])
	}
	if got["USER"] != "agent" {
		t.Errorf("USER = %q, want agent", got["USER"])
	}
	if _, leaked := got["SECRET_TOKEN"]; leaked {
		t.Errorf("SECRET_TOKEN leaked through allowlist filter: %v", got)
	}
}

func TestSecurityEnforcer_AppliedEnvEmptyAllowlistReturnsNil(t *testing.T) {
	enf := newTestEnforcer(t, nil) // empty EnvVarAllowlist
	got := enf.AppliedEnv(CommandRequest{
		Env: map[string]string{"PATH": "/usr/bin"},
	})
	if got != nil {
		t.Errorf("AppliedEnv with empty allowlist = %v, want nil", got)
	}
}

func TestSecurityEnforcer_ComputeHMACDeterministic(t *testing.T) {
	enf := newTestEnforcer(t, nil)
	req := CommandRequest{
		MessageID: "m",
		Principal: "admin",
		Command:   "/usr/bin/uptime",
		Args:      []string{"-p"},
		Env:       map[string]string{"A": "1", "B": "2", "C": "3"},
	}
	first := enf.ComputeHMAC(req)
	second := enf.ComputeHMAC(req)
	if first != second {
		t.Errorf("ComputeHMAC not deterministic: %q vs %q", first, second)
	}
	// Hex-decode round-trip to confirm the format.
	if _, err := hex.DecodeString(first); err != nil {
		t.Errorf("ComputeHMAC output not hex: %v", err)
	}
}

func TestSecurityEnforcer_ComputeHMACEnvOrderStable(t *testing.T) {
	enf := newTestEnforcer(t, nil)
	// Two requests differing only in Go's map ordering produce the
	// same HMAC because canonical sorts env keys.
	req1 := CommandRequest{
		MessageID: "m",
		Env:       map[string]string{"A": "1", "B": "2"},
	}
	req2 := CommandRequest{
		MessageID: "m",
		Env:       map[string]string{"B": "2", "A": "1"},
	}
	if enf.ComputeHMAC(req1) != enf.ComputeHMAC(req2) {
		t.Error("ComputeHMAC differed across maps with same content but different declared order")
	}
}

func TestSecurityEnforcer_AuditLogsRejection(t *testing.T) {
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	enf, err := NewSecurityEnforcer(SecurityPolicy{
		HMACSecret:    []byte("test-secret"),
		DefaultPolicy: PolicyDeny,
	}, log)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	req := goodRequest(t)
	if err := enf.Validate(context.Background(), req); !errors.Is(err, ErrCommandDenied) {
		t.Fatalf("Validate: %v", err)
	}

	// One WARN line should mention "command rejected" + reason.
	found := false
	for _, line := range strings.Split(buf.String(), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec["level"] == "WARN" && strings.Contains(rec["msg"].(string), "command rejected") {
			found = true
			if rec["reason"] == nil {
				t.Errorf("WARN line missing reason: %s", line)
			}
		}
	}
	if !found {
		t.Errorf("no WARN audit line for rejection: %s", buf.String())
	}
}

func TestSecurityEnforcer_AuditLogsAcceptance(t *testing.T) {
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	enf, err := NewSecurityEnforcer(SecurityPolicy{
		HMACSecret:    []byte("test-secret"),
		DefaultPolicy: PolicyAllow,
	}, log)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	if err := enf.Validate(context.Background(), goodRequest(t)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.Contains(buf.String(), `"level":"INFO"`) || !strings.Contains(buf.String(), "command allowed") {
		t.Errorf("expected INFO 'command allowed' log line: %s", buf.String())
	}
}
