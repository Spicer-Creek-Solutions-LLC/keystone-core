package scenarios

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/vm"
)

// TestDemoSingleNodeBootstrap tests the complete single-node demo bootstrap workflow.
// This validates Phase 5 of Epic 100: VM Bootstrap Validation.
//
// Run with:
//
//	KSCORE_VM_TESTS=1 KSCORE_VM_CONFIG=test/bootstrap/vm/single-node-demo.yaml \
//	  go test -v ./test/bootstrap/vm/scenarios -run TestDemoSingleNodeBootstrap
func TestDemoSingleNodeBootstrap(t *testing.T) {
	vm.RunVMTests(t, "", []func(*testing.T, vm.Provider, *vm.Config){
		testDemoBootstrap,
		testDemoHealth,
		testDemoAPI,
	})
}

// testDemoBootstrap installs and bootstraps Keystone Core on a single node.
func testDemoBootstrap(t *testing.T, provider vm.Provider, cfg *vm.Config) {
	t.Helper()

	nodes := provider.ListNodes()
	if len(nodes) == 0 {
		t.Fatal("expected at least one VM node configured")
	}

	node := nodes[0]
	t.Logf("Running demo bootstrap on node %s (%s)", node.Name, node.Host)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 1: Detect OS and package manager
	osInfo := detectOS(t, ctx, node)
	t.Logf("Detected OS: %s, package manager: %s", osInfo.name, osInfo.pkgManager)

	// Step 2: Clean previous installation if configured
	if cfg.SSH.CleanNodes {
		cleanPreviousInstall(t, ctx, node, osInfo)
	}

	// Step 3: Configure package repository
	configureRepo(t, ctx, node, osInfo)

	// Step 4: Install packages
	installPackages(t, ctx, node, osInfo)

	// Step 5: Bootstrap in demo mode
	bootstrapDemo(t, ctx, node)
}

// testDemoHealth verifies that services are healthy after bootstrap.
func testDemoHealth(t *testing.T, provider vm.Provider, cfg *vm.Config) {
	t.Helper()

	nodes := provider.ListNodes()
	if len(nodes) == 0 {
		t.Fatal("expected at least one VM node configured")
	}

	node := nodes[0]
	t.Logf("Checking service health on node %s", node.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Check kscore-server service
	checkServiceActive(t, ctx, node, "kscore-server")

	// Check kscore-agent service
	checkServiceActive(t, ctx, node, "kscore-agent")

	// Check embedded NATS is running
	checkPortOpen(t, ctx, node, 4222, "NATS")

	// Check API port is listening
	checkPortOpen(t, ctx, node, 8443, "API")
}

// testDemoAPI verifies that API endpoints respond correctly.
func testDemoAPI(t *testing.T, provider vm.Provider, cfg *vm.Config) {
	t.Helper()

	nodes := provider.ListNodes()
	if len(nodes) == 0 {
		t.Fatal("expected at least one VM node configured")
	}

	node := nodes[0]
	t.Logf("Checking API endpoints on node %s", node.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Check health endpoint
	checkAPIEndpoint(t, ctx, node, "/health", 200)

	// Check version endpoint
	checkAPIEndpoint(t, ctx, node, "/api/v1/version", 200)

	// Check agents endpoint (should return empty list initially)
	checkAPIEndpoint(t, ctx, node, "/api/v1/agents", 200)
}

type osInfo struct {
	name       string
	version    string
	pkgManager string // apt, dnf, or zypper
}

func detectOS(t *testing.T, ctx context.Context, node *vm.Node) osInfo {
	t.Helper()

	result, err := node.Exec(ctx, "cat", "/etc/os-release")
	if err != nil {
		t.Fatalf("failed to detect OS: %v", err)
	}

	info := osInfo{}
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.HasPrefix(line, "ID=") {
			info.name = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			info.version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}

	// Determine package manager
	switch info.name {
	case "ubuntu", "debian":
		info.pkgManager = "apt"
	case "rocky", "almalinux", "centos", "rhel", "fedora":
		info.pkgManager = "dnf"
	case "opensuse", "opensuse-leap", "sles":
		info.pkgManager = "zypper"
	default:
		t.Fatalf("unsupported OS: %s", info.name)
	}

	return info
}

func cleanPreviousInstall(t *testing.T, ctx context.Context, node *vm.Node, info osInfo) {
	t.Helper()
	t.Log("Cleaning previous installation...")

	// Stop services if running
	_, _ = node.Exec(ctx, "systemctl", "stop", "kscore-server", "kscore-agent")

	// Remove packages
	var removeCmd string
	switch info.pkgManager {
	case "apt":
		removeCmd = "apt-get remove -y kscore-server kscore-agent kscore-cli 2>/dev/null || true"
	case "dnf":
		removeCmd = "dnf remove -y kscore-server kscore-agent kscore-cli 2>/dev/null || true"
	case "zypper":
		removeCmd = "zypper remove -y kscore-server kscore-agent kscore-cli 2>/dev/null || true"
	}

	if _, err := node.Exec(ctx, "sh", "-c", removeCmd); err != nil {
		t.Logf("Warning: failed to remove previous packages: %v", err)
	}

	// Clean state directories
	_, _ = node.Exec(ctx, "rm", "-rf", "/var/lib/kscore", "/etc/kscore")
}

func configureRepo(t *testing.T, ctx context.Context, node *vm.Node, info osInfo) {
	t.Helper()
	t.Logf("Configuring %s repository...", info.pkgManager)

	// For now, we'll use the local repository served via HTTP
	// In a real deployment, this would point to packages.keystonecore.io
	repoURL := "http://localhost:8080/repos"

	switch info.pkgManager {
	case "apt":
		// Add APT repository
		distro := "jammy" // Default to Ubuntu 22.04
		if info.name == "debian" {
			distro = "bookworm"
		}

		repoLine := fmt.Sprintf("deb [trusted=yes] %s/apt %s main", repoURL, distro)
		cmd := fmt.Sprintf("echo '%s' > /etc/apt/sources.list.d/keystonecore.list", repoLine)
		if _, err := node.Exec(ctx, "sh", "-c", cmd); err != nil {
			t.Fatalf("failed to configure apt repo: %v", err)
		}

		if result, err := node.Exec(ctx, "apt-get", "update"); err != nil {
			t.Fatalf("failed to update apt cache: %v\n%s", err, result.Stdout)
		}

	case "dnf":
		// Add DNF repository
		version := "el9" // Default to EL9
		if strings.Contains(info.version, "8") {
			version = "el8"
		}

		repoContent := fmt.Sprintf(`[keystonecore]
name=Keystone Core
baseurl=%s/dnf/%s/$basearch
enabled=1
gpgcheck=0
`, repoURL, version)

		cmd := fmt.Sprintf("cat > /etc/yum.repos.d/keystonecore.repo << 'EOF'\n%sEOF", repoContent)
		if _, err := node.Exec(ctx, "sh", "-c", cmd); err != nil {
			t.Fatalf("failed to configure dnf repo: %v", err)
		}

	case "zypper":
		// Add Zypper repository
		cmd := fmt.Sprintf("zypper ar -G %s/dnf/el9/x86_64 keystonecore 2>/dev/null || true", repoURL)
		if _, err := node.Exec(ctx, "sh", "-c", cmd); err != nil {
			t.Fatalf("failed to configure zypper repo: %v", err)
		}
	}
}

func installPackages(t *testing.T, ctx context.Context, node *vm.Node, info osInfo) {
	t.Helper()
	t.Log("Installing Keystone Core packages...")

	var installCmd string
	switch info.pkgManager {
	case "apt":
		installCmd = "DEBIAN_FRONTEND=noninteractive apt-get install -y kscore-server kscore-agent kscore-cli"
	case "dnf":
		installCmd = "dnf install -y kscore-server kscore-agent kscore-cli"
	case "zypper":
		installCmd = "zypper install -y kscore-server kscore-agent kscore-cli"
	}

	result, err := node.Exec(ctx, "sh", "-c", installCmd)
	if err != nil {
		t.Fatalf("failed to install packages: %v\nOutput: %s", err, result.Stdout)
	}

	t.Log("Packages installed successfully")
}

func bootstrapDemo(t *testing.T, ctx context.Context, node *vm.Node) {
	t.Helper()
	t.Log("Running demo bootstrap...")

	// Run the bootstrap command
	result, err := node.Exec(ctx, "kscore-agent", "bootstrap",
		"--mode", "demo",
		"--non-interactive",
	)
	if err != nil {
		t.Fatalf("bootstrap failed: %v\nOutput: %s", err, result.Stdout)
	}

	t.Logf("Bootstrap output:\n%s", result.Stdout)

	// Wait for services to stabilize
	time.Sleep(5 * time.Second)
}

func checkServiceActive(t *testing.T, ctx context.Context, node *vm.Node, service string) {
	t.Helper()

	result, err := node.Exec(ctx, "systemctl", "is-active", service)
	if err != nil || strings.TrimSpace(result.Stdout) != "active" {
		// Get service status for debugging
		status, _ := node.Exec(ctx, "systemctl", "status", service)
		t.Fatalf("service %s is not active:\n%s", service, status.Stdout)
	}

	t.Logf("Service %s is active", service)
}

func checkPortOpen(t *testing.T, ctx context.Context, node *vm.Node, port int, name string) {
	t.Helper()

	cmd := fmt.Sprintf("ss -tlnp | grep :%d || netstat -tlnp | grep :%d", port, port)
	result, err := node.Exec(ctx, "sh", "-c", cmd)
	if err != nil || result.Stdout == "" {
		t.Fatalf("port %d (%s) is not listening", port, name)
	}

	t.Logf("Port %d (%s) is listening", port, name)
}

func checkAPIEndpoint(t *testing.T, ctx context.Context, node *vm.Node, path string, expectedStatus int) {
	t.Helper()

	// Use curl to check API endpoint
	cmd := fmt.Sprintf("curl -sk -o /dev/null -w '%%{http_code}' https://localhost:8443%s", path)
	result, err := node.Exec(ctx, "sh", "-c", cmd)
	if err != nil {
		t.Fatalf("failed to check API endpoint %s: %v", path, err)
	}

	statusCode := strings.TrimSpace(result.Stdout)
	if statusCode != fmt.Sprintf("%d", expectedStatus) {
		t.Fatalf("API endpoint %s returned %s, expected %d", path, statusCode, expectedStatus)
	}

	t.Logf("API endpoint %s returned %s", path, statusCode)
}
