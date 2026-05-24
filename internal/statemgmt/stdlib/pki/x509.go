// SPDX-License-Identifier: Apache-2.0

// (Package doc lives in params.go.)
//
// State semantics:
//
//	present — a private key exists at `key_path` (parsing as RSA /
//	          ECDSA / Ed25519) and a certificate exists at the
//	          declaration path, the cert matches the key, its CN /
//	          subject-alt-names / IsCA match the declaration, it is
//	          currently valid with at least `renew_days` days left,
//	          and it is self-signed (no `ca_cert`) or signed by the
//	          declared `ca_cert`. Anything stale is regenerated:
//	          a bad/missing key ⇒ new key + new cert; a bad/missing/
//	          mismatched/expiring cert ⇒ new cert from the existing
//	          key. `days` only affects newly generated certs.
//	absent  — the certificate file and the key file are removed
//	          (like `file: <path>` `state: absent`).
//
// v0.1 out of scope (v0.x candidates):
//   - Combined cert+key PEM in a single file.
//   - OpenSSL-style SAN prefixes (`IP:` / `DNS:` / `email:` / `URI:`)
//     — v1.0 auto-detects IP vs DNS; email/URI SANs are not emitted.
//   - More Subject fields (Country, Locality, OU, …); encrypted
//     (passphrase-protected) private keys.
//   - Explicit key/cert file mode + owner params (v1.0: new key
//     0600, new cert 0644; rewrites preserve the existing mode).
//   - Key reuse policy on regeneration (v1.0 keeps a valid key).
//   - CRL / OCSP / AIA extensions; PKCS#12 bundles; ACME / external
//     issuers.
package pki

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New is the Factory registered with the engine Registry.
func New() statemgmt.Module { return &Module{} }

// Module is the x509 state module. It is stateless; concurrent
// Check/Apply/Test calls on different Declarations are safe.
type Module struct{}

func (m *Module) Name() string { return "x509" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a missing / expired / mismatched / wrongly-signed
// TLS certificate is a security-or-availability problem, rarely
// cosmetic — HIGH. Same for a cert declared absent but still on
// disk. Operators override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	return statemgmt.DriftSeverityHigh
}

func (m *Module) Check(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	if p.State == StateAbsent {
		present, err := certOrKeyPresent(p)
		if err != nil {
			return nil, err
		}
		if !present {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: "certificate and/or key present; want absent"}, nil
	}
	converged, _, _, diff, err := checkState(p, time.Now())
	if err != nil {
		return nil, err
	}
	if converged {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: diff}, nil
}

func (m *Module) Apply(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}

	if p.State == StateAbsent {
		removed, err := removeCertAndKey(p)
		if err != nil {
			return failure(start), err
		}
		if !removed {
			return ok(start, false, "", "already converged"), nil
		}
		return ok(start, true, "removed certificate and key", "applied"), nil
	}

	converged, needKey, needCert, diff, err := checkState(p, time.Now())
	if err != nil {
		return nil, err
	}
	if converged {
		return ok(start, false, "", "already converged"), nil
	}
	if err := regenerate(p, needKey, needCert); err != nil {
		return failure(start), err
	}
	return ok(start, true, diff, "applied"), nil
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}

// regenerate writes a fresh key (when needKey) and/or a fresh cert
// (when needCert). It assumes checkState reported drift.
func regenerate(p *params, needKey, needCert bool) error {
	if needKey {
		key, err := generateKey(p)
		if err != nil {
			return fmt.Errorf("generate private key: %w", err)
		}
		keyPEM, err := marshalKeyPEM(key)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(p.KeyPath, keyPEM, keyFileMode); err != nil {
			return fmt.Errorf("write key %s: %w", p.KeyPath, err)
		}
	}
	if !needCert {
		return nil
	}
	leafKey, err := loadPrivateKey(p.KeyPath)
	if err != nil {
		return fmt.Errorf("load private key %s: %w", p.KeyPath, err)
	}
	tmpl, err := buildTemplate(p, time.Now())
	if err != nil {
		return err
	}
	var caCert *x509.Certificate
	var caKey crypto.Signer
	if p.CACertPath != "" {
		if caCert, err = loadCertificate(p.CACertPath); err != nil {
			return fmt.Errorf("load ca_cert %s: %w", p.CACertPath, err)
		}
		if caKey, err = loadPrivateKey(p.CAKeyPath); err != nil {
			return fmt.Errorf("load ca_key %s: %w", p.CAKeyPath, err)
		}
	}
	der, err := signCert(tmpl, leafKey, caCert, caKey)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	return writeFileAtomic(p.CertPath, marshalCertPEM(der), certFileMode)
}

func certOrKeyPresent(p *params) (bool, error) {
	for _, path := range []string{p.CertPath, p.KeyPath} {
		_, err := os.Stat(path)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, fmt.Errorf("stat %s: %w", path, err)
		}
	}
	return false, nil
}

func removeCertAndKey(p *params) (changed bool, err error) {
	for _, path := range []string{p.CertPath, p.KeyPath} {
		err := os.Remove(path)
		if err == nil {
			changed = true
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return changed, fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return changed, nil
}
