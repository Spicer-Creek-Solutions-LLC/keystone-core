// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"reflect"
	"testing"
	"time"
)

func TestSecret_IsDynamic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Secret
		want bool
	}{
		{
			name: "static secret has no lease id",
			in:   Secret{Path: "kv/app/db"},
			want: false,
		},
		{
			name: "dynamic secret carries lease id",
			in:   Secret{Path: "database/creds/app", LeaseID: "lease-123"},
			want: true,
		},
		{
			name: "lease id alone is the signal even with zero duration",
			in:   Secret{LeaseID: "lease-abc", LeaseDuration: 0},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.in.IsDynamic(); got != tc.want {
				t.Fatalf("IsDynamic() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSecret_MaskForLog(t *testing.T) {
	t.Parallel()

	t.Run("masks every leaf and preserves structure", func(t *testing.T) {
		t.Parallel()

		orig := Secret{
			Path: "kv/app/db",
			Data: map[string]any{
				"username": "alice",
				"password": "hunter2",
				"meta": map[string]any{
					"region": "us-east-1",
					"keys":   []any{"k1", "k2"},
				},
			},
			Metadata: map[string]string{"owner": "platform-team"},
		}

		masked := orig.MaskForLog()

		// Top-level structure preserved.
		if masked.Path != orig.Path {
			t.Errorf("Path mutated: %q -> %q", orig.Path, masked.Path)
		}
		if len(masked.Data) != len(orig.Data) {
			t.Fatalf("Data length mismatch: got %d want %d", len(masked.Data), len(orig.Data))
		}

		// Top-level leaves masked.
		if masked.Data["username"] != MaskedValue {
			t.Errorf("username not masked: %v", masked.Data["username"])
		}
		if masked.Data["password"] != MaskedValue {
			t.Errorf("password not masked: %v", masked.Data["password"])
		}

		// Nested map masked recursively.
		nested, ok := masked.Data["meta"].(map[string]any)
		if !ok {
			t.Fatalf("nested map was flattened: %T", masked.Data["meta"])
		}
		if nested["region"] != MaskedValue {
			t.Errorf("nested.region not masked: %v", nested["region"])
		}

		// Nested slice masked element-wise.
		keys, ok := nested["keys"].([]any)
		if !ok {
			t.Fatalf("nested slice changed type: %T", nested["keys"])
		}
		for i, v := range keys {
			if v != MaskedValue {
				t.Errorf("keys[%d] not masked: %v", i, v)
			}
		}

		// Metadata preserved verbatim (operator-visible by contract).
		if masked.Metadata["owner"] != "platform-team" {
			t.Errorf("metadata mutated: %v", masked.Metadata)
		}

		// Original untouched.
		if orig.Data["password"] != "hunter2" {
			t.Errorf("original Data mutated: %v", orig.Data)
		}
	})

	t.Run("nil Data round-trips to nil", func(t *testing.T) {
		t.Parallel()
		masked := Secret{Path: "kv/empty"}.MaskForLog()
		if masked.Data != nil {
			t.Errorf("nil Data masked to non-nil: %#v", masked.Data)
		}
	})
}

func TestSecret_Clone(t *testing.T) {
	t.Parallel()

	orig := Secret{
		Path:      "kv/app/db",
		Version:   3,
		LeaseID:   "lease-1",
		Renewable: true,
		CreatedAt: time.Unix(1700000000, 0).UTC(),
		Data: map[string]any{
			"password": "hunter2",
			"nested": map[string]any{
				"k": "v",
			},
			"list": []any{"a", "b"},
		},
		Metadata: map[string]string{"owner": "platform"},
	}

	clone := orig.Clone()

	// Equal at first.
	if !reflect.DeepEqual(orig, clone) {
		t.Fatalf("clone differs from original\norig:  %#v\nclone: %#v", orig, clone)
	}

	// Mutating clone must not touch original — top-level map.
	clone.Data["password"] = "new"
	if orig.Data["password"] != "hunter2" {
		t.Errorf("mutating clone.Data leaked into original: %v", orig.Data["password"])
	}

	// Mutating clone must not touch original — nested map.
	clone.Data["nested"].(map[string]any)["k"] = "changed"
	if orig.Data["nested"].(map[string]any)["k"] != "v" {
		t.Errorf("mutating clone nested map leaked into original: %v", orig.Data["nested"])
	}

	// Mutating clone must not touch original — nested slice.
	clone.Data["list"].([]any)[0] = "z"
	if orig.Data["list"].([]any)[0] != "a" {
		t.Errorf("mutating clone slice leaked into original: %v", orig.Data["list"])
	}

	// Metadata isolated too.
	clone.Metadata["owner"] = "other"
	if orig.Metadata["owner"] != "platform" {
		t.Errorf("mutating clone.Metadata leaked into original: %v", orig.Metadata)
	}
}

func TestSecret_Clone_NilMaps(t *testing.T) {
	t.Parallel()
	orig := Secret{Path: "kv/empty"}
	clone := orig.Clone()
	if clone.Data != nil {
		t.Errorf("nil Data cloned to non-nil: %#v", clone.Data)
	}
	if clone.Metadata != nil {
		t.Errorf("nil Metadata cloned to non-nil: %#v", clone.Metadata)
	}
}
