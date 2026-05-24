// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestDefaultSPIFFERoleResolver_Mapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want Role
	}{
		{"server/control-plane", RoleAdmin},
		{"server/api-gateway", RoleAdmin}, // server/<other> → admin per the v0.1 forward-compat rule
		{"agent/agent-1", RoleOperator},
		{"agent/web-prod-001", RoleOperator},
		{"service/state-runner", RoleOperator},
		{"service/reactor", RoleOperator},
	}
	r := DefaultSPIFFERoleResolver(nil)
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got, err := r(c.path)
			if err != nil {
				t.Fatalf("%q: err = %v", c.path, err)
			}
			if got != c.want {
				t.Errorf("%q → %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestDefaultSPIFFERoleResolver_UnknownPathDefaultsReadonly(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r := DefaultSPIFFERoleResolver(logger)
	got, err := r("unrecognized/thing")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != RoleReadonly {
		t.Errorf("got = %v, want RoleReadonly", got)
	}
	if !strings.Contains(buf.String(), "unrecognized path") {
		t.Errorf("missing WARN log; got: %s", buf.String())
	}
}

func TestDefaultSPIFFERoleResolver_EmptyPathErrors(t *testing.T) {
	t.Parallel()
	r := DefaultSPIFFERoleResolver(nil)
	got, err := r("")
	if err == nil {
		t.Fatal("empty path should error")
	}
	if got != RoleNone {
		t.Errorf("err Role = %v, want RoleNone", got)
	}
}

func TestDefaultSPIFFERoleResolver_AgentPathMissingID(t *testing.T) {
	t.Parallel()
	r := DefaultSPIFFERoleResolver(nil)
	if _, err := r("agent/"); err == nil {
		t.Error("agent/ (no id) should error")
	}
}

func TestDefaultSPIFFERoleResolver_ServicePathMissingID(t *testing.T) {
	t.Parallel()
	r := DefaultSPIFFERoleResolver(nil)
	if _, err := r("service/"); err == nil {
		t.Error("service/ (no name) should error")
	}
}

func TestDefaultSPIFFERoleResolver_NilLoggerSafe(t *testing.T) {
	t.Parallel()
	// nil logger must not panic when the unknown-path branch
	// fires.
	r := DefaultSPIFFERoleResolver(nil)
	if _, err := r("random/thing"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
