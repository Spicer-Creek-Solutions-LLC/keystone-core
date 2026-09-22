//go:build contract

package authorizationcontract

import (
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

// The JetStream cases need their targets to EXIST. ADR-0004 § 8 and § 9 require
// each of them to distinguish a permissions violation from a missing stream or
// consumer, and if the target is absent an AUTHORIZED publish fails too -- with
// a different error, but a case that only asserts "this failed" cannot tell
// them apart. C03.md § 12 permits exactly this: "C03 may create a stream or
// consumer as a generated fixture so the $JS.API cases can distinguish a
// permissions denial from a missing target".
//
// THE PROVISIONING IDENTITY IS MINTED HERE, NOT BY THE GENERATOR, and the
// reason is a property worth keeping. ADR-0004 § 4 denies administrative
// $JS.API to every Keystone identity, so a deployment principal cannot create a
// stream. If this user came from internal/natsauth it would be a principal the
// frozen matrix does not contain, and GEN-3 would fail -- correctly. It is
// signed with the account signing key the deployment already exposes, lives
// only for the test, and appears in no generated artifact.
//
// **Who provisions streams in a real deployment is NOT settled here.** ADR-0002
// § 7 defines the two streams and says nothing about the identity that creates
// them, and C06 owns their production configuration. This fixture is a test
// target, not an answer to that question, and the question is raised in the
// pull request rather than decided in it.
func provisionJetStream(t *testing.T, d deployment) {
	t.Helper()

	signing, err := nkeys.FromSeed(d.set.Keystone.SigningSeed)
	if err != nil {
		t.Fatalf("account signing key: %v", err)
	}
	kp, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := kp.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	seed, err := kp.Seed()
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.NewUserClaims(pub)
	claims.Name = "contract-provisioner"
	claims.IssuerAccount = d.set.Keystone.PublicKey
	claims.Permissions.Pub.Allow = jwt.StringList{"$JS.API.>"}
	claims.Permissions.Sub.Allow = jwt.StringList{"_INBOX.>"}
	token, err := claims.Encode(signing)
	if err != nil {
		t.Fatalf("encode provisioner: %v", err)
	}

	conn, err := nats.Connect(d.broker.URL,
		nats.UserJWTAndSeed(token, string(seed)),
		nats.Timeout(10*time.Second))
	if err != nil {
		t.Fatalf("connect as provisioner: %v", err)
	}
	defer conn.Close()

	js, err := conn.JetStream()
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}

	// ADR-0002 § 7's two streams and their subjects. Retention, limits and
	// consumer policy are C06's; what matters here is only that the targets the
	// authorization cases name exist.
	for _, stream := range []*nats.StreamConfig{
		{Name: natsauth.CommandStream, Subjects: []string{"ks.job.*.>"}},
		{Name: natsauth.ResultStream, Subjects: []string{"ks.out.*.result"}},
	} {
		if _, err := js.AddStream(stream); err != nil {
			t.Fatalf("create %s: %v", stream.Name, err)
		}
	}

	// One durable pull consumer per agent on the command stream, with the exact
	// filter ADR-0002 § 7 requires, and one for the result consumer. BOTH agents
	// get one: NEG-12 asserts an agent is refused ANOTHER agent's consumer-next
	// API and must tell that refusal from a consumer that does not exist.
	for _, id := range []string{agentOne, agentTwo} {
		if _, err := js.AddConsumer(natsauth.CommandStream, &nats.ConsumerConfig{
			Durable:       natsauth.CommandConsumer(id),
			FilterSubject: "ks.job." + id + ".>",
			AckPolicy:     nats.AckExplicitPolicy,
			MaxAckPending: 1,
		}); err != nil {
			t.Fatalf("create consumer for %s: %v", id, err)
		}
	}
	if _, err := js.AddConsumer(natsauth.ResultStream, &nats.ConsumerConfig{
		Durable:   natsauth.ResultConsumerName,
		AckPolicy: nats.AckExplicitPolicy,
	}); err != nil {
		t.Fatalf("create result consumer: %v", err)
	}
}
