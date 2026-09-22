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

	deployment, err := natsauth.Generate(natsauth.Config{FleetSize: 4, Agents: agents})
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
	conn, err := nats.Connect(url,
		nats.UserCredentials(credsPath),
		nats.CustomInboxPrefix(natsauth.InboxPrefix(agentOne)),
		nats.Timeout(15*time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			select {
			case violations <- err:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("the topology's broker refused a credential this deployment generated, "+
			"so it is not running this configuration: %v", err)
	}
	defer conn.Close()

	// Permitted, and it has to be, or the refusal below would prove only that
	// the connection is useless.
	if err := conn.Publish("ks.out."+agentOne+".result", []byte{0}); err != nil {
		t.Fatal(err)
	}
	// Refused. ARCH-NATS-003: no wildcard across agent identifiers.
	if err := conn.Publish("ks.job."+agentTwo+".cmd", []byte{0}); err != nil {
		t.Fatal(err)
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-violations:
		if !strings.Contains(strings.ToLower(err.Error()), "permissions violation") {
			t.Fatalf("the broker refused something, but not as a permissions violation: %v", err)
		}
		if !strings.Contains(err.Error(), agentTwo) {
			t.Fatalf("a permissions violation arrived, but for the wrong subject, so the "+
				"permitted publish was refused too: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the broker permitted one agent to publish another agent's command, " +
			"so it is not running the generated authorization configuration")
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

	// The broker's networks are internal and publish no host port -- TESTING.md
	// requires that -- so it is reached the way the contract reaches its own
	// broker: by container address from a sibling, or through this job
	// container's own namespace when it has one.
	out := output(t, "docker", "compose", "-f", composeFile, "ps", "--quiet", "broker")
	id := strings.TrimSpace(out)
	if id == "" {
		t.Fatal("compose started no broker container")
	}
	address := strings.TrimSpace(output(t, "docker", "inspect",
		"--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", id))
	first := strings.Fields(address)
	if len(first) == 0 {
		t.Fatal("the broker has no container address; nothing can connect to it")
	}
	return "nats://" + first[0] + ":4222"
}

// compose runs a compose subcommand and ignores its failure, which is right for
// teardown and wrong for anything else: on entry there may be nothing to tear
// down, and on exit the test has already reported whatever went wrong.
func compose(t *testing.T, args ...string) {
	t.Helper()
	_ = exec.Command("docker", append([]string{"compose", "-f", composeFile}, args...)...).Run()
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
