// Package selfmgmt holds the bootstrap-from-seed and ongoing
// self-management surfaces consumed by the kscore-bootstrap binary.
//
// This file ships only the seed-document contract: types, a strict
// YAML loader, and a validator. The bootstrap state machine that
// consumes a validated SeedConfig (detect → configure → validate →
// install → blueprints → verify) lands in Epic 18 task 2; the
// kscore-bootstrap CLI in task 7. SeedConfig deliberately stays
// separate from internal/config.Config — the seed is a one-shot
// document driving cluster formation, whereas Config is the
// long-running runtime configuration of an already-formed node.
package selfmgmt

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"

	yaml "go.yaml.in/yaml/v3"
)

// SeedMode is the deployment mode the bootstrapped cluster will run in.
// Matches internal/config.Mode values verbatim so that operators only
// learn one vocabulary.
type SeedMode string

const (
	SeedModeDevelopment SeedMode = "development"
	SeedModeProduction  SeedMode = "production"
)

// SeedNodeRole tells the bootstrap state machine whether this node
// forms a new cluster or joins an existing one. Join-time discovery
// (endpoints, join token) is plumbed in task 2; the seed contract only
// records the operator's intent here.
type SeedNodeRole string

const (
	SeedNodeRoleSeed SeedNodeRole = "seed"
	SeedNodeRoleJoin SeedNodeRole = "join"
)

// SeedStorageDriver is the backend the bootstrapped server uses for
// its state store. Mirrors the runtime config.StorageConfig.Driver
// set so a seed YAML and the eventual runtime config line up.
type SeedStorageDriver string

const (
	SeedStorageSQLite   SeedStorageDriver = "sqlite"
	SeedStoragePostgres SeedStorageDriver = "postgres"
)

// SeedNATSMode mirrors the runtime config.NATSMode set.
type SeedNATSMode string

const (
	SeedNATSEmbedded SeedNATSMode = "embedded"
	SeedNATSExternal SeedNATSMode = "external"
)

// SeedTLSStrategy is how bootstrap obtains the server certificate.
// "self-signed" generates a local CA + server cert (demo/dev);
// "csr" emits a CSR for an external CA to sign (production path);
// "disabled" runs without TLS (rejected in production mode).
type SeedTLSStrategy string

const (
	SeedTLSSelfSigned SeedTLSStrategy = "self-signed"
	SeedTLSCSR        SeedTLSStrategy = "csr"
	SeedTLSDisabled   SeedTLSStrategy = "disabled"
)

// SeedConfig is the parsed seed document.
//
//	mode: development
//	cluster_name: dev-cluster
//	node_role: seed
//	storage:
//	  driver: sqlite
//	  dsn: ./data/keystone.db
//	nats:
//	  mode: embedded
//	  endpoints: []
//	tls_strategy: self-signed
//	blueprints:
//	  - base-system
//	  - hardening
type SeedConfig struct {
	Mode        SeedMode        `yaml:"mode"`
	ClusterName string          `yaml:"cluster_name"`
	NodeRole    SeedNodeRole    `yaml:"node_role"`
	Storage     SeedStorage     `yaml:"storage"`
	NATS        SeedNATS        `yaml:"nats"`
	TLSStrategy SeedTLSStrategy `yaml:"tls_strategy"`
	Blueprints  []string        `yaml:"blueprints"`
}

// SeedStorage is the storage block of the seed doc.
type SeedStorage struct {
	Driver SeedStorageDriver `yaml:"driver"`
	DSN    string            `yaml:"dsn"`
}

// SeedNATS is the NATS block of the seed doc. Endpoints is required
// for external mode and ignored (must be empty) for embedded mode.
type SeedNATS struct {
	Mode      SeedNATSMode `yaml:"mode"`
	Endpoints []string     `yaml:"endpoints"`
}

// ErrSeedNotFound is returned by LoadSeed when the path does not
// resolve to a regular file.
var ErrSeedNotFound = errors.New("selfmgmt: seed file not found")

// clusterNameRe matches the lower-case DNS-label shape we require for
// cluster_name: 1-63 chars, alphanumeric + hyphen, must start with an
// alphanumeric. Stricter than runtime config.NATSConfig.ClusterName
// (which is non-empty only) because the seed's cluster name flows
// into identifiers downstream of bootstrap.
var clusterNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// LoadSeed reads, strictly decodes, and validates the seed YAML at
// path. Strict decoding (KnownFields=true) rejects unknown keys so
// typos in an operator's seed file fail loudly at bootstrap time
// rather than silently selecting defaults.
func LoadSeed(path string) (*SeedConfig, error) {
	f, err := os.Open(path) //nolint:gosec // path is operator-supplied seed file
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrSeedNotFound, path)
		}
		return nil, fmt.Errorf("selfmgmt: open seed %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var s SeedConfig
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("selfmgmt: decode seed %q: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("selfmgmt: validate seed %q: %w", path, err)
	}
	return &s, nil
}

// Validate enforces the seed contract. Error messages mirror the
// field-prefix style used in internal/config (e.g.
// `mode: "foo" (must be development or production)`) so operator
// errors read consistently across runtime and bootstrap docs.
func (s *SeedConfig) Validate() error {
	switch s.Mode {
	case SeedModeDevelopment, SeedModeProduction:
	case "":
		return fmt.Errorf("mode: must be set (development or production)")
	default:
		return fmt.Errorf("mode: %q (must be development or production)", string(s.Mode))
	}

	if s.ClusterName == "" {
		return fmt.Errorf("cluster_name: must not be empty")
	}
	if !clusterNameRe.MatchString(s.ClusterName) {
		return fmt.Errorf("cluster_name: %q (must match %s)", s.ClusterName, clusterNameRe.String())
	}

	switch s.NodeRole {
	case SeedNodeRoleSeed, SeedNodeRoleJoin:
	case "":
		return fmt.Errorf("node_role: must be set (seed or join)")
	default:
		return fmt.Errorf("node_role: %q (must be seed or join)", string(s.NodeRole))
	}

	if err := s.Storage.validate(); err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	if err := s.NATS.validate(); err != nil {
		return fmt.Errorf("nats: %w", err)
	}

	switch s.TLSStrategy {
	case SeedTLSSelfSigned, SeedTLSCSR:
	case SeedTLSDisabled:
		if s.Mode == SeedModeProduction {
			return fmt.Errorf("tls_strategy: %q is not permitted in production mode", string(s.TLSStrategy))
		}
	case "":
		return fmt.Errorf("tls_strategy: must be set (self-signed, csr, or disabled)")
	default:
		return fmt.Errorf("tls_strategy: %q (must be self-signed, csr, or disabled)", string(s.TLSStrategy))
	}

	for i, bp := range s.Blueprints {
		if bp == "" {
			return fmt.Errorf("blueprints[%d]: must not be empty", i)
		}
	}
	return nil
}

func (st *SeedStorage) validate() error {
	switch st.Driver {
	case SeedStorageSQLite, SeedStoragePostgres:
	case "":
		return fmt.Errorf("driver: must be set (sqlite or postgres)")
	default:
		return fmt.Errorf("driver: %q (must be sqlite or postgres)", string(st.Driver))
	}
	if st.DSN == "" {
		return fmt.Errorf("dsn: must not be empty")
	}
	return nil
}

func (n *SeedNATS) validate() error {
	switch n.Mode {
	case SeedNATSEmbedded:
		if len(n.Endpoints) > 0 {
			return fmt.Errorf("endpoints: must be empty when mode is embedded, got %d", len(n.Endpoints))
		}
	case SeedNATSExternal:
		if len(n.Endpoints) == 0 {
			return fmt.Errorf("endpoints: must list at least one URL when mode is external")
		}
		for i, ep := range n.Endpoints {
			if ep == "" {
				return fmt.Errorf("endpoints[%d]: must not be empty", i)
			}
		}
	case "":
		return fmt.Errorf("mode: must be set (embedded or external)")
	default:
		return fmt.Errorf("mode: %q (must be embedded or external)", string(n.Mode))
	}
	return nil
}
