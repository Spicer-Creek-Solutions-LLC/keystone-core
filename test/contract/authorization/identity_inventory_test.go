//go:build contract

package authorizationcontract

import (
	"errors"
	"strings"
	"testing"

	"github.com/nats-io/nkeys"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

// ARCH-NATS-002. Co-location is a deployment fact, not an identity fact: a
// server process holding four service credentials is still four principals, so
// the inventory is checked for distinctness rather than for a count alone.
func TestGEN2EveryPrincipalHasDistinctIdentity(t *testing.T) {
	d := generated(t)

	inventory := everyIdentity(t, d)
	expected := len(natsauth.Services()) + 2 + 2
	if len(inventory) != expected {
		t.Fatalf("inventory holds %d identities, want %d", len(inventory), expected)
	}

	keys := map[string]string{}
	seeds := map[string]string{}
	for _, n := range inventory {
		if n.identity.PublicKey == "" {
			t.Fatalf("%s has no public key", n.name)
		}
		if owner, clash := keys[n.identity.PublicKey]; clash {
			t.Errorf("%s and %s share a public key", n.name, owner)
		}
		keys[n.identity.PublicKey] = n.name

		// A permanent agent has no seed on this side at all: ADR-0003 § 4 keeps
		// it on the agent. The exception is asserted rather than assumed --
		// skipping an empty seed silently would let a service role lose its own
		// seed without this case noticing.
		if len(n.identity.Seed) == 0 {
			if n.principal != "agent" {
				t.Errorf("%s has no seed, and only a permanent agent may have none", n.name)
			}
			continue
		}
		if n.principal == "agent" {
			t.Errorf("%s is a permanent agent and this side holds its seed", n.name)
		}
		if owner, clash := seeds[string(n.identity.Seed)]; clash {
			t.Errorf("%s and %s share a seed", n.name, owner)
		}
		seeds[string(n.identity.Seed)] = n.name
	}

	// The account signing key is a principal too, and one that can mint the
	// others. It must not be any of them.
	if owner, clash := keys[d.Keystone.SigningKey]; clash {
		t.Errorf("the account signing key is also %s's identity", owner)
	}
}

// ADR-0004 section 1's grammar and ADR-0005 section 3's reserved tokens. The
// generator consumes an identifier and refuses a bad one; WHERE a valid one
// comes from is ADR-0003's, still unresolved, and deliberately not decided here.
func TestGEN6IdentifiersAreValidated(t *testing.T) {
	refused := []struct {
		name string
		id   string
	}{
		{"empty", ""},
		{"uppercase", "AgentOne"},
		{"a dot creates subject structure the supplier controls", "agent.one"},
		{"a star is a NATS wildcard", "agent*"},
		{"a greater-than is a NATS wildcard", "agent>"},
		{"underscore is outside the grammar", "agent_one"},
		{"longer than 64 characters", strings.Repeat("a", 65)},
	}
	for _, p := range natsauth.Services() {
		refused = append(refused, struct {
			name string
			id   string
		}{"reserved for a service principal", string(p)})
	}

	// A VALID public key is supplied every time, so the identifier is the only
	// thing left to reject. With an empty key each case would fail on
	// ErrAgentPublicKey and this would be a test of the wrong rule -- which the
	// errors.Is assertion below is what catches.
	key := validAgentKey(t)

	for _, tc := range refused {
		t.Run(tc.name+"/"+tc.id, func(t *testing.T) {
			_, err := natsauth.Generate(natsauth.Config{
				FleetSize: 1,
				Agents:    []natsauth.AgentKey{{ID: tc.id, PublicKey: key}},
			})
			if err == nil {
				t.Fatalf("generated an authorization set for identifier %q", tc.id)
			}
			if !errors.Is(err, natsauth.ErrIdentifier) {
				t.Fatalf("refused %q with %v, want an identifier error", tc.id, err)
			}
		})
	}

	// A rule that refuses everything is not a rule. Exactly 64 characters is
	// the boundary the grammar permits, so it is the accepted case.
	accepted := strings.Repeat("a", 64)
	if _, err := natsauth.Generate(natsauth.Config{
		FleetSize: 1,
		Agents:    []natsauth.AgentKey{{ID: accepted, PublicKey: key}},
	}); err != nil {
		t.Fatalf("refused a well-formed 64-character identifier: %v", err)
	}

	// An enrollment token is a subject token too, and the same grammar governs
	// it -- ADR-0004 section 1 constrains the <id> position, which a token
	// occupies on the enrollment plane.
	if _, err := natsauth.Generate(natsauth.Config{FleetSize: 1, Tokens: []string{"token.one"}}); err == nil {
		t.Fatal("generated an authorization set for an enrollment token containing a dot")
	}
}

func validAgentKey(t *testing.T) string {
	t.Helper()
	kp, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := kp.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	return pub
}
