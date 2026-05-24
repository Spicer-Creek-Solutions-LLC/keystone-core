// SPDX-License-Identifier: Apache-2.0

package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "x509:" + name,
		Module: "x509",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// pathsIn returns (certPath, keyPath) under a fresh tempdir.
func pathsIn(t *testing.T) (string, string) {
	t.Helper()
	d := t.TempDir()
	return filepath.Join(d, "server.crt"), filepath.Join(d, "server.key")
}

func mustApply(t *testing.T, d *statemgmt.Declaration) *statemgmt.StateResult {
	t.Helper()
	sr, err := New().Apply(context.Background(), d)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return sr
}

func loadCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	c, err := loadCertificate(path)
	if err != nil {
		t.Fatalf("loadCertificate(%s): %v", path, err)
	}
	return c
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("/c.crt", StatePresent, map[string]any{"keypath": "/k"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_SANsAndInts(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("/c.crt", StatePresent, map[string]any{
		"key_path": "/k", "common_name": "h", "subject_alt_names": []any{"a.example", "10.0.0.1"},
		"days": 90, "renew_days": int64(7), "rsa_bits": float64(4096),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.SANs) != 2 || p.Days != 90 || p.RenewDays != 7 || p.RSABits != 4096 {
		t.Errorf("unexpected: %+v", p)
	}
	// single SAN as a bare string
	p, _ = parseParams(decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "subject_alt_names": "only.example"}))
	if len(p.SANs) != 1 || p.SANs[0] != "only.example" {
		t.Errorf("bare-string SAN: %+v", p.SANs)
	}
	// bad SAN element
	if _, err := parseParams(decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "subject_alt_names": []any{"", "x"}})); err == nil {
		t.Error("empty SAN element should be rejected")
	}
	if _, err := parseParams(decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "subject_alt_names": 7})); err == nil {
		t.Error("non-string/list SAN should be rejected")
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present ok cn", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h"}), false},
		{"present ok sans", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "subject_alt_names": []any{"a"}}), false},
		{"present needs cn or sans", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k"}), true},
		{"needs key_path", decl("/c.crt", StatePresent, map[string]any{"common_name": "h"}), true},
		{"key_path == cert path", decl("/same", StatePresent, map[string]any{"key_path": "/same", "common_name": "h"}), true},
		{"rsa_bits too small", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "rsa_bits": 1024}), true},
		{"bad key_type", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "key_type": "dsa"}), true},
		{"bad ecdsa curve", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "key_type": "ecdsa", "ecdsa_curve": "p999"}), true},
		{"days < 1", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "days": 0}), true},
		{"renew_days < 0", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "renew_days": -1}), true},
		{"ca_cert without ca_key", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "ca_cert": "/ca.crt"}), true},
		{"ca_key without ca_cert", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "ca_key": "/ca.key"}), true},
		{"ca pair ok", decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "ca_cert": "/ca.crt", "ca_key": "/ca.key"}), false},
		{"absent ok", decl("/c.crt", StateAbsent, map[string]any{"key_path": "/k"}), false},
		{"absent rejects cert params", decl("/c.crt", StateAbsent, map[string]any{"key_path": "/k", "common_name": "h"}), true},
		{"bad state", decl("/c.crt", "frob", map[string]any{"key_path": "/k"}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseParams(tc.d)
			if err == nil {
				err = p.validate()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestParseInt_Forms(t *testing.T) {
	t.Parallel()
	mk := func(v any) (int, error) {
		p, err := parseParams(decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h", "days": v}))
		if err != nil {
			return 0, err
		}
		return p.Days, nil
	}
	if n, err := mk(int64(45)); err != nil || n != 45 {
		t.Errorf("int64: %d %v", n, err)
	}
	if n, err := mk("60"); err != nil || n != 60 {
		t.Errorf("string: %d %v", n, err)
	}
	if _, err := mk("xyz"); err == nil {
		t.Error("non-numeric string should error")
	}
	if _, err := mk(float64(1.5)); err == nil {
		t.Error("fractional float should error")
	}
	if _, err := mk(true); err == nil {
		t.Error("bool should error")
	}
}

// --- PEM helpers ------------------------------------------------------

func TestLoadPrivateKey_LegacyForms(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// PKCS#1 ("RSA PRIVATE KEY")
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pkcs1 := filepath.Join(dir, "pkcs1.pem")
	writePEM(t, pkcs1, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey))
	if got, err := loadPrivateKey(pkcs1); err != nil || !publicKeysEqual(got.Public(), &rsaKey.PublicKey) {
		t.Errorf("PKCS#1: %v", err)
	}

	// SEC1 ("EC PRIVATE KEY")
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalECPrivateKey(ecKey)
	sec1 := filepath.Join(dir, "sec1.pem")
	writePEM(t, sec1, "EC PRIVATE KEY", der)
	if got, err := loadPrivateKey(sec1); err != nil || !publicKeysEqual(got.Public(), &ecKey.PublicKey) {
		t.Errorf("SEC1: %v", err)
	}

	// a PEM block of the wrong type → error
	wrong := filepath.Join(dir, "wrong.pem")
	writePEM(t, wrong, "CERTIFICATE", []byte("not a key"))
	if _, err := loadPrivateKey(wrong); err == nil {
		t.Error("a non-key PEM block should error")
	}
}

func TestKeyPEM_RoundTrip(t *testing.T) {
	t.Parallel()
	for _, p := range []*params{
		{KeyType: KeyRSA, RSABits: 2048},
		{KeyType: KeyECDSA, ECDSACurve: CurveP256},
		{KeyType: KeyEd25519},
	} {
		key, err := generateKey(p)
		if err != nil {
			t.Fatalf("generateKey(%s): %v", p.KeyType, err)
		}
		pemBytes, err := marshalKeyPEM(key)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "k.pem")
		if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadPrivateKey(path)
		if err != nil {
			t.Fatalf("loadPrivateKey(%s): %v", p.KeyType, err)
		}
		if !publicKeysEqual(got.Public(), key.Public()) {
			t.Errorf("%s: round-tripped key's public part differs", p.KeyType)
		}
	}
	// garbage / missing
	if _, err := loadPrivateKey(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing key should error")
	}
	bad := filepath.Join(t.TempDir(), "bad.pem")
	_ = os.WriteFile(bad, []byte("not pem"), 0o600)
	if _, err := loadPrivateKey(bad); err == nil {
		t.Error("non-PEM key should error")
	}
}

// --- Check / Apply: self-signed ---------------------------------------

func TestApply_SelfSigned_RSA(t *testing.T) {
	t.Parallel()
	cert, key := pathsIn(t)
	d := decl(cert, StatePresent, map[string]any{
		"key_path": key, "common_name": "host.example.com",
		"subject_alt_names": []any{"host.example.com", "10.1.2.3"},
		"organization":      "Keystone Test",
	})

	r, err := New().Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("nothing on disk → should drift")
	}
	sr := mustApply(t, d)
	if !sr.Changed {
		t.Error("first apply should change")
	}

	c := loadCert(t, cert)
	if c.Subject.CommonName != "host.example.com" {
		t.Errorf("CN = %q", c.Subject.CommonName)
	}
	if len(c.Subject.Organization) != 1 || c.Subject.Organization[0] != "Keystone Test" {
		t.Errorf("O = %v", c.Subject.Organization)
	}
	if len(c.DNSNames) != 1 || c.DNSNames[0] != "host.example.com" {
		t.Errorf("DNSNames = %v", c.DNSNames)
	}
	if len(c.IPAddresses) != 1 || c.IPAddresses[0].String() != "10.1.2.3" {
		t.Errorf("IPAddresses = %v", c.IPAddresses)
	}
	if c.IsCA {
		t.Error("should not be a CA")
	}
	if _, isRSA := c.PublicKey.(*rsa.PublicKey); !isRSA {
		t.Errorf("default key type should be RSA, got %T", c.PublicKey)
	}
	// file modes
	if fi, _ := os.Stat(key); fi.Mode().Perm() != 0o600 {
		t.Errorf("key mode = %o, want 0600", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(cert); fi.Mode().Perm() != 0o644 {
		t.Errorf("cert mode = %o, want 0644", fi.Mode().Perm())
	}

	// converged
	r, _ = New().Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("should match after apply, diff=%q", r.Diff)
	}
	sr = mustApply(t, d)
	if sr.Changed || sr.Comment != "already converged" {
		t.Errorf("second apply: changed=%v comment=%q", sr.Changed, sr.Comment)
	}
}

func TestApply_SelfSigned_ECDSAandEd25519(t *testing.T) {
	t.Parallel()
	for _, kt := range []string{KeyECDSA, KeyEd25519} {
		cert, key := pathsIn(t)
		mustApply(t, decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "h", "key_type": kt}))
		c := loadCert(t, cert)
		switch kt {
		case KeyECDSA:
			if _, ok := c.PublicKey.(*ecdsa.PublicKey); !ok {
				t.Errorf("ecdsa: cert public key is %T", c.PublicKey)
			}
		case KeyEd25519:
			if _, ok := c.PublicKey.(ed25519.PublicKey); !ok {
				t.Errorf("ed25519: cert public key is %T", c.PublicKey)
			}
		}
	}
}

func TestApply_RegenerateScenarios(t *testing.T) {
	t.Parallel()
	cert, key := pathsIn(t)
	d := decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "a.example"})
	mustApply(t, d)
	origKey, _ := os.ReadFile(key)
	origCert, _ := os.ReadFile(cert)

	// (a) delete the cert → regenerate cert from the existing key
	if err := os.Remove(cert); err != nil {
		t.Fatal(err)
	}
	r, _ := New().Check(context.Background(), d)
	if r.Matches {
		t.Error("missing cert → drift")
	}
	mustApply(t, d)
	if k, _ := os.ReadFile(key); string(k) != string(origKey) {
		t.Error("the key should not have changed when only the cert was regenerated")
	}
	if c, _ := os.ReadFile(cert); string(c) == string(origCert) {
		t.Error("the cert should have been regenerated (new serial)")
	}

	// (b) delete the key → regenerate both
	if err := os.Remove(key); err != nil {
		t.Fatal(err)
	}
	r, _ = New().Check(context.Background(), d)
	if r.Matches {
		t.Error("missing key → drift")
	}
	mustApply(t, d)
	if _, err := os.Stat(key); err != nil {
		t.Errorf("key should exist again: %v", err)
	}
	newCert := loadCert(t, cert)
	newKey, _ := loadPrivateKey(key)
	if !publicKeysEqual(newKey.Public(), newCert.PublicKey) {
		t.Error("regenerated key/cert pair is inconsistent")
	}

	// (c) change the CN → regenerate cert, key unchanged
	keyBefore, _ := os.ReadFile(key)
	d2 := decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "b.example"})
	r, _ = New().Check(context.Background(), d2)
	if r.Matches {
		t.Error("CN change → drift")
	}
	mustApply(t, d2)
	if c := loadCert(t, cert); c.Subject.CommonName != "b.example" {
		t.Errorf("CN after change = %q", c.Subject.CommonName)
	}
	if k, _ := os.ReadFile(key); string(k) != string(keyBefore) {
		t.Error("key should not change on a CN-only update")
	}

	// (d) corrupt the cert file → drift → regenerate
	if err := os.WriteFile(cert, []byte("garbage not a cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, _ = New().Check(context.Background(), d2)
	if r.Matches {
		t.Error("corrupt cert → drift")
	}
	mustApply(t, d2)
	if _, err := loadCertificate(cert); err != nil {
		t.Errorf("cert should be valid again: %v", err)
	}
}

func TestApply_RegenerateOnIsCAChange(t *testing.T) {
	t.Parallel()
	cert, key := pathsIn(t)
	mustApply(t, decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "Root", "is_ca": true}))
	if !loadCert(t, cert).IsCA {
		t.Fatal("first cert should be a CA")
	}
	// re-declare without is_ca (default false) → drift → regenerate as a leaf
	d2 := decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "Root"})
	r, _ := New().Check(context.Background(), d2)
	if r.Matches {
		t.Error("IsCA mismatch → should drift")
	}
	mustApply(t, d2)
	if loadCert(t, cert).IsCA {
		t.Error("regenerated cert should not be a CA")
	}
}

func TestApply_RenewOnExpiry(t *testing.T) {
	t.Parallel()
	cert, key := pathsIn(t)
	// a 1-day cert
	mustApply(t, decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "h", "days": 1}))
	c1 := loadCert(t, cert)

	// re-declare with renew_days=2 (> 1 day left) and days=365 → drift, then a long-lived cert
	d2 := decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "h", "renew_days": 2, "days": 365})
	r, _ := New().Check(context.Background(), d2)
	if r.Matches {
		t.Error("cert with 1 day left and renew_days=2 → should drift")
	}
	mustApply(t, d2)
	c2 := loadCert(t, cert)
	if !c2.NotAfter.After(c1.NotAfter) {
		t.Errorf("renewed cert NotAfter (%v) should be later than the old one (%v)", c2.NotAfter, c1.NotAfter)
	}
	// renew_days=0 disables expiry-proximity renewal
	mustApply(t, decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "h", "days": 1}))
	d3 := decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "h", "renew_days": 0})
	r, _ = New().Check(context.Background(), d3)
	if !r.Matches {
		t.Errorf("renew_days=0 should not trigger expiry drift, diff=%q", r.Diff)
	}
}

func TestApply_CASigned(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	caCert := filepath.Join(d, "ca.crt")
	caKey := filepath.Join(d, "ca.key")
	leafCert := filepath.Join(d, "server.crt")
	leafKey := filepath.Join(d, "server.key")

	// the CA
	mustApply(t, decl(caCert, StatePresent, map[string]any{"key_path": caKey, "common_name": "Keystone Test CA", "is_ca": true}))
	ca := loadCert(t, caCert)
	if !ca.IsCA {
		t.Fatal("CA cert should have IsCA=true")
	}

	// a leaf signed by the CA
	leafDecl := decl(leafCert, StatePresent, map[string]any{
		"key_path": leafKey, "common_name": "host.example.com",
		"ca_cert": caCert, "ca_key": caKey,
	})
	mustApply(t, leafDecl)
	leaf := loadCert(t, leafCert)
	if err := ca.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature); err != nil {
		t.Errorf("leaf not signed by the CA: %v", err)
	}
	if leaf.IsCA {
		t.Error("leaf should not be a CA")
	}
	// converged
	r, _ := New().Check(context.Background(), leafDecl)
	if !r.Matches {
		t.Errorf("CA-signed leaf should match, diff=%q", r.Diff)
	}

	// drop the CA from the declaration → should now be self-signed → drift → regenerate
	selfDecl := decl(leafCert, StatePresent, map[string]any{"key_path": leafKey, "common_name": "host.example.com"})
	r, _ = New().Check(context.Background(), selfDecl)
	if r.Matches {
		t.Error("a CA-signed cert should drift from a self-signed declaration")
	}
	mustApply(t, selfDecl)
	r, _ = New().Check(context.Background(), selfDecl)
	if !r.Matches {
		t.Errorf("should be self-signed and converged now, diff=%q", r.Diff)
	}
}

func TestApply_Absent(t *testing.T) {
	t.Parallel()
	cert, key := pathsIn(t)
	mustApply(t, decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "h"}))

	abs := decl(cert, StateAbsent, map[string]any{"key_path": key})
	r, _ := New().Check(context.Background(), abs)
	if r.Matches {
		t.Error("files present → should drift from absent")
	}
	sr := mustApply(t, abs)
	if !sr.Changed {
		t.Error("removal should change")
	}
	if _, err := os.Stat(cert); !os.IsNotExist(err) {
		t.Error("cert not removed")
	}
	if _, err := os.Stat(key); !os.IsNotExist(err) {
		t.Error("key not removed")
	}
	// already absent → no-op
	sr = mustApply(t, abs)
	if sr.Changed {
		t.Error("absent on missing files should be a no-op")
	}
	r, _ = New().Check(context.Background(), abs)
	if !r.Matches {
		t.Error("absent on missing files should match")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "x509" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("x509 should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("present drift → HIGH")
	}
	if dsm.DriftSeverity(decl("/c.crt", StateAbsent, map[string]any{"key_path": "/k"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("/c.crt", StatePresent, map[string]any{"key_path": "/k", "common_name": "h"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("/c.crt", StatePresent, map[string]any{"key_path": "/k"})); err == nil {
		t.Error("present without CN/SANs should be rejected")
	}

	// Test() round-trip
	cert, key := pathsIn(t)
	d := decl(cert, StatePresent, map[string]any{"key_path": key, "common_name": "h"})
	if ok, err := m.Test(context.Background(), d); err != nil || ok {
		t.Errorf("Test before apply should be false: ok=%v err=%v", ok, err)
	}
	mustApply(t, d)
	if ok, err := m.Test(context.Background(), d); err != nil || !ok {
		t.Errorf("Test after apply should be true: ok=%v err=%v", ok, err)
	}
}
