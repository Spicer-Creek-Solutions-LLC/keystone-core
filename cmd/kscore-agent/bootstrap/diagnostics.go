package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const diagnosticsFilePrefix = "kscore-bootstrap-diagnostics"

func collectDiagnostics(ctx context.Context, state *State, phase PhaseName, failure error) {
	report := buildDiagnosticsReport(ctx, state, phase, failure)
	path, err := writeDiagnosticsReport(report)
	if err != nil {
		fmt.Fprintf(state.Output, "diagnostics collection failed: %v\n", err)
		return
	}

	hints := diagnosticHints(nil, failure)
	if state.BootstrapConfig != nil {
		hints = diagnosticHints(state.BootstrapConfig, failure)
	}

	if state.JSONOutput {
		payload := map[string]interface{}{
			"event": "diagnostics",
			"path":  path,
			"phase": phase,
			"hints": hints,
		}
		if encoded, marshalErr := json.Marshal(payload); marshalErr == nil {
			fmt.Fprintln(state.Output, string(encoded))
		}
		return
	}

	fmt.Fprintf(state.Output, "diagnostics saved to %s\n", path)
}

func buildDiagnosticsReport(ctx context.Context, state *State, phase PhaseName, failure error) string {
	var builder strings.Builder
	builder.WriteString("bootstrap diagnostics\n")
	builder.WriteString(fmt.Sprintf("timestamp: %s\n", time.Now().UTC().Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("phase: %s\n", phase))
	if failure != nil {
		builder.WriteString(fmt.Sprintf("error: %s\n", failure.Error()))
	}

	builder.WriteString("\nsystem:\n")
	builder.WriteString(renderSystemSummary(state))

	builder.WriteString("\nconfig:\n")
	builder.WriteString(renderConfigSummary(state.BootstrapConfig))

	builder.WriteString("\nartifacts:\n")
	builder.WriteString(renderArtifactsSummary(state.InstallArtifacts))

	hints := diagnosticHints(state.BootstrapConfig, failure)
	if len(hints) > 0 {
		builder.WriteString("\nhints:\n")
		for _, hint := range hints {
			builder.WriteString("- " + hint + "\n")
		}
	}

	builder.WriteString("\nlogs:\n")
	builder.WriteString(renderLogSummary(ctx, state))

	return builder.String()
}

func renderSystemSummary(state *State) string {
	if state == nil || state.System == nil || state.System.Platform == nil {
		return "- system detection unavailable\n"
	}
	info := state.System
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("- os: %s/%s\n", info.Platform.OS, info.Platform.Arch))
	if info.Platform.Distro != "" {
		builder.WriteString(fmt.Sprintf("- distro: %s %s\n", info.Platform.Distro, info.Platform.Version))
	}
	builder.WriteString(fmt.Sprintf("- package manager: %s\n", info.Platform.PackageManager))
	builder.WriteString(fmt.Sprintf("- init system: %s\n", info.Platform.InitSystem))
	builder.WriteString(fmt.Sprintf("- resources: cpu=%d, memory=%dMB, disk free=%dGB\n",
		info.Resources.CPUCount, info.Resources.MemoryTotalMB, info.Resources.DiskFreeGB))
	if info.Network != nil {
		builder.WriteString(fmt.Sprintf("- primary ipv4: %s\n", info.Network.PrimaryIPv4))
		builder.WriteString(fmt.Sprintf("- primary ipv6: %s\n", info.Network.PrimaryIPv6))
	}
	if info.ExistingInstall {
		builder.WriteString("- existing install: detected\n")
	} else {
		builder.WriteString("- existing install: not detected\n")
	}
	return builder.String()
}

func renderConfigSummary(cfg *BootstrapConfig) string {
	if cfg == nil {
		return "- no config available\n"
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("- mode: %s\n", cfg.Mode))
	builder.WriteString(fmt.Sprintf("- cluster: %s\n", cfg.ClusterName))
	builder.WriteString(fmt.Sprintf("- role: %s\n", cfg.NodeRole))
	if cfg.NodeName != "" {
		builder.WriteString(fmt.Sprintf("- node: %s\n", cfg.NodeName))
	}
	if cfg.Join != "" {
		builder.WriteString(fmt.Sprintf("- join endpoint: %s\n", cfg.Join))
		if cfg.JoinToken != "" {
			builder.WriteString("- join token: configured\n")
		}
	}
	if cfg.BindAddress != "" {
		builder.WriteString(fmt.Sprintf("- bind address: %s\n", cfg.BindAddress))
	}
	if cfg.Advertise != "" {
		builder.WriteString(fmt.Sprintf("- advertise address: %s\n", cfg.Advertise))
	}
	builder.WriteString(fmt.Sprintf("- storage: %s\n", cfg.Storage))
	if strings.EqualFold(cfg.Storage, "postgres") {
		if cfg.PostgresHost != "" {
			builder.WriteString(fmt.Sprintf("- postgres host: %s\n", cfg.PostgresHost))
		}
		if cfg.PostgresDatabase != "" {
			builder.WriteString(fmt.Sprintf("- postgres database: %s\n", cfg.PostgresDatabase))
		}
		if cfg.PostgresUser != "" {
			builder.WriteString(fmt.Sprintf("- postgres user: %s\n", cfg.PostgresUser))
		}
		if cfg.PostgresSSLMode != "" {
			builder.WriteString(fmt.Sprintf("- postgres ssl mode: %s\n", cfg.PostgresSSLMode))
		}
	}
	builder.WriteString(fmt.Sprintf("- nats mode: %s\n", cfg.NATSMode))
	if len(cfg.NATSURLs) > 0 {
		builder.WriteString(fmt.Sprintf("- nats urls: %s\n", strings.Join(cfg.NATSURLs, ", ")))
	}
	if cfg.GenerateCerts {
		builder.WriteString("- tls: generate self-signed certs\n")
	} else {
		builder.WriteString("- tls: provided\n")
	}
	if cfg.TLSCertFile != "" {
		builder.WriteString(fmt.Sprintf("- tls cert: %s\n", cfg.TLSCertFile))
	}
	if cfg.TLSKeyFile != "" {
		builder.WriteString(fmt.Sprintf("- tls key: %s\n", cfg.TLSKeyFile))
	}
	if cfg.TLSCAFile != "" {
		builder.WriteString(fmt.Sprintf("- tls ca: %s\n", cfg.TLSCAFile))
	}
	if cfg.TLSCSRFile != "" {
		builder.WriteString(fmt.Sprintf("- tls csr: %s\n", cfg.TLSCSRFile))
	}
	if cfg.TLSRenewalCommand != "" {
		builder.WriteString("- tls renewal: configured\n")
	}
	if cfg.TLSRenewalScriptPath != "" {
		builder.WriteString(fmt.Sprintf("- tls renewal script: %s\n", cfg.TLSRenewalScriptPath))
	}
	if cfg.PackageChannel != "" {
		builder.WriteString(fmt.Sprintf("- package channel: %s\n", cfg.PackageChannel))
	}
	if cfg.PackageVersion != "" {
		builder.WriteString(fmt.Sprintf("- package version: %s\n", cfg.PackageVersion))
	}
	if cfg.MigrateFromSQLite != "" {
		builder.WriteString(fmt.Sprintf("- migrate from sqlite: %s\n", cfg.MigrateFromSQLite))
	}
	if cfg.MigrateBatchSize > 0 {
		builder.WriteString(fmt.Sprintf("- migrate batch size: %d\n", cfg.MigrateBatchSize))
	}
	if cfg.MigrateContinueOnError {
		builder.WriteString("- migrate continue on error: true\n")
	}
	if cfg.MigrateSkipExisting {
		builder.WriteString("- migrate skip existing: true\n")
	}
	if cfg.BlueprintsDir != "" {
		builder.WriteString(fmt.Sprintf("- blueprints dir: %s\n", cfg.BlueprintsDir))
	}
	if len(cfg.ApplyBlueprints) > 0 {
		builder.WriteString(fmt.Sprintf("- apply blueprints: %s\n", strings.Join(cfg.ApplyBlueprints, ", ")))
	}
	if cfg.ExportStatesDir != "" {
		builder.WriteString(fmt.Sprintf("- export states dir: %s\n", cfg.ExportStatesDir))
	}
	return builder.String()
}

func renderArtifactsSummary(artifacts *InstallArtifacts) string {
	if artifacts == nil {
		return "- no install artifacts recorded\n"
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("- package manager: %s\n", artifacts.PackageManager))
	if len(artifacts.Packages) > 0 {
		builder.WriteString(fmt.Sprintf("- packages: %s\n", strings.Join(limitList(artifacts.Packages, 20), ", ")))
	}
	if len(artifacts.CreatedFiles) > 0 {
		builder.WriteString(fmt.Sprintf("- created files: %s\n", strings.Join(limitList(artifacts.CreatedFiles, 40), ", ")))
	}
	return builder.String()
}

func renderLogSummary(ctx context.Context, state *State) string {
	if state == nil || state.System == nil || state.System.Platform == nil {
		return "- system detection unavailable\n"
	}

	initSystem := string(state.System.Platform.InitSystem)
	var builder strings.Builder
	services := []string{}
	if state.BootstrapConfig != nil {
		if requiresServer(state.BootstrapConfig) {
			services = append(services, "kscore-server")
		}
		if requiresAgent(state.BootstrapConfig) {
			services = append(services, "kscore-agent")
		}
	}
	if len(services) == 0 {
		services = append(services, "kscore-agent")
	}

	for _, service := range services {
		builder.WriteString(fmt.Sprintf("- %s status:\n", service))
		builder.WriteString(indentLines(collectServiceStatus(ctx, initSystem, service), 2))
		builder.WriteString(fmt.Sprintf("- %s logs:\n", service))
		builder.WriteString(indentLines(collectServiceLogs(ctx, initSystem, service, 120), 2))
	}

	return builder.String()
}

func collectServiceStatus(ctx context.Context, initSystem, service string) string {
	command, ok := serviceStatusCommand(initSystem, service)
	if !ok {
		return "status command unavailable\n"
	}
	output, err := runCommandWithTimeout(ctx, 5*time.Second, command.Name, command.Args...)
	if err != nil {
		return fmt.Sprintf("command failed: %v\noutput:\n%s\n", err, strings.TrimSpace(output))
	}
	if output == "" {
		return "no output\n"
	}
	return strings.TrimSpace(output) + "\n"
}

func collectServiceLogs(ctx context.Context, initSystem, service string, lines int) string {
	if strings.ToLower(initSystem) != "systemd" {
		return "log collection unsupported for init system\n"
	}
	output, err := runCommandWithTimeout(ctx, 5*time.Second, "journalctl", "-u", service, "-n", fmt.Sprintf("%d", lines), "--no-pager")
	if err != nil {
		return fmt.Sprintf("command failed: %v\noutput:\n%s\n", err, strings.TrimSpace(output))
	}
	if strings.TrimSpace(output) == "" {
		return "no log output\n"
	}
	return strings.TrimRight(output, "\n") + "\n"
}

func runCommandWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return execCommand(timeoutCtx, name, args...)
}

func writeDiagnosticsReport(report string) (string, error) {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	filename := fmt.Sprintf("%s-%s.log", diagnosticsFilePrefix, timestamp)
	dir := "/var/log/kscore"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func diagnosticHints(cfg *BootstrapConfig, failure error) []string {
	if failure == nil {
		return nil
	}

	message := strings.ToLower(failure.Error())
	hints := []string{}
	add := func(text string) {
		for _, existing := range hints {
			if existing == text {
				return
			}
		}
		hints = append(hints, text)
	}

	if strings.Contains(message, "permission denied") {
		add("run bootstrap as root or with sudo")
	}
	if strings.Contains(message, "unsupported package manager") {
		add("verify the host distribution is supported by the bootstrap package manager")
	}
	if strings.Contains(message, "postgres host is required") {
		add("set --postgres-host or KSCORE_POSTGRES_HOST for postgres storage")
	}
	if strings.Contains(message, "nats urls") {
		add("set --nats-urls or KSCORE_NATS_URLS for external or leaf nats mode")
	}
	if strings.Contains(message, "no packages to install") {
		add("select a node role with packages (agent, control-plane, or both)")
	}
	if strings.Contains(message, "certificate") || strings.Contains(message, "tls") {
		add("provide TLS certs or use --generate-certs for demo/standalone modes")
	}
	if strings.Contains(message, "connection refused") {
		add("check network access and confirm required services are reachable")
	}

	if cfg != nil {
		if strings.EqualFold(cfg.Storage, "postgres") && cfg.PostgresHost == "" {
			add("postgres storage requires --postgres-host or KSCORE_POSTGRES_HOST")
		}
		if cfg.Join != "" && cfg.JoinToken == "" {
			add("join mode requires --join-token or KSCORE_JOIN_TOKEN")
		}
		if cfg.NATSMode == "external" && len(cfg.NATSURLs) == 0 {
			add("external nats mode requires --nats-urls or KSCORE_NATS_URLS")
		}
		if !cfg.GenerateCerts && cfg.TLSCertFile == "" && cfg.TLSCSRFile == "" {
			add("provide TLS certs, CSR, or enable --generate-certs")
		}
	}

	return hints
}

func limitList(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	copied := append([]string(nil), items[:limit]...)
	copied = append(copied, fmt.Sprintf("... (%d more)", len(items)-limit))
	return copied
}

func indentLines(output string, spaces int) string {
	if output == "" {
		return strings.Repeat(" ", spaces) + "(no output)\n"
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}
