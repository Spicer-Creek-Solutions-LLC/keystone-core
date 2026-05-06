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
			name: "hostname not bracketed",
			cfg:  PostgreSQLConfig{Host: "db.example.com", Database: "d", User: "u", Password: "p"},
			want: "postgres://u:p@db.example.com:5432/d?sslmode=require",
		},
		{
			name: "IPv4 literal not bracketed",
			cfg:  PostgreSQLConfig{Host: "10.0.0.1", Database: "d", User: "u", Password: "p"},
			want: "postgres://u:p@10.0.0.1:5432/d?sslmode=require",
		},
		{
			name: "IPv6 loopback bracketed",
			cfg:  PostgreSQLConfig{Host: "::1", Database: "d", User: "u", Password: "p"},
			want: "postgres://u:p@[::1]:5432/d?sslmode=require",
		},
		{
			name: "IPv6 full address bracketed",
			cfg:  PostgreSQLConfig{Host: "2001:db8::1", Database: "d", User: "u", Password: "p"},
			want: "postgres://u:p@[2001:db8::1]:5432/d?sslmode=require",
		},
		{
			name: "IPv6 link-local bracketed",
			cfg:  PostgreSQLConfig{Host: "fe80::1", Database: "d", User: "u", Password: "p"},
			want: "postgres://u:p@[fe80::1]:5432/d?sslmode=require",
		},
		{
			name: "explicit port + sslmode",
			cfg: PostgreSQLConfig{
				Host: "h", Port: 6543, Database: "d", User: "u", Password: "p",
				SSLMode: "disable",
			},
			want: "postgres://u:p@h:6543/d?sslmode=disable",
		},
		{
			name: "special chars in password are URL-encoded",
			cfg: PostgreSQLConfig{
				Host: "h", Database: "d", User: "u", Password: "p@ss/w%rd",
			},
			want: "postgres://u:p%40ss%2Fw%25rd@h:5432/d?sslmode=require",
		},
		{
			name: "special chars in user are URL-encoded",
			cfg: PostgreSQLConfig{
				Host: "h", Database: "d", User: "user@host", Password: "p",
			},
			want: "postgres://user%40host:p@h:5432/d?sslmode=require",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPostgresDSN(&tt.cfg)
			if got != tt.want {
				t.Errorf("buildPostgresDSN =\n  got:  %q\n  want: %q", got, tt.want)
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
