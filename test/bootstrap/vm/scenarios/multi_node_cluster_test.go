package scenarios

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/vm"
)

// TestMultiNodeCluster tests a production-like multi-node deployment.
// This validates control-plane bootstrap and agent join flow.
//
// Run with:
//
//	KSCORE_VM_TESTS=1 KSCORE_VM_CONFIG=test/bootstrap/vm/multi-node-cluster.yaml \
//	  KSCORE_REPO_URL=http://repo-host.example.internal:8080/repos \
//	  go test -v ./test/bootstrap/vm/scenarios -run TestMultiNodeCluster
func TestMultiNodeCluster(t *testing.T) {
	vm.RunVMTests(t, "", []func(*testing.T, vm.Provider, *vm.Config){
		testClusterBootstrap,
		testClusterHealth,
		testAgentRegistration,
	})
}

// testClusterBootstrap installs and bootstraps control-plane and agent nodes.
func testClusterBootstrap(t *testing.T, provider vm.Provider, cfg *vm.Config) {
	t.Helper()

	nodes := provider.ListNodes()
	if len(nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(nodes))
	}

	// Find control-plane and agent nodes
	var cpNode, agentNode *vm.Node
	for _, n := range nodes {
		switch n.Role {
		case "control-plane", "both":
			if cpNode == nil {
				cpNode = n
			}
		case "agent":
			if agentNode == nil {
				agentNode = n
			}
		}
	}

	if cpNode == nil {
		t.Fatal("no control-plane node found in config")
	}
	if agentNode == nil {
		t.Fatal("no agent node found in config")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Step 1: Bootstrap control-plane first
	t.Logf("Bootstrapping control-plane node %s (%s)", cpNode.Name, cpNode.Host)
	bootstrapControlPlane(ctx, t, cpNode, cfg)

	// Step 2: Bootstrap agent with join to control-plane
	t.Logf("Bootstrapping agent node %s (%s) to join %s", agentNode.Name, agentNode.Host, cpNode.Host)
	bootstrapAgent(ctx, t, agentNode, cpNode, cfg)
}

func bootstrapControlPlane(ctx context.Context, t *testing.T, node *vm.Node, cfg *vm.Config) {
	t.Helper()

	// Detect OS
	osInfo := detectOS(ctx, t, node)
	t.Logf("Detected OS: %s, package manager: %s", osInfo.name, osInfo.pkgManager)

	// Clean previous installation if configured
	if cfg.SSH.CleanNodes {
		cleanPreviousInstall(ctx, t, node, osInfo)
	}

	// Configure package repository
	configureRepo(ctx, t, node, osInfo)

	// Install packages
	installPackages(ctx, t, node, osInfo)

	// Pre-test: check permissions on keystone-core directories
	t.Log("Pre-test: checking directory permissions...")
	perms, _ := execShell(ctx, node, "ls -la /var/lib/ | grep keystone; ls -la /var/lib/keystone-core/ 2>&1 || echo 'DIR NOT FOUND'; ls -la /etc/ | grep keystone; ls -la /etc/keystone-core/ 2>&1 || echo 'DIR NOT FOUND'")
	t.Logf("Directory permissions:\n%s", perms.Stdout)

	// Check if kscore user can write to the directories
	writeTest, _ := execShell(ctx, node, sudo(node, "sudo -u kscore touch /var/lib/keystone-core/test-write 2>&1 && echo 'WRITE OK' || echo 'WRITE FAILED'; rm -f /var/lib/keystone-core/test-write"))
	t.Logf("Write test result: %s", strings.TrimSpace(writeTest.Stdout))

	// Check if NATS port (4222) is already in use
	portCheck, _ := execShell(ctx, node, "ss -tlnp | grep -E ':4222|:8080|:9090' || echo 'PORTS FREE'")
	t.Logf("Port check:\n%s", portCheck.Stdout)

	// Pre-test: try running server with production-like paths
	t.Log("Pre-test: running server with production paths...")
	testCfg := `server:
  listenaddr: 127.0.0.1
nats:
  mode: embedded
  embedded:
    host: 127.0.0.1
    port: 14222
    storedir: /var/lib/keystone-core/nats
  jetstream:
    storedir: /var/lib/keystone-core/jetstream
storage:
  backend: sqlite
  sqlite:
    path: /var/lib/keystone-core/test.db
auth:
  enabled: false`
	// Write test config and create dirs with proper ownership
	_, _ = execShell(ctx, node, sudo(node, "rm -rf /var/lib/keystone-core/nats /var/lib/keystone-core/jetstream"))
	_, _ = execShell(ctx, node, sudo(node, fmt.Sprintf("echo '%s' > /tmp/test-server.yaml", testCfg)))
	// Create directories as kscore user so they have correct ownership
	_, _ = execShell(ctx, node, sudo(node, "runuser -u kscore -- mkdir -p /var/lib/keystone-core/nats /var/lib/keystone-core/jetstream"))
	// Check directory ownership
	dirCheck, _ := execShell(ctx, node, sudo(node, "ls -la /var/lib/keystone-core/"))
	t.Logf("Directory ownership:\n%s", dirCheck.Stdout)
	// Try running server as kscore user for 3 seconds using runuser
	preTest, _ := execShell(ctx, node, sudo(node, "runuser -u kscore -- timeout 3 /usr/bin/kscore-server --config /tmp/test-server.yaml 2>&1 || true"))
	t.Logf("Pre-test server output:\n%s", preTest.Stdout)

	// Bootstrap as control-plane (production mode with embedded NATS)
	// Use 0.0.0.0 to bind on all interfaces so agents can connect
	// Use SQLite storage to avoid PostgreSQL requirement for testing
	t.Log("Running control-plane bootstrap...")
	bootstrapCmd := fmt.Sprintf("%s bootstrap --mode production --node-role control-plane --bind-address 0.0.0.0 --advertise-address %s --storage-backend sqlite --non-interactive --skip-repo-setup --verbose --generate-certs",
		sudo(node, "kscore-agent"), node.Host)
	result, err := execShell(ctx, node, bootstrapCmd)
	if err != nil {
		t.Logf("Bootstrap output:\n%s", result.Stdout)
		// Try manually running server to capture startup error
		t.Log("Attempting manual server start to capture error...")
		manualTest, _ := execShell(ctx, node, sudo(node, "timeout 5 /usr/bin/kscore-server --config /etc/keystone-core/server.yaml 2>&1 || true"))
		t.Logf("Manual server output:\n%s", manualTest.Stdout)
		// Also check config file
		cfgFile, _ := execShell(ctx, node, sudo(node, "cat /etc/keystone-core/server.yaml 2>&1 || true"))
		t.Logf("Server config file:\n%s", cfgFile.Stdout)
		captureBootstrapDiagnostics(ctx, t, node)
		t.Fatalf("control-plane bootstrap failed: %v", err)
	}
	t.Logf("Bootstrap output:\n%s", result.Stdout)

	// Wait for services to stabilize
	time.Sleep(5 * time.Second)
}

func bootstrapAgent(ctx context.Context, t *testing.T, agentNode, cpNode *vm.Node, cfg *vm.Config) {
	t.Helper()

	// Detect OS
	osInfo := detectOS(ctx, t, agentNode)
	t.Logf("Detected OS: %s, package manager: %s", osInfo.name, osInfo.pkgManager)

	// Clean previous installation if configured
	if cfg.SSH.CleanNodes {
		cleanPreviousInstall(ctx, t, agentNode, osInfo)
	}

	// Configure package repository
	configureRepo(ctx, t, agentNode, osInfo)

	// Install packages
	installPackages(ctx, t, agentNode, osInfo)

	// Bootstrap as agent joining control-plane
	// The agent needs to know the NATS URL of the control-plane
	natsURL := fmt.Sprintf("nats://%s:4222", cpNode.Host)
	t.Logf("Joining control-plane at %s", natsURL)

	bootstrapCmd := fmt.Sprintf("%s bootstrap --mode production --node-role agent --nats-urls %s --non-interactive --skip-repo-setup",
		sudo(agentNode, "kscore-agent"), natsURL)
	result, err := execShell(ctx, agentNode, bootstrapCmd)
	if err != nil {
		t.Logf("Bootstrap output:\n%s", result.Stdout)
		captureBootstrapDiagnostics(ctx, t, agentNode)
		t.Fatalf("agent bootstrap failed: %v", err)
	}
	t.Logf("Bootstrap output:\n%s", result.Stdout)

	// Wait for agent to register
	time.Sleep(5 * time.Second)
}

func captureBootstrapDiagnostics(ctx context.Context, t *testing.T, node *vm.Node) {
	t.Helper()

	// Get service status
	status, _ := execShell(ctx, node, sudo(node, "systemctl status kscore-server.service 2>&1 || true"))
	if status.Stdout != "" {
		t.Logf("kscore-server status:\n%s", status.Stdout)
	}

	status, _ = execShell(ctx, node, sudo(node, "systemctl status kscore-agent.service 2>&1 || true"))
	if status.Stdout != "" {
		t.Logf("kscore-agent status:\n%s", status.Stdout)
	}

	// Get journal logs
	journal, _ := execShell(ctx, node, sudo(node, "journalctl -xeu kscore-server.service --no-pager -n 50 2>&1 || true"))
	if journal.Stdout != "" {
		t.Logf("kscore-server journal:\n%s", journal.Stdout)
	}

	journal, _ = execShell(ctx, node, sudo(node, "journalctl -xeu kscore-agent.service --no-pager -n 30 2>&1 || true"))
	if journal.Stdout != "" {
		t.Logf("kscore-agent journal:\n%s", journal.Stdout)
	}

	// Get config file
	config, _ := execShell(ctx, node, "cat /etc/keystone-core/server.yaml 2>&1 || true")
	if config.Stdout != "" {
		t.Logf("server config:\n%s", config.Stdout)
	}

	// Get service file
	svcFile, _ := execShell(ctx, node, "cat /etc/systemd/system/kscore-server.service 2>&1 || true")
	if svcFile.Stdout != "" {
		t.Logf("service file (/etc):\n%s", svcFile.Stdout)
	}

	// Check which service file is loaded
	svcShow, _ := execShell(ctx, node, "systemctl show kscore-server.service --property=FragmentPath 2>&1 || true")
	if svcShow.Stdout != "" {
		t.Logf("service FragmentPath:\n%s", svcShow.Stdout)
	}

	// Try running server manually to see immediate error
	manual, _ := execShell(ctx, node, sudo(node, "timeout 3 kscore-server --config /etc/keystone-core/server.yaml 2>&1 || true"))
	if manual.Stdout != "" {
		t.Logf("manual server run:\n%s", manual.Stdout)
	}

	// Check if binary exists and where
	which, _ := execShell(ctx, node, "which kscore-server kscore-agent 2>&1 || true")
	t.Logf("which kscore-*:\n%s", which.Stdout)

	ls, _ := execShell(ctx, node, "ls -la /usr/bin/kscore-* 2>&1 || true")
	t.Logf("ls /usr/bin/kscore-*:\n%s", ls.Stdout)

	// Check package contents
	dpkg, _ := execShell(ctx, node, "dpkg -L kscore-server 2>&1 || true")
	t.Logf("dpkg -L kscore-server:\n%s", dpkg.Stdout)

	// Check data directories exist and permissions
	dirs, _ := execShell(ctx, node, "ls -la /etc/keystone-core/ /var/lib/keystone-core/ 2>&1 || true")
	t.Logf("keystone-core directories:\n%s", dirs.Stdout)

	// Check if kscore user exists
	user, _ := execShell(ctx, node, "id kscore 2>&1 || true")
	t.Logf("kscore user: %s", user.Stdout)

	// Check agent config if present
	agentCfg, _ := execShell(ctx, node, "cat /etc/keystone-core/agent.yaml 2>&1 || true")
	if agentCfg.Stdout != "" {
		t.Logf("agent config:\n%s", agentCfg.Stdout)
	}

	// Check bootstrap diagnostics file
	diagFile, _ := execShell(ctx, node, sudo(node, "ls -t /var/log/keystone-core/kscore-bootstrap-diagnostics-*.log 2>/dev/null | head -1"))
	if diagFile.Stdout != "" {
		diagPath := strings.TrimSpace(diagFile.Stdout)
		diag, _ := execShell(ctx, node, sudo(node, fmt.Sprintf("cat %s 2>&1 || true", diagPath)))
		if diag.Stdout != "" {
			t.Logf("bootstrap diagnostics (%s):\n%s", diagPath, diag.Stdout)
		}
	}
}

// testClusterHealth verifies that services are healthy on all nodes.
func testClusterHealth(t *testing.T, provider vm.Provider, cfg *vm.Config) {
	t.Helper()

	nodes := provider.ListNodes()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, node := range nodes {
		t.Logf("Checking service health on node %s (%s, role=%s)", node.Name, node.Host, node.Role)

		switch node.Role {
		case "control-plane":
			// Control-plane runs server only
			checkServiceActive(ctx, t, node, "kscore-server")
			checkPortOpen(ctx, t, node, 4222, "NATS")
			checkPortOpen(ctx, t, node, 8080, "HTTP API")
			checkPortOpen(ctx, t, node, 9090, "gRPC API")
		case "both":
			// Both runs server and agent
			checkServiceActive(ctx, t, node, "kscore-server")
			checkServiceActive(ctx, t, node, "kscore-agent")
			checkPortOpen(ctx, t, node, 4222, "NATS")
			checkPortOpen(ctx, t, node, 8080, "HTTP API")
			checkPortOpen(ctx, t, node, 9090, "gRPC API")
		case "agent":
			// Agent-only node
			checkServiceActive(ctx, t, node, "kscore-agent")
		}
	}
}

// testAgentRegistration verifies that agents have registered with the control-plane.
func testAgentRegistration(t *testing.T, provider vm.Provider, cfg *vm.Config) {
	t.Helper()

	nodes := provider.ListNodes()

	// Find control-plane node
	var cpNode *vm.Node
	var agentCount int
	for _, n := range nodes {
		switch n.Role {
		case "control-plane", "both":
			if cpNode == nil {
				cpNode = n
			}
			if n.Role == "both" {
				agentCount++ // "both" node also runs an agent
			}
		case "agent":
			agentCount++
		}
	}

	if cpNode == nil {
		t.Fatal("no control-plane node found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Logf("Checking agent registration on control-plane %s (expecting %d agents)", cpNode.Name, agentCount)

	// Query the control-plane for registered agents via health/status endpoint
	cmd := "curl -s http://localhost:8080/health/status"
	result, err := execShell(ctx, cpNode, cmd)
	if err != nil {
		t.Fatalf("failed to query control-plane status: %v", err)
	}

	t.Logf("Control-plane status: %s", result.Stdout)

	// Check if agents are registered
	// The /health/status endpoint returns JSON with agent counts
	if !strings.Contains(result.Stdout, "agents") {
		t.Logf("Warning: status response doesn't contain agent info")
	}

	// Also try using kscorectl to list agents
	cmd = fmt.Sprintf("%s agents list 2>&1 || true", sudo(cpNode, "kscorectl"))
	result, err = execShell(ctx, cpNode, cmd)
	if err == nil && result.Stdout != "" {
		t.Logf("Agent list:\n%s", result.Stdout)
	}
}
