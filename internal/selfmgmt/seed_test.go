// SPDX-License-Identifier: Apache-2.0

package selfmgmt

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSeed_HappyPaths(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		want    SeedConfig
		wantBPs int
	}{
		{
			name: "dev good",
			file: "testdata/seed-dev-good.yaml",
			want: SeedConfig{
				Mode:        SeedModeDevelopment,
				ClusterName: "dev-cluster",
				NodeRole:    SeedNodeRoleSeed,
				Storage:     SeedStorage{Driver: SeedStorageSQLite, DSN: "./data/keystone.db"},
				NATS:        SeedNATS{Mode: SeedNATSEmbedded, Endpoints: []string{}},
				TLSStrategy: SeedTLSSelfSigned,
			},
			wantBPs: 2,
		},
		{
			name: "prod good",
			file: "testdata/seed-prod-good.yaml",
			want: SeedConfig{
				Mode:        SeedModeProduction,
				ClusterName: "prod-east",
				NodeRole:    SeedNodeRoleSeed,
				Storage:     SeedStorage{Driver: SeedStoragePostgres},
				NATS:        SeedNATS{Mode: SeedNATSExternal},
				TLSStrategy: SeedTLSCSR,
			},
			wantBPs: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LoadSeed(tc.file)
			if err != nil {
				t.Fatalf("LoadSeed(%s): %v", tc.file, err)
			}
			if got.Mode != tc.want.Mode {
				t.Errorf("Mode = %q, want %q", got.Mode, tc.want.Mode)
			}
			if got.ClusterName != tc.want.ClusterName {
				t.Errorf("ClusterName = %q, want %q", got.ClusterName, tc.want.ClusterName)
			}
			if got.NodeRole != tc.want.NodeRole {
				t.Errorf("NodeRole = %q, want %q", got.NodeRole, tc.want.NodeRole)
			}
			if got.Storage.Driver != tc.want.Storage.Driver {
				t.Errorf("Storage.Driver = %q, want %q", got.Storage.Driver, tc.want.Storage.Driver)
			}
			if got.NATS.Mode != tc.want.NATS.Mode {
				t.Errorf("NATS.Mode = %q, want %q", got.NATS.Mode, tc.want.NATS.Mode)
			}
			if got.TLSStrategy != tc.want.TLSStrategy {
				t.Errorf("TLSStrategy = %q, want %q", got.TLSStrategy, tc.want.TLSStrategy)
			}
			if len(got.Blueprints) != tc.wantBPs {
				t.Errorf("len(Blueprints) = %d, want %d", len(got.Blueprints), tc.wantBPs)
			}
		})
	}
}

func TestLoadSeed_FileErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := LoadSeed(filepath.Join(t.TempDir(), "nope.yaml"))
		if !errors.Is(err, ErrSeedNotFound) {
			t.Fatalf("err = %v, want ErrSeedNotFound", err)
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")
		if err := os.WriteFile(path, []byte("mode: [unterminated"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadSeed(path)
		if err == nil || errors.Is(err, ErrSeedNotFound) {
			t.Fatalf("expected decode error, got %v", err)
		}
		if !strings.Contains(err.Error(), "decode seed") {
			t.Errorf("err = %v, want decode error", err)
		}
	})

	t.Run("unknown field rejected by strict decode", func(t *testing.T) {
		_, err := LoadSeed("testdata/seed-unknown-field.yaml")
		if err == nil {
			t.Fatal("expected strict-decode error, got nil")
		}
		if !strings.Contains(err.Error(), "unexpected_field") &&
			!strings.Contains(err.Error(), "field unexpected_field") {
			t.Errorf("err = %v, want mention of unknown field", err)
		}
	})

	t.Run("prod + tls disabled rejected", func(t *testing.T) {
		_, err := LoadSeed("testdata/seed-prod-tls-disabled.yaml")
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), "tls_strategy") {
			t.Errorf("err = %v, want tls_strategy error", err)
		}
	})

	t.Run("missing required cluster_name", func(t *testing.T) {
		_, err := LoadSeed("testdata/seed-missing-required.yaml")
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), "cluster_name") {
			t.Errorf("err = %v, want cluster_name error", err)
		}
	})

	t.Run("bad enum mode", func(t *testing.T) {
		_, err := LoadSeed("testdata/seed-bad-enum.yaml")
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), "mode") {
			t.Errorf("err = %v, want mode error", err)
		}
	})
}

// TestSeedConfig_Validate exhaustively covers every branch of
// Validate without going through the YAML loader. The parser tests
// above prove the strict-decode + happy-path read; this test isolates
// the validator so a parser regression doesn't mask a validator bug.
func TestSeedConfig_Validate(t *testing.T) {
	base := func() SeedConfig {
		return SeedConfig{
			Mode:        SeedModeDevelopment,
			ClusterName: "dev-cluster",
			NodeRole:    SeedNodeRoleSeed,
			Storage:     SeedStorage{Driver: SeedStorageSQLite, DSN: "./data/keystone.db"},
			NATS:        SeedNATS{Mode: SeedNATSEmbedded},
			TLSStrategy: SeedTLSSelfSigned,
		}
	}

	cases := []struct {
		name    string
		mutate  func(*SeedConfig)
		wantSub string // empty => expect no error
	}{
		{name: "baseline valid", mutate: func(*SeedConfig) {}},
		{name: "mode empty", mutate: func(s *SeedConfig) { s.Mode = "" }, wantSub: "mode: must be set"},
		{name: "mode invalid", mutate: func(s *SeedConfig) { s.Mode = "staging" }, wantSub: "mode: \"staging\""},
		{name: "cluster_name empty", mutate: func(s *SeedConfig) { s.ClusterName = "" }, wantSub: "cluster_name: must not be empty"},
		{name: "cluster_name uppercase", mutate: func(s *SeedConfig) { s.ClusterName = "Dev-Cluster" }, wantSub: "cluster_name:"},
		{name: "cluster_name leading hyphen", mutate: func(s *SeedConfig) { s.ClusterName = "-dev" }, wantSub: "cluster_name:"},
		{name: "cluster_name too long", mutate: func(s *SeedConfig) { s.ClusterName = strings.Repeat("a", 64) }, wantSub: "cluster_name:"},
		{name: "cluster_name max len ok", mutate: func(s *SeedConfig) { s.ClusterName = strings.Repeat("a", 63) }},
		{name: "node_role empty", mutate: func(s *SeedConfig) { s.NodeRole = "" }, wantSub: "node_role: must be set"},
		{name: "node_role invalid", mutate: func(s *SeedConfig) { s.NodeRole = "leader" }, wantSub: "node_role: \"leader\""},
		{name: "node_role join ok", mutate: func(s *SeedConfig) { s.NodeRole = SeedNodeRoleJoin }},
		{name: "storage driver empty", mutate: func(s *SeedConfig) { s.Storage.Driver = "" }, wantSub: "storage: driver: must be set"},
		{name: "storage driver invalid", mutate: func(s *SeedConfig) { s.Storage.Driver = "mysql" }, wantSub: "storage: driver: \"mysql\""},
		{name: "storage dsn empty", mutate: func(s *SeedConfig) { s.Storage.DSN = "" }, wantSub: "storage: dsn: must not be empty"},
		{name: "storage postgres ok", mutate: func(s *SeedConfig) {
			s.Storage.Driver = SeedStoragePostgres
			s.Storage.DSN = "postgres://u@h/db"
		}},
		{name: "nats mode empty", mutate: func(s *SeedConfig) { s.NATS.Mode = "" }, wantSub: "nats: mode: must be set"},
		{name: "nats mode invalid", mutate: func(s *SeedConfig) { s.NATS.Mode = "remote" }, wantSub: "nats: mode: \"remote\""},
		{name: "nats embedded with endpoints", mutate: func(s *SeedConfig) {
			s.NATS.Endpoints = []string{"nats://x:4222"}
		}, wantSub: "endpoints: must be empty when mode is embedded"},
		{name: "nats external without endpoints", mutate: func(s *SeedConfig) {
			s.NATS.Mode = SeedNATSExternal
		}, wantSub: "endpoints: must list at least one URL"},
		{name: "nats external empty endpoint string", mutate: func(s *SeedConfig) {
			s.NATS.Mode = SeedNATSExternal
			s.NATS.Endpoints = []string{""}
		}, wantSub: "endpoints[0]: must not be empty"},
		{name: "nats external ok", mutate: func(s *SeedConfig) {
			s.NATS.Mode = SeedNATSExternal
			s.NATS.Endpoints = []string{"nats://a:4222", "nats://b:4222"}
		}},
		{name: "tls_strategy empty", mutate: func(s *SeedConfig) { s.TLSStrategy = "" }, wantSub: "tls_strategy: must be set"},
		{name: "tls_strategy invalid", mutate: func(s *SeedConfig) { s.TLSStrategy = "acme" }, wantSub: "tls_strategy: \"acme\""},
		{name: "tls_strategy disabled in dev ok", mutate: func(s *SeedConfig) { s.TLSStrategy = SeedTLSDisabled }},
		{name: "tls_strategy disabled in prod rejected", mutate: func(s *SeedConfig) {
			s.Mode = SeedModeProduction
			s.TLSStrategy = SeedTLSDisabled
		}, wantSub: "tls_strategy: \"disabled\" is not permitted in production"},
		{name: "tls_strategy csr ok", mutate: func(s *SeedConfig) { s.TLSStrategy = SeedTLSCSR }},
		{name: "blueprint entry empty", mutate: func(s *SeedConfig) {
			s.Blueprints = []string{"base-system", "", "hardening"}
		}, wantSub: "blueprints[1]: must not be empty"},
		{name: "blueprints nil ok", mutate: func(s *SeedConfig) { s.Blueprints = nil }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base()
			tc.mutate(&s)
			err := s.Validate()
			if tc.wantSub == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("Validate() err = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}
