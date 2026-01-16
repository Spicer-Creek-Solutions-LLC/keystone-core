package bootstrap

import (
	"strings"
	"testing"
)

func TestRenderCompletionReport(t *testing.T) {
	cfg := &BootstrapConfig{
		ClusterName:     "keystone",
		NodeRole:        "both",
		Storage:         "sqlite",
		NATSMode:        "embedded",
		ApplyBlueprints: []string{"blueprints/demo"},
	}

	report := renderCompletionReport(cfg)
	if report == "" {
		t.Fatal("expected completion report output")
	}
	if !containsAll(report, []string{"bootstrap complete", "cluster: keystone", "role: both"}) {
		t.Fatalf("unexpected report output: %s", report)
	}
}

func TestServiceStatusCommand(t *testing.T) {
	cmd, ok := serviceStatusCommand("systemd", "kscore-server")
	if !ok || cmd.Name != "systemctl" {
		t.Fatalf("unexpected systemd command: %#v", cmd)
	}
	cmd, ok = serviceStatusCommand("openrc", "kscore-agent")
	if !ok || cmd.Name != "rc-service" {
		t.Fatalf("unexpected openrc command: %#v", cmd)
	}
}

func TestIsServiceActive(t *testing.T) {
	if !isServiceActive("systemd", "active", "kscore-server") {
		t.Fatal("expected systemd active to be true")
	}
	if !isServiceActive("sysv", "is running", "kscore-server") {
		t.Fatal("expected sysv running to be true")
	}
	if !isServiceActive("launchd", "1234\t0\tkscore-server", "kscore-server") {
		t.Fatal("expected launchd to match service name")
	}
	if isServiceActive("systemd", "inactive", "kscore-server") {
		t.Fatal("expected inactive to be false")
	}
}

func TestResolveDialHost(t *testing.T) {
	if host := resolveDialHost("0.0.0.0", ""); host != "127.0.0.1" {
		t.Fatalf("expected 127.0.0.1, got %s", host)
	}
	if host := resolveDialHost("[::]", ""); host != "127.0.0.1" {
		t.Fatalf("expected 127.0.0.1 for [::], got %s", host)
	}
	if host := resolveDialHost("192.168.1.10", ""); host != "192.168.1.10" {
		t.Fatalf("expected 192.168.1.10, got %s", host)
	}
	if host := resolveDialHost("0.0.0.0", "10.0.0.5"); host != "10.0.0.5" {
		t.Fatalf("expected advertise host, got %s", host)
	}
	if host := resolveDialHost("", "10.0.0.5:9090"); host != "10.0.0.5" {
		t.Fatalf("expected advertise host without port, got %s", host)
	}
}

func TestResolveNATSAddresses(t *testing.T) {
	cfg := &BootstrapConfig{
		NATSMode: "external",
		NATSURLs: []string{"nats://nats1:4223", "nats://nats2"},
	}
	addresses, err := resolveNATSAddresses(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addresses) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addresses))
	}
	if addresses[0] != "nats1:4223" {
		t.Fatalf("unexpected address: %s", addresses[0])
	}
	if addresses[1] != "nats2:4222" {
		t.Fatalf("unexpected address: %s", addresses[1])
	}

	cfg = &BootstrapConfig{
		NATSMode: "embedded",
	}
	addresses, err = resolveNATSAddresses(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addresses) != 1 || addresses[0] != "127.0.0.1:4222" {
		t.Fatalf("unexpected default address: %v", addresses)
	}
}

func TestParseNATSURL(t *testing.T) {
	address, err := parseNATSURL("nats://nats1:4222")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if address != "nats1:4222" {
		t.Fatalf("unexpected address: %s", address)
	}

	address, err = parseNATSURL("nats://nats2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if address != "nats2:4222" {
		t.Fatalf("unexpected address: %s", address)
	}

	if _, err := parseNATSURL("://bad"); err == nil {
		t.Fatal("expected error for invalid url")
	}
}

func TestResolvePostgresAddress(t *testing.T) {
	cfg := &BootstrapConfig{
		Storage:      "postgres",
		PostgresHost: "db.example.com",
		PostgresPort: 5432,
	}
	address, err := resolvePostgresAddress(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if address != "db.example.com:5432" {
		t.Fatalf("unexpected address: %s", address)
	}

	cfg.PostgresHost = "db.example.com:5433"
	address, err = resolvePostgresAddress(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if address != "db.example.com:5433" {
		t.Fatalf("unexpected address: %s", address)
	}
}

func TestShouldCheckClusterMembership(t *testing.T) {
	cfg := &BootstrapConfig{
		NodeRole: "agent",
	}
	if shouldCheckClusterMembership(cfg) {
		t.Fatal("expected agent-only to skip membership checks")
	}

	cfg = &BootstrapConfig{
		NodeRole:  "control-plane",
		HAEnabled: true,
	}
	if !shouldCheckClusterMembership(cfg) {
		t.Fatal("expected HA control-plane to check membership")
	}

	cfg = &BootstrapConfig{
		NodeRole: "control-plane",
		NATSMode: "cluster",
	}
	if !shouldCheckClusterMembership(cfg) {
		t.Fatal("expected cluster nats mode to check membership")
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
