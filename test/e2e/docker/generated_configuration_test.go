package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

// The volume compose.yaml's broker mounts. Named here and there and nowhere
// else, because it is the whole interface between the generator's output and
// the topology.
const configVolume = "keystone-nats-config"

// Denied to every principal ADR-0004 section 4 defines, and published last so
// its refusal marks the point after which no earlier one can still arrive.
const controlSubject = "$SYS.keystone.topology-control"

// compose.yaml's project name, and the broker network a job container joins.
// Both are compose.yaml's to declare and this file's to read; if either moves,
// the network connect below fails loudly rather than the test timing out.
const (
	composeProject = "keystone-acceptance"
	brokerNetwork  = "server-net"
)

// The broker's TLS identity (ADR-0002 § 12, G49). compose.yaml names the
// service "broker" and mounts the generated directory at /etc/nats; clients
// dial its address on the network and verify the certificate against the name.
const (
	brokerName       = "broker"
	brokerRuntimeDir = "/etc/nats"
)

// C03.md § 3.2 requires "broker configuration the topology consumes", and until
// C03-I6 nothing showed that it did. The container suite ran `docker compose
// config`, which parses the topology and never brings it up, so the broker
// could have been running anything -- or, as it was, running nothing in
// particular.
//
// WHAT THIS ASSERTS IS NOT THAT THE BROKER STARTED. A broker that started would
// prove a container ran, not that it ran THIS configuration, and the difference
// is the whole of the output. It connects with a credential the generator made
// and asserts an authorization outcome only these JWTs can produce: an agent is
// refused another agent's command subject while its own identity is accepted. A
// broker running any other configuration either refuses the credential outright
// or permits the subject.
func TestTheTopologyRunsTheGeneratedConfiguration(t *testing.T) {
	requireDocker(t)

	const agentOne, agentTwo = "agent-one", "agent-two"
	seeds := map[string][]byte{}
	var agents []natsauth.AgentKey
	for _, id := range []string{agentOne, agentTwo} {
		// ADR-0003 § 4 generates the agent's NATS key ON THE AGENT and sends
		// only the public half. The seed stays here; Generate never sees one.
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
		seeds[id] = seed
		agents = append(agents, natsauth.AgentKey{ID: id, PublicKey: pub})
	}

	deployment, err := natsauth.Generate(natsauth.Config{
		FleetSize: 4, Agents: agents, BrokerNames: []string{brokerName}, RuntimeDir: brokerRuntimeDir,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	dir := t.TempDir()
	if err := deployment.Write(dir); err != nil {
		t.Fatalf("write deployment: %v", err)
	}

	seedConfigVolume(t, dir)
	url := composeUp(t)

	// The credential the generator made, against the broker compose started.
	creds, err := jwt.FormatUserConfig(deployment.Agents[agentOne].JWT, seeds[agentOne])
	if err != nil {
		t.Fatal(err)
	}
	credsPath := filepath.Join(dir, "agent.creds")
	if err := os.WriteFile(credsPath, creds, 0o600); err != nil {
		t.Fatal(err)
	}

	violations := make(chan error, 8)
	conn, err := nats.Connect(url, append(brokerTLS(t, deployment),
		nats.UserCredentials(credsPath),
		nats.CustomInboxPrefix(natsauth.InboxPrefix(agentOne)),
		nats.Timeout(15*time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			select {
			case violations <- err:
			default:
			}
		}),
	)...)
	if err != nil {
		t.Fatalf("the topology's broker refused a credential this deployment generated, "+
			"so it is not running this configuration: %v", err)
	}
	defer conn.Close()

	// Permitted, and it has to be, or the refusal below would prove only that
	// the connection is useless.
	publish(t, conn, "ks.out."+agentOne+".result")
	// Refused. ARCH-NATS-003: no wildcard across agent identifiers.
	publish(t, conn, "ks.job."+agentTwo+".cmd")
	// THE CONTROL, AND IT IS WHAT MAKES THE SET COMPLETE. A permissions
	// violation is dispatched asynchronously, after Flush has returned, so
	// reading one callback proves only that one arrived. This subject is denied
	// to every principal, callbacks are dispatched in order, and once its
	// refusal is in hand every earlier one must be too -- the same
	// synchronisation the authorization contract uses, and for the same reason.
	publish(t, conn, controlSubject)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(20 * time.Second)
	var seen []string
	for {
		select {
		case err := <-violations:
			if !strings.Contains(strings.ToLower(err.Error()), "permissions violation") {
				t.Fatalf("the broker refused something, but not as a permissions violation: %v", err)
			}
			if strings.Contains(err.Error(), controlSubject) {
				// Everything earlier has arrived. Assert the WHOLE set.
				var wrong []string
				denied := 0
				for _, v := range seen {
					if strings.Contains(v, agentTwo) {
						denied++
						continue
					}
					wrong = append(wrong, v)
				}
				if len(wrong) > 0 {
					t.Fatalf("the broker refused an operation this agent is granted, so its "+
						"refusal of %s proves nothing in particular: %v", agentTwo, wrong)
				}
				if denied != 1 {
					t.Fatalf("expected exactly one refusal, for %s; got %d: %v", agentTwo, denied, seen)
				}
				return
			}
			seen = append(seen, err.Error())
		case <-deadline:
			t.Fatalf("the broker never refused %s, so this connection cannot report a "+
				"permissions violation and nothing it asserts means anything", controlSubject)
		}
	}
}

// seedConfigVolume puts the generated directory where compose can mount it.
//
// A bind mount cannot do this. Its source is resolved by the DAEMON, on the
// host, and CI runs this suite inside a job container whose t.TempDir() the
// daemon cannot see -- measured: a broker given such a mount does not start.
// `docker cp` streams through the daemon API instead, so it does not care whose
// filesystem the source is on.
func seedConfigVolume(t *testing.T, dir string) {
	t.Helper()

	// TEARDOWN ON ENTRY AS WELL AS EXIT. The container suite already states why:
	// these containers are siblings on the host's daemon and outlive the job
	// that created them, and t.Cleanup does not run when a test binary is killed
	// or a run is cancelled -- which this forge does on push. A leftover broker
	// holds the volume, and `docker volume rm` then fails rather than the run
	// starting clean.
	compose(t, "down", "--volumes", "--timeout", "5")
	run(t, "docker", "volume", "rm", "--force", configVolume)
	run(t, "docker", "volume", "create", configVolume)
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", "--force", configVolume).Run() })

	seeder := fmt.Sprintf("keystone-config-seed-%d", time.Now().UnixNano())
	run(t, "docker", "create", "--name", seeder, "--volume", configVolume+":/etc/nats", "alpine", "true")
	defer func() { _ = exec.Command("docker", "rm", "--force", seeder).Run() }()
	run(t, "docker", "cp", dir+"/.", seeder+":/etc/nats")
}

// composeUp brings the topology up and returns a URL for its broker.
func composeUp(t *testing.T) string {
	t.Helper()
	run(t, "docker", "compose", "-f", composeFile, "up", "--detach", "--wait", "broker")
	t.Cleanup(func() { compose(t, "down", "--volumes", "--timeout", "5") })

	// THE BROKER IS ON INTERNAL NETWORKS AND PUBLISHES NO HOST PORT. TESTING.md
	// requires both, and ARCH-COMM-002 is why the networks are internal at all,
	// so neither can be relaxed to make a test easier to write.
	//
	// From the host that is enough: the bridge is routable. From a job
	// container it is not -- CI runs this suite inside one, on a network of its
	// own, and Docker isolates bridge networks from each other. Measured:
	// `dial tcp 172.19.0.2:4222: i/o timeout`.
	//
	// So the JOB CONTAINER joins the broker's network for the duration, rather
	// than the broker leaving its own. The topology is unchanged and the test
	// reaches it.
	joinBrokerNetwork(t)

	out := output(t, "docker", "compose", "-f", composeFile, "ps", "--quiet", "broker")
	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatal("compose started no broker container")
	}
	// THE ADDRESS ON THE NETWORK WE JOINED, not whichever the daemon lists
	// first. The broker is on every internal network, and a Go template ranging
	// over a map visits its keys SORTED -- so `agent-1-net` comes before
	// `server-net`, and reading the first address dials the network this test
	// is not on. That failed as `dial tcp ...: i/o timeout`, which names the
	// symptom and not the cause.
	network := composeProject + "_" + brokerNetwork
	address := strings.TrimSpace(output(t, "docker", "inspect",
		"--format", fmt.Sprintf("{{ (index .NetworkSettings.Networks %q).IPAddress }}", network), id))
	if address == "" {
		t.Fatalf("the broker has no address on %s; nothing can connect to it", network)
	}
	return "nats://" + address + ":4222"
}

// joinBrokerNetwork attaches this process's own container, when it has one the
// daemon can see, to the network compose put the broker on.
//
// The question is not "am I in a container" but "can the daemon I am talking to
// see the container I am in" -- only then can it be attached to anything. Asking
// the daemon answers both at once, and on a developer machine the answer is no
// and nothing needs doing, because the bridge is routable from the host.
func joinBrokerNetwork(t *testing.T) {
	t.Helper()
	host, err := os.Hostname()
	if err != nil || host == "" {
		return
	}
	if exec.Command("docker", "inspect", "--format", "{{.Id}}", host).Run() != nil {
		return
	}
	network := composeProject + "_" + brokerNetwork
	run(t, "docker", "network", "connect", network, host)
	t.Cleanup(func() { _ = exec.Command("docker", "network", "disconnect", network, host).Run() })
}

// compose runs a compose subcommand and ignores its failure, which is right for
// teardown and wrong for anything else: on entry there may be nothing to tear
// down, and on exit the test has already reported whatever went wrong.
func compose(t *testing.T, args ...string) {
	t.Helper()
	_ = exec.Command("docker", append([]string{"compose", "-f", composeFile}, args...)...).Run()
}

func publish(t *testing.T, conn *nats.Conn, subject string) {
	t.Helper()
	if err := conn.Publish(subject, []byte{0}); err != nil {
		t.Fatalf("publish to %s: %v", subject, err)
	}
}

// brokerTLS is how a client reaches the topology's broker: TLS-first, verifying
// the certificate against the deployment's CA and brokerName.
func brokerTLS(t *testing.T, d *natsauth.Deployment) []nats.Option {
	t.Helper()
	cfg, err := natsauth.ClientTLSConfig(d.TLS.CACertPEM, brokerName)
	if err != nil {
		t.Fatalf("client TLS configuration: %v", err)
	}
	return []nats.Option{nats.Secure(cfg), nats.TLSHandshakeFirst()}
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func output(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}
