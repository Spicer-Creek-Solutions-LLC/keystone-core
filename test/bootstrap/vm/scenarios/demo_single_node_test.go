package scenarios

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/bootstrap/vm"
)

// Environment variables for test configuration
const (
	// KSCORE_REPO_URL sets the package repository URL (default: http://localhost:8080/repos)
	// This must be reachable from the VM, e.g., http://repo-host.example.internal:8080/repos
	envRepoURL = "KSCORE_REPO_URL"
)

// TestDemoSingleNodeBootstrap tests the complete single-node demo bootstrap workflow.
// This validates VM Bootstrap Validation from the release readiness epic.
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
	osInfo := detectOS(ctx, t, node)
	t.Logf("Detected OS: %s, package manager: %s", osInfo.name, osInfo.pkgManager)

	// Step 2: Clean previous installation if configured
	if cfg.SSH.CleanNodes {
		cleanPreviousInstall(ctx, t, node, osInfo)
	}

	// Step 3: Configure package repository
	configureRepo(ctx, t, node, osInfo)

	// Step 4: Install packages
	installPackages(ctx, t, node, osInfo)

	// Step 5: Bootstrap in demo mode
	bootstrapDemo(ctx, t, node)
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
	checkServiceActive(ctx, t, node, "kscore-server")

	// Check kscore-agent service
	checkServiceActive(ctx, t, node, "kscore-agent")

	// Check embedded NATS is running
	checkPortOpen(ctx, t, node, 4222, "NATS")

	// Check HTTP health API port is listening
	checkPortOpen(ctx, t, node, 8080, "HTTP API")

	// Check gRPC API port is listening
	checkPortOpen(ctx, t, node, 9090, "gRPC API")
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

	// Check HTTP health endpoints (runs on port 8080)
	checkHTTPEndpoint(ctx, t, node, 8080, "/health/live", 200)
	checkHTTPEndpoint(ctx, t, node, 8080, "/health/ready", 200)
}

type osInfo struct {
	name       string
	version    string
	pkgManager string // apt, dnf, or zypper
}

func detectOS(ctx context.Context, t *testing.T, node *vm.Node) osInfo {
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

// sudo wraps a command with sudo if the user is not root
func sudo(node *vm.Node, cmd string) string {
	if node.User == "root" {
		return cmd
	}
	return "sudo " + cmd
}

// sudoPrefix returns "sudo" or "" depending on user
func sudoPrefix(node *vm.Node) string {
	if node.User == "root" {
		return ""
	}
	return "sudo"
}

// execShell runs a shell command on the node. Since SSH already runs commands
// through a shell, we pass the command directly as a single string.
func execShell(ctx context.Context, node *vm.Node, cmd string) (*vm.ExecResult, error) {
	return node.Exec(ctx, cmd)
}

// ubuntuCodename maps Ubuntu version to codename
func ubuntuCodename(version string) string {
	switch {
	case strings.HasPrefix(version, "24.04"):
		return "noble"
	case strings.HasPrefix(version, "22.04"):
		return "jammy"
	case strings.HasPrefix(version, "20.04"):
		return "focal"
	default:
		return "jammy" // fallback
	}
}

// debianCodename maps Debian version to codename
func debianCodename(version string) string {
	switch {
	case strings.HasPrefix(version, "13"):
		return "trixie"
	case strings.HasPrefix(version, "12"):
		return "bookworm"
	case strings.HasPrefix(version, "11"):
		return "bullseye"
	default:
		return "bookworm" // fallback
	}
}

func cleanPreviousInstall(ctx context.Context, t *testing.T, node *vm.Node, info osInfo) {
	t.Helper()
	t.Log("Cleaning previous installation...")

	// Stop services if running
	_, _ = execShell(ctx, node, sudo(node, "systemctl stop kscore-server kscore-agent 2>/dev/null || true"))

	// Remove packages and clean cache
	var removeCmd string
	switch info.pkgManager {
	case "apt":
		removeCmd = "apt-get remove --purge -y kscore-server kscore-agent kscore-cli 2>/dev/null || true"
	case "dnf":
		removeCmd = "dnf remove -y kscore-server kscore-agent kscore-cli 2>/dev/null || true"
	case "zypper":
		removeCmd = "zypper remove -y kscore-server kscore-agent kscore-cli 2>/dev/null || true"
	}

	if _, err := execShell(ctx, node, sudo(node, removeCmd)); err != nil {
		t.Logf("Warning: failed to remove previous packages: %v", err)
	}

	// Clean apt cache to force fresh download
	if info.pkgManager == "apt" {
		_, _ = execShell(ctx, node, sudo(node, "apt-get clean"))
		// Clean all possible cached package variations
		_, _ = execShell(ctx, node, sudo(node, "rm -rf /var/cache/apt/archives/kscore*.deb"))
		// Also remove lists to force fresh metadata fetch
		_, _ = execShell(ctx, node, sudo(node, "rm -rf /var/lib/apt/lists/*keystonecore*"))
	}

	// Clean state directories
	_, _ = execShell(ctx, node, sudo(node, "rm -rf /var/lib/keystone-core /etc/keystone-core /var/lib/keystone-core /etc/keystone-core"))
}

func configureRepo(ctx context.Context, t *testing.T, node *vm.Node, info osInfo) {
	t.Helper()
	t.Logf("Configuring %s repository...", info.pkgManager)

	// Get repo URL from environment or use default
	// NOTE: localhost won't work - use host IP reachable from VM
	repoURL := os.Getenv(envRepoURL)
	if repoURL == "" {
		repoURL = "http://localhost:8080/repos"
		t.Logf("WARNING: Using default repo URL %s - set KSCORE_REPO_URL to your host's IP", repoURL)
	}
	t.Logf("Using repository URL: %s", repoURL)

	switch info.pkgManager {
	case "apt":
		// Add APT repository - map version to codename
		distro := ubuntuCodename(info.version)
		if info.name == "debian" {
			distro = debianCodename(info.version)
		}
		t.Logf("Using distro codename: %s", distro)

		// allow-insecure=yes is needed because the test repo lacks GPG signing
		repoLine := fmt.Sprintf("deb [trusted=yes allow-insecure=yes] %s/apt %s main", repoURL, distro)
		cmd := fmt.Sprintf("echo '%s' | %s tee /etc/apt/sources.list.d/keystonecore.list > /dev/null", repoLine, sudoPrefix(node))
		result, err := execShell(ctx, node, cmd)
		if err != nil {
			t.Fatalf("failed to configure apt repo: %v\nOutput: %s", err, result.Stdout)
		}

		// Verify the repo file was created
		result, err = execShell(ctx, node, "cat /etc/apt/sources.list.d/keystonecore.list")
		if err != nil {
			t.Fatalf("failed to read repo file: %v", err)
		}
		t.Logf("Repo file contents: %s", result.Stdout)

		// Test if repo URL is reachable
		testURL := fmt.Sprintf("%s/apt/dists/%s/Release", repoURL, distro)
		result, _ = execShell(ctx, node, fmt.Sprintf("curl -sI %s | head -1", testURL))
		t.Logf("Repository connectivity test (%s): %s", testURL, strings.TrimSpace(result.Stdout))

		result, err = execShell(ctx, node, sudo(node, "apt-get update 2>&1"))
		t.Logf("apt-get update output:\n%s", result.Stdout)
		if err != nil {
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

		cmd := fmt.Sprintf("cat << 'EOF' | %s tee /etc/yum.repos.d/keystonecore.repo > /dev/null\n%sEOF", sudoPrefix(node), repoContent)
		result, err := execShell(ctx, node, cmd)
		if err != nil {
			t.Fatalf("failed to configure dnf repo: %v\nOutput: %s", err, result.Stdout)
		}

	case "zypper":
		// Add Zypper repository
		cmd := sudo(node, fmt.Sprintf("zypper ar -G %s/dnf/el9/x86_64 keystonecore 2>/dev/null || true", repoURL))
		result, err := execShell(ctx, node, cmd)
		if err != nil {
			t.Fatalf("failed to configure zypper repo: %v\nOutput: %s", err, result.Stdout)
		}
	}
}

func installPackages(ctx context.Context, t *testing.T, node *vm.Node, info osInfo) {
	t.Helper()
	t.Log("Installing Keystone Core packages...")

	// Check disk space first
	df, _ := execShell(ctx, node, "df -h /usr /var")
	t.Logf("Disk space:\n%s", df.Stdout)

	var installCmd string
	switch info.pkgManager {
	case "apt":
		// Use --reinstall to force update even if version number is the same
		installCmd = "DEBIAN_FRONTEND=noninteractive apt-get install --reinstall -y kscore-server kscore-agent kscore-cli"
	case "dnf":
		// Use reinstall to force update
		installCmd = "dnf reinstall -y kscore-server kscore-agent kscore-cli || dnf install -y kscore-server kscore-agent kscore-cli"
	case "zypper":
		installCmd = "zypper install -y --force kscore-server kscore-agent kscore-cli"
	}

	result, err := execShell(ctx, node, sudo(node, installCmd))
	t.Logf("Install output:\n%s", result.Stdout)
	if err != nil {
		t.Fatalf("failed to install packages: %v\nOutput: %s", err, result.Stdout)
	}

	// Verify package contents immediately after install
	if info.pkgManager == "apt" {
		// Check if binary exists on filesystem
		lsBin, _ := execShell(ctx, node, "ls -la /usr/bin/kscore-server 2>&1 || echo 'FILE NOT FOUND'")
		t.Logf("kscore-server binary on disk: %s", strings.TrimSpace(lsBin.Stdout))

		// Check dpkg thinks it installed
		verify, _ := execShell(ctx, node, "dpkg -L kscore-server 2>&1")
		t.Logf("dpkg -L kscore-server:\n%s", verify.Stdout)

		version, _ := execShell(ctx, node, "dpkg -s kscore-server | grep -E 'Version|Status'")
		t.Logf("kscore-server status: %s", strings.TrimSpace(version.Stdout))

		// Test if binary can execute by running version command
		verTest, _ := execShell(ctx, node, "/usr/bin/kscore-server version 2>&1 || echo 'EXEC FAILED'")
		t.Logf("kscore-server version test: %s", strings.TrimSpace(verTest.Stdout))
	}

	t.Log("Packages installed successfully")
}

func bootstrapDemo(ctx context.Context, t *testing.T, node *vm.Node) {
	t.Helper()
	t.Log("Running demo bootstrap...")

	// Run the bootstrap command with --skip-repo-setup since packages are already installed
	result, err := execShell(ctx, node, sudo(node, "kscore-agent bootstrap --mode demo --non-interactive --skip-repo-setup"))
	if err != nil {
		// Capture diagnostics on failure
		t.Logf("Bootstrap output:\n%s", result.Stdout)

		// Get service status
		status, _ := execShell(ctx, node, sudo(node, "systemctl status kscore-server.service 2>&1 || true"))
		t.Logf("kscore-server status:\n%s", status.Stdout)

		// Get journal logs
		journal, _ := execShell(ctx, node, sudo(node, "journalctl -xeu kscore-server.service --no-pager -n 50 2>&1 || true"))
		t.Logf("kscore-server journal:\n%s", journal.Stdout)

		// Get bootstrap diagnostics if available
		diag, _ := execShell(ctx, node, "ls -t /var/log/keystone-core/kscore-bootstrap-diagnostics-*.log 2>/dev/null | head -1 | xargs cat 2>/dev/null || true")
		if diag.Stdout != "" {
			t.Logf("Bootstrap diagnostics:\n%s", diag.Stdout)
		}

		t.Fatalf("bootstrap failed: %v", err)
	}

	t.Logf("Bootstrap output:\n%s", result.Stdout)

	// Wait for services to stabilize
	time.Sleep(5 * time.Second)
}

func checkServiceActive(ctx context.Context, t *testing.T, node *vm.Node, service string) {
	t.Helper()

	result, err := node.Exec(ctx, "systemctl", "is-active", service)
	if err != nil || strings.TrimSpace(result.Stdout) != "active" {
		// Get service status for debugging
		status, _ := node.Exec(ctx, "systemctl", "status", service)
		t.Fatalf("service %s is not active:\n%s", service, status.Stdout)
	}

	t.Logf("Service %s is active", service)
}

func checkPortOpen(ctx context.Context, t *testing.T, node *vm.Node, port int, name string) {
	t.Helper()

	cmd := fmt.Sprintf("ss -tlnp | grep :%d || netstat -tlnp | grep :%d", port, port)
	result, err := execShell(ctx, node, cmd)
	if err != nil || result.Stdout == "" {
		t.Fatalf("port %d (%s) is not listening", port, name)
	}

	t.Logf("Port %d (%s) is listening", port, name)
}

func checkHTTPEndpoint(ctx context.Context, t *testing.T, node *vm.Node, port int, path string, expectedStatus int) {
	t.Helper()

	// Use curl to check HTTP endpoint
	cmd := fmt.Sprintf("curl -s -o /dev/null -w '%%{http_code}' http://localhost:%d%s", port, path)
	result, err := execShell(ctx, node, cmd)
	if err != nil {
		t.Fatalf("failed to check HTTP endpoint %s on port %d: %v", path, port, err)
	}

	statusCode := strings.TrimSpace(result.Stdout)
	if statusCode != fmt.Sprintf("%d", expectedStatus) {
		t.Fatalf("HTTP endpoint %s on port %d returned %s, expected %d", path, port, statusCode, expectedStatus)
	}

	t.Logf("HTTP endpoint %s on port %d returned %s", path, port, statusCode)
}
