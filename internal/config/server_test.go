// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
)

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServerConfig
		wantErr string
	}{
		{"ok", ServerConfig{Host: "0.0.0.0", GRPCPort: 9090, HTTPPort: 8080}, ""},
		{"empty host", ServerConfig{Host: "", GRPCPort: 9090, HTTPPort: 8080}, "host"},
		{"grpc port too low", ServerConfig{Host: "x", GRPCPort: 0, HTTPPort: 8080}, "grpcport"},
		{"grpc port too high", ServerConfig{Host: "x", GRPCPort: 99999, HTTPPort: 8080}, "grpcport"},
		{"http port too low", ServerConfig{Host: "x", GRPCPort: 9090, HTTPPort: 0}, "httpport"},
		{"http port too high", ServerConfig{Host: "x", GRPCPort: 9090, HTTPPort: 99999}, "httpport"},
		{"tls enabled but no cert", ServerConfig{
			Host: "x", GRPCPort: 9090, HTTPPort: 8080,
			TLS: TLSConfig{Enabled: true, KeyFile: "/k"},
		}, "certfile"},
		{"cors enabled but no origins", ServerConfig{
			Host: "x", GRPCPort: 9090, HTTPPort: 8080,
			CORS: CORSConfig{Enabled: true},
		}, "allowedorigins"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate = nil, want error containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate = %v, want error containing %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestCORSConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     CORSConfig
		wantErr string
	}{
		{"disabled", CORSConfig{Enabled: false}, ""},
		{"disabled with values is ok", CORSConfig{Enabled: false, AllowedOrigins: []string{"*"}}, ""},
		{"enabled with origins", CORSConfig{Enabled: true, AllowedOrigins: []string{"https://x"}}, ""},
		{"enabled missing origins", CORSConfig{Enabled: true}, "allowedorigins"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate = nil, want error containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate = %v, want error containing %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestTLSConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     TLSConfig
		wantErr string
	}{
		{"disabled", TLSConfig{Enabled: false}, ""},
		{"disabled with paths is ok", TLSConfig{Enabled: false, CertFile: "/x", KeyFile: "/y"}, ""},
		{"enabled with both", TLSConfig{Enabled: true, CertFile: "/c", KeyFile: "/k"}, ""},
		{"enabled missing cert", TLSConfig{Enabled: true, KeyFile: "/k"}, "certfile"},
		{"enabled missing key", TLSConfig{Enabled: true, CertFile: "/c"}, "keyfile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate = nil, want error containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate = %v, want error containing %q", err, tt.wantErr)
				}
			}
		})
	}
}
