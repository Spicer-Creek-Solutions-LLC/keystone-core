---
title: "Edge Deployment"
weight: 25
description: >
  Deploy and manage distributed edge infrastructure with intermittent connectivity
---

This scenario demonstrates managing edge locations with Keystone Core, handling intermittent connectivity, local caching, and autonomous operation.

## Overview

Edge deployments present unique challenges:

- **Intermittent Connectivity**: Network links may be unreliable
- **Local Autonomy**: Edge nodes must operate independently when disconnected
- **Resource Constraints**: Limited compute and storage at edge locations
- **Scale**: Potentially thousands of edge locations

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Central Data Center                         │
│  ┌─────────────────┐  ┌─────────────────┐  ┌────────────────┐  │
│  │ Control Plane   │  │ State Store     │  │ File Server    │  │
│  │ (HA Cluster)    │  │ (PostgreSQL)    │  │ (CDN Origin)   │  │
│  └────────┬────────┘  └────────┬────────┘  └────────┬───────┘  │
└───────────┼─────────────────────┼─────────────────────┼─────────┘
            │                     │                     │
     ┌──────┴─────────────────────┴─────────────────────┴───────┐
     │                    WAN / Internet                         │
     └──────┬─────────────────────┬─────────────────────┬───────┘
            │                     │                     │
    ┌───────┴───────┐     ┌───────┴───────┐     ┌───────┴───────┐
    │  Edge Site 1  │     │  Edge Site 2  │     │  Edge Site N  │
    │ ┌───────────┐ │     │ ┌───────────┐ │     │ ┌───────────┐ │
    │ │   Agent   │ │     │ │   Agent   │ │     │ │   Agent   │ │
    │ │  (Proxy)  │ │     │ │  (Proxy)  │ │     │ │  (Proxy)  │ │
    │ └─────┬─────┘ │     │ └─────┬─────┘ │     │ └─────┬─────┘ │
    │ ┌─────┴─────┐ │     │ ┌─────┴─────┐ │     │ ┌─────┴─────┐ │
    │ │  Devices  │ │     │ │  Devices  │ │     │ │  Devices  │ │
    │ └───────────┘ │     │ └───────────┘ │     │ └───────────┘ │
    └───────────────┘     └───────────────┘     └───────────────┘
```

## Prerequisites

- Central control plane with HA
- Edge locations with network connectivity (even intermittent)
- Proxy agent at each edge site

## Implementation

### 1. Edge Agent Configuration

> **Note:** Offline caching, command queuing, and proxy mode for downstream devices are planned for a future release. The configuration below covers the currently supported agent and NATS settings for edge deployments.

```yaml
# edge-agent-config.yaml
metadata:
  name: edge-agent-config

target: "role:edge-proxy"

file:
  edge_agent_config:
    state: present
    name: /etc/keystone-core/agent.yaml
    contents: |
      # Edge-optimized agent configuration
      agent:
        id: "{{ .vars.site_id }}-agent"
        heartbeat_interval: 60s
        command_timeout: 10m
        labels:
          role: edge-proxy
          site: "{{ .vars.site_id }}"

      # Leaf node mode connects to central NATS via leaf protocol
      nats:
        mode: leaf
        maxreconnects: -1
        reconnectwait: 5s
        embedded:
          port: 4222
          listen: "0.0.0.0:4222"
          leaf_node_urls:
            - "{{ .vars.control_plane_url }}"
          enablejetstream: true
          storedir: /var/lib/keystone-core/nats

      logging:
        level: info
        format: json
```

### 2. Edge Site State

```yaml
# edge-site-base.yaml
metadata:
  name: edge-site-base

variables:
  site_id: ""
  timezone: "UTC"
  ntp_server: "time.example.com"

# System configuration
timezone:
  set_timezone:
    state: present
    name: "{{ .vars.timezone }}"

# Local NTP for time sync
package:
  chrony:
    state: installed
    name: chrony

  prometheus_node_exporter:
    state: installed
    name: prometheus-node-exporter

file:
  chrony_config:
    state: present
    name: /etc/chrony.conf
    contents: |
      # Use central NTP when available, local fallback
      server {{ .vars.ntp_server }} iburst prefer
      server 0.pool.ntp.org iburst
      server 1.pool.ntp.org iburst

      # Allow local network to sync
      allow 10.0.0.0/8
      allow 192.168.0.0/16

      # Serve time even when not synced
      local stratum 10

service:
  chronyd:
    state: running
    name: chronyd
    enabled: true
    require:
      - file: chrony_config

  node_exporter:
    state: running
    name: node_exporter
    enabled: true
```

### 3. Device Management

```yaml
# edge-device-management.yaml
metadata:
  name: edge-device-management

target: "role:edge-proxy"

# Device discovery script
file:
  discover_devices_script:
    state: present
    name: /usr/local/bin/discover-devices.sh
    mode: "0755"
    contents: |
      #!/bin/bash
      # Discover devices on local network
      nmap -sn 192.168.1.0/24 -oG - | \
        awk '/Up$/{print $2}' | \
        while read ip; do
          # Register device with agent
          curl -s -X POST http://localhost:8080/api/devices \
            -d "{\"ip\": \"$ip\", \"site\": \"{{ .facts.site_id }}\"}"
        done

# Scheduled discovery
cron:
  device_discovery:
    state: present
    name: device-discovery
    user: root
    minute: "*/15"
    command: /usr/local/bin/discover-devices.sh
```

### 4. Offline Behavior

> **Planned Feature:** Dedicated offline policy configuration (cached state application, priority-based sync, and reconnect conflict resolution) is planned for a future release. Currently, edge agents using NATS leaf mode will automatically reconnect and resume normal operation when connectivity is restored. NATS JetStream ensures messages are persisted and delivered once the leaf node reconnects.

## Verification

```bash
# Check edge site connectivity
kscorectl agents list --label "role=edge-proxy"

# View offline queue status
kscorectl exec run "site:edge-001" -- \
  cat /var/lib/keystone-core/queue/status.json

# Check cache health
kscorectl exec run "role:edge-proxy" -- \
  du -sh /var/lib/keystone-core/cache

# Verify device discovery
kscorectl exec run "site:edge-001" -- \
  /usr/local/bin/discover-devices.sh
```

## Troubleshooting

### Agent Not Reconnecting

Check reconnection settings and network:

```bash
kscorectl exec run "site:edge-001" -- \
  journalctl -u kscore-agent -f
```

### Stale Cache

Clear and rebuild cache:

```bash
kscorectl exec run "site:edge-001" -- \
  sh -c "rm -rf /var/lib/keystone-core/cache/* && systemctl restart kscore-agent"
```

### Device Discovery Failing

Verify network connectivity and nmap:

```bash
kscorectl exec run "site:edge-001" -- \
  nmap -sn 192.168.1.0/24
```

## Next Steps

- [Disaster Recovery]({{< relref "disaster-recovery" >}}) - Backup edge configurations
- [Compliance Automation]({{< relref "compliance-automation" >}}) - Security at the edge
