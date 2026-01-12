# Security Baseline Blueprint

A comprehensive security hardening blueprint implementing industry best practices for Linux servers.

## Overview

This blueprint applies security hardening configurations including:

- **SSH Hardening** - Secure SSH configuration with modern ciphers
- **Firewall** - UFW (Debian/Ubuntu) or firewalld (RHEL/CentOS)
- **System Hardening** - Password policies, secure permissions
- **Kernel Hardening** - Sysctl security parameters
- **Audit Logging** - Auditd for security event logging

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/security-baseline@1.0.0
    params:
      ssh_allowed_users:
        - admin
        - deploy
```

## Parameters

### SSH Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `ssh_port` | integer | 22 | SSH service port |
| `ssh_permit_root_login` | string | no | PermitRootLogin setting |
| `ssh_password_authentication` | string | no | Password authentication |
| `ssh_max_auth_tries` | integer | 3 | Max authentication attempts |
| `ssh_client_alive_interval` | integer | 300 | Client alive interval |
| `ssh_client_alive_count_max` | integer | 2 | Client alive count max |
| `ssh_allowed_users` | array | [] | Users allowed SSH access |
| `ssh_allowed_groups` | array | [] | Groups allowed SSH access |

### Firewall Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `firewall_default_policy` | string | deny | Default incoming policy |
| `firewall_allowed_ports` | array | [22/tcp] | Ports to allow |
| `firewall_allowed_ips` | array | [] | IPs to allow all traffic from |

### System Hardening

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `disable_root_login` | boolean | true | Disable direct root login |
| `password_max_days` | integer | 90 | Password max age |
| `password_min_days` | integer | 7 | Password min age |
| `password_warn_days` | integer | 14 | Password warning days |
| `umask` | string | 027 | Default umask |

### Feature Flags

| Feature | Default | Description |
|---------|---------|-------------|
| `ssh_hardening` | true | Apply SSH hardening |
| `firewall` | true | Configure firewall |
| `system_hardening` | true | Apply system hardening |
| `audit_logging` | true | Configure auditd |
| `kernel_hardening` | true | Apply kernel sysctl settings |
| `fail2ban` | false | Install fail2ban |
| `automatic_updates` | false | Enable automatic security updates |

## Usage Examples

### Basic Hardening

```yaml
include:
  - blueprint: blueprints/kscore/security-baseline@1.0.0
```

### Custom SSH Configuration

```yaml
include:
  - blueprint: blueprints/kscore/security-baseline@1.0.0
    params:
      ssh_port: 2222
      ssh_password_authentication: "no"
      ssh_allowed_users:
        - admin
        - deploy
      ssh_allowed_groups:
        - wheel
        - ssh-users
```

### Production Web Server

```yaml
include:
  - blueprint: blueprints/kscore/security-baseline@1.0.0
    params:
      firewall_allowed_ports:
        - 22/tcp
        - 80/tcp
        - 443/tcp
      password_max_days: 60
      ssh_allowed_groups:
        - sysadmins
    features:
      fail2ban: true
      automatic_updates: true
```

### Hardening Without Firewall (Managed Elsewhere)

```yaml
include:
  - blueprint: blueprints/kscore/security-baseline@1.0.0
    features:
      firewall: false
```

## Security Features Applied

### SSH Hardening

- Modern cipher suites only (chacha20-poly1305, aes256-gcm)
- Strong key exchange algorithms
- Disabled password authentication (by default)
- Restricted root login
- Session timeout
- Login banner

### Kernel Hardening (sysctl)

- IP forwarding disabled
- Source routing disabled
- ICMP redirects disabled
- SYN flood protection
- Memory protection (ASLR, restricted dmesg)
- Kernel pointer restriction
- Unprivileged BPF disabled

### System Hardening

- Secure file permissions
- Password aging policies
- Umask restrictions
- Core dumps disabled
- Unnecessary packages removed
- Restricted su to wheel group

### Audit Logging

- User/group changes
- Authentication events
- Privileged command execution
- System configuration changes
- File access attempts
- Kernel module loading

## Compliance Mapping

This blueprint helps meet requirements from:

- **CIS Benchmarks** - Center for Internet Security hardening guidelines
- **STIG** - Security Technical Implementation Guides
- **PCI DSS** - Payment Card Industry Data Security Standard
- **HIPAA** - Health Insurance Portability and Accountability Act

## Platform Support

| Platform | Firewall | Support |
|----------|----------|---------|
| Debian 11/12 | UFW | Full |
| Ubuntu 20.04+ | UFW | Full |
| RHEL 8/9 | firewalld | Full |
| CentOS Stream | firewalld | Full |

## Post-Deployment Verification

Run security scan after deployment:

```bash
# SSH configuration test
ssh-audit localhost

# Check open ports
ss -tlnp

# Verify audit rules
auditctl -l

# Check sysctl settings
sysctl -a | grep -E "net.ipv4.conf.all.(rp_filter|accept_redirects)"
```

## Important Notes

1. **SSH Keys** - Ensure SSH keys are deployed before enabling this blueprint (password auth disabled by default)
2. **Firewall** - Review firewall rules before deployment to avoid lockout
3. **Root Login** - Console root login is disabled; use sudo
4. **Audit Logs** - Monitor /var/log/audit/audit.log for security events

## Troubleshooting

### Locked out of SSH

1. Access via console/IPMI
2. Edit /etc/ssh/sshd_config
3. Set PasswordAuthentication yes temporarily
4. Restart SSH: `systemctl restart sshd`
5. Fix SSH keys, then disable password auth again

### Firewall blocking legitimate traffic

```bash
# UFW
ufw status verbose
ufw allow <port>/tcp

# firewalld
firewall-cmd --list-all
firewall-cmd --permanent --add-port=<port>/tcp && firewall-cmd --reload
```

## Version History

- **1.0.0** - Initial release with SSH, firewall, system, kernel, and audit hardening
