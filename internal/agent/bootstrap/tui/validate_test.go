package tui

import (
	"strings"
	"testing"
)

func TestValidateClusterName(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"happy", "default", ""},
		{"trim ok", "  prod  ", ""},
		{"empty", "", "required"},
		{"whitespace only", "   ", "required"},
		{"way too long", strings.Repeat("a", maxIdentifierLen+1), "keep it under"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateClusterName(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("err=%v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err=%v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateAgentID(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"happy", "agent-1", ""},
		{"hostname-style", "kscore-agent-east-01.local", ""},
		{"empty", "", "required"},
		{"way too long", strings.Repeat("x", maxIdentifierLen+1), "keep it under"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAgentID(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("err=%v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err=%v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateJoinURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"happy nats", "nats://server:4222", ""},
		{"happy tls", "tls://server:4443", ""},
		{"happy ipv4", "nats://10.0.0.1:4222", ""},
		{"happy ipv6 bracketed", "nats://[::1]:4222", ""},
		{"empty", "", "required"},
		{"missing scheme", "server:4222", "scheme must be"},
		{"http scheme", "http://server:4222", "scheme must be"},
		{"missing host", "nats://", "missing host"},
		{"unbracketed ipv6 typo", "nats://::1:4222", "bracketed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateJoinURL(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("err=%v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err=%v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateConfigPath(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"happy abs", "/etc/keystone-core/keystone-core-agent.yaml", ""},
		{"happy abs nested", "/var/keystone/agent.yaml", ""},
		{"empty", "", "required"},
		{"relative", "etc/agent.yaml", "must be absolute"},
		{"dot relative", "./agent.yaml", "must be absolute"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfigPath(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("err=%v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err=%v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
