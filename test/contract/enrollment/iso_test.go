//go:build contract

package enrollmentcontract

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"go.keystone-core.io/keystone-core/internal/enrollment"
	"go.keystone-core.io/keystone-core/internal/natsauth"
	"go.keystone-core.io/keystone-core/internal/protocol"
)

// ISO-1 runs over compose.yaml itself: the topology file the project ships
// for acceptance, not this package's copy of its shape. It pays the workflow's
// owed "journey across the production Docker topology".

var composeFile = filepath.Join(repoRoot(), "test", "e2e", "docker", "compose.yaml")

const (
	isoProject          = "keystone-c05-iso"
	natsConfigVolume    = "keystone-nats-config"
	serverConfigVolume  = "keystone-server-config"
	agentConfigVolume   = "keystone-agent-config"
	composeAdminGroup   = "wheel" // Alpine's own group, which root is in: the CLI runs as root
	composeStorePath    = "/var/lib/keystone/server.db"
	composeAgentBundle  = "/root/bundle.json"
	composeServerConfig = "/etc/keystone"
)

func compose(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command("docker", append([]string{"compose", "-p", isoProject, "-f", composeFile}, args...)...).CombinedOutput()
	return string(out), err
}

func mustCompose(t *testing.T, args ...string) string {
	t.Helper()
	out, err := compose(t, args...)
	if err != nil {
		t.Fatalf("docker compose %v: %v\n%s", args, err, out)
	}
	return out
}

// seedVolume replaces a named volume with a directory's contents, through the
// daemon rather than a bind mount (see compose.yaml for why).
func seedVolume(t *testing.T, volume, dir string) {
	t.Helper()
	exec.Command("docker", "volume", "rm", "--force", volume).Run()
	if out, err := exec.Command("docker", "volume", "create", volume).CombinedOutput(); err != nil {
		t.Fatalf("create %s: %v\n%s", volume, err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "volume", "rm", "--force", volume).Run() })
	seeder := volume + "-seed"
	exec.Command("docker", "rm", "--force", seeder).Run()
	if out, err := exec.Command("docker", "create", "--name", seeder, "--volume", volume+":/seed", "alpine:3", "true").CombinedOutput(); err != nil {
		t.Fatalf("seeder: %v\n%s", err, out)
	}
	defer exec.Command("docker", "rm", "--force", seeder).Run()
	abs, _ := filepath.Abs(dir)
	if out, err := exec.Command("docker", "cp", abs+"/.", seeder+":/seed").CombinedOutput(); err != nil {
		t.Fatalf("seed %s: %v\n%s", volume, err, out)
	}
}

func writeTree(t *testing.T, files map[string]struct {
	content string
	mode    os.FileMode
}) string {
	t.Helper()
	dir := t.TempDir()
	for name, f := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.content), f.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, f.mode); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type file = struct {
	content string
	mode    os.FileMode
}

func TestISO1JourneyUsesOnlyBrokerBetweenServerAndAgents(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("the enrollment contract requires Docker: %v", err)
	}
	// Teardown on entry too: a cancelled run leaves the project behind.
	compose(t, "down", "--volumes", "--timeout", "5")
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := compose(t, "logs", "--no-color", "--tail", "40", "server")
			t.Logf("server log:\n%s", logs)
		}
		compose(t, "down", "--volumes", "--timeout", "5")
	})

	dep := newDeployment(t)
	natsDirOut := t.TempDir()
	if err := dep.nats.Write(natsDirOut); err != nil {
		t.Fatal(err)
	}
	seedVolume(t, natsConfigVolume, natsDirOut)

	serverFiles := map[string]file{
		"nats/ca.pem":             {string(dep.nats.TLS.CACertPEM), 0o644},
		"nats/account-signing.nk": {string(dep.nats.Keystone.SigningSeed), 0o600},
		"service-signing.key":     {enrollment.EncodeKey(protocol.MarshalSigningKey(dep.signing)) + "\n", 0o600},
		"result-encryption.pub":   {enrollment.EncodeKey(dep.result.Recipient().Bytes()) + "\n", 0o644},
		"keystone-server.toml":    {composeServerConf(), 0o600},
	}
	for _, p := range []natsauth.Principal{natsauth.EnrollmentService, natsauth.PresenceConsumer} {
		creds, err := natsauth.Credentials(dep.nats.Services[p])
		if err != nil {
			t.Fatal(err)
		}
		serverFiles["nats/"+string(p)+".creds"] = file{string(creds), 0o600}
	}
	seedVolume(t, serverConfigVolume, writeTree(t, serverFiles))
	seedVolume(t, agentConfigVolume, writeTree(t, map[string]file{
		"ca.pem":              {string(dep.nats.TLS.CACertPEM), 0o644},
		"keystone-agent.toml": {composeAgentConf(), 0o600},
	}))

	mustCompose(t, "up", "--detach", "--build", "--wait", "broker", "server", "agent-1", "agent-2")
	waitForSocket(t)

	// The journey: the operator issues on the server's host, delivers each
	// bundle out of band, and each agent enrolls across the broker.
	var agents []tokenBundleV1
	for _, svc := range []string{"agent-1", "agent-2"} {
		out, err := exec.Command("docker", "compose", "-p", isoProject, "-f", composeFile,
			"exec", "-T", "server", "keystone", "enroll", "create", "--agent-name", svc).Output()
		if err != nil {
			t.Fatalf("enroll create for %s: %v", svc, err)
		}
		var bundle tokenBundleV1
		if err := json.Unmarshal(out, &bundle); err != nil {
			t.Fatalf("not a bundle: %v\n%s", err, out)
		}
		deliver := exec.Command("docker", "compose", "-p", isoProject, "-f", composeFile,
			"exec", "-T", svc, "sh", "-c", "umask 077 && cat > "+composeAgentBundle)
		deliver.Stdin = bytes.NewReader(out)
		if o, err := deliver.CombinedOutput(); err != nil {
			t.Fatalf("deliver to %s: %v\n%s", svc, err, o)
		}
		if o, err := compose(t, "exec", "-T", svc, "keystone-agent", "enroll", "--token-file", composeAgentBundle); err != nil {
			t.Fatalf("%s did not enroll: %v\n%s", svc, err, o)
		}
		agents = append(agents, bundle)
	}

	db := composeStore(t)
	for _, bundle := range agents {
		var state, token string
		if err := db.QueryRow(`SELECT a.state, e.state FROM agents a JOIN enrollment e ON e.agent_id = a.agent_id WHERE a.agent_id = ?`,
			bundle.AgentID).Scan(&state, &token); err != nil || state != "active" || token != "spent" {
			t.Fatalf("agent %s after the journey: %q %q %v", bundle.AgentID, state, token, err)
		}
	}

	// Only the broker between them: the server shares no network with an
	// agent, the agents share none with each other, and a probe finds no
	// route. The probe's control is that the server reaches the broker.
	shape := composeShape(t)
	for svc, nets := range shape {
		t.Logf("%s: %v", svc, nets)
	}
	for _, pair := range [][2]string{{"server", "agent-1"}, {"server", "agent-2"}, {"agent-1", "agent-2"}} {
		if shared := intersect(shape[pair[0]], shape[pair[1]]); len(shared) != 0 {
			t.Fatalf("%s and %s share %v", pair[0], pair[1], shared)
		}
		ip := serviceIP(t, pair[1])
		if out, err := compose(t, "exec", "-T", pair[0], "ping", "-c", "1", "-W", "2", ip); err == nil {
			t.Fatalf("%s reached %s at %s directly:\n%s", pair[0], pair[1], ip, out)
		}
	}
	if out, err := compose(t, "exec", "-T", "server", "ping", "-c", "1", "-W", "2", "broker"); err != nil {
		t.Fatalf("control: the probe cannot reach the broker from the server either, so it proves nothing:\n%s", out)
	}

	// No agent private key on the server.
	for _, svc := range []string{"agent-1", "agent-2"} {
		keysJSON, err := exec.Command("docker", "compose", "-p", isoProject, "-f", composeFile,
			"exec", "-T", svc, "cat", "/var/lib/keystone-agent/keys.json").Output()
		if err != nil {
			t.Fatal(err)
		}
		var k agentKeyArtifactV1
		if err := json.Unmarshal(keysJSON, &k); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{k.NATSSeed, k.SigningPrivateKey, k.DecryptionPrivateKey} {
			out, _ := compose(t, "exec", "-T", "server", "sh", "-c",
				"find / -xdev -type f -exec grep -lF '"+secret+"' {} \\; 2>/dev/null; echo search-done")
			if strings.TrimSpace(out) != "search-done" {
				t.Fatalf("%s's private key is on the server:\n%s", svc, out)
			}
		}
	}
}

func composeServerConf() string {
	return fmt.Sprintf(`[operator]
admin_group = %q

[store]
path = %q

[nats]
url = "tls://broker:4222"
ca_file = "%[3]s/nats/ca.pem"
server_name = "broker"
enrollment_credentials = "%[3]s/nats/enrollment-service.creds"
presence_credentials = "%[3]s/nats/presence-consumer.creds"
account_signing_seed = "%[3]s/nats/account-signing.nk"

[service]
signing_private_key_file = "%[3]s/service-signing.key"
result_encryption_public_key_file = "%[3]s/result-encryption.pub"
`, composeAdminGroup, composeStorePath, composeServerConfig)
}

func composeAgentConf() string {
	return `[nats]
url = "tls://broker:4222"
ca_file = "/etc/keystone/ca.pem"
server_name = "broker"

[agent]
identity_path = "/var/lib/keystone-agent/identity.json"
private_keys_path = "/var/lib/keystone-agent/keys.json"
ledger_path = "/var/lib/keystone-agent/ledger.db"
`
}

func waitForSocket(t *testing.T) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if _, err := compose(t, "exec", "-T", "server", "test", "-S", "/run/keystone/operator.sock"); err == nil {
			return
		}
		sleepBrief()
	}
	t.Fatal("the compose server never opened its socket")
}

// composeStore copies the server's store out of its tmpfs as a tar stream --
// docker cp cannot read a tmpfs -- and opens the copy read-only.
func composeStore(t *testing.T) *sql.DB {
	t.Helper()
	out, err := exec.Command("docker", "compose", "-p", isoProject, "-f", composeFile,
		"exec", "-T", "server", "tar", "-C", "/var/lib/keystone", "-cf", "-", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tr := tar.NewReader(bytes.NewReader(out))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, _ := io.ReadAll(tr)
		os.WriteFile(filepath.Join(dir, filepath.Base(h.Name)), b, 0o600)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// composeShape is each running service's networks, from the daemon.
func composeShape(t *testing.T) map[string][]string {
	t.Helper()
	shape := map[string][]string{}
	for _, svc := range []string{"broker", "server", "agent-1", "agent-2"} {
		id := strings.TrimSpace(mustCompose(t, "ps", "--quiet", svc))
		out, err := exec.Command("docker", "inspect", "--format", "{{json .NetworkSettings.Networks}}", id).Output()
		if err != nil {
			t.Fatal(err)
		}
		var nets map[string]any
		json.Unmarshal(out, &nets)
		for n := range nets {
			shape[svc] = append(shape[svc], strings.TrimPrefix(n, isoProject+"_"))
		}
		sort.Strings(shape[svc])
	}
	return shape
}

func serviceIP(t *testing.T, svc string) string {
	t.Helper()
	id := strings.TrimSpace(mustCompose(t, "ps", "--quiet", svc))
	out, err := exec.Command("docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", id).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(out))[0]
}

func intersect(a, b []string) []string {
	var out []string
	for _, x := range a {
		for _, y := range b {
			if x == y {
				out = append(out, x)
			}
		}
	}
	return out
}
