---
title: "NATS Mesh Deployment"
weight: 2
description: >
  Deployment guides for various NATS mesh topologies
---

## Overview

This guide covers deploying Keystone Core with different NATS mesh topologies, from simple embedded deployments to multi-region superclusters.

## Deployment Patterns

| Pattern | Use Case | Complexity | Scale |
|---------|----------|------------|-------|
| [Simple](#simple-deployment) | Development, testing | Low | < 10 agents |
| [Standalone](#standalone-production) | Small production | Low | < 100 agents |
| [HA Cluster](#ha-cluster) | Production | Medium | < 1000 agents |
| [Edge/Leaf](#edge-deployment) | Edge, IoT, NAT | Medium | Unlimited |
| [Multi-Region](#multi-region-supercluster) | Global deployment | High | Unlimited |
| [Hybrid](#hybrid-deployment) | Mixed environments | High | Unlimited |

## Simple Deployment

For development and testing with embedded NATS:

### Single Server

```yaml
# keystone-core.yaml
server:
  listen: 0.0.0.0:8080
  nats:
    mode: embedded
    listen: 0.0.0.0:4222
  state:
    driver: sqlite
    path: /var/lib/kscore/state.db
```

```bash
# Start server
kscore-server --config keystone-core.yaml
```

### Agent Configuration

```yaml
# agent.yaml
agent:
  id: agent-1
  nats:
    urls:
      - nats://server:4222
```

```bash
# Start agent
kscore-agent --config agent.yaml
```

### Docker Compose

```yaml
version: '3.8'
services:
  server:
    image: keystonecore/server:latest
    ports:
      - "8080:8080"
      - "4222:4222"
    volumes:
      - ./keystone-core.yaml:/etc/kscore/config.yaml
      - kscore-data:/var/lib/kscore

  agent:
    image: keystonecore/agent:latest
    environment:
      KSCORE_NATS_URLS: nats://server:4222
    depends_on:
      - server

volumes:
  kscore-data:
```

## Standalone Production

Single server with external NATS for improved reliability:

### NATS Server

```conf
# nats.conf
port: 4222
http_port: 8222

jetstream {
  store_dir: /var/lib/nats/jetstream
  max_memory_store: 1GB
  max_file_store: 10GB
}

authorization {
  users: [
    { user: kscore, password: $NATS_PASSWORD }
  ]
}
```

### Control Plane

```yaml
# keystone-core.yaml
server:
  listen: 0.0.0.0:8080
  nats:
    mode: external
    urls:
      - nats://kscore:$NATS_PASSWORD@nats:4222
  state:
    driver: postgresql
    url: postgres://user:pass@postgres:5432/kscore
```

### Agent

```yaml
# agent.yaml
agent:
  id: ${HOSTNAME}
  nats:
    urls:
      - nats://kscore:$NATS_PASSWORD@nats:4222
    tls:
      enabled: true
      ca: /etc/kscore/ca.crt
```

## HA Cluster

Production deployment with NATS cluster and multiple control planes:

### Architecture

```mermaid
flowchart TB
    LB["Load Balancer"]

    subgraph ControlPlanes["Control Planes"]
        CP1["CP 1"]
        CP2["CP 2"]
        CP3["CP 3"]
    end

    subgraph NATS["NATS Cluster"]
        N1["NATS 1"]
        N2["NATS 2"]
        N3["NATS 3"]
        N1 <--> N2 <--> N3
    end

    subgraph Agents["Agents"]
        A1["Agent 1"]
        A2["Agent 2"]
        AN["Agent N"]
    end

    LB --> CP1
    LB --> CP2
    LB --> CP3
    CP1 --> NATS
    CP2 --> NATS
    CP3 --> NATS
    NATS --> A1
    NATS --> A2
    NATS --> AN
```

### NATS Cluster Configuration

```conf
# nats-1.conf
server_name: nats-1
port: 4222
http_port: 8222

cluster {
  name: kscore-nats
  listen: 0.0.0.0:6222
  routes: [
    nats-route://nats-2:6222
    nats-route://nats-3:6222
  ]
}

jetstream {
  store_dir: /var/lib/nats/jetstream
  max_memory_store: 2GB
  max_file_store: 50GB
}
```

### Control Plane Configuration

```yaml
# keystone-core.yaml (each control plane)
server:
  id: cp-1  # Unique per instance
  listen: 0.0.0.0:8080

  nats:
    mode: external
    urls:
      - nats://nats-1:4222
      - nats://nats-2:4222
      - nats://nats-3:4222

  cluster:
    enabled: true
    etcd:
      endpoints:
        - etcd-1:2379
        - etcd-2:2379
        - etcd-3:2379

  state:
    driver: postgresql
    url: postgres://user:pass@postgres:5432/kscore
```

### Agent Configuration

```yaml
# agent.yaml
agent:
  id: ${HOSTNAME}
  nats:
    endpoints:
      - url: nats://nats-1:4222
        priority: 1
      - url: nats://nats-2:4222
        priority: 2
      - url: nats://nats-3:4222
        priority: 3
    routing:
      strategy: least_latency
      failback: true
```

### Kubernetes Deployment

```yaml
# nats-cluster.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: nats
spec:
  serviceName: nats
  replicas: 3
  selector:
    matchLabels:
      app: nats
  template:
    metadata:
      labels:
        app: nats
    spec:
      containers:
      - name: nats
        image: nats:2.10
        ports:
        - containerPort: 4222
        - containerPort: 6222
        - containerPort: 8222
        volumeMounts:
        - name: config
          mountPath: /etc/nats
        - name: data
          mountPath: /var/lib/nats
      volumes:
      - name: config
        configMap:
          name: nats-config
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 50Gi
---
apiVersion: v1
kind: Service
metadata:
  name: nats
spec:
  clusterIP: None
  ports:
  - port: 4222
    name: client
  - port: 6222
    name: cluster
  selector:
    app: nats
```

## Edge Deployment

For edge locations behind NAT or firewalls using leaf nodes:

### Architecture

```mermaid
flowchart TB
    subgraph Hub["Hub Data Center"]
        subgraph NATS["NATS Cluster :7422"]
            N1["NATS 1"]
            N2["NATS 2"]
            N1 <--> N2
        end
        CP["Control Plane"]
        NATS --> CP
    end

    subgraph Edge1["Edge Site 1"]
        L1["Leaf NATS"]
        A1["Agents"]
        L1 --> A1
    end

    subgraph Edge2["Edge Site 2"]
        L2["Leaf NATS"]
        A2["Agents"]
        L2 --> A2
    end

    L1 -.->|"Leaf Connection<br/>(outbound)"| NATS
    L2 -.->|"Leaf Connection<br/>(outbound)"| NATS
```

### Hub NATS Configuration

```conf
# hub-nats.conf
port: 4222
http_port: 8222

cluster {
  name: hub
  listen: 0.0.0.0:6222
}

leafnodes {
  listen: 0.0.0.0:7422

  authorization {
    users: [
      { user: edge-site-1, password: $EDGE1_PASSWORD }
      { user: edge-site-2, password: $EDGE2_PASSWORD }
    ]
  }

  tls {
    cert_file: /etc/nats/leaf-server.crt
    key_file: /etc/nats/leaf-server.key
    ca_file: /etc/nats/ca.crt
    verify: true
  }
}

jetstream {
  store_dir: /var/lib/nats/jetstream
  max_memory_store: 4GB
  max_file_store: 100GB
}
```

### Edge Agent Configuration

```yaml
# edge-agent.yaml
agent:
  id: edge-${SITE}-${HOSTNAME}

  nats:
    mode: leaf

    hub:
      urls:
        - nats-leaf://edge-site-1:$EDGE1_PASSWORD@hub.example.com:7422
      tls:
        ca: /etc/kscore/ca.crt
        cert: /etc/kscore/edge.crt
        key: /etc/kscore/edge.key

    embedded:
      listen: 127.0.0.1:4222
      store_dir: /var/lib/kscore/nats

    buffer:
      enabled: true
      max_size: 100MB
      max_messages: 100000
      max_age: 24h
      persistence: true
```

### Edge Buffering Behavior

When the leaf node loses connectivity to the hub:

1. Agent continues operating locally
2. Messages queue in the local buffer
3. Buffer persists to disk for crash recovery
4. On reconnection, buffer flushes to hub
5. Deduplication prevents duplicate processing

Configure buffering for your edge requirements:

```yaml
agent:
  nats:
    buffer:
      # Size limits (first reached wins)
      max_size: 100MB        # Total buffer size
      max_messages: 100000   # Maximum messages
      max_age: 24h           # Message TTL

      # Persistence
      persistence: true      # Survive agent restarts
      persist_dir: /var/lib/kscore/buffer

      # Overflow behavior
      overflow: drop_oldest  # drop_oldest, drop_newest, block
```

## Multi-Region Supercluster

Global deployment with NATS gateways connecting regional clusters:

### Architecture

```mermaid
flowchart TB
    subgraph USWest["US-WEST"]
        subgraph NATSWest["NATS Cluster :7222"]
            NW1["NATS 1"]
            NW2["NATS 2"]
            NW1 <--> NW2
        end
        CPW["Control Plane"]
        AW["US Agents"]
        NATSWest --> CPW --> AW
    end

    subgraph USEast["US-EAST"]
        subgraph NATSEast["NATS Cluster"]
            NE1["NATS"]
        end
        CPE["Control Plane"]
        AE["East Agents"]
        NATSEast --> CPE --> AE
    end

    subgraph EU["EU"]
        subgraph NATSEU["NATS Cluster"]
            NEU1["NATS"]
        end
        CPEU["Control Plane"]
        AEU["EU Agents"]
        NATSEU --> CPEU --> AEU
    end

    NATSWest <-->|"Gateway"| NATSEast
    NATSWest <-->|"Gateway"| NATSEU
    NATSEast <-->|"Gateway"| NATSEU
```

### Gateway Configuration

```conf
# us-west-nats.conf
server_name: us-west-1
port: 4222

cluster {
  name: us-west
  listen: 0.0.0.0:6222
  routes: [
    nats-route://us-west-2:6222
  ]
}

gateway {
  name: us-west
  listen: 0.0.0.0:7222

  gateways: [
    { name: us-east, urls: ["nats://gateway.us-east.example.com:7222"] }
    { name: eu, urls: ["nats://gateway.eu.example.com:7222"] }
  ]

  tls {
    cert_file: /etc/nats/gateway.crt
    key_file: /etc/nats/gateway.key
    ca_file: /etc/nats/ca.crt
  }
}

jetstream {
  store_dir: /var/lib/nats/jetstream
  domain: us-west  # JetStream domain for cross-cluster
}
```

### Control Plane Configuration

```yaml
# keystone-core.yaml (US-West)
server:
  id: cp-us-west-1
  cluster_name: us-west

  nats:
    mode: external
    cluster: us-west
    urls:
      - nats://us-west-1:4222
      - nats://us-west-2:4222

    gateway:
      enabled: true
      remotes:
        - name: us-east
          urls: ["nats://gateway.us-east.example.com:7222"]
        - name: eu
          urls: ["nats://gateway.eu.example.com:7222"]

    routing:
      prefer_local: true
      cross_cluster_timeout: 5s
```

### Agent Configuration

```yaml
# agent.yaml (US-West agent)
agent:
  id: ${HOSTNAME}
  region: us-west

  nats:
    cluster: us-west
    endpoints:
      - url: nats://us-west-1:4222
        priority: 1
      - url: nats://us-west-2:4222
        priority: 2
      # Failover to other regions
      - url: nats://us-east-1:4222
        priority: 10
      - url: nats://eu-1:4222
        priority: 20
```

### Cross-Region Failover

Configure failover between regions:

```yaml
server:
  nats:
    failover:
      enabled: true
      detection_timeout: 10s
      failover_timeout: 30s
      min_healthy_nodes: 1
      auto_failback: true
      failback_delay: 60s
```

## Hybrid Deployment

Mix of cloud, on-premise, and edge with different connection types:

### Architecture

```mermaid
flowchart TB
    subgraph Cloud["Cloud (AWS)"]
        subgraph NATS["NATS Cluster (Hub)<br/>:4222 :7422 :7222"]
            NC["NATS"]
        end
        CP["Control Plane"]
        NATS --> CP
    end

    subgraph OnPrem["On-Premise Data Center"]
        NP["NATS Cluster"]
        AP["Agents"]
        NP --> AP
    end

    subgraph Edge["Edge Factory"]
        LE["Leaf NATS"]
        AE["Agents"]
        LE --> AE
    end

    subgraph Remote["Remote Office<br/>(HTTP only)"]
        AR["Agent"]
    end

    NATS <-->|"Gateway"| NP
    LE -.->|"Leaf"| NATS
    AR -.->|"WebSocket"| NATS
```

### Cloud Hub Configuration

```conf
# cloud-nats.conf
port: 4222

# Leaf node listener for edge sites
leafnodes {
  listen: 0.0.0.0:7422
  tls {
    cert_file: /etc/nats/leaf.crt
    key_file: /etc/nats/leaf.key
    ca_file: /etc/nats/ca.crt
  }
}

# Gateway for on-premise clusters
gateway {
  name: cloud
  listen: 0.0.0.0:7222
  gateways: [
    { name: onprem, urls: ["nats://gateway.onprem.example.com:7222"] }
  ]
}

# WebSocket for HTTP-only networks
websocket {
  listen: 0.0.0.0:443
  tls {
    cert_file: /etc/nats/websocket.crt
    key_file: /etc/nats/websocket.key
  }
}
```

### On-Premise Configuration

```conf
# onprem-nats.conf
port: 4222

cluster {
  name: onprem
  listen: 0.0.0.0:6222
}

gateway {
  name: onprem
  listen: 0.0.0.0:7222
  gateways: [
    { name: cloud, urls: ["nats://gateway.cloud.example.com:7222"] }
  ]
}
```

### Edge Agent (Leaf)

```yaml
# edge-agent.yaml
agent:
  nats:
    mode: leaf
    hub:
      urls: ["nats-leaf://hub.cloud.example.com:7422"]
    buffer:
      enabled: true
      persistence: true
```

### Remote Office Agent (WebSocket)

```yaml
# remote-agent.yaml
agent:
  nats:
    urls: ["wss://hub.cloud.example.com:443/nats"]
    websocket:
      compression: true
      proxy:
        url: http://corporate-proxy:8080
```

## Migration Guide

### From Embedded to External NATS

1. **Deploy external NATS cluster**:
   ```bash
   # Start NATS cluster
   docker-compose -f nats-cluster.yml up -d
   ```

2. **Update control plane configuration**:
   ```yaml
   server:
     nats:
       mode: external  # Changed from embedded
       urls:
         - nats://nats-1:4222
         - nats://nats-2:4222
   ```

3. **Rolling update agents**:
   ```bash
   # Update agent configuration
   kscorectl agent update-config --nats-urls nats://nats-1:4222,nats://nats-2:4222
   ```

4. **Verify connectivity**:
   ```bash
   kscorectl cluster status
   kscorectl agent list
   ```

### From Direct to Leaf Node

1. **Configure hub for leaf connections**:
   ```yaml
   # Add to hub NATS config
   leafnodes {
     listen: 0.0.0.0:7422
   }
   ```

2. **Update edge agent configuration**:
   ```yaml
   agent:
     nats:
       mode: leaf
       hub:
         urls: ["nats-leaf://hub:7422"]
   ```

3. **Restart edge agents**:
   ```bash
   systemctl restart kscore-agent
   ```

### Adding Gateway (Supercluster)

1. **Configure gateway on existing cluster**:
   ```yaml
   gateway {
     name: region-1
     listen: 0.0.0.0:7222
     gateways: [
       { name: region-2, urls: ["nats://region-2:7222"] }
     ]
   }
   ```

2. **Configure new region cluster**:
   ```yaml
   gateway {
     name: region-2
     listen: 0.0.0.0:7222
     gateways: [
       { name: region-1, urls: ["nats://region-1:7222"] }
     ]
   }
   ```

3. **Update control plane routing**:
   ```yaml
   server:
     nats:
       routing:
         prefer_local: true
   ```

## Verification

After deployment, verify the mesh is working:

```bash
# Check NATS cluster health
nats server report jetstream

# Verify control plane connectivity
kscorectl cluster status

# List connected agents
kscorectl agent list

# Test cross-cluster routing (supercluster)
kscorectl exec run --target "region:us-east" -- hostname

# Check leaf node connections
nats server report leafnodes

# Check gateway connections
nats server report gateways
```

## See Also

- [NATS Mesh Concepts]({{< ref "../concepts/nats-mesh" >}}) - Architecture overview
- [NATS Mesh Operations]({{< ref "nats-mesh-operations" >}}) - Monitoring and troubleshooting
- [NATS Mesh Reference]({{< ref "../reference/nats-mesh" >}}) - Configuration reference
