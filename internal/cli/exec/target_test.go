// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"errors"
	"testing"
)

func TestParseTarget_Empty(t *testing.T) {
	t.Parallel()
	got, err := ParseTarget("")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestParseTarget_SingleLabel(t *testing.T) {
	t.Parallel()
	got, err := ParseTarget("role:web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Labels["role"] != "web" {
		t.Errorf("labels = %v", got.Labels)
	}
	if len(got.AgentIds) != 0 || got.HostnamePattern != "" {
		t.Errorf("expected labels-only: %+v", got)
	}
}

func TestParseTarget_AndOfLabels(t *testing.T) {
	t.Parallel()
	got, err := ParseTarget("role:web AND env:prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.Labels["role"] != "web" || got.Labels["env"] != "prod" {
		t.Errorf("labels = %v, want {role:web, env:prod}", got.Labels)
	}
}

func TestParseTarget_IDExact(t *testing.T) {
	t.Parallel()
	got, err := ParseTarget("id:web-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AgentIds) != 1 || got.AgentIds[0] != "web-01" {
		t.Errorf("AgentIds = %v", got.AgentIds)
	}
}

func TestParseTarget_IDCommaSeparated(t *testing.T) {
	t.Parallel()
	got, err := ParseTarget("id:web-01,web-02,db-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AgentIds) != 3 {
		t.Errorf("AgentIds = %v", got.AgentIds)
	}
}

func TestParseTarget_HostnameGlob(t *testing.T) {
	t.Parallel()
	got, err := ParseTarget("hostname:web-prod-*")
	if err != nil {
		t.Fatal(err)
	}
	if got.HostnamePattern != "web-prod-*" {
		t.Errorf("HostnamePattern = %q", got.HostnamePattern)
	}
}

func TestParseTarget_CombinedDimensions(t *testing.T) {
	t.Parallel()
	got, err := ParseTarget("role:web AND hostname:web-prod-* AND env:prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.HostnamePattern != "web-prod-*" {
		t.Errorf("HostnamePattern = %q", got.HostnamePattern)
	}
	if got.Labels["role"] != "web" || got.Labels["env"] != "prod" {
		t.Errorf("labels = %v", got.Labels)
	}
}

func TestParseTarget_LabelsPrefix(t *testing.T) {
	t.Parallel()
	got, err := ParseTarget("labels.env:prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.Labels["env"] != "prod" {
		t.Errorf("labels = %v", got.Labels)
	}
}

func TestParseTarget_Unsupported(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
	}{
		{"OR", "role:web OR role:cache"},
		{"NOT", "NOT role:legacy"},
		{"parens", "(role:web OR role:cache)"},
		{"id glob", "id:web-*"},
		{"os field", "os:linux"},
		{"arch field", "arch:amd64"},
		{"status field", "status:online"},
		{"ip field", "ip:10.0.0.0/8"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseTarget(tc.in)
			if !errors.Is(err, ErrTargetUnsupported) {
				t.Errorf("err = %v, want ErrTargetUnsupported", err)
			}
		})
	}
}

func TestParseTarget_DuplicateLabelKey(t *testing.T) {
	t.Parallel()
	_, err := ParseTarget("role:web AND role:cache")
	if err == nil {
		t.Error("duplicate label key should error")
	}
}

func TestParseTarget_DuplicateHostname(t *testing.T) {
	t.Parallel()
	_, err := ParseTarget("hostname:web-* AND hostname:db-*")
	if err == nil {
		t.Error("duplicate hostname clause should error")
	}
}

func TestParseTarget_MalformedClause(t *testing.T) {
	t.Parallel()
	cases := []string{
		"role",  // missing ':'
		"role:", // empty value
		":web",  // empty field
		"role:web AND",
	}
	for _, in := range cases {
		_, err := ParseTarget(in)
		if err == nil {
			t.Errorf("ParseTarget(%q) = nil, want error", in)
		}
	}
}
