// Package module implements the kscore-module CLI (Epic 14 task
// 14) — the module author/distribution flow:
//
//	init build validate sign verify resolve install update
//	test clean tree
//
// It is the integration capstone: wires the manifest, verify,
// cas, resolver, cache, registry, loader, runtime, and SDK
// packages. The `test` subcommand delegates to an injected
// TestRunner seam filled by task 15 (pkg/module/testing).
//
// Once cmd/kscore-module builds, `kscorectl module …` dispatches
// to it automatically via the task-13 plugin mechanism.
package module

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/registry"
	"go.keystone-core.io/keystone-core/pkg/module/resolver"
	"go.keystone-core.io/keystone-core/pkg/module/verify"
	"go.keystone-core.io/keystone-core/pkg/semver"
)

// ErrTestFrameworkPending is returned by the default TestRunner —
// the real runner is task 15 (pkg/module/testing).
var ErrTestFrameworkPending = errors.New("module: test framework not wired (Epic 14 task 15)")

// RegistryClient is the registry surface the CLI needs (a remote
// kscore-registry; *registry.Client satisfies it). Injected for
// tests.
type RegistryClient interface {
	resolver.Source
	FetchZip(ctx context.Context, module string, v semver.Version) ([]byte, error)
	FetchSignature(ctx context.Context, module string, v semver.Version) (verify.Signature, bool, error)
	Publish(ctx context.Context, manifestYAML, zip, sig []byte) error
}

// TestRunner runs a module's Starlark unit tests. The default
// returns ErrTestFrameworkPending; task 15 injects the real one.
type TestRunner interface {
	RunTests(ctx context.Context, moduleDir string, audit AuditOptions) (passed, failed int, err error)
}

// AuditOptions carries the --audit-level / --audit-output flags
// into the (task-15) test runner's capability auditor.
type AuditOptions struct {
	Level  string
	Output string
}

type defaultRunner struct{}

func (defaultRunner) RunTests(context.Context, string, AuditOptions) (int, int, error) {
	return 0, 0, ErrTestFrameworkPending
}

// Deps wires the CLI's seams (registry client factory + test
// runner). A zero Deps uses the production registry HTTP client
// and the pending test runner.
type Deps struct {
	NewClient  func(base string) RegistryClient
	TestRunner TestRunner
}

func (d Deps) client(base string) RegistryClient {
	if d.NewClient != nil {
		return d.NewClient(base)
	}
	return registry.NewClient(base, nil)
}

func (d Deps) runner() TestRunner {
	if d.TestRunner != nil {
		return d.TestRunner
	}
	return defaultRunner{}
}

// NewCommand returns the kscore-module root command.
func NewCommand(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "kscore-module",
		Short:         "Keystone Core module author + distribution CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		initCmd(), validateCmd(), buildCmd(), signCmd(), verifyCmd(),
		resolveCmd(d), publishCmd(d), installCmd(d), updateCmd(d), treeCmd(d),
		testCmd(d), cleanCmd(),
	)
	cli.AddVersion(root)
	return root
}

// ---- shared helpers -----------------------------------------------------

func readManifest(dir string) (*manifest.Manifest, []byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.yaml")) //nolint:gosec // operator-supplied module dir
	if err != nil {
		return nil, nil, fmt.Errorf("read manifest: %w", err)
	}
	m, err := manifest.UnmarshalManifest(b)
	if err != nil {
		return nil, nil, err
	}
	return m, b, nil
}

// parsePrivateKey accepts a PEM PKCS#8, SEC1 EC, or PKCS#1 RSA
// private key (the plain "local.key" — cosign's encrypted keyfile
// is a deferred ROADMAP item).
func parsePrivateKey(pemBytes []byte) (crypto.Signer, error) {
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		return nil, fmt.Errorf("module: no PEM block in key")
	}
	if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
		if s, ok := k.(crypto.Signer); ok {
			return s, nil
		}
		return nil, fmt.Errorf("module: PKCS8 key is not a signer")
	}
	if k, err := x509.ParseECPrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	return nil, fmt.Errorf("module: unsupported private key (want PKCS8 / SEC1 EC / PKCS1 RSA PEM)")
}

// rootFor builds a synthetic root manifest depending on a single
// module at an exact version (the `install vendor/pkg@x.y.z` form).
func rootFor(module, ver string) (*manifest.Manifest, error) {
	if _, err := semver.Parse(ver); err != nil {
		return nil, fmt.Errorf("module: bad version %q: %w", ver, err)
	}
	return &manifest.Manifest{
		Name: "kscore-module/root", Version: "0.0.0",
		Type: manifest.TypeStarlark, Entrypoint: "main.star",
		Dependencies: map[string]string{module: ver}, // exact pin
	}, nil
}
