package enrollment

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/natsauth"
	"go.keystone-core.io/keystone-core/internal/protocol"
	"go.keystone-core.io/keystone-core/internal/store"
)

type fixture struct {
	dir     string
	cfg     config.Server
	deploy  *natsauth.Deployment
	signing *protocol.SigningKey
	result  *protocol.DecryptionKey
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d, err := natsauth.Generate(natsauth.Config{FleetSize: 1, BrokerNames: []string{"broker"}, RuntimeDir: "/etc/nats"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := d.Write(dir); err != nil {
		t.Fatal(err)
	}
	signing, err := protocol.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	result, err := protocol.GenerateDecryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{dir: dir, deploy: d, signing: signing, result: result}
	f.write(t, "service-signing.key", EncodeKey(protocol.MarshalSigningKey(signing))+"\n", 0o600)
	f.write(t, "result-encryption.pub", EncodeKey(result.Recipient().Bytes())+"\n", 0o644)
	creds := filepath.Join(dir, natsauth.CredentialsDir)
	f.cfg = config.Server{
		Store: config.Store{Path: filepath.Join(dir, "server.db")},
		NATS: config.NATS{
			URL: "tls://broker:4222", ServerName: "broker",
			CAFile:                filepath.Join(dir, natsauth.TLSDir, natsauth.CACertFile),
			EnrollmentCredentials: filepath.Join(creds, string(natsauth.EnrollmentService)+".creds"),
			PresenceCredentials:   filepath.Join(creds, string(natsauth.PresenceConsumer)+".creds"),
			AccountSigningSeed:    filepath.Join(creds, "account-signing.nk"),
		},
		Service: config.Service{
			SigningPrivateKeyFile:         filepath.Join(dir, "service-signing.key"),
			ResultEncryptionPublicKeyFile: filepath.Join(dir, "result-encryption.pub"),
		},
	}
	return f
}

func (f *fixture) write(t *testing.T, name, content string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(f.dir, name)
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) open(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(f.cfg.Store.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s, err := Open(f.cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	return s, st
}

func TestSigningKeyFileMustBeOwnerOnly(t *testing.T) {
	f := newFixture(t)
	path := f.cfg.Service.SigningPrivateKeyFile
	for _, mode := range []os.FileMode{0o400, 0o600} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadServiceKeys(path, f.cfg.Service.ResultEncryptionPublicKeyFile); err != nil {
			t.Errorf("mode %04o refused: %v", mode, err)
		}
	}
	for _, mode := range []os.FileMode{0o640, 0o604, 0o610, 0o601, 0o660} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadServiceKeys(path, f.cfg.Service.ResultEncryptionPublicKeyFile); !errors.Is(err, ErrKeyFilePermissions) {
			t.Errorf("mode %04o: %v", mode, err)
		}
	}
}

// A damaged key file says it is a damaged key file, not "signature invalid".
func TestDamagedKeyFilesAreNamed(t *testing.T) {
	f := newFixture(t)
	f.write(t, "service-signing.key", EncodeKey([]byte("too short"))+"\n", 0o600)
	if _, err := LoadServiceKeys(f.cfg.Service.SigningPrivateKeyFile, f.cfg.Service.ResultEncryptionPublicKeyFile); !errors.Is(err, ErrKeyFile) {
		t.Errorf("short signing key: %v", err)
	}
	f.write(t, "service-signing.key", "not base64 at all!\n", 0o600)
	if _, err := LoadServiceKeys(f.cfg.Service.SigningPrivateKeyFile, f.cfg.Service.ResultEncryptionPublicKeyFile); !errors.Is(err, ErrKeyFile) {
		t.Errorf("non-base64 signing key: %v", err)
	}
	f.write(t, "service-signing.key", EncodeKey(protocol.MarshalSigningKey(f.signing))+"\n", 0o600)
	f.write(t, "result-encryption.pub", EncodeKey(f.signing.Verifying().Bytes())+"\n", 0o644)
	if _, err := LoadServiceKeys(f.cfg.Service.SigningPrivateKeyFile, f.cfg.Service.ResultEncryptionPublicKeyFile); !errors.Is(err, ErrKeyFile) {
		t.Errorf("a verifying key in the result-key slot: %v", err)
	}
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestIssueRecordsTheTokenAndReturnsTheBundle(t *testing.T) {
	f := newFixture(t)
	s, st := f.open(t)
	name := "alice"
	before := time.Now()
	b, err := s.Issuer.Issue(context.Background(), Actor{UID: 1501, UsernameSnapshot: &name}, "web-1", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != BundleVersion || !hex64.MatchString(b.TokenID) || !hex32.MatchString(b.AgentID) {
		t.Fatalf("bundle identifiers %+v", b)
	}
	if b.ServiceSigningPublicKey != base64.StdEncoding.EncodeToString(f.signing.Verifying().Bytes()) ||
		b.ResultEncryptionPublicKey != base64.StdEncoding.EncodeToString(f.result.Recipient().Bytes()) {
		t.Fatal("the bundle does not carry the configured service halves")
	}
	token, err := jwt.ParseDecoratedJWT([]byte(b.BootstrapCredentials))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwt.DecodeUserClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Name != b.TokenID || claims.IssuerAccount != f.deploy.Keystone.PublicKey {
		t.Fatalf("claims %+v", claims)
	}
	if exp := time.Unix(claims.Expires, 0); exp.Before(before.Add(2*time.Minute-time.Second)) || exp.After(time.Now().Add(2*time.Minute)) {
		t.Fatalf("expires %v", exp)
	}
	var agent, label, state string
	var uid int64
	if err := st.DB().QueryRow(`SELECT agent_id, agent_name, state, issued_by_uid FROM enrollment WHERE token_id = ?`, b.TokenID).
		Scan(&agent, &label, &state, &uid); err != nil {
		t.Fatal(err)
	}
	if agent != b.AgentID || label != "web-1" || state != "issued" || uid != 1501 {
		t.Fatalf("record %s %s %s %d", agent, label, state, uid)
	}
}

func TestIssueRefusesBadNamesAndLifetimes(t *testing.T) {
	f := newFixture(t)
	s, st := f.open(t)
	for _, c := range []struct {
		name string
		ttl  time.Duration
	}{
		{"", 0}, {"bad\nname", 0}, {string(make([]byte, MaxAgentName+1)), 0}, {"\xff", 0},
		// Format characters are not controls: a zero-width space, a
		// right-to-left override, a zero-width joiner, a byte-order mark.
		{"web\u200b1", 0}, {"web\u202e1", 0}, {"web\u200d1", 0}, {"\ufeffweb", 0},
		// Whitespace other than the ASCII space.
		{"web\t1", 0}, {"web\u00a01", 0},
		{"ok", 59 * time.Second}, {"ok", 3541 * time.Second}, {"ok", -time.Minute},
	} {
		if _, err := s.Issuer.Issue(context.Background(), Actor{UID: 1}, c.name, c.ttl); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%q ttl %v: %v", c.name, c.ttl, err)
		}
	}
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM enrollment`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("refused requests left %d records (%v)", n, err)
	}
	for _, name := range []string{"web-1", "web 1", "wéb-ünïcode", "服务器"} {
		if !ValidAgentName(name) {
			t.Errorf("printable label %q refused", name)
		}
	}
	for _, ttl := range []time.Duration{0, MinTokenTTL, MaxTokenTTL} {
		if _, err := s.Issuer.Issue(context.Background(), Actor{UID: 1}, "ok", ttl); err != nil {
			t.Errorf("ttl %v refused: %v", ttl, err)
		}
	}
}

// Each credential is read from its own slot; the wrong principal in either is
// refused, and so is a signing seed from another deployment.
func TestOpenRefusesWrongRolesAndAForeignSeed(t *testing.T) {
	f := newFixture(t)
	creds := filepath.Join(f.dir, natsauth.CredentialsDir)
	wrong := f.cfg
	wrong.NATS.EnrollmentCredentials = f.cfg.NATS.PresenceCredentials
	if _, err := Open(wrong, nil); !errors.Is(err, natsauth.ErrWrongRole) {
		t.Errorf("presence credential in the enrollment slot: %v", err)
	}
	wrong = f.cfg
	wrong.NATS.PresenceCredentials = filepath.Join(creds, string(natsauth.CommandPublisher)+".creds")
	if _, err := Open(wrong, nil); !errors.Is(err, natsauth.ErrWrongRole) {
		t.Errorf("command publisher in the presence slot: %v", err)
	}
	other, err := natsauth.Generate(natsauth.Config{FleetSize: 1, BrokerNames: []string{"broker"}, RuntimeDir: "/etc/nats"})
	if err != nil {
		t.Fatal(err)
	}
	f.write(t, "foreign.nk", string(other.Keystone.SigningSeed), 0o600)
	wrong = f.cfg
	wrong.NATS.AccountSigningSeed = filepath.Join(f.dir, "foreign.nk")
	if _, err := Open(wrong, nil); !errors.Is(err, ErrMismatchedDeployment) {
		t.Errorf("a foreign signing seed: %v", err)
	}
}
