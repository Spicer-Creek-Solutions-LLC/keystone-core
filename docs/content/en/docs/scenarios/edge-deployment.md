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

```yaml
# edge-agent-config.yaml
apiVersion: v1
kind: state
metadata:
  name: edge-agent-config

target: "role:edge-proxy"

resources:
  - type: file.managed
    name: /etc/kscore/agent.yaml
    properties:
      contents: |
        # Edge-optimized agent configuration
        server:
          urls:
            - "{{ pillar.control_plane_url }}"

        # Aggressive reconnection for intermittent connectivity
        reconnect:
          initial_delay: 1s
          max_delay: 5m
          multiplier: 2

        # Local state caching for offline operation
        cache:
          enabled: true
          path: /var/lib/kscore/cache
          max_size: 1GB
          ttl: 24h

        # Queue commands when disconnected
        offline:
          enabled: true
          queue_size: 1000
          persist_path: /var/lib/kscore/queue

        # Proxy mode for downstream devices
        proxy:
          enabled: true
          listen: "0.0.0.0:4222"
          allowed_networks:
            - "10.0.0.0/8"
            - "192.168.0.0/16"
```

### 2. Edge Site State

```yaml
# edge-site-base.yaml
apiVersion: v1
kind: state
metadata:
  name: edge-site-base

parameters:
  site_id:
    type: string
    required: true
  timezone:
    type: string
    default: "UTC"

resources:
  # System configuration
  - type: timezone.system
    name: set-timezone
    properties:
      name: "{{ parameters.timezone }}"

  # Local NTP for time sync
  - type: pkg.installed
    name: chrony

  - type: file.managed
    name: /etc/chrony.conf
    properties:
      contents: |
        # Use central NTP when available, local fallback
        server {{ pillar.ntp_server }} iburst prefer
        server 0.pool.ntp.org iburst
        server 1.pool.ntp.org iburst

        # Allow local network to sync
        allow 10.0.0.0/8
        allow 192.168.0.0/16

        # Serve time even when not synced
        local stratum 10

  - type: service.running
    name: chronyd
    properties:
      enable: true
    require:
      - file: /etc/chrony.conf

  # Local monitoring
  - type: pkg.installed
    name: prometheus-node-exporter

  - type: service.running
    name: node_exporter
    properties:
      enable: true
```

### 3. Device Management

```yaml
# edge-device-management.yaml
apiVersion: v1
kind: state
metadata:
  name: edge-device-management

target: "role:edge-proxy"

resources:
  # Device discovery script
  - type: file.managed
    name: /usr/local/bin/discover-devices.sh
    properties:
      mode: "0755"
      contents: |
        #!/bin/bash
        # Discover devices on local network
        nmap -sn 192.168.1.0/24 -oG - | \
          awk '/Up$/{print $2}' | \
          while read ip; do
            # Register device with agent
            curl -s -X POST http://localhost:8080/api/devices \
              -d "{\"ip\": \"$ip\", \"site\": \"{{ grains.site_id }}\"}"
          done

  # Scheduled discovery
  - type: cron.present
    name: device-discovery
    properties:
      user: root
      minute: "*/15"
      job: /usr/local/bin/discover-devices.sh
```

### 4. Offline State Application

```yaml
# edge-offline-policy.yaml
apiVersion: v1
kind: policy
metadata:
  name: edge-offline-policy

spec:
  # Apply cached state when disconnected
  offline:
    enabled: true
    max_staleness: 24h
    fallback_action: apply_cached

  # Prioritize critical updates
  priority:
    - pattern: "security-*"
      weight: 100
    - pattern: "monitoring-*"
      weight: 50
    - pattern: "*"
      weight: 10

  # Sync strategy when reconnected
  reconnect:
    sync_mode: incremental
    conflict_resolution: server_wins
    report_drift: true
```

## Verification

```bash
# Check edge site connectivity
kscorectl ping -t "role:edge-proxy" --timeout 30s

# View offline queue status
kscorectl exec -t "site:edge-001" -- \
  cat /var/lib/kscore/queue/status.json

# Check cache health
kscorectl exec -t "role:edge-proxy" -- \
  du -sh /var/lib/kscore/cache

# Verify device discovery
kscorectl exec -t "site:edge-001" -- \
  /usr/local/bin/discover-devices.sh
```

## Troubleshooting

### Agent Not Reconnecting

Check reconnection settings and network:
```bash
kscorectl exec -t "site:edge-001" -- \
  journalctl -u kscore-agent -f
```

### Stale Cache

Clear and rebuild cache:
```bash
kscorectl exec -t "site:edge-001" -- \
  rm -rf /var/lib/kscore/cache/* && \
  systemctl restart kscore-agent
```

### Device Discovery Failing

Verify network connectivity and nmap:
```bash
kscorectl exec -t "site:edge-001" -- \
  nmap -sn 192.168.1.0/24
```

## Next Steps

- [Disaster Recovery]({{< relref "disaster-recovery" >}}) - Backup edge configurations
- [Compliance Automation]({{< relref "compliance-automation" >}}) - Security at the edge
