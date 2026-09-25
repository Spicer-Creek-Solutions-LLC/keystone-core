//go:build contract

package enrollmentcontract

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The topology ADR-0010 § 2 designs and compose.yaml runs: two internal
// networks, the broker the only service on both, no published port. Built with
// the Docker CLI rather than compose so each case owns its processes -- a crash
// case kills and restarts one of them -- and TestTopologyMatchesCompose holds
// the two definitions to the same shape.

const (
	brokerImage      = "nats:2.15.0-alpine"
	brokerAlias      = "broker"
	brokerRuntimeDir = "/etc/nats"
	agentConfig      = "/etc/keystone/keystone-agent.toml"
	agentStateDir    = "/var/lib/keystone-agent"
	agentBundle      = "/root/bundle.json"
	agentLog         = "/tmp/agent.log"
)

type topology struct {
	t         *testing.T
	dep       *deployment
	prefix    string
	serverNet string
	agentNet  string
	broker    string

	mu       sync.Mutex
	cleanups []func()
}

var topologySeq struct {
	sync.Mutex
	n int
}

func newTopology(t *testing.T) *topology {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("the enrollment contract requires Docker: %v", err)
	}
	buildImage(t)
	topologySeq.Lock()
	topologySeq.n++
	n := topologySeq.n
	topologySeq.Unlock()
	prefix := fmt.Sprintf("keystone-c05-%d-%d", time.Now().UnixNano(), n)
	tp := &topology{t: t, dep: newDeployment(t), prefix: prefix,
		serverNet: prefix + "-server-net", agentNet: prefix + "-agent-net", broker: prefix + "-broker"}
	// Cleanups run in reverse: containers first, networks last.
	t.Cleanup(func() {
		tp.mu.Lock()
		defer tp.mu.Unlock()
		for i := len(tp.cleanups) - 1; i >= 0; i-- {
			tp.cleanups[i]()
		}
	})
	for _, net := range []string{tp.serverNet, tp.agentNet} {
		if out, err := exec.Command("docker", "network", "create", "--internal", "--label", containerLabel, net).CombinedOutput(); err != nil {
			t.Fatalf("create network %s: %v\n%s", net, err, out)
		}
		net := net
		tp.cleanup(func() { exec.Command("docker", "network", "rm", net).Run() })
	}
	tp.startBroker()
	return tp
}

func (tp *topology) cleanup(f func()) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.cleanups = append(tp.cleanups, f)
}

// startBroker runs the pinned broker on the generator's own configuration,
// copied in rather than bind-mounted (see C03's fixture for why), with
// protocol tracing enabled from the command line so broker-visible bytes can be
// read. The alias is the name its certificate carries, on both networks.
func (tp *topology) startBroker() {
	t := tp.t
	dir := t.TempDir()
	if err := tp.dep.nats.Write(dir); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("docker", "create", "--name", tp.broker, "--label", containerLabel,
		"--network", tp.serverNet, "--network-alias", brokerAlias,
		brokerImage, "-c", brokerRuntimeDir+"/nats-server.conf", "-DV").CombinedOutput(); err != nil {
		t.Fatalf("create broker: %v\n%s", err, out)
	}
	tp.cleanup(func() {
		if t.Failed() {
			logs, _ := exec.Command("docker", "logs", "--tail", "40", tp.broker).CombinedOutput()
			t.Logf("broker log (tail):\n%s", logs)
		}
		exec.Command("docker", "rm", "--force", tp.broker).Run()
	})
	if out, err := exec.Command("docker", "network", "connect", "--alias", brokerAlias, tp.agentNet, tp.broker).CombinedOutput(); err != nil {
		t.Fatalf("attach broker to the agent network: %v\n%s", err, out)
	}
	abs, _ := filepath.Abs(dir)
	if out, err := exec.Command("docker", "cp", abs+"/.", tp.broker+":"+brokerRuntimeDir).CombinedOutput(); err != nil {
		t.Fatalf("copy broker configuration: %v\n%s", err, out)
	}
	if out, err := exec.Command("docker", "start", tp.broker).CombinedOutput(); err != nil {
		t.Fatalf("start broker: %v\n%s", err, out)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		logs, _ := exec.Command("docker", "logs", tp.broker).CombinedOutput()
		if strings.Contains(string(logs), "Server is ready") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the broker did not become ready:\n%s", logs)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// brokerTrace is everything the broker logged, which with -DV includes every
// protocol line and payload it saw.
func (tp *topology) brokerTrace() string {
	out, _ := exec.Command("docker", "logs", tp.broker).CombinedOutput()
	return string(out)
}

// agentBox is one agent host on the agent network.
type agentBox struct {
	*box
}

// agent starts an agent host with its configuration and the broker's CA. It
// holds no credential until a bundle is delivered to it.
func (tp *topology) agent(name string) *agentBox {
	t := tp.t
	t.Helper()
	b := &box{t: t, name: tp.prefix + "-" + name, dep: tp.dep, tp: tp}
	if out, err := exec.Command("docker", "run", "-d", "--name", b.name, "--label", containerLabel,
		"--network", tp.agentNet, image).CombinedOutput(); err != nil {
		t.Fatalf("start agent: %v\n%s", err, out)
	}
	tp.cleanup(func() {
		if t.Failed() {
			log, _ := b.exec("", "cat", agentLog)
			t.Logf("%s log:\n%s", name, log)
		}
		exec.Command("docker", "rm", "--force", b.name).Run()
	})
	b.sh("mkdir -p /etc/keystone " + agentStateDir + " && chmod 0700 " + agentStateDir)
	b.writeFile("/etc/keystone/ca.pem", string(tp.dep.nats.TLS.CACertPEM), 0o644)
	a := &agentBox{b}
	a.configure(brokerAlias)
	return a
}

func (a *agentBox) configure(serverName string) {
	a.writeFile(agentConfig, fmt.Sprintf(`[%s]
%s = %q
%s = %q
%s = %q

[%s]
%s = %q
%s = %q
%s = %q
`, agentTableNATS, agentKeyBrokerURL, "tls://"+brokerAlias+":4222", agentKeyBrokerCA, "/etc/keystone/ca.pem",
		agentKeyBrokerName, serverName,
		agentTableAgent, agentKeyIdentityPath, agentStateDir+"/identity.json",
		agentKeyPrivateKeysPath, agentStateDir+"/keys.json", agentKeyLedgerPath, agentStateDir+"/ledger.db"), 0o600)
}

// deliver writes a bundle to the agent host out of band, as an operator would.
func (a *agentBox) deliver(bundle string, mode os.FileMode) {
	a.writeFile(agentBundle, bundle, mode)
}

// enroll runs the production agent's enrollment with the delivered bundle.
func (a *agentBox) enroll(args ...string) cliResult {
	a.t.Helper()
	if len(args) == 0 {
		args = []string{"--token-file", agentBundle}
	}
	cmd := exec.Command("docker", append([]string{"exec", "-e", "KEYSTONE_AGENT_CONFIG=" + agentConfig, a.name,
		"sh", "-c", `exec keystone-agent enroll "$@" 2>` + agentLog, "sh"}, args...)...)
	return runCmd(a.t, cmd, "")
}

// enrollStdin runs enrollment with the bundle on standard input.
func (a *agentBox) enrollStdin(bundle string) cliResult {
	a.t.Helper()
	cmd := exec.Command("docker", "exec", "-i", "-e", "KEYSTONE_AGENT_CONFIG="+agentConfig, a.name,
		"sh", "-c", `exec keystone-agent enroll --token-file - 2>`+agentLog)
	return runCmd(a.t, cmd, bundle)
}

func runCmd(t *testing.T, cmd *exec.Cmd, stdin string) cliResult {
	t.Helper()
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	err := cmd.Run()
	r := cliResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		r.Exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %v: %v", cmd.Args, err)
	}
	return r
}

// log is the agent's standard error from its last run.
func (a *agentBox) log() string {
	out, _ := a.exec("", "cat", agentLog)
	return out
}
