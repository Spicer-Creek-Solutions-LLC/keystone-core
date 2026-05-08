package config

import (
	"strings"
	"testing"
)

func TestSecurityConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SecurityConfig
		wantErr string
	}{
		{"defaults ok", SecurityConfig{}, ""},
		{"empty secret ok (escape hatch)", SecurityConfig{HMACSecret: ""}, ""},
		{"hex secret ok", SecurityConfig{HMACSecret: "deadbeefcafe"}, ""},
		{"non-hex secret rejected", SecurityConfig{HMACSecret: "not-hex-zzz"}, "not hex"},
		{"empty default policy ok", SecurityConfig{DefaultPolicy: ""}, ""},
		{"allow ok", SecurityConfig{DefaultPolicy: "allow"}, ""},
		{"deny ok", SecurityConfig{DefaultPolicy: "deny"}, ""},
		{"weird default rejected", SecurityConfig{DefaultPolicy: "perhaps"}, "defaultpolicy"},
		{"negative maxargs rejected", SecurityConfig{MaxArgsBytes: -1}, "maxargsbytes"},
		{"zero maxargs ok", SecurityConfig{MaxArgsBytes: 0}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityConfig_DecodedHMACSecret(t *testing.T) {
	cfg := SecurityConfig{HMACSecret: "deadbeef"}
	got := cfg.DecodedHMACSecret()
	want := []byte{0xde, 0xad, 0xbe, 0xef}
	if string(got) != string(want) {
		t.Errorf("DecodedHMACSecret = %x, want %x", got, want)
	}
}

func TestSecurityConfig_DecodedHMACSecret_Empty(t *testing.T) {
	cfg := SecurityConfig{HMACSecret: ""}
	if got := cfg.DecodedHMACSecret(); got != nil {
		t.Errorf("DecodedHMACSecret = %v, want nil for empty secret", got)
	}
}

func TestSecurityConfig_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Security.MaxArgsBytes != 64*1024 {
		t.Errorf("MaxArgsBytes = %d, want 64 KiB", cfg.Security.MaxArgsBytes)
	}
	if cfg.Security.DefaultPolicy != "deny" {
		t.Errorf("DefaultPolicy = %q, want deny", cfg.Security.DefaultPolicy)
	}
}
