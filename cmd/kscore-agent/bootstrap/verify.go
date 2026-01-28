package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/config"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func verifyPhase(ctx context.Context, state *State) error {
	if state.BootstrapConfig == nil {
		return nil
	}

	cfg := state.BootstrapConfig
	if state.System == nil || state.System.Platform == nil {
		return fmt.Errorf("system detection not completed")
	}

	initSystem := string(state.System.Platform.InitSystem)
	if requiresServer(cfg) {
		if err := checkServiceActive(ctx, initSystem, "kscore-server"); err != nil {
			return err
		}
	}
	if requiresAgent(cfg) {
		if err := checkServiceActive(ctx, initSystem, "kscore-agent"); err != nil {
			return err
		}
	}
	if requiresServer(cfg) {
		if err := checkAPIConnectivity(ctx, cfg); err != nil {
			return err
		}
	}
	if err := checkPostgresConnectivity(ctx, cfg); err != nil {
		return err
	}
	if err := checkNATSConnectivity(ctx, cfg); err != nil {
		return err
	}
	if err := checkClusterMembership(ctx, cfg); err != nil {
		return err
	}

	fmt.Fprintln(state.Output, renderCompletionReport(cfg))
	return nil
}

func checkServiceActive(ctx context.Context, initSystem, service string) error {
	command, ok := serviceStatusCommand(initSystem, service)
	if !ok {
		return fmt.Errorf("unsupported init system for health checks: %s", initSystem)
	}

	output, err := execCommand(ctx, command.Name, command.Args...)
	if err != nil {
		return fmt.Errorf("service %s check failed: %w (output: %s)", service, err, strings.TrimSpace(output))
	}
	if !isServiceActive(initSystem, output, service) {
		return fmt.Errorf("service %s not active: %s", service, strings.TrimSpace(output))
	}
	return nil
}

func execCommand(ctx context.Context, name string, args ...string) (string, error) {
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are derived from init/system status checks
	result, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return string(result), fmt.Errorf("command %s failed: %w", name, err)
	}
	return string(result), nil
}

func renderCompletionReport(cfg *BootstrapConfig) string {
	var builder strings.Builder
	builder.WriteString("bootstrap complete\n\n")
	builder.WriteString(fmt.Sprintf("cluster: %s\n", cfg.ClusterName))
	builder.WriteString(fmt.Sprintf("role: %s\n", cfg.NodeRole))
	if cfg.NodeName != "" {
		builder.WriteString(fmt.Sprintf("node: %s\n", cfg.NodeName))
	}
	builder.WriteString(fmt.Sprintf("storage: %s\n", cfg.Storage))
	builder.WriteString(fmt.Sprintf("nats mode: %s\n", cfg.NATSMode))
	if cfg.Storage == "postgres" {
		builder.WriteString(fmt.Sprintf("postgres host: %s\n", cfg.PostgresHost))
	}
	if cfg.NATSMode == "external" && len(cfg.NATSURLs) > 0 {
		builder.WriteString(fmt.Sprintf("nats urls: %s\n", strings.Join(cfg.NATSURLs, ", ")))
	}
	builder.WriteString("\nnext steps:\n")
	if requiresServer(cfg) {
		builder.WriteString("- check server logs: journalctl -u kscore-server -n 50\n")
	}
	if requiresAgent(cfg) {
		builder.WriteString("- check agent logs: journalctl -u kscore-agent -n 50\n")
	}
	builder.WriteString("- review configs in /etc/kscore\n")
	if len(cfg.ApplyBlueprints) > 0 {
		builder.WriteString("- review applied blueprints in your state system\n")
	}
	return builder.String()
}

func requiresServer(cfg *BootstrapConfig) bool {
	return cfg.NodeRole == "control-plane" || cfg.NodeRole == "both"
}

func requiresAgent(cfg *BootstrapConfig) bool {
	return cfg.NodeRole == "agent" || cfg.NodeRole == "both"
}

func checkAPIConnectivity(ctx context.Context, cfg *BootstrapConfig) error {
	host := resolveDialHost(cfg.BindAddress, cfg.Advertise)
	grpcAddr := net.JoinHostPort(host, strconv.Itoa(config.DefaultGRPCPort))
	if err := checkTCP(ctx, grpcAddr); err != nil {
		return fmt.Errorf("grpc api check failed for %s: %w", grpcAddr, err)
	}
	httpAddr := net.JoinHostPort(host, strconv.Itoa(config.DefaultHTTPPort))
	if err := checkTCP(ctx, httpAddr); err != nil {
		return fmt.Errorf("http api check failed for %s: %w", httpAddr, err)
	}
	return nil
}

func checkNATSConnectivity(ctx context.Context, cfg *BootstrapConfig) error {
	addresses, err := resolveNATSAddresses(cfg)
	if err != nil {
		return err
	}
	for _, addr := range addresses {
		if err := checkTCP(ctx, addr); err != nil {
			return fmt.Errorf("nats check failed for %s: %w", addr, err)
		}
	}
	return nil
}

func checkClusterMembership(ctx context.Context, cfg *BootstrapConfig) error {
	if !shouldCheckClusterMembership(cfg) {
		return nil
	}
	host := resolveDialHost(cfg.BindAddress, cfg.Advertise)
	grpcAddr := net.JoinHostPort(host, strconv.Itoa(config.DefaultGRPCPort))
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(timeoutCtx, grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("cluster health dial failed: %w", err)
	}
	defer conn.Close()

	client := pb.NewCoordinationServiceClient(conn)
	resp, err := client.ClusterHealth(timeoutCtx, &pb.ClusterHealthRequest{
		IncludeMembers: true,
		IncludeNats:    true,
	})
	if err != nil {
		return fmt.Errorf("cluster health check failed: %w", err)
	}

	if resp.Cluster != "" && cfg.ClusterName != "" && resp.Cluster != cfg.ClusterName {
		return fmt.Errorf("cluster name mismatch: expected %s, got %s", cfg.ClusterName, resp.Cluster)
	}
	if resp.TotalMembers == 0 {
		return fmt.Errorf("cluster membership not detected")
	}
	if cfg.HAEnabled && !resp.HasQuorum {
		return fmt.Errorf("cluster quorum not reached")
	}
	if resp.Status == pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_UNHEALTHY ||
		resp.Status == pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_UNKNOWN ||
		resp.Status == pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_UNSPECIFIED {
		return fmt.Errorf("cluster health status %s", resp.Status.String())
	}
	return nil
}

func checkPostgresConnectivity(ctx context.Context, cfg *BootstrapConfig) error {
	if !strings.EqualFold(cfg.Storage, "postgres") {
		return nil
	}
	address, err := resolvePostgresAddress(cfg)
	if err != nil {
		return err
	}
	if err := checkTCP(ctx, address); err != nil {
		return fmt.Errorf("postgres check failed for %s: %w", address, err)
	}
	return nil
}

func resolveNATSAddresses(cfg *BootstrapConfig) ([]string, error) {
	if strings.EqualFold(cfg.NATSMode, "external") && len(cfg.NATSURLs) > 0 {
		addresses := make([]string, 0, len(cfg.NATSURLs))
		for _, raw := range cfg.NATSURLs {
			addr, err := parseNATSURL(raw)
			if err != nil {
				return nil, err
			}
			addresses = append(addresses, addr)
		}
		return addresses, nil
	}
	host := resolveDialHost(cfg.BindAddress, cfg.Advertise)
	return []string{net.JoinHostPort(host, "4222")}, nil
}

func resolvePostgresAddress(cfg *BootstrapConfig) (string, error) {
	host := strings.TrimSpace(cfg.PostgresHost)
	if host == "" {
		return "", fmt.Errorf("postgres host is required")
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host, nil
	}
	port := cfg.PostgresPort
	if port == 0 {
		port = 5432
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func shouldCheckClusterMembership(cfg *BootstrapConfig) bool {
	if cfg == nil {
		return false
	}
	if !requiresServer(cfg) {
		return false
	}
	return cfg.HAEnabled || strings.EqualFold(cfg.NATSMode, "cluster")
}

func parseNATSURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid nats url %q: %w", raw, err)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("invalid nats url %q: missing host", raw)
	}
	port := parsed.Port()
	if port == "" {
		port = "4222"
	}
	return net.JoinHostPort(host, port), nil
}

func resolveDialHost(bindAddress, advertise string) string {
	if host := normalizeHost(advertise); host != "" {
		return host
	}
	host := normalizeHost(bindAddress)
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

func normalizeHost(value string) string {
	host := strings.TrimSpace(value)
	if host == "" {
		return ""
	}
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if host == "[::]" {
		return "::"
	}
	return host
}

func checkTCP(ctx context.Context, address string) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	if err := conn.Close(); err != nil {
		return err
	}
	return nil
}

func serviceStatusCommand(initSystem, service string) (CommandPlan, bool) {
	switch strings.ToLower(initSystem) {
	case "systemd":
		return CommandPlan{Name: "systemctl", Args: []string{"is-active", service}}, true
	case "openrc":
		return CommandPlan{Name: "rc-service", Args: []string{service, "status"}}, true
	case "sysv":
		return CommandPlan{Name: "service", Args: []string{service, "status"}}, true
	case "launchd":
		return CommandPlan{Name: "launchctl", Args: []string{"list", service}}, true
	case "windows_service":
		return CommandPlan{Name: "sc", Args: []string{"query", service}}, true
	default:
		return CommandPlan{}, false
	}
}

func isServiceActive(initSystem, output, service string) bool {
	normalized := strings.ToLower(output)
	switch strings.ToLower(initSystem) {
	case "launchd":
		return strings.Contains(normalized, strings.ToLower(service))
	case "systemd":
		return strings.TrimSpace(normalized) == "active"
	default:
		return strings.Contains(normalized, "running") ||
			strings.Contains(normalized, "started")
	}
}
