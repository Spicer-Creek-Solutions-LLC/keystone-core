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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "docker", "run", "--detach", "--rm",
		"--name", name,
		"--label", "io.keystone-core.test=c03-authorization",
		"--publish", "127.0.0.1::4222",
		"--volume", abs+":/etc/nats:ro",
		authorizationBrokerImage, "-c", "/etc/nats/nats-server.conf", "-js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("start authorization broker: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		cleanup := exec.Command("docker", "rm", "--force", name)
		if output, err := cleanup.CombinedOutput(); err != nil && !strings.Contains(string(output), "No such container") {
			t.Errorf("remove authorization broker: %v\n%s", err, output)
		}
	})

	port, err := exec.CommandContext(ctx, "docker", "port", name, "4222/tcp").CombinedOutput()
	if err != nil {
		t.Fatalf("find authorization broker port: %v\n%s", err, port)
	}
	address := strings.TrimSpace(string(port))
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
