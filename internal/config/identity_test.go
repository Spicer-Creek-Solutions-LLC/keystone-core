// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
)

func TestIdentityConfig_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		key     string
		wantErr string
	}{
		{"empty is plaintext default", "", ""},
		{"env scheme", "env:KSCORE_CA_MASTER", ""},
		{"file scheme", "file:/etc/kscore/ca.key", ""},
		{"inline scheme", "inline:deadbeef", ""},
		{"no scheme", "deadbeef", "scheme-prefixed"},
		{"unknown scheme", "vault:secret/ca", "scheme-prefixed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := IdentityConfig{EncryptionKey: tc.key}.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate(%q) = %v, want nil", tc.key, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate(%q) = %v, want substring %q", tc.key, err, tc.wantErr)
			}
		})
	}
}
