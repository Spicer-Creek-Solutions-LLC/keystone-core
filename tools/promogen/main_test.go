// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestRun_Dispatch(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errFrag string
	}{
		{"no subcommand", nil, true, "expected a subcommand"},
		{"unknown subcommand", []string{"montage"}, true, "unknown subcommand"},
		{"bad flag", []string{"validate", "--nope"}, true, "nope"},
		{"validate repo", []string{"validate", "-repo-root", "../.."}, false, ""},
		{"plan repo", []string{"plan", "-repo-root", "../.."}, false, ""},
		{"facts repo", []string{"facts", "-repo-root", "../.."}, false, ""},
		{"sync repo check", []string{"sync", "-repo-root", "../..", "-check"}, false, ""},
		{"reconcile repo", []string{"reconcile", "-repo-root", "../.."}, false, ""},
		{"missing promo dir", []string{"validate", "-promo-dir", "nowhere"}, true, "read manifest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("run(%v) = nil error, want one containing %q", tt.args, tt.errFrag)
				}
				if !strings.Contains(err.Error(), tt.errFrag) {
					t.Errorf("run(%v) error = %q, want it to contain %q", tt.args, err, tt.errFrag)
				}
				return
			}
			if err != nil {
				t.Errorf("run(%v) = %v, want nil", tt.args, err)
			}
		})
	}
}
