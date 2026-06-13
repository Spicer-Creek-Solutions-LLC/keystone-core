// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/identity"
)

func seedPlaintextRootCA(t *testing.T, dir string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(7),
		Subject:               pkix.Name{CommonName: "cli-encrypt-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	st, err := identity.NewFileCAStorage(dir)
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	if err := st.SaveRootCA(cert, key); err != nil {
		t.Fatalf("SaveRootCA: %v", err)
	}
}

func inlineKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return "inline:" + hex.EncodeToString(b)
}

func TestRunCAEncrypt_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedPlaintextRootCA(t, dir)

	var out bytes.Buffer
	if err := runCAEncrypt(&out, dir, inlineKey(t)); err != nil {
		t.Fatalf("runCAEncrypt: %v", err)
	}
	if !strings.Contains(out.String(), "encrypted 1") {
		t.Errorf("output = %q, want it to report encrypting 1 pair", out.String())
	}
	// The key file is no longer a plaintext PEM.
	keyBytes, _ := os.ReadFile(filepath.Join(dir, "root-key.pem"))
	if bytes.Contains(keyBytes, []byte("PRIVATE KEY")) {
		t.Error("key file still contains plaintext PEM after encrypt")
	}
}

func TestRunCAEncrypt_BadKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seedPlaintextRootCA(t, dir)
	if err := runCAEncrypt(&bytes.Buffer{}, dir, "not-a-scheme"); err == nil {
		t.Error("runCAEncrypt accepted a malformed key source")
	}
}

func TestRunCAEncrypt_NoCA(t *testing.T) {
	t.Parallel()
	if err := runCAEncrypt(&bytes.Buffer{}, t.TempDir(), inlineKey(t)); err == nil {
		t.Error("runCAEncrypt succeeded against an empty dir")
	}
}
