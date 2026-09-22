//go:build contract

package authorizationcontract

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const authorizationBrokerImage = "nats:2.15.0-alpine"

type brokerFixture struct {
	URL string
}

// startAuthorizationBroker starts the exact broker version C03 is measured
// against with configuration produced by the production generator. It accepts
// no fallback configuration and no committed credential fixture.
func startAuthorizationBroker(t *testing.T, generatedDir string) brokerFixture {
	t.Helper()
	abs, err := filepath.Abs(generatedDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(abs, "nats-server.conf")); err != nil {
		t.Fatalf("production generator did not produce nats-server.conf: %v", err)
	}
	name := fmt.Sprintf("keystone-c03-contract-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	// A port of its own, so two brokers can exist at once without a collision.
	// It is passed as a server argument rather than written into the generated
	// configuration: the configuration is the one a deployment runs, and a
	// test-chosen port is not part of it.
	port := 20000 + int(time.Now().UnixNano()%10000)

	args := []string{"create", "--name", name, "--label", "io.keystone-core.test=c03-authorization"}
	host, shared := jobContainer(ctx)
	if shared {
		// THE BROKER SHARES THIS JOB CONTAINER'S NETWORK NAMESPACE.
		//
		// CI runs the suite inside a job container with the host's socket
		// mounted, so every container it starts is a SIBLING, and the runner
		// puts the job container on a network of its own. Docker isolates
		// bridge networks from each other, so neither a published host port nor
		// the broker's own address reaches it. Measured, from a job container on
		// its own network:
		//
		//   broker on the default bridge, 172.17.0.2:4222 ... unreachable
		//   --network container:<job container>, 127.0.0.1  ... reachable
		//
		// Sharing the namespace makes the broker's listener the job container's
		// own loopback, which is the one address that is certainly reachable
		// from the process making the assertions.
		args = append(args, "--network", "container:"+host)
	} else {
		// On a developer machine there is no job container to share, so the port
		// is published and reached the same way: through loopback.
		args = append(args, "--publish", fmt.Sprintf("127.0.0.1::%d", port))
	}
	args = append(args, authorizationBrokerImage,
		"-c", "/etc/nats/nats-server.conf", "-js", "-p", strconv.Itoa(port))

	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("create authorization broker: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		cleanup := exec.Command("docker", "rm", "--force", name)
		if output, err := cleanup.CombinedOutput(); err != nil && !strings.Contains(string(output), "No such container") {
			t.Errorf("remove authorization broker: %v\n%s", err, output)
		}
	})

	// THE CONFIGURATION IS COPIED IN, NOT BIND-MOUNTED.
	//
	// A bind mount's source path is resolved by the DAEMON, on the host, and the
	// generated directory is a t.TempDir() inside the job container. Mounting it
	// produced a broker that started with no configuration and exited at once,
	// which surfaced as a readiness failure pointing nowhere near the cause.
	// `docker cp` streams through the daemon API and does not care whose
	// filesystem the source is on.
	if out, err := exec.CommandContext(ctx, "docker", "cp", abs+"/.", name+":/etc/nats").CombinedOutput(); err != nil {
		t.Fatalf("copy generated configuration into the broker: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "docker", "start", name).CombinedOutput(); err != nil {
		t.Fatalf("start authorization broker: %v\n%s", err, out)
	}

	address := fmt.Sprintf("127.0.0.1:%d", port)
	if !shared {
		published, err := exec.CommandContext(ctx, "docker", "port", name, fmt.Sprintf("%d/tcp", port)).CombinedOutput()
		if err != nil {
			t.Fatalf("find authorization broker port: %v\n%s", err, published)
		}
		address = strings.TrimSpace(string(published))
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return brokerFixture{URL: "nats://" + address}
		}
		if time.Now().After(deadline) {
			logs, _ := exec.Command("docker", "logs", name).CombinedOutput()
			t.Fatalf("authorization broker did not become ready: %v\n%s", dialErr, logs)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// jobContainer reports this process's own container, when it has one the daemon
// knows about.
//
// The question is not "am I in a container" but "can the daemon I am talking to
// see the container I am in" -- only then can a new container share its network
// namespace. Asking the daemon is the answer to both at once, and it is why this
// does not test for /.dockerenv: that file says something about this process and
// nothing about the daemon on the other end of the socket.
func jobContainer(ctx context.Context) (string, bool) {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "", false
	}
	if err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}}", host).Run(); err != nil {
		return "", false
	}
	return host, true
}
