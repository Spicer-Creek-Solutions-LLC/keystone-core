---
title: "Blueprint Parameter Defaults"
weight: 45
description: "Reference guide for standard blueprint parameter defaults and rationale"
---

This document provides a comprehensive review of parameter defaults across Keystone Core standard blueprints, explaining the rationale behind each default value and providing guidance on when to override them.

## Design Principles

Blueprint parameter defaults follow these principles:

1. **Secure by default** - Security-sensitive parameters default to the most secure option
2. **Production-ready** - Defaults suitable for production use without modification
3. **Conservative resources** - Reasonable resource limits that work on modest hardware
4. **Explicit over implicit** - Required parameters for critical values rather than risky defaults
5. **Standards compliance** - Follow industry standards (CIS benchmarks, NIST guidelines)

## Security Baseline Blueprint

The `security-baseline` blueprint implements security hardening best practices.

### SSH Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `ssh_port` | `22` | Standard port; change for security-through-obscurity if desired |
| `ssh_permit_root_login` | `no` | CIS benchmark requirement; use sudo instead |
| `ssh_password_authentication` | `no` | Key-based auth is more secure; prevents brute force |
| `ssh_max_auth_tries` | `3` | Limits brute force attempts while allowing typos |
| `ssh_client_alive_interval` | `300` | 5-minute timeout for idle sessions |
| `ssh_client_alive_count_max` | `2` | Total 10-minute timeout (300s × 2) before disconnect |
| `ssh_allowed_users` | `[]` | Empty = all users; set explicitly in production |
| `ssh_allowed_groups` | `[]` | Empty = all groups; recommend setting to specific group |

**Recommendations:**

- Always set `ssh_allowed_users` or `ssh_allowed_groups` in production
- Consider changing `ssh_port` on internet-facing servers
- Use `prohibit-password` for root if you need emergency root access via console

### Firewall Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `firewall_default_policy` | `deny` | Whitelist approach; only allow explicit ports |
| `firewall_allowed_ports` | `["22/tcp"]` | Minimum required for SSH management |
| `firewall_allowed_ips` | `[]` | No IP whitelisting by default |

**Recommendations:**

- Add application-specific ports to `firewall_allowed_ports`
- Use `firewall_allowed_ips` for management networks in enterprise environments

### Password Policy

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `password_max_days` | `90` | PCI-DSS and many compliance frameworks require ≤90 days |
| `password_min_days` | `7` | Prevents rapid password cycling to reuse old passwords |
| `password_warn_days` | `14` | Two weeks notice for password expiration |
| `password_min_length` | `12` | NIST 800-63B recommends 8+ chars; 12 provides better security |
| `password_require_upper` | `true` | Character complexity requirement |
| `password_require_lower` | `true` | Character complexity requirement |
| `password_require_numeric` | `true` | Character complexity requirement |
| `password_require_special` | `true` | Character complexity requirement |

**Note:** NIST 800-63B (2024 revision) now recommends length over complexity. Consider:

- Increasing `password_min_length` to 14-16 characters
- Setting complexity requirements to `false` with longer minimum length
- Using passphrases instead of complex passwords

### System Hardening

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `umask` | `027` | Restrictive default; new files readable only by owner/group |
| `disable_root_login` | `true` | Prevents direct root console login |
| `sysctl_disable_ipv6` | `false` | IPv6 is increasingly required; disable only if not needed |
| `sysctl_disable_ip_forward` | `true` | Disabled unless system is a router |

### Audit Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `audit_enabled` | `true` | Essential for security compliance and forensics |
| `audit_max_log_file` | `50` | 50MB per log file balances detail and disk usage |
| `audit_num_logs` | `10` | 500MB total audit storage; increase for compliance |

### Fail2ban Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `fail2ban_bantime` | `3600` | 1-hour ban is effective without permanent lockout |
| `fail2ban_findtime` | `600` | 10-minute window for counting failures |
| `fail2ban_maxretry` | `5` | Allows for typos; low enough to stop brute force |

**Recommendations:**

- Enable `fail2ban` feature (disabled by default) for internet-facing servers
- Consider increasing `fail2ban_bantime` to 86400 (24 hours) for repeated offenders

### Automatic Updates

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `updates_auto_reboot` | `false` | Manual control over reboots in production |
| `updates_reboot_time` | `02:00` | Off-peak hours if auto-reboot enabled |

**Recommendations:**

- Enable `automatic_updates` feature for security updates
- Consider enabling `updates_auto_reboot` for non-critical systems

---

## Monitoring Stack Blueprint

The `monitoring-stack` blueprint deploys Prometheus, Grafana, and related tools.

### Version Selection

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `prometheus_version` | `2.48.0` | LTS-stable version with proven reliability |
| `grafana_version` | `10.2.2` | Recent stable release with security patches |
| `node_exporter_version` | `1.7.0` | Current stable release |
| `alertmanager_version` | `0.26.0` | Current stable release |

**Recommendations:**

- Pin versions explicitly for reproducible deployments
- Update versions regularly for security patches
- Test new versions in staging before production

### Prometheus Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `prometheus_retention` | `15d` | 2 weeks balances disk usage and historical analysis |
| `prometheus_storage_path` | `/var/lib/prometheus` | Standard FHS location |
| `prometheus_port` | `9090` | Standard Prometheus port |
| `prometheus_scrape_interval` | `15s` | Good balance of granularity and resource usage |

**Sizing guidance:**

- 15d retention uses ~1-2GB per 100 time series at 15s scrape
- Increase retention for capacity planning analysis
- Consider remote storage (Thanos, Cortex) for long-term retention

### Grafana Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `grafana_port` | `3000` | Standard Grafana port |
| `grafana_admin_user` | `admin` | Standard default; consider changing for security |
| `grafana_domain` | `localhost` | Safe default; set to actual domain in production |

**Security notes:**

- `grafana_admin_password` is **required** (no default) - must be set explicitly
- `grafana_secret_key` is **required** - used for signing cookies and tokens
- Consider placing Grafana behind a reverse proxy with TLS

### Node Exporter Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `node_exporter_port` | `9100` | Standard Node Exporter port |
| `node_exporter_collectors` | `[cpu, diskstats, ...]` | Essential collectors for system monitoring |

**Default collectors explanation:**

- `cpu` - CPU usage metrics
- `diskstats` - Disk I/O metrics
- `filesystem` - Disk space metrics
- `loadavg` - System load averages
- `meminfo` - Memory usage
- `netdev` - Network interface metrics
- `stat` - System statistics
- `time` - System time
- `uname` - System information

---

## Production Cluster Blueprint

The `production-cluster` blueprint deploys Keystone Core in HA configuration.

### Cluster Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `cluster_name` | `keystone` | Descriptive default; change for multi-cluster environments |
| `node_role` | `control-plane` | Most common deployment target |
| `nats_mode` | `embedded-cluster` | Simplest deployment; external for large scale |

### Database Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `postgres_port` | `5432` | Standard PostgreSQL port |
| `postgres_database` | `keystone` | Clear naming convention |
| `postgres_user` | `keystone` | Application-specific user (not superuser) |

**Security notes:**

- `postgres_password` is **required** - no default for security
- `postgres_host` is **required** - explicit configuration required
- Consider using connection pooling (PgBouncer) for production

### TLS Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `tls_mode` | `generate` | Auto-generates self-signed certs for initial setup |

**Modes explanation:**

- `generate` - Auto-generate self-signed certificates (development/testing)
- `provided` - Use existing certificates (production with internal CA)
- `letsencrypt` - Automatic Let's Encrypt certificates (public-facing)

**Recommendations:**

- Use `provided` mode with organizational PKI for production
- Use `letsencrypt` for internet-accessible deployments
- Never use `generate` mode in production

### Backup Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| `backup_enabled` | `true` | Backups essential for production |
| `backup_destination` | `local` | Local storage; configure remote for DR |

---

## Enterprise Platform Blueprint

The `enterprise-platform` blueprint extends production-cluster with additional features.

### Feature Defaults

| Feature | Default | Rationale |
|---------|---------|-----------|
| `nats_cluster` | `true` | Core messaging infrastructure |
| `postgres_ha` | `true` | Database high availability |
| `monitoring` | `true` | Observability for enterprise |
| `security` | `true` | Security baseline applied |
| `gitops` | `false` | Requires Git repository configuration |
| `identity` | `true` | SPIFFE/SPIRE identity management |
| `proxy` | `false` | Network device management |
| `file_distribution` | `false` | File distribution service |

---

## Parameter Validation

All blueprints enforce validation rules:

### Type Validation

- `string` - Any text value
- `integer` - Whole numbers only
- `boolean` - `true` or `false`
- `array` - List of values

### Enum Validation

Parameters with `enum` only accept listed values.

### Required Parameters

Parameters marked `required: true` must be explicitly set:

- `postgres_password` - Database credentials
- `postgres_host` - Database connection
- `control_plane_nodes` - Cluster membership
- `grafana_admin_password` - Dashboard security
- `grafana_secret_key` - Session signing

### Sensitive Parameters

Parameters marked `sensitive: true` are:

- Masked in logs and output
- Stored encrypted in state
- Not displayed in dry-run output

---

## Overriding Defaults

### Environment-Specific Overrides

```yaml
# development.yaml
parameters:
  prometheus_retention: "7d"
  tls_mode: "generate"
  backup_enabled: false

# production.yaml
parameters:
  prometheus_retention: "30d"
  tls_mode: "provided"
  backup_enabled: true
  backup_destination: "s3://backups/keystone"
```

### Best Practices

1. **Use parameter files** - Don't embed values in commands
2. **Version control** - Track parameter files in Git
3. **Secrets management** - Use external secrets for sensitive values
4. **Environment separation** - Maintain separate files per environment
5. **Document overrides** - Comment why defaults were changed

---

## Changelog

### Version 1.0.0

- Initial parameter defaults review
- Documented rationale for all security-baseline parameters
- Added sizing guidance for monitoring stack
- Clarified required vs optional parameters
