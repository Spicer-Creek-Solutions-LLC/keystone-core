package natsauth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tlsDeployment(t *testing.T, names ...string) *Deployment {
	t.Helper()
	d, err := Generate(Config{BrokerNames: names, RuntimeDir: testRuntimeDir, FleetSize: 1, Tokens: []string{"token-one"}})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func certificate(t *testing.T, pemBytes []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("not a PEM certificate: %q", pemBytes)
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The broker's certificate chains to the deployment's CA for every name it was
// issued for, as a server certificate, on P-256.
func TestTheBrokerCertificateChainsToTheCAForEachName(t *testing.T) {
	d := tlsDeployment(t, "broker", "nats.example.internal", "10.0.0.7")
	ca := certificate(t, d.TLS.CACertPEM)
	leaf := certificate(t, d.TLS.BrokerCertPEM)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	for _, name := range []string{"broker", "nats.example.internal", "10.0.0.7"} {
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: name, Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			t.Errorf("the broker certificate does not verify for %q: %v", name, err)
		}
	}
	if !ca.IsCA || ca.MaxPathLen != 0 || !ca.MaxPathLenZero {
		t.Errorf("the CA may issue intermediates: IsCA=%v MaxPathLen=%d zero=%v", ca.IsCA, ca.MaxPathLen, ca.MaxPathLenZero)
	}
	if leaf.IsCA {
		t.Error("the broker certificate is a CA")
	}
	for _, c := range []*x509.Certificate{ca, leaf} {
		k, ok := c.PublicKey.(*ecdsa.PublicKey)
		if !ok || k.Curve != elliptic.P256() {
			t.Errorf("%s key is %T, want ECDSA P-256", c.Subject.CommonName, c.PublicKey)
		}
	}
	if !leaf.NotAfter.Before(ca.NotAfter.Add(time.Second)) {
		t.Errorf("the broker certificate outlives its CA: %v after %v", leaf.NotAfter, ca.NotAfter)
	}
}

// A name the certificate was not issued for is refused, so verification is
// against the configured name and not merely against the CA.
func TestTheBrokerCertificateRefusesAnotherName(t *testing.T) {
	d := tlsDeployment(t, "broker")
	pool := x509.NewCertPool()
	pool.AddCert(certificate(t, d.TLS.CACertPEM))
	if _, err := certificate(t, d.TLS.BrokerCertPEM).Verify(x509.VerifyOptions{DNSName: "not-the-broker", Roots: pool}); err == nil {
		t.Fatal("the broker certificate verified for a name it was not issued for")
	}
}

// Nothing Write produces contains the CA's private key, in PEM or in DER --
// the same reading of every byte that GEN-7 applies to the operator seed.
func TestTheCAKeyIsNeverWritten(t *testing.T) {
	d := tlsDeployment(t, "broker")
	if len(d.CAKey) == 0 {
		t.Fatal("no CA key was returned to the caller, so this test would find nothing")
	}
	block, _ := pem.Decode(d.CAKey)
	if block == nil {
		t.Fatal("the CA key is not PEM")
	}
	dir := t.TempDir()
	if err := d.Write(dir); err != nil {
		t.Fatal(err)
	}
	files := 0
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		files++
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(b, d.CAKey) || bytes.Contains(b, block.Bytes) {
			t.Errorf("%s contains the CA's private key", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("Write produced no files, so this test read nothing")
	}
}

// The broker's private key is readable by its owner only; the certificates are
// public and the directory holding them is the deployment's.
func TestTheBrokerKeyIsNotReadableByOthers(t *testing.T) {
	d := tlsDeployment(t, "broker")
	dir := t.TempDir()
	if err := d.Write(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, TLSDir, BrokerKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != credentialMode {
		t.Fatalf("the broker key is mode %o, want %o", info.Mode().Perm(), credentialMode)
	}
	key, err := os.ReadFile(filepath.Join(dir, TLSDir, BrokerKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	cert, err := os.ReadFile(filepath.Join(dir, TLSDir, BrokerCertFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(cert, key); err != nil {
		t.Fatalf("the written certificate and key are not a pair: %v", err)
	}
}

// The broker configuration requires TLS-first and TLS 1.3, and names the
// certificate files absolutely under the runtime directory -- the broker
// resolves a relative path against its working directory and would not start.
func TestTheServerConfigRequiresTLSFirstAtTLS13WithAbsolutePaths(t *testing.T) {
	d := tlsDeployment(t, "broker")
	conf := d.ServerConfig()
	for _, want := range []string{
		"tls {",
		`cert_file: "/etc/nats/tls/broker.pem"`,
		`key_file: "/etc/nats/tls/broker-key.pem"`,
		"handshake_first: true",
		`min_version: "1.3"`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("the broker configuration lacks %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "verify") {
		t.Error("the broker configuration asks clients for certificates; identity is the NATS JWT")
	}
}

func TestGenerationRefusesMissingBrokerNamesAndARelativeRuntimeDir(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no names":      {RuntimeDir: testRuntimeDir, FleetSize: 1},
		"an empty name": {BrokerNames: []string{""}, RuntimeDir: testRuntimeDir, FleetSize: 1},
	} {
		if _, err := Generate(cfg); !errors.Is(err, ErrBrokerNames) {
			t.Errorf("%s: got %v, want ErrBrokerNames", name, err)
		}
	}
	for _, dir := range []string{"", "etc/nats", "./nats"} {
		if _, err := Generate(Config{BrokerNames: testBrokerNames, RuntimeDir: dir, FleetSize: 1}); !errors.Is(err, ErrRuntimeDir) {
			t.Errorf("runtime dir %q: got %v, want ErrRuntimeDir", dir, err)
		}
	}
}

// Clients verify: the configuration trusts only the deployment's CA, names the
// broker, and refuses anything below TLS 1.3. No option turns verification off.
func TestClientTLSConfigVerifiesTheBroker(t *testing.T) {
	d := tlsDeployment(t, "broker")
	cfg, err := ClientTLSConfig(d.TLS.CACertPEM, "broker")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InsecureSkipVerify || cfg.ServerName != "broker" || cfg.MinVersion != tls.VersionTLS13 || cfg.RootCAs == nil {
		t.Fatalf("client configuration does not verify the broker: %+v", cfg)
	}
	if _, err := ClientTLSConfig(d.TLS.CACertPEM, ""); !errors.Is(err, ErrBrokerNames) {
		t.Errorf("an empty broker name was accepted: %v", err)
	}
	if _, err := ClientTLSConfig([]byte("not a certificate"), "broker"); err == nil {
		t.Error("a client configuration was built with no CA")
	}
}

// A mistyped lifetime is refused, never replaced by a long default; zero means
// the default; and an unset broker lifetime follows a shorter CA rather than
// outliving it.
func TestCertificateLifetimesRejectNegativesAndExplicitOverreach(t *testing.T) {
	base := Config{BrokerNames: testBrokerNames, RuntimeDir: testRuntimeDir, FleetSize: 1}
	refused := map[string]Config{
		"negative CA lifetime":                 {CALifetime: -time.Hour},
		"negative broker lifetime":             {BrokerCertLifetime: -time.Hour},
		"broker lifetime longer than the CA's": {CALifetime: 24 * time.Hour, BrokerCertLifetime: 48 * time.Hour},
	}
	for name, lt := range refused {
		cfg := base
		cfg.CALifetime, cfg.BrokerCertLifetime = lt.CALifetime, lt.BrokerCertLifetime
		if _, err := Generate(cfg); !errors.Is(err, ErrLifetime) {
			t.Errorf("%s: got %v, want ErrLifetime", name, err)
		}
	}

	within := func(t *testing.T, c *x509.Certificate, want time.Duration) {
		t.Helper()
		got := c.NotAfter.Sub(c.NotBefore) - clockAllowance
		if d := got - want; d < -time.Minute || d > time.Minute {
			t.Errorf("%s lifetime is %v, want %v", c.Subject.CommonName, got, want)
		}
	}
	d, err := Generate(base)
	if err != nil {
		t.Fatal(err)
	}
	within(t, certificate(t, d.TLS.CACertPEM), DefaultCALifetime)
	within(t, certificate(t, d.TLS.BrokerCertPEM), DefaultBrokerCertLifetime)

	short := base
	short.CALifetime = 30 * 24 * time.Hour
	d, err = Generate(short)
	if err != nil {
		t.Fatalf("an unset broker lifetime under a short CA was refused: %v", err)
	}
	within(t, certificate(t, d.TLS.BrokerCertPEM), short.CALifetime)
}
