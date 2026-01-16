# metrics-only Blueprint

Lightweight Prometheus-only metrics blueprint.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/metrics-only@0.1.0
    params:
      retention: 7d
      remote_write_url: https://prom.example.com/api/v1/write
```

## Parameters

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `retention` | string | 15d | Metrics retention period |
| `remote_write_url` | string |  | Remote write endpoint |
| `scrape_interval` | string | 15s | Scrape interval |
| `prometheus_version` | string | 2.48.0 | Prometheus version |
| `prometheus_port` | int | 9090 | Prometheus port |
| `storage_path` | string | /var/lib/prometheus | Prometheus storage path |
