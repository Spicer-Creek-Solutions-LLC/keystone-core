# Monitoring Stack Blueprint

A production-ready monitoring stack blueprint deploying Prometheus, Grafana, Node Exporter, and Alertmanager.

## Overview

This blueprint provides a complete observability stack for infrastructure monitoring:

- **Prometheus** - Metrics collection and alerting
- **Grafana** - Visualization and dashboards
- **Node Exporter** - System-level metrics
- **Alertmanager** - Alert routing and notification

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/monitoring-stack@1.0.0
    params:
      grafana_admin_password: !secret monitoring/grafana_admin
```

## Parameters

### Prometheus

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `prometheus_version` | string | 2.48.0 | Prometheus version |
| `prometheus_retention` | string | 15d | Data retention period |
| `prometheus_storage_path` | string | /var/lib/prometheus | Data directory |
| `prometheus_port` | integer | 9090 | Web UI port |
| `prometheus_scrape_interval` | string | 15s | Default scrape interval |
| `alertmanager_enabled` | boolean | true | Include Alertmanager targets in Prometheus config |
| `node_exporter_enabled` | boolean | true | Include Node Exporter targets in Prometheus config |
| `os_family` | string | debian | OS family for package selection |

### Grafana

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `grafana_version` | string | 10.2.2 | Grafana version |
| `grafana_port` | integer | 3000 | Web UI port |
| `grafana_admin_user` | string | admin | Admin username |
| `grafana_admin_password` | string | **required** | Admin password |
| `grafana_domain` | string | localhost | Server domain |
| `grafana_root_url` | string | %(protocol)s://%(domain)s:%(http_port)s/ | Root URL |
| `grafana_secret_key` | string | (generated) | Secret key for signing |

### Node Exporter

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `node_exporter_version` | string | 1.7.0 | Node Exporter version |
| `node_exporter_port` | integer | 9100 | Metrics port |
| `node_exporter_collectors` | array | [cpu, diskstats, ...] | Enabled collectors |

### Alertmanager

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `alertmanager_version` | string | 0.26.0 | Alertmanager version |
| `alertmanager_port` | integer | 9093 | Web UI port |

### Feature Flags

| Feature | Default | Description |
|---------|---------|-------------|
| `alertmanager` | true | Install Alertmanager |
| `grafana` | true | Install Grafana |
| `node_exporter` | true | Install Node Exporter |
| `blackbox_exporter` | false | Install Blackbox Exporter |
| `default_dashboards` | true | Install default dashboards |
| `default_alerts` | true | Install default alert rules |

## Usage Examples

### Minimal Setup

```yaml
include:
  - blueprint: blueprints/kscore/monitoring-stack@1.0.0
    params:
      grafana_admin_password: changeme
```

### Production Setup

```yaml
include:
  - blueprint: blueprints/kscore/monitoring-stack@1.0.0
    params:
      prometheus_retention: "30d"
      prometheus_storage_path: /data/prometheus
      grafana_admin_password: !secret monitoring/grafana
      grafana_domain: monitoring.example.com
    features:
      alertmanager: true
      default_alerts: true
```

### Prometheus Only

```yaml
include:
  - blueprint: blueprints/kscore/monitoring-stack@1.0.0
    features:
      grafana: false
      alertmanager: false
      node_exporter: true
```

## Default Dashboards

When `features.default_dashboards` is enabled, the following dashboards are installed:

1. **Node Exporter Dashboard** - System metrics (CPU, memory, disk, network)
2. **Prometheus Stats** - Prometheus internal metrics

## Default Alert Rules

When `features.default_alerts` is enabled, alert rules for:

- Instance availability
- High CPU/memory usage
- Disk space warnings
- Network errors
- Prometheus health

## Adding Custom Targets

Create a file in `/etc/monitoring/prometheus/targets/`:

```yaml
# /etc/monitoring/prometheus/targets/kscore-agents.yml
- targets:
    - agent1.example.com:9100
    - agent2.example.com:9100
  labels:
    job: node
    env: production
```

## Accessing Services

After deployment:

- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000
- **Alertmanager**: http://localhost:9093
- **Node Exporter**: http://localhost:9100/metrics

## Platform Support

| Platform | Support |
|----------|---------|
| Debian 11/12 | Full |
| Ubuntu 20.04+ | Full |
| RHEL 8/9 | Full |
| CentOS Stream | Full |

## Security Notes

1. Change default Grafana admin password immediately
2. Configure firewall rules to restrict access
3. Use TLS for production deployments
4. Configure proper authentication for Alertmanager webhooks

## Troubleshooting

### Prometheus won't start

Check configuration syntax:
```bash
promtool check config /etc/monitoring/prometheus/prometheus.yml
```

### No metrics in Grafana

1. Verify Prometheus is running: `systemctl status prometheus`
2. Check target status in Prometheus UI: http://localhost:9090/targets
3. Verify datasource in Grafana is configured correctly

### Alerts not firing

1. Check Alertmanager is running: `systemctl status alertmanager`
2. Verify Prometheus can reach Alertmanager
3. Check alert rules: `promtool check rules /etc/monitoring/prometheus/rules/*.yml`

## Version History

- **1.0.0** - Initial release with Prometheus 2.48.0, Grafana 10.2.2
