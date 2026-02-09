package agent

import (
	"testing"
)

func TestDefaultSecurityConfig(t *testing.T) {
	cfg := DefaultSecurityConfig()

	if cfg.Authorization.Enabled {
		t.Error("expected authorization to be disabled by default")
	}

	if cfg.CommandFilter.Mode != "blocklist" {
		t.Errorf("expected blocklist mode, got %s", cfg.CommandFilter.Mode)
	}

	if len(cfg.CommandFilter.BlockedPatterns) == 0 {
		t.Error("expected default blocked patterns")
	}

	if len(cfg.CommandFilter.BlockedEnvVars) == 0 {
		t.Error("expected default blocked env vars")
	}
}

func TestSecurityEnforcer_ValidateCommand(t *testing.T) {
	tests := []struct {
		name       string
		config     *SecurityConfig
		command    string
		args       []string
		env        map[string]string
		workingDir string
		wantErr    bool
	}{
		{
			name:    "simple command allowed",
			config:  DefaultSecurityConfig(),
			command: "echo",
			args:    []string{"hello"},
			wantErr: false,
		},
		{
			name:    "blocked pattern rm -rf /",
			config:  DefaultSecurityConfig(),
			command: "bash",
			args:    []string{"-c", "; rm -rf /"},
			wantErr: true,
		},
		{
			name:    "blocked pattern mkfs",
			config:  DefaultSecurityConfig(),
			command: "mkfs.ext4",
			args:    []string{"/dev/sda1"},
			wantErr: true,
		},
		{
			name:    "blocked env var LD_PRELOAD",
			config:  DefaultSecurityConfig(),
			command: "echo",
			args:    []string{"test"},
			env:     map[string]string{"LD_PRELOAD": "/tmp/evil.so"},
			wantErr: true,
		},
		{
			name:       "relative path with traversal in working dir",
			config:     DefaultSecurityConfig(),
			command:    "ls",
			args:       []string{"-la"},
			workingDir: "../../../etc",
			wantErr:    true,
		},
		{
			name: "argument too long",
			config: &SecurityConfig{
				CommandFilter: CommandFilterConfig{
					Mode:         "blocklist",
					MaxArgLength: 10,
				},
			},
			command: "echo",
			args:    []string{"this is a very long argument that exceeds the limit"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			se, err := NewSecurityEnforcer(tt.config)
			if err != nil {
				t.Fatalf("failed to create enforcer: %v", err)
			}

			err = se.ValidateCommand(tt.command, tt.args, tt.env, tt.workingDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityEnforcer_ExemptCommands(t *testing.T) {
	tests := []struct {
		name           string
		exemptCommands []string
		command        string
		args           []string
		wantErr        bool
	}{
		{
			name:           "mkfs blocked by default",
			exemptCommands: nil,
			command:        "mkfs.ext4",
			args:           []string{"/dev/sda1"},
			wantErr:        true,
		},
		{
			name:           "mkfs allowed when exempt",
			exemptCommands: []string{"mkfs.*"},
			command:        "mkfs.ext4",
			args:           []string{"/dev/sda1"},
			wantErr:        false,
		},
		{
			name:           "mkfs.xfs allowed when exempt",
			exemptCommands: []string{"mkfs.*"},
			command:        "mkfs.xfs",
			args:           []string{"/dev/sdb1"},
			wantErr:        false,
		},
		{
			name:           "full path mkfs exempt",
			exemptCommands: []string{"/sbin/mkfs*", "mkfs*"},
			command:        "/sbin/mkfs.ext4",
			args:           []string{"/dev/sda1"},
			wantErr:        false,
		},
		{
			name:           "dd to device blocked by default",
			exemptCommands: nil,
			command:        "dd",
			args:           []string{"if=/dev/zero", "of=/dev/sda"},
			wantErr:        true,
		},
		{
			name:           "dd to device allowed when exempt",
			exemptCommands: []string{"dd"},
			command:        "dd",
			args:           []string{"if=/dev/zero", "of=/dev/sda"},
			wantErr:        false,
		},
		{
			name:           "other dangerous command still blocked",
			exemptCommands: []string{"mkfs.*"},
			command:        "dd",
			args:           []string{"if=/dev/zero", "of=/dev/sda"},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultSecurityConfig()
			cfg.CommandFilter.ExemptCommands = tt.exemptCommands

			se, err := NewSecurityEnforcer(cfg)
			if err != nil {
				t.Fatalf("failed to create enforcer: %v", err)
			}

			err = se.ValidateCommand(tt.command, tt.args, nil, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityEnforcer_AllowlistMode(t *testing.T) {
	cfg := &SecurityConfig{
		CommandFilter: CommandFilterConfig{
			Mode:      "allowlist",
			Allowlist: []string{"echo", "ls", "cat"},
		},
	}

	se, err := NewSecurityEnforcer(cfg)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	// Allowed commands
	if err := se.ValidateCommand("echo", []string{"hello"}, nil, ""); err != nil {
		t.Errorf("echo should be allowed: %v", err)
	}
	if err := se.ValidateCommand("ls", []string{"-la"}, nil, ""); err != nil {
		t.Errorf("ls should be allowed: %v", err)
	}

	// Not allowed command
	if err := se.ValidateCommand("rm", []string{"-rf", "/"}, nil, ""); err == nil {
		t.Error("rm should not be allowed")
	}
}

func TestSecurityEnforcer_BlocklistMode(t *testing.T) {
	cfg := &SecurityConfig{
		CommandFilter: CommandFilterConfig{
			Mode:      "blocklist",
			Blocklist: []string{"rm", "shutdown", "reboot"},
		},
	}

	se, err := NewSecurityEnforcer(cfg)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	// Blocked commands
	if err := se.ValidateCommand("rm", []string{"-rf", "/"}, nil, ""); err == nil {
		t.Error("rm should be blocked")
	}
	if err := se.ValidateCommand("shutdown", []string{"-h", "now"}, nil, ""); err == nil {
		t.Error("shutdown should be blocked")
	}

	// Not blocked command
	if err := se.ValidateCommand("echo", []string{"hello"}, nil, ""); err != nil {
		t.Errorf("echo should be allowed: %v", err)
	}
}

func TestSecurityEnforcer_AuthorizeCommand(t *testing.T) {
	tests := []struct {
		name      string
		config    *SecurityConfig
		principal string
		command   string
		signature string
		wantErr   bool
	}{
		{
			name: "auth disabled",
			config: &SecurityConfig{
				Authorization: AuthorizationConfig{
					Enabled: false,
				},
			},
			principal: "anyone",
			command:   "anything",
			wantErr:   false,
		},
		{
			name: "allowed principal",
			config: &SecurityConfig{
				Authorization: AuthorizationConfig{
					Enabled:           true,
					AllowedPrincipals: []string{"admin", "operator"},
				},
			},
			principal: "admin",
			command:   "echo",
			wantErr:   false,
		},
		{
			name: "disallowed principal",
			config: &SecurityConfig{
				Authorization: AuthorizationConfig{
					Enabled:           true,
					AllowedPrincipals: []string{"admin"},
				},
			},
			principal: "hacker",
			command:   "echo",
			wantErr:   true,
		},
		{
			name: "wildcard principal",
			config: &SecurityConfig{
				Authorization: AuthorizationConfig{
					Enabled:           true,
					AllowedPrincipals: []string{"*"},
				},
			},
			principal: "anyone",
			command:   "echo",
			wantErr:   false,
		},
		{
			name: "signature required but missing",
			config: &SecurityConfig{
				Authorization: AuthorizationConfig{
					Enabled:          true,
					RequireSignature: true,
					SharedSecret:     "secret",
				},
			},
			principal: "admin",
			command:   "echo",
			signature: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			se, err := NewSecurityEnforcer(tt.config)
			if err != nil {
				t.Fatalf("failed to create enforcer: %v", err)
			}

			err = se.AuthorizeCommand(tt.principal, tt.command, tt.signature)
			if (err != nil) != tt.wantErr {
				t.Errorf("AuthorizeCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityEnforcer_SignCommand(t *testing.T) {
	cfg := &SecurityConfig{
		Authorization: AuthorizationConfig{
			SharedSecret: "my-secret-key",
		},
	}

	se, err := NewSecurityEnforcer(cfg)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	sig := se.SignCommand("echo hello")
	if sig == "" {
		t.Error("expected non-empty signature")
	}

	// Same command should produce same signature
	sig2 := se.SignCommand("echo hello")
	if sig != sig2 {
		t.Error("expected deterministic signatures")
	}

	// Different command should produce different signature
	sig3 := se.SignCommand("echo world")
	if sig == sig3 {
		t.Error("expected different signatures for different commands")
	}
}

func TestSecurityEnforcer_UpdateConfig(t *testing.T) {
	se, err := NewSecurityEnforcer(DefaultSecurityConfig())
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	// Initially mkfs is blocked
	if err := se.ValidateCommand("mkfs.ext4", []string{"/dev/sda1"}, nil, ""); err == nil {
		t.Error("mkfs should be blocked initially")
	}

	// Update config to exempt mkfs
	newCfg := DefaultSecurityConfig()
	newCfg.CommandFilter.ExemptCommands = []string{"mkfs.*"}

	if err := se.UpdateConfig(newCfg); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Now mkfs should be allowed
	if err := se.ValidateCommand("mkfs.ext4", []string{"/dev/sda1"}, nil, ""); err != nil {
		t.Errorf("mkfs should be allowed after update: %v", err)
	}
}
