package natsauth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
)

// The contract is the acceptance authority. These cover what it does not reach:
// properties of the generator that no frozen case names, and that would
// otherwise be checked by nobody.

func generate(t *testing.T) *Deployment {
	t.Helper()
	d, err := Generate(Config{
		FleetSize:     4,
		Agents:        []string{"agent-one"},
		Tokens:        []string{"token-one", "token-two"},
		RevokedTokens: []string{"token-two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// ADR-0003 section 6 S5 revokes at the account, so the account JWT is the
// artifact that carries it -- not the absence of a user.
func TestRevocationIsRecordedInTheAccount(t *testing.T) {
	d := generate(t)

	claims, err := jwt.DecodeAccountClaims(d.Keystone.JWT)
	if err != nil {
		t.Fatal(err)
	}
	revoked := d.Bootstraps["token-two"].PublicKey
	if !claims.Revocations.IsRevoked(revoked, time.Now()) {
		t.Error("the revoked bootstrap identity is not in the account's revocation list")
	}
	live := d.Bootstraps["token-one"].PublicKey
	if claims.Revocations.IsRevoked(live, time.Now()) {
		t.Error("an unrevoked bootstrap identity is in the revocation list")
	}
}

// ADR-0003 section 8's short expiry is the failure backstop, so it has to be on
// the claim rather than enforced by whoever remembers to stop using it.
func TestBootstrapIdentitiesExpireAndPermanentOnesDoNot(t *testing.T) {
	d := generate(t)

	bootstrap, err := jwt.DecodeUserClaims(d.Bootstraps["token-one"].JWT)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Expires == 0 {
		t.Error("a bootstrap identity does not expire")
	}
	agent, err := jwt.DecodeUserClaims(d.Agents["agent-one"].JWT)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Expires != 0 {
		t.Error("a permanent agent identity expires")
	}
}

// Two generations must never agree. A generator that produced a stable key
// would make a committed fixture possible, which ADR-0010 section 3 forbids.
func TestGenerationIsNotReproducible(t *testing.T) {
	first, second := generate(t), generate(t)
	if first.OperatorPublicKey == second.OperatorPublicKey {
		t.Error("two generations produced the same operator key")
	}
	if first.Keystone.PublicKey == second.Keystone.PublicKey {
		t.Error("two generations produced the same account key")
	}
}

// Credentials are secrets on disk. Nothing in ADR-0008 section 4 covers them --
// that table is the stores -- so the mode is asserted here or nowhere.
func TestWrittenCredentialsAreNotReadableByOthers(t *testing.T) {
	d := generate(t)
	dir := t.TempDir()
	if err := d.Write(dir); err != nil {
		t.Fatal(err)
	}

	seen := 0
	err := filepath.WalkDir(filepath.Join(dir, CredentialsDir), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		seen++
		if info.Mode().Perm() != credentialMode {
			t.Errorf("%s mode = %o, want %o", filepath.Base(path), info.Mode().Perm(), credentialMode)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("no credential file was written; the walk checked nothing")
	}
}

// The service tokens are valid agent identifiers, which is why ADR-0005
// section 3 reserves them. The credential file names have to keep them apart
// too, or an agent's file would overwrite a service role's.
func TestAgentCredentialsCannotCollideWithAServiceRole(t *testing.T) {
	d, err := Generate(Config{FleetSize: 1, Agents: []string{"one"}, Tokens: []string{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	set := d.credentialSet()
	if len(set) != len(Services())+2 {
		t.Fatalf("credential set holds %d entries, want %d", len(set), len(Services())+2)
	}
	for name := range set {
		if strings.HasPrefix(name, agentPrefix) || strings.HasPrefix(name, bootstrapPrefix) {
			continue
		}
		if _, service := map[string]bool{
			string(CommandPublisher): true, string(EnrollmentService): true,
			string(ResultConsumer): true, string(PresenceConsumer): true,
			string(MonitoringRole): true,
		}[name]; !service {
			t.Errorf("credential %q is neither a service role nor prefixed", name)
		}
	}
}
