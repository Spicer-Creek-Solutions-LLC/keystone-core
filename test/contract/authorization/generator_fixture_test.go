//go:build contract

package authorizationcontract

import (
	"strings"
	"testing"

	"github.com/nats-io/jwt/v2"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

// The identities every generator case is generated with. Two agents, because a
// cross-agent assertion is meaningless with one, and two tokens so a revocation
// can name one and leave the other.
const (
	agentOne   = "agent-one"
	agentTwo   = "agent-two"
	tokenOne   = "token-one"
	tokenTwo   = "token-two"
	fleetSize  = 16
	inboxSplit = ".>"
)

func generated(t *testing.T) *natsauth.Deployment {
	t.Helper()
	d, err := natsauth.Generate(natsauth.Config{
		FleetSize:     fleetSize,
		Agents:        []string{agentOne, agentTwo},
		Tokens:        []string{tokenOne, tokenTwo},
		RevokedTokens: []string{tokenTwo},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return d
}

func writtenDeployment(t *testing.T) (*natsauth.Deployment, string) {
	t.Helper()
	d := generated(t)
	dir := t.TempDir()
	if err := d.Write(dir); err != nil {
		t.Fatalf("write deployment: %v", err)
	}
	return d, dir
}

// userClaims decodes what was actually signed rather than reading the
// generator's own record of it. GEN-3 and GEN-4 are about the JWT a broker
// will be handed, not about a Go field beside it.
func userClaims(t *testing.T, token string) *jwt.UserClaims {
	t.Helper()
	claims, err := jwt.DecodeUserClaims(token)
	if err != nil {
		t.Fatalf("decode user JWT: %v", err)
	}
	return claims
}

// normalize rewrites a generated subject into the vocabulary
// frozenPermissionMatrix uses. It is given the identity's own names, so a
// subject scoped to a DIFFERENT agent, consumer or token does not normalize and
// fails to match -- which is the property that makes GEN-3 able to catch a
// wildcard where a scoped entry belongs.
func normalize(subject, name, identifier string) string {
	// Order matters. The inbox prefix contains the identifier for an agent, so
	// replacing the identifier first would leave the inbox unmatchable.
	s := strings.ReplaceAll(subject, natsauth.InboxPrefix(name), "<own-inbox>")
	s = strings.ReplaceAll(s, "ks-res", "<own-consumer>")
	if identifier != "" {
		s = strings.ReplaceAll(s, "ks-cmd-"+identifier, "<own-consumer>")
		s = strings.ReplaceAll(s, identifier, "<own-id>")
	}
	// The monitoring role is deployment-scoped, so its advisory subjects carry
	// a wildcard where ADR-0004 section 6 writes <consumer>: it is a fleet view
	// of every consumer's advisories, not one consumer's.
	if strings.HasPrefix(s, "$JS.EVENT.ADVISORY.CONSUMER.") {
		s = strings.TrimSuffix(s, ".*") + ".<consumer>"
	}
	return s
}

// bootstrapNormalize is separate because a bootstrap identity's scope token is
// its enrollment token rather than an agent identifier, and the matrix spells
// it <own-token>.
func bootstrapNormalize(subject, token string) string {
	return strings.ReplaceAll(subject, token, "<own-token>")
}

type namedIdentity struct {
	principal  string
	name       string
	identifier string
	identity   natsauth.Identity
	bootstrap  bool
}

// everyIdentity is the full inventory each generator case walks. Service roles
// carry their principal token as both matrix key and inbox name; agents and
// bootstrap identities carry the matrix's generic key with their own scope.
func everyIdentity(t *testing.T, d *natsauth.Deployment) []namedIdentity {
	t.Helper()
	var all []namedIdentity
	for _, p := range natsauth.Services() {
		identity, ok := d.Services[p]
		if !ok {
			t.Fatalf("no identity was generated for %s", p)
		}
		all = append(all, namedIdentity{principal: string(p), name: string(p), identity: identity})
	}
	for _, id := range []string{agentOne, agentTwo} {
		identity, ok := d.Agents[id]
		if !ok {
			t.Fatalf("no identity was generated for agent %s", id)
		}
		all = append(all, namedIdentity{principal: "agent", name: id, identifier: id, identity: identity})
	}
	for _, token := range []string{tokenOne, tokenTwo} {
		identity, ok := d.Bootstraps[token]
		if !ok {
			t.Fatalf("no identity was generated for token %s", token)
		}
		all = append(all, namedIdentity{principal: "bootstrap", name: token, identifier: token, identity: identity, bootstrap: true})
	}
	return all
}

// matrixKey is the set of grants the frozen matrix holds, keyed for lookup in
// both directions.
func matrixKey(principal, direction, subject string) string {
	return principal + "\x00" + direction + "\x00" + subject
}

func frozenSet() map[string]bool {
	set := map[string]bool{}
	for _, g := range frozenPermissionMatrix {
		set[matrixKey(g.Principal, g.Direction, g.Subject)] = true
	}
	return set
}

// generatedSet is every grant actually present in a generated JWT, normalized
// into the matrix's vocabulary.
func generatedSet(t *testing.T, d *natsauth.Deployment) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, n := range everyIdentity(t, d) {
		claims := userClaims(t, n.identity.JWT)
		add := func(direction string, subjects jwt.StringList) {
			for _, subject := range subjects {
				normalized := normalize(subject, n.name, n.identifier)
				if n.bootstrap {
					normalized = bootstrapNormalize(subject, n.identifier)
				}
				set[matrixKey(n.principal, direction, normalized)] = true
			}
		}
		add("publish", claims.Permissions.Pub.Allow)
		add("subscribe", claims.Permissions.Sub.Allow)
	}
	return set
}
