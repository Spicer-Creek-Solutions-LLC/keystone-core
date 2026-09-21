//go:build contract

package authorizationcontract

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

// deployment is one generated authorization set with a broker running it. It is
// built once per test that needs one, because a shared broker would let one
// case's subscriptions change what another observes.
type deployment struct {
	set    *natsauth.Deployment
	seeds  map[string][]byte
	broker brokerFixture
}

func runningBroker(t *testing.T) deployment {
	t.Helper()
	set, seeds := generatedWithAgentSeeds(t)
	dir := t.TempDir()
	if err := set.Write(dir); err != nil {
		t.Fatalf("write deployment: %v", err)
	}
	return deployment{set: set, seeds: seeds, broker: startAuthorizationBroker(t, dir)}
}

// client is a connection held as one principal, with its authorization errors
// collected.
//
// NATS reports a permissions violation ASYNCHRONOUSLY: the publish or subscribe
// call itself succeeds locally and the server answers with -ERR on the wire.
// A case that only checked the call's return value would pass against a broker
// that refused everything, which is the whole of what these cases exist to
// tell apart.
type client struct {
	conn *nats.Conn

	mu     sync.Mutex
	errors []error
}

func (c *client) record(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors = append(c.errors, err)
}

// isPermissionsViolation distinguishes the broker's authorization refusal from
// every other error a connection can report. The frozen requirements for the
// JetStream cases demand exactly this distinction, and the same helper is used
// everywhere so no case can quietly settle for "something failed".
func isPermissionsViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "permissions violation")
}

// controlSubject is denied to every Keystone identity -- ADR-0004 § 4 denies
// `$SYS.>` to all of them -- and is what every assertion below synchronises on.
const controlSubject = "$SYS.keystone.authorization-control"

// settle waits until the broker's refusal of controlSubject has been delivered
// to this connection's error handler, and returns every violation seen.
//
// THE CONTROL IS THE SYNCHRONISATION, and it replaces a sleep. A permissions
// violation is reported ASYNCHRONOUSLY: the publish or subscribe call succeeds
// locally, the server answers -ERR on the wire, and nats.go dispatches the
// handler on its own goroutine AFTER Flush has returned. The first draft of
// this file read the slice straight after Flush and saw nothing, so every
// positive case passed against a broker that had refused the operation --
// DL-1, in the fixture rather than the product.
//
// Because nats.go dispatches async callbacks in order, a violation for the
// control cannot arrive before one for an operation issued earlier. So once the
// control's refusal is in hand, the record is complete.
//
// It also makes every case below unable to pass vacuously: if the control is
// not refused, the mechanism is not working and the assertion fails rather than
// reporting no violations.
func (c *client) settle(t *testing.T) []error {
	t.Helper()
	if err := c.conn.Publish(controlSubject, []byte{0}); err != nil {
		t.Fatalf("publish control: %v", err)
	}
	if err := c.conn.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		c.mu.Lock()
		seen := append([]error(nil), c.errors...)
		c.mu.Unlock()
		for _, err := range seen {
			if isPermissionsViolation(err) && strings.Contains(err.Error(), controlSubject) {
				var violations []error
				for _, e := range seen {
					if isPermissionsViolation(e) && !strings.Contains(e.Error(), controlSubject) {
						violations = append(violations, e)
					}
				}
				return violations
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the broker never refused %s; this connection cannot report a "+
				"permissions violation, so nothing it asserts means anything", controlSubject)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// connectAs opens a connection as one principal, with that principal's own
// inbox prefix. The prefix matters: ADR-0004 § 6 grants each principal its own
// subtree, and a client using the library default would be denied its replies
// while appearing to have the wrong permission.
func connectAs(t *testing.T, d deployment, name string, identity natsauth.Identity, seed []byte) *client {
	t.Helper()
	if len(seed) == 0 {
		t.Fatalf("no seed for %s; a connection cannot be made without one", name)
	}
	creds, err := jwt.FormatUserConfig(identity.JWT, seed)
	if err != nil {
		t.Fatalf("format %s credentials: %v", name, err)
	}
	path := filepath.Join(t.TempDir(), "user.creds")
	if err := os.WriteFile(path, creds, 0o600); err != nil {
		t.Fatalf("write %s credentials: %v", name, err)
	}

	c := &client{}
	conn, err := nats.Connect(d.broker.URL,
		nats.UserCredentials(path),
		nats.CustomInboxPrefix(natsauth.InboxPrefix(name)),
		nats.Timeout(10*time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) { c.record(err) }),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				c.record(err)
			}
		}),
	)
	if err != nil {
		t.Fatalf("connect as %s: %v", name, err)
	}
	t.Cleanup(conn.Close)
	c.conn = conn
	return c
}

func connectAsService(t *testing.T, d deployment, p natsauth.Principal) *client {
	t.Helper()
	identity, ok := d.set.Services[p]
	if !ok {
		t.Fatalf("no identity for %s", p)
	}
	return connectAs(t, d, string(p), identity, identity.Seed)
}

func connectAsAgent(t *testing.T, d deployment, id string) *client {
	t.Helper()
	identity, ok := d.set.Agents[id]
	if !ok {
		t.Fatalf("no identity for agent %s", id)
	}
	return connectAs(t, d, id, identity, d.seeds[id])
}

func connectAsBootstrap(t *testing.T, d deployment, token string) *client {
	t.Helper()
	identity, ok := d.set.Bootstraps[token]
	if !ok {
		t.Fatalf("no identity for token %s", token)
	}
	return connectAs(t, d, token, identity, identity.Seed)
}

// publishPermitted asserts the broker accepted a publish. The payload is a
// single byte and carries no Keystone envelope: C03.md § 12 puts publishing a
// message at C06 and C08, and these cases assert an authorization outcome.
func (c *client) publishPermitted(t *testing.T, subject string) {
	t.Helper()
	if err := c.conn.Publish(subject, []byte{0}); err != nil {
		t.Fatalf("publish to %s: %v", subject, err)
	}
	if found := c.settle(t); len(found) > 0 {
		t.Fatalf("publish to %s was refused: %v", subject, found)
	}
}

// subscribePermitted asserts the broker accepted a subscription.
func (c *client) subscribePermitted(t *testing.T, subject string) {
	t.Helper()
	sub, err := c.conn.SubscribeSync(subject)
	if err != nil {
		t.Fatalf("subscribe to %s: %v", subject, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if found := c.settle(t); len(found) > 0 {
		t.Fatalf("subscribe to %s was refused: %v", subject, found)
	}
}

// subscribeToOwnInboxPermitted exercises the inbox grant through the library's
// own request machinery rather than a hand-built subject, because the inbox
// subtree exists so that replies arrive: a subject this test invented would
// prove the permission and not the thing it is for.
func (c *client) subscribeToOwnInboxPermitted(t *testing.T) {
	t.Helper()
	sub, err := c.conn.SubscribeSync(c.conn.NewRespInbox())
	if err != nil {
		t.Fatalf("subscribe to own inbox: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if found := c.settle(t); len(found) > 0 {
		t.Fatalf("subscribing to its own inbox was refused: %v", found)
	}
}
