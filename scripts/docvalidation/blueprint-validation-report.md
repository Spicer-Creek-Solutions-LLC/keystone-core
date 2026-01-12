# Blueprint Validation Report

## Summary

- **Blueprints**: 3
- **State Files**: 18
- **Total States**: 148
- **Valid Files**: 18
- **Invalid Files**: 0
- **Files with Warnings**: 0
- **Go Template Usage**: 17 files

## Blueprints

| Blueprint | Version | Parameters | Features | Valid |
|-----------|---------|------------|----------|-------|
| lamp-stack | 1.0.0 | 16 | 4 | ✓ |
| monitoring-stack | 1.0.0 | 19 | 6 | ✓ |
| security-baseline | 1.0.0 | 20 | 7 | ✓ |

## State Files by Blueprint

### security-baseline

| File | States | Valid | Template |
|------|--------|-------|----------|
| audit.yaml | 7 | ✓ | Go |
| fail2ban.yaml | 5 | ✓ | Go |
| firewall.yaml | 10 | ✓ | Go |
| kernel.yaml | 3 | ✓ |  |
| main.yaml | 0 | ✓ | Go |
| ssh.yaml | 9 | ✓ | Go |
| system.yaml | 18 | ✓ | Go |
| updates.yaml | 8 | ✓ | Go |

### lamp-stack

| File | States | Valid | Template |
|------|--------|-------|----------|
| apache.yaml | 11 | ✓ | Go |
| main.yaml | 1 | ✓ | Go |
| mysql.yaml | 15 | ✓ | Go |
| php.yaml | 15 | ✓ | Go |

### monitoring-stack

| File | States | Valid | Template |
|------|--------|-------|----------|
| alertmanager.yaml | 9 | ✓ | Go |
| common.yaml | 8 | ✓ | Go |
| grafana.yaml | 13 | ✓ | Go |
| main.yaml | 0 | ✓ | Go |
| node_exporter.yaml | 6 | ✓ | Go |
| prometheus.yaml | 10 | ✓ | Go |

