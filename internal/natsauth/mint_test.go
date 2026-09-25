package natsauth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

func testMinter(t *testing.T) (*Deployment, *Minter) {
	t.Helper()
	d, err := Generate(Config{BrokerNames: testBrokerNames, RuntimeDir: testRuntimeDir, FleetSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewMinter(d.Keystone.PublicKey, d.Keystone.SigningSeed)
	if err != nil {
		t.Fatal(err)
	}
	return d, m
}

// A runtime bootstrap identity carries exactly the generator's grants for its
// token, expires when it was told to, and is issued by the account signing key.
func TestBootstrapIsTheGeneratorsIdentityWithTheGivenExpiry(t *testing.T) {
	d, m := testMinter(t)
	token := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	expires := time.Now().Add(7 * time.Minute).Truncate(time.Second)
	id, err := m.Bootstrap(token, expires)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jwt.DecodeUserClaims(id.JWT)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Expires != expires.Unix() {
		t.Errorf("expires %d, want %d", claims.Expires, expires.Unix())
	}
	if claims.Issuer != d.Keystone.SigningKey || claims.IssuerAccount != d.Keystone.PublicKey {
		t.Errorf("issued by %s for %s", claims.Issuer, claims.IssuerAccount)
	}
	if !sameSet(claims.Permissions.Pub.Allow, jwt.StringList{"ks.enroll." + token + ".request"}) ||
		!sameSet(claims.Permissions.Sub.Allow, jwt.StringList{"ks.enroll." + token + ".reply"}) ||
		len(claims.Permissions.Pub.Deny) != 0 || len(claims.Permissions.Sub.Deny) != 0 {
		t.Errorf("grants %+v", claims.Permissions)
	}
	if _, err := Credentials(id); err != nil {
		t.Errorf("no credentials file: %v", err)
	}
}

func TestBootstrapRefusesNoExpiryAndABadToken(t *testing.T) {
	_, m := testMinter(t)
	if _, err := m.Bootstrap("0123abcd", time.Time{}); err == nil {
		t.Error("a bootstrap identity that never expires was minted")
	}
	if _, err := m.Bootstrap("NOT.a.token", time.Now().Add(time.Minute)); !errors.Is(err, ErrIdentifier) {
		t.Errorf("a token the subject grammar refuses was minted: %v", err)
	}
}

// The account's own seed also signs; the minter refuses it so the key that can
// re-encode the account never becomes the server's.
func TestNewMinterRefusesTheAccountSeedAndAMismatchedKey(t *testing.T) {
	d, _ := testMinter(t)
	if _, err := NewMinter(d.Keystone.PublicKey, d.Keystone.Seed); err == nil {
		t.Error("the account's own seed was accepted as a signing key")
	}
	user, _ := nkeys.CreateUser()
	seed, _ := user.Seed()
	if _, err := NewMinter(d.Keystone.PublicKey, seed); err == nil {
		t.Error("a user seed was accepted as an account signing key")
	}
	if _, err := NewMinter("not-an-account", d.Keystone.SigningSeed); err == nil {
		t.Error("a malformed account key was accepted")
	}
}

// Each service credential is accepted in its own slot and refused in every
// other, and a credential whose seed is not the JWT's key is refused.
func TestReadServiceCredentialAcceptsOnlyItsOwnSlot(t *testing.T) {
	d, _ := testMinter(t)
	for _, have := range Services() {
		file, err := Credentials(d.Services[have])
		if err != nil {
			t.Fatal(err)
		}
		for _, slot := range Services() {
			got, err := ReadServiceCredential(slot, file)
			if slot == have {
				if err != nil {
					t.Errorf("%s refused in its own slot: %v", have, err)
				} else if got.Account != d.Keystone.PublicKey || got.Issuer != d.Keystone.SigningKey {
					t.Errorf("%s: account %s issuer %s", have, got.Account, got.Issuer)
				}
			} else if !errors.Is(err, ErrWrongRole) {
				t.Errorf("%s accepted in the %s slot", have, slot)
			}
		}
	}
	// FormatUserConfig refuses the mismatch itself, so the file is spliced by
	// hand: the enrollment service's JWT above another principal's seed.
	valid, err := Credentials(d.Services[PresenceConsumer])
	if err != nil {
		t.Fatal(err)
	}
	spliced := []byte(strings.Replace(string(valid), d.Services[PresenceConsumer].JWT, d.Services[EnrollmentService].JWT, 1))
	if _, err := ReadServiceCredential(EnrollmentService, spliced); !errors.Is(err, ErrWrongRole) {
		t.Errorf("a JWT with someone else's seed was accepted: %v", err)
	}
}
