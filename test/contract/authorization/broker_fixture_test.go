//go:build contract

package authorizationcontract

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

	// THE CONFIGURATION IS COPIED IN, NOT BIND-MOUNTED.
	//
	// A bind mount's source path is resolved by the DAEMON, on the host. CI runs
	// this suite inside a job container with the host's socket mounted, so the
	// generated directory -- a t.TempDir() inside that job container -- does not
	// exist as far as the daemon is concerned. Mounting it produced a broker
	// that started with no configuration and exited immediately, and the first
	// symptom was a readiness failure with no obvious cause.
	//
	// `docker cp` streams through the daemon API, so it does not care whose
	// filesystem the source is on. Create, copy, then start.
	create := exec.CommandContext(ctx, "docker", "create",
		"--name", name,
		"--label", "io.keystone-core.test=c03-authorization",
		authorizationBrokerImage, "-c", "/etc/nats/nats-server.conf", "-js")
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create authorization broker: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		cleanup := exec.Command("docker", "rm", "--force", name)
		if output, err := cleanup.CombinedOutput(); err != nil && !strings.Contains(string(output), "No such container") {
			t.Errorf("remove authorization broker: %v\n%s", err, output)
		}
	})

	if out, err := exec.CommandContext(ctx, "docker", "cp", abs+"/.", name+":/etc/nats").CombinedOutput(); err != nil {
		t.Fatalf("copy generated configuration into the broker: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "docker", "start", name).CombinedOutput(); err != nil {
		t.Fatalf("start authorization broker: %v\n%s", err, out)
	}

	// THE BROKER IS REACHED BY ITS CONTAINER IP, not by a published host port.
	//
	// CI runs this suite inside a job container with the host's docker socket
	// mounted, so every container it starts is a SIBLING rather than a child --
	// CI-RUNNER.md states that as the cost of mounting the socket. A port
	// published with --publish lands on the HOST's loopback, and 127.0.0.1
	// inside the job container is the job container's own. Measured:
	//
	//   host publishes at: 127.0.0.1:32829
	//     from job container via 127.0.0.1: NO
	//     from job container via 172.17.0.3: YES
	//
	// The container IP works from a sibling and from the host, so one mechanism
	// serves both. It assumes a Linux daemon with a routable bridge, which is
	// what CI-RUNNER.md establishes and what the development VM runs; on a
	// Docker Desktop host the bridge is not routable and this fixture will not
	// reach the broker.
	inspect, err := exec.CommandContext(ctx, "docker", "inspect",
		"--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name).CombinedOutput()
	if err != nil {
		t.Fatalf("find authorization broker address: %v\n%s", err, inspect)
	}
	ip := strings.TrimSpace(string(inspect))
	if ip == "" {
		t.Fatalf("the authorization broker has no container address; nothing can connect to it")
	}
	address := ip + ":4222"
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
