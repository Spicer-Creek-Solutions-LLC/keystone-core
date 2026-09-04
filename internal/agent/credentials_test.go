// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testCertPEM mints a self-signed leaf expiring at notAfter and
// returns it as a PEM CERTIFICATE block. Only NotAfter is asserted on
// anywhere, so the rest of the template is the minimum x509 accepts.
func testCertPEM(t *testing.T, notAfter time.Time) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "agent-1"},
		NotBefore:    notAfter.Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestCredentials_HasSVID(t *testing.T) {
	tests := []struct {
		name  string
		creds *Credentials
		want  bool
	}{
		{"nil", nil, false},
		{"api key only", &Credentials{APIKey: "k"}, false},
		{"chain without key", &Credentials{APIKey: "k", CertChainPEM: "c"}, false},
		{"key without chain", &Credentials{APIKey: "k", PrivateKeyPEM: "p"}, false},
		{"both", &Credentials{APIKey: "k", CertChainPEM: "c", PrivateKeyPEM: "p"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.creds.HasSVID(); got != tt.want {
				t.Errorf("HasSVID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentials_LeafNotAfter(t *testing.T) {
	want := time.Now().Add(24 * time.Hour).Truncate(time.Second)

	t.Run("no svid returns zero", func(t *testing.T) {
		got, err := (&Credentials{APIKey: "k"}).LeafNotAfter()
		if err != nil {
			t.Fatalf("LeafNotAfter: %v", err)
		}
		if !got.IsZero() {
			t.Errorf("LeafNotAfter() = %v, want zero", got)
		}
	})

	t.Run("parses the leaf", func(t *testing.T) {
		c := &Credentials{APIKey: "k", PrivateKeyPEM: "p", CertChainPEM: testCertPEM(t, want)}
		got, err := c.LeafNotAfter()
		if err != nil {
			t.Fatalf("LeafNotAfter: %v", err)
		}
		if !got.Equal(want) {
			t.Errorf("LeafNotAfter() = %v, want %v", got, want)
		}
	})

	t.Run("non-PEM chain is an error", func(t *testing.T) {
		c := &Credentials{APIKey: "k", PrivateKeyPEM: "p", CertChainPEM: "not pem at all"}
		if _, err := c.LeafNotAfter(); err == nil {
			t.Error("LeafNotAfter() error = nil, want an error for a non-PEM chain")
		}
	})

	t.Run("PEM that is not a certificate is an error", func(t *testing.T) {
		bad := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("garbage")}))
		c := &Credentials{APIKey: "k", PrivateKeyPEM: "p", CertChainPEM: bad}
		if _, err := c.LeafNotAfter(); err == nil {
			t.Error("LeafNotAfter() error = nil, want a parse error")
		}
	})
}

func TestCredentials_Valid(t *testing.T) {
	now := time.Now()
	live := testCertPEM(t, now.Add(time.Hour))
	expired := testCertPEM(t, now.Add(-time.Hour))

	tests := []struct {
		name  string
		creds *Credentials
		want  bool
	}{
		{"nil", nil, false},
		{"no api key", &Credentials{CertChainPEM: live, PrivateKeyPEM: "p"}, false},
		{"api key only is valid", &Credentials{APIKey: "k"}, true},
		{"live svid", &Credentials{APIKey: "k", CertChainPEM: live, PrivateKeyPEM: "p"}, true},
		{"expired svid", &Credentials{APIKey: "k", CertChainPEM: expired, PrivateKeyPEM: "p"}, false},
		{"unparseable svid", &Credentials{APIKey: "k", CertChainPEM: "nope", PrivateKeyPEM: "p"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.creds.Valid(now); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentialStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "credentials.json")
	store := &CredentialStore{Path: path}

	issued := time.Now().Truncate(time.Second)
	want := &Credentials{
		APIKey:         "api-key-1",
		AgentID:        "agent-1",
		IssuedAt:       issued,
		CertChainPEM:   testCertPEM(t, issued.Add(time.Hour)),
		PrivateKeyPEM:  "-----BEGIN PRIVATE KEY-----\nx\n-----END PRIVATE KEY-----\n",
		TrustBundlePEM: "-----BEGIN CERTIFICATE-----\ny\n-----END CERTIFICATE-----\n",
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.APIKey != want.APIKey || got.AgentID != want.AgentID {
		t.Errorf("Load() = %+v, want api key %q agent %q", got, want.APIKey, want.AgentID)
	}
	if got.CertChainPEM != want.CertChainPEM || got.PrivateKeyPEM != want.PrivateKeyPEM {
		t.Error("Load() did not round-trip the SVID material")
	}
	if !got.IssuedAt.Equal(want.IssuedAt) {
		t.Errorf("IssuedAt = %v, want %v", got.IssuedAt, want.IssuedAt)
	}
}

// The credential file holds a private key. If these permissions ever
// loosen, the agent is leaking its identity to every local user.
func TestCredentialStore_SavePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	path := filepath.Join(dir, "credentials.json")
	store := &CredentialStore{Path: path}
	if err := store.Save(&Credentials{APIKey: "k", AgentID: "agent-1"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("credential file mode = %o, want 600", perm)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("credential dir mode = %o, want 700", perm)
	}
}

func TestCredentialStore_SaveReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	store := &CredentialStore{Path: path}
	if err := store.Save(&Credentials{APIKey: "first", AgentID: "agent-1"}); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	if err := store.Save(&Credentials{APIKey: "second", AgentID: "agent-1"}); err != nil {
		t.Fatalf("Save second: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.APIKey != "second" {
		t.Errorf("APIKey = %q, want %q", got.APIKey, "second")
	}
	// The temp file used for the atomic rename must not survive.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the credential file: %v", len(entries), entries)
	}
}

func TestCredentialStore_LoadErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		store := &CredentialStore{Path: filepath.Join(t.TempDir(), "absent.json")}
		_, err := store.Load()
		if !errors.Is(err, ErrNoCredentials) {
			t.Errorf("Load() error = %v, want ErrNoCredentials", err)
		}
	})

	t.Run("no path configured", func(t *testing.T) {
		_, err := (&CredentialStore{}).Load()
		if !errors.Is(err, ErrNoCredentials) {
			t.Errorf("Load() error = %v, want ErrNoCredentials", err)
		}
	})

	t.Run("nil store", func(t *testing.T) {
		var store *CredentialStore
		_, err := store.Load()
		if !errors.Is(err, ErrNoCredentials) {
			t.Errorf("Load() error = %v, want ErrNoCredentials", err)
		}
	})

	t.Run("corrupt json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "credentials.json")
		if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		_, err := (&CredentialStore{Path: path}).Load()
		if err == nil {
			t.Fatal("Load() error = nil, want a decode error")
		}
		if errors.Is(err, ErrNoCredentials) {
			t.Error("a corrupt file must not be reported as an absent one")
		}
	})
}

func TestCredentialStore_SaveErrors(t *testing.T) {
	t.Run("no path configured", func(t *testing.T) {
		if err := (&CredentialStore{}).Save(&Credentials{APIKey: "k"}); err == nil {
			t.Error("Save() error = nil, want an error with no path")
		}
	})

	t.Run("nil credentials", func(t *testing.T) {
		store := &CredentialStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
		if err := store.Save(nil); err == nil {
			t.Error("Save(nil) error = nil, want an error")
		}
	})

	t.Run("unwritable parent", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: mode 0500 does not block writes")
		}
		dir := t.TempDir()
		locked := filepath.Join(dir, "locked")
		if err := os.Mkdir(locked, 0o500); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		store := &CredentialStore{Path: filepath.Join(locked, "sub", "credentials.json")}
		if err := store.Save(&Credentials{APIKey: "k"}); err == nil {
			t.Error("Save() error = nil, want a mkdir failure")
		}
	})
}
