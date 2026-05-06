package state

import "testing"

// Pure unit tests for postgres.go helpers — no live DB needed.
// Live integration tests live in postgres_integration_test.go (gated
// by the `integration` build tag and KSCORE_TEST_POSTGRES_DSN).

func TestBuildPostgresDSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  PostgreSQLConfig
		want string
	}{
		{
			name: "DSN takes precedence",
			cfg:  PostgreSQLConfig{DSN: "postgres://x", Host: "ignored"},
			want: "postgres://x",
		},
		{
			name: "struct fields with defaults",
			cfg:  PostgreSQLConfig{Host: "h", Database: "d", User: "u", Password: "p"},
			want: "host=h port=5432 user=u password=p dbname=d sslmode=require",
		},
		{
			name: "explicit port + sslmode",
			cfg: PostgreSQLConfig{
				Host: "h", Port: 6543, Database: "d", User: "u", Password: "p",
				SSLMode: "disable",
			},
			want: "host=h port=6543 user=u password=p dbname=d sslmode=disable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPostgresDSN(&tt.cfg)
			if got != tt.want {
				t.Errorf("buildPostgresDSN = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnmarshalJSONBytes(t *testing.T) {
	t.Run("nil slice is no-op", func(t *testing.T) {
		var v map[string]string
		if err := unmarshalJSONBytes(nil, &v); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if v != nil {
			t.Errorf("v should be nil, got %v", v)
		}
	})
	t.Run("valid json", func(t *testing.T) {
		var v map[string]string
		if err := unmarshalJSONBytes([]byte(`{"a":"b"}`), &v); err != nil {
			t.Fatal(err)
		}
		if v["a"] != "b" {
			t.Errorf("v = %v", v)
		}
	})
	t.Run("malformed json surfaces", func(t *testing.T) {
		var v map[string]string
		err := unmarshalJSONBytes([]byte(`not-json`), &v)
		if err == nil {
			t.Fatal("expected error for malformed json")
		}
	})
}

func TestPlaceholderGen(t *testing.T) {
	p := newPlaceholderGen()
	if got := p.next(); got != "$1" {
		t.Errorf("first = %q, want $1", got)
	}
	if got := p.next(); got != "$2" {
		t.Errorf("second = %q, want $2", got)
	}
	if got := p.next(); got != "$3" {
		t.Errorf("third = %q, want $3", got)
	}
}
