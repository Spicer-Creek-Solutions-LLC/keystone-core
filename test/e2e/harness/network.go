// Package harness provides a test harness for container-based E2E testing.
// This file contains network partition helpers for HA resilience tests.
package harness

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PartitionService blocks all network traffic from one service to another using iptables.
// Requires NET_ADMIN capability on the source container.
func (e *HAClusterEnvironment) PartitionService(ctx context.Context, from, to string) error {
	ip, err := e.resolveServiceIP(ctx, from, to)
	if err != nil {
		return fmt.Errorf("resolve %s from %s: %w", to, from, err)
	}

	_, err = e.execInServiceAsRoot(ctx, from, "iptables", "-A", "OUTPUT", "-d", ip, "-j", "DROP")
	if err != nil {
		return fmt.Errorf("partition %s -> %s: %w", from, to, err)
	}

	_, err = e.execInServiceAsRoot(ctx, from, "iptables", "-A", "INPUT", "-s", ip, "-j", "DROP")
	if err != nil {
		return fmt.Errorf("partition %s <- %s: %w", from, to, err)
	}

	return nil
}

// HealPartition restores network traffic from one service to another.
func (e *HAClusterEnvironment) HealPartition(ctx context.Context, from, to string) error {
	ip, err := e.resolveServiceIP(ctx, from, to)
	if err != nil {
		return fmt.Errorf("resolve %s from %s: %w", to, from, err)
	}

	// Delete OUTPUT rule — ignore errors if rule doesn't exist.
	e.execInServiceAsRoot(ctx, from, "iptables", "-D", "OUTPUT", "-d", ip, "-j", "DROP") //nolint:errcheck // best-effort cleanup
	// Delete INPUT rule.
	e.execInServiceAsRoot(ctx, from, "iptables", "-D", "INPUT", "-s", ip, "-j", "DROP") //nolint:errcheck // best-effort cleanup

	return nil
}

// HealAllPartitions flushes all iptables rules in a service container, restoring full connectivity.
func (e *HAClusterEnvironment) HealAllPartitions(ctx context.Context, service string) error {
	_, err := e.execInServiceAsRoot(ctx, service, "iptables", "-F")
	if err != nil {
		return fmt.Errorf("flush iptables in %s: %w", service, err)
	}
	return nil
}

// resolveServiceIP resolves a target service's IP address from within a source container.
func (e *HAClusterEnvironment) resolveServiceIP(ctx context.Context, from, to string) (string, error) {
	out, err := e.execInServiceAsRoot(ctx, from, "getent", "hosts", to)
	if err != nil {
		return "", err
	}
	// getent hosts output: "172.20.0.5      nats-1"
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("could not resolve %s: empty output", to)
	}
	return fields[0], nil
}

// execInServiceAsRoot runs a command inside a service container as root user.
func (e *HAClusterEnvironment) execInServiceAsRoot(ctx context.Context, service string, cmd ...string) (string, error) {
	args := make([]string, 0, 10+len(cmd))
	args = append(args,
		"compose",
		"-f", e.ComposeFile,
		"-p", e.ProjectName,
		"exec", "-T", "--user", "root", service,
	)
	args = append(args, cmd...)

	c := exec.CommandContext(ctx, "docker", args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec in %s (root): %w: %s", service, err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}
