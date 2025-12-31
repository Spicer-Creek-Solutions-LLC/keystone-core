# Epic 14: NATS Mesh Communication

## Overview

Decouple all agent↔server communication to use NATS as the sole transport layer, enabling flexible deployment topologies that work across NAT, firewalls, and complex network boundaries. Support multiple NATS deployment modes including embedded instances, external clusters, leaf nodes, gateways, and superclusters to enable connectivity in any network environment.

**Goal**: Enable Keystone Core deployments where agents and servers can communicate regardless of network topology, NAT configuration, or firewall restrictions. All agent communication flows through NATS, allowing operators to choose the connectivity pattern that fits their infrastructure.

**Important Distinction**:
- **Agent↔Server**: NATS-only (no direct gRPC/TCP)
- **Server↔Server**: gRPC allowed for cluster coordination (setup, maintenance, recovery when NATS is unavailable)
- **Client→Server**: gRPC/REST for API (kubectl-style tools) - unchanged

## Success Criteria

- [ ] All agent↔server communication exclusively uses NATS (no direct gRPC/TCP)
- [ ] Server↔server gRPC coordination channel for cluster operations when NATS unavailable
- [ ] Secure agent bootstrap: new agents only get registration topic access until authenticated
- [ ] Agents can operate with embedded NATS that servers connect to (reverse connection)
- [ ] Agents can connect to server's embedded NATS instance
- [ ] Agents and servers can connect to any external NATS cluster
- [ ] NATS supercluster support for multi-region/multi-cloud deployments
- [ ] NATS leaf node chains for hierarchical edge deployments
- [ ] NATS gateway support for firewall/NAT traversal
- [ ] WebSocket transport support for restrictive firewalls (port 443/80)
- [ ] Automatic connection strategy selection with fallback
- [ ] Connection health monitoring with automatic failover
- [ ] Zero message loss during connection transitions
- [ ] Documentation for all deployment topologies

## Problem Statement

**Current State:**
- Agents connect to NATS cluster (embedded or external)
- Server(s) connect to same NATS cluster
- Both parties must reach the same NATS endpoint
- NAT/firewall between agent and NATS cluster blocks connectivity
- No support for hierarchical (supercluster) NATS deployments
- Edge agents behind restrictive firewalls cannot connect
- Multi-cloud deployments require complex networking

**Target State:**
- Multiple connection strategies available per agent/server
- Agents can host NATS (servers connect inbound)
- Agents can connect outbound to any reachable NATS
- Supercluster support for spanning regions/clouds
- WebSocket transport for firewall traversal
- Leaf node chains for hierarchical edge deployments
- Automatic strategy selection based on network reachability
- Graceful failover between connection strategies

## Architecture

### Connection Topology Options

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    TOPOLOGY 1: Traditional (Current)                     │
│                                                                          │
│     ┌──────────┐         ┌──────────────┐         ┌──────────┐         │
│     │  Agent   │───────→ │ NATS Cluster │ ←─────── │  Server  │         │
│     └──────────┘         │  (External)  │         └──────────┘         │
│                          └──────────────┘                               │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                    TOPOLOGY 2: Server Embedded                           │
│                                                                          │
│     ┌──────────┐         ┌──────────────────┐                           │
│     │  Agent   │───────→ │     Server       │                           │
│     └──────────┘         │  ┌────────────┐  │                           │
│                          │  │ Embedded   │  │                           │
│                          │  │   NATS     │  │                           │
│                          │  └────────────┘  │                           │
│                          └──────────────────┘                           │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                    TOPOLOGY 3: Agent Embedded (Reverse)                  │
│                                                                          │
│     ┌──────────────────┐         ┌──────────┐                           │
│     │      Agent       │ ←─────── │  Server  │                           │
│     │  ┌────────────┐  │         └──────────┘                           │
│     │  │ Embedded   │  │                                                 │
│     │  │   NATS     │  │   Server connects TO agent's NATS              │
│     │  └────────────┘  │   (for agents behind NAT with public IP)       │
│     └──────────────────┘                                                 │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                    TOPOLOGY 4: Leaf Node Chain                           │
│                                                                          │
│     ┌──────────┐         ┌──────────┐         ┌──────────────┐         │
│     │  Agent   │───────→ │ Edge NATS│───────→ │ NATS Cluster │←─Server │
│     │  (leaf)  │         │  (leaf)  │         │   (hub)      │         │
│     └──────────┘         └──────────┘         └──────────────┘         │
│                                                                          │
│     Agent → Edge gateway → Hub cluster                                   │
│     For hierarchical edge deployments                                    │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                    TOPOLOGY 5: Supercluster                              │
│                                                                          │
│     Region A                           Region B                          │
│     ┌────────────────┐                 ┌────────────────┐               │
│     │ NATS Cluster A │←── Gateway ───→ │ NATS Cluster B │               │
│     │    + Server    │                 │    + Server    │               │
│     └───────┬────────┘                 └───────┬────────┘               │
│             │                                   │                        │
│        ┌────┴────┐                         ┌────┴────┐                  │
│        │ Agents  │                         │ Agents  │                  │
│        │ (local) │                         │ (local) │                  │
│        └─────────┘                         └─────────┘                  │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                    TOPOLOGY 6: WebSocket Tunnel                          │
│                                                                          │
│     Restrictive Network          │ Firewall │         Cloud              │
│                                  │ (443/80) │                            │
│     ┌──────────┐                 │    │     │    ┌──────────────┐       │
│     │  Agent   │────WebSocket───────→│─────────→ │ NATS + Server│       │
│     └──────────┘                 │    │     │    └──────────────┘       │
│                                  │          │                            │
│     Agent uses WSS to traverse firewall                                  │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                    TOPOLOGY 7: Bidirectional Gateway                     │
│                                                                          │
│     On-Prem (outbound only)                       Cloud                  │
│                                                                          │
│     ┌──────────────────┐        ┌──────────────────┐                    │
│     │ NATS + Agents    │───────→│ NATS + Server    │                    │
│     │ (initiates conn) │  WS/TLS│ (accepts conn)   │                    │
│     └──────────────────┘        └──────────────────┘                    │
│                                                                          │
│     On-prem NATS initiates outbound connection to cloud                 │
│     Server commands flow back through same connection                   │
└─────────────────────────────────────────────────────────────────────────┘
```

### Protocol Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Message Flow (All via NATS)                      │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                        NATS Subject Hierarchy                      │ │
│  │                                                                    │ │
│  │  kscore.{cluster}.agent.register          - Agent registration     │ │
│  │  kscore.{cluster}.agent.heartbeat         - Agent heartbeats       │ │
│  │  kscore.{cluster}.agent.{id}.command      - Commands to agent      │ │
│  │  kscore.{cluster}.agent.{id}.response     - Responses from agent   │ │
│  │  kscore.{cluster}.agent.{id}.state        - State operations       │ │
│  │  kscore.{cluster}.agent.{id}.events       - Agent events           │ │
│  │  kscore.{cluster}.server.announce         - Server announcements   │ │
│  │  kscore.{cluster}.server.{id}.control     - Server control channel │ │
│  │  kscore.{cluster}.discovery               - Peer discovery         │ │
│  │                                                                    │ │
│  │  {cluster} = logical cluster name for supercluster routing         │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                      Connection Strategies                         │ │
│  │                                                                    │ │
│  │  1. Direct TCP     - Standard NATS connection (nats://host:4222)   │ │
│  │  2. TLS            - Encrypted NATS (tls://host:4222)              │ │
│  │  3. WebSocket      - WS transport (ws://host:80, wss://host:443)   │ │
│  │  4. Leaf Node      - Hierarchical connection (leaf://host:7422)    │ │
│  │  5. Gateway        - Supercluster connection                        │ │
│  │                                                                    │ │
│  │  Fallback chain: TLS → WebSocket → Leaf → Gateway                  │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
```

## User Stories

### US14.1: NATS-Only Agent Communication
**As a** platform operator
**I want to** all agent↔server communication to use NATS
**So that** I have a single, well-understood transport layer for agents

**Acceptance Criteria**:
- Remove all direct gRPC connections between agents and servers
- Agent registration flows through NATS
- Command execution flows through NATS
- State synchronization flows through NATS
- Health monitoring flows through NATS
- Existing functionality preserved (no regression)
- Performance within 10% of current implementation
- Server↔server gRPC retained for cluster coordination (NATS outage recovery, cluster setup)
- Server gRPC channel provides cluster health, leader info, and coordination when NATS unavailable

### US14.2: Agent Embedded NATS (Reverse Connection)
**As a** platform operator
**I want to** agents to host embedded NATS that servers connect to
**So that** agents behind NAT with public/reachable IPs can participate

**Acceptance Criteria**:
- Agent can start embedded NATS server
- Agent advertises its NATS endpoint during discovery
- Server connects to agent's NATS endpoint
- Bidirectional communication works normally
- Agent handles multiple server connections
- Graceful handling when agent restarts
- Configuration to enable/disable this mode

### US14.3: Flexible NATS Endpoint Configuration
**As a** platform operator
**I want to** configure multiple NATS endpoints with fallback
**So that** agents can connect through available paths

**Acceptance Criteria**:
- Configure multiple NATS URLs per agent/server
- Automatic failover between endpoints
- Health-based endpoint selection
- Latency-based endpoint selection (optional)
- Manual override for testing
- Connection status visible in metrics/logs

### US14.4: NATS Supercluster Support
**As a** platform operator
**I want to** deploy NATS superclusters across regions
**So that** I can have a unified control plane across clouds/regions

**Acceptance Criteria**:
- Configure gateway connections between NATS clusters
- Subjects route correctly across gateways
- Agents in region A can be managed by server in region B
- Latency metrics per region
- Automatic failover to local server on partition
- Interest-only mode for efficient routing

### US14.5: Leaf Node Chains
**As a** platform operator
**I want to** create hierarchical leaf node deployments
**So that** edge locations can have local NATS with upstream connectivity

**Acceptance Criteria**:
- Agent connects to local (edge) NATS as leaf
- Edge NATS connects to regional NATS as leaf
- Regional NATS connects to central cluster
- Commands flow down the chain correctly
- Responses flow up the chain correctly
- Local persistence during upstream outage
- Automatic reconnection on recovery

### US14.6: WebSocket Transport
**As a** platform operator
**I want to** agents to connect via WebSocket
**So that** restrictive firewalls (only 443/80) don't block connectivity

**Acceptance Criteria**:
- Agent can connect via wss://host:443
- TLS certificate validation works
- Works through HTTP proxies (with auth)
- Automatic upgrade from WS to native when available
- Performance within 20% of native connection
- Connection stays alive through idle periods

### US14.7: NAT Traversal
**As a** platform operator
**I want to** agents behind NAT to connect without port forwarding
**So that** I don't need to modify network configuration

**Acceptance Criteria**:
- Agent initiates outbound connection to reachable NATS
- Server commands flow back through same connection
- Works with symmetric NAT
- Works with carrier-grade NAT (CGNAT)
- No public IP required on agent side
- Connection keepalive prevents NAT timeout

### US14.8: Connection Discovery
**As a** platform operator
**I want to** automatic discovery of connection endpoints
**So that** I don't need to manually configure every agent

**Acceptance Criteria**:
- Agents discover available NATS endpoints
- DNS-based discovery (SRV records)
- mDNS/Bonjour for local network discovery
- Kubernetes service discovery integration
- Consul/etcd service discovery integration
- Manual configuration takes precedence

### US14.9: Connection Health Monitoring
**As a** platform operator
**I want to** monitor connection health across all paths
**So that** I can diagnose connectivity issues

**Acceptance Criteria**:
- Per-connection latency metrics
- Per-connection error rate metrics
- Connection state transitions logged
- Alerting on connection failures
- Dashboard showing connection topology
- Debugging tools for connection issues

### US14.10: Message Persistence During Transitions
**As a** platform operator
**I want to** no message loss during connection transitions
**So that** failover is seamless

**Acceptance Criteria**:
- Messages buffered during reconnection
- JetStream used for critical messages
- At-least-once delivery guarantee
- Duplicate detection on receiver
- Configurable buffer size
- Buffer overflow handling (backpressure/drop)

### US14.11: Secure Agent Bootstrap Registration
**As a** platform operator
**I want to** new agents to have minimal NATS permissions until authenticated
**So that** untrusted agents cannot access or disrupt other agents' communication

**Acceptance Criteria**:
- New agents receive bootstrap credentials with minimal permissions
- Bootstrap credentials only allow publishing to `kscore.{cluster}.agent.register`
- Bootstrap credentials only allow subscribing to `kscore.{cluster}.agent.{bootstrap_id}.register-response`
- After successful registration, agent receives full credentials with agent-specific permissions
- Full credentials allow agent's own subjects (`kscore.{cluster}.agent.{agent_id}.*`)
- Credential exchange is atomic (no window where agent has both credentials)
- Bootstrap credentials can be rotated without affecting registered agents
- Design supports future identity provider integration (SPIFFE/SPIRE, cloud IAM)
- Credential format is extensible (currently: NKey/JWT, future: SPIFFE SVID)
- Audit log of all bootstrap and registration events

### US14.12: Server-to-Server Coordination Channel
**As a** platform operator
**I want to** servers to coordinate directly when NATS is unavailable
**So that** the cluster can recover from NATS outages and maintain consistency

**Acceptance Criteria**:
- Servers maintain gRPC connections to other cluster members
- Coordination channel works independently of NATS connectivity
- Channel supports: cluster health checks, leader election fallback, NATS recovery coordination
- Servers can determine cluster quorum via gRPC when NATS is partitioned
- Servers can coordinate NATS cluster recovery/restart
- Channel uses mTLS for authentication
- Channel is lightweight (minimal traffic when NATS is healthy)
- Metrics for coordination channel health

## Technical Tasks

### Phase 1: NATS-Only Communication Refactor (Week 1-3)

**T1.1: Subject Namespace Design**
- Design hierarchical subject namespace with cluster prefix
- Document subject routing for superclusters
- Define subject access control policies
- Create subject prefix configuration
- Implement subject helper functions (pkg/nats/subjects.go)
- Add subject validation

**T1.2: Remove Direct gRPC Agent Connections**
- Identify all direct agent↔server connections
- Replace gRPC registration with NATS request/reply
- Replace gRPC command stream with NATS pub/sub
- Replace gRPC heartbeat with NATS pub/sub
- Deprecate agent-facing gRPC services
- Keep gRPC for client API (kubectl-style tools)

**T1.3: Message Protocol Enhancement**
- Add message envelope with routing metadata
- Add message deduplication IDs
- Add message priority levels
- Add message TTL
- Add correlation IDs for request/response
- Implement message serialization helpers

**T1.4: Connection Manager Refactor**
- Refactor ConnectionManager to be NATS-centric
- Remove agent socket tracking
- Add NATS subscription management
- Implement per-agent subject subscriptions
- Add subscription cleanup on agent disconnect
- Handle subscription failures gracefully

**T1.5: Agent Communication Refactor**
- Replace gRPC client with NATS-only communication
- Implement NATS request/reply for registration
- Implement NATS pub for heartbeat
- Implement NATS sub for commands
- Implement NATS pub for responses
- Add reconnection with message replay

**T1.6: Server-to-Server gRPC Coordination Channel**
- Define gRPC service for server coordination (pkg/api/coordination.proto)
  - ClusterHealth RPC: Get cluster health status
  - GetLeader RPC: Determine current leader
  - NATSStatus RPC: Report NATS connectivity status per server
  - RecoveryCoordinate RPC: Coordinate NATS recovery actions
  - Heartbeat RPC: Server-to-server liveness check
- Implement coordination gRPC server (pkg/cluster/coordination_server.go)
- Implement coordination gRPC client (pkg/cluster/coordination_client.go)
- Maintain mesh connections to all known cluster members
- Use mTLS for authentication (same certs as existing server TLS)
- Lightweight heartbeat when NATS is healthy (fallback only)
- Coordinate NATS restart/recovery when NATS cluster fails
- Add coordination channel metrics (kscore_server_coordination_*)
- Integration with existing Epic 11 cluster membership

**T1.7: Secure Agent Bootstrap Registration**
- Define bootstrap credential format (pkg/nats/bootstrap.go)
  - Bootstrap JWT with minimal claims
  - Bootstrap NKey for initial connection
  - Short TTL (configurable, default 5 minutes)
- Implement BootstrapCredentialProvider interface
  - Generate bootstrap credentials on demand
  - Validate bootstrap credentials
  - Revoke bootstrap credentials
  - Extensibility point for future SPIFFE/SPIRE integration
- Configure NATS authorization for bootstrap:
  - Bootstrap account with limited permissions
  - Publish: `kscore.{cluster}.agent.register` only
  - Subscribe: `kscore.{cluster}.agent.{bootstrap_id}.register-response` only
  - No access to other agent subjects, events, or commands
- Implement credential exchange in registration flow:
  - Agent connects with bootstrap credentials
  - Agent publishes registration request
  - Server validates agent identity (pre-shared key, cloud metadata, etc.)
  - Server generates agent-specific credentials
  - Server responds with credentials on bootstrap response subject
  - Agent reconnects with new credentials
- Add NATS account switching support (bootstrap → agent account)
- Implement credential rotation without agent restart
- Audit logging for all bootstrap/registration events
- Design extensibility for future identity providers:
  - Interface for IdentityVerifier (verify agent claims)
  - Interface for CredentialIssuer (issue agent credentials)
  - Placeholder for SPIFFE SVID verification
  - Placeholder for cloud IAM token verification (AWS STS, GCP metadata)
- **Dual-mode authentication support (for Epic 17 migration)**:
  - NATS server accepts both JWT/NKey AND mTLS/SVID simultaneously
  - Configure NATS with multiple authentication methods:
    ```
    authorization {
      # JWT/NKey for legacy agents
      users: [ ... ]
      # mTLS for SVID-authenticated agents (via TLS verify)
    }
    ```
  - CredentialIssuer returns credential type indicator (jwt, nkey, svid)
  - Agent handles different credential types appropriately
  - Metrics track authentication method usage for migration visibility
  - Gradual deprecation: warn on JWT/NKey, enforce SVID after cutover date

### Phase 2: Multi-Endpoint Support (Week 4-5)

**T2.1: NATS URL Configuration**
- Support multiple NATS URLs in config
- Parse URL schemes (nats://, tls://, ws://, wss://)
- Support URL with credentials (nats://user:pass@host)
- Support URL with token (nats://host?token=xxx)
- Configuration validation
- Environment variable interpolation

**T2.2: Connection Strategy Framework**
- Define ConnectionStrategy interface (pkg/nats/strategy.go)
- Implement DirectStrategy (standard TCP)
- Implement TLSStrategy (encrypted)
- Implement WebSocketStrategy
- Implement LeafNodeStrategy
- Strategy selection logic

**T2.3: Multi-Endpoint Connection Manager**
- Connect to primary endpoint
- Automatic failover to secondary endpoints
- Health-based endpoint selection
- Configurable failover timeout
- Connection state machine
- Endpoint priority configuration

**T2.4: Health-Based Routing**
- Monitor connection health per endpoint
- Track latency per endpoint
- Track error rate per endpoint
- Select healthiest endpoint
- Avoid flapping between endpoints
- Configurable health thresholds

**T2.5: Connection Metrics**
- Metric: active connections per endpoint
- Metric: connection latency per endpoint
- Metric: connection errors per endpoint
- Metric: failover count
- Metric: messages per endpoint
- Prometheus exporter

### Phase 3: Agent Embedded NATS (Reverse Connection) (Week 6-7)

**T3.1: Agent Embedded NATS Server**
- Add embedded NATS server capability to agent (pkg/agent/nats_server.go)
- Configure listening address/port
- Configure TLS for embedded server
- Configure authentication
- Resource limits (connections, memory)
- Graceful shutdown

**T3.2: Agent Endpoint Advertisement**
- Agent publishes its NATS endpoint to discovery
- Include public IP detection
- Include port mapping (for NAT)
- Include health status
- TTL-based expiry
- Heartbeat renewal

**T3.3: Server Outbound Connection**
- Server discovers agent NATS endpoints
- Server initiates connection to agent NATS
- Bidirectional message flow
- Handle agent restart (reconnection)
- Connection pooling for many agents
- Timeout and retry logic

**T3.4: Hybrid Mode**
- Some agents host NATS, others connect
- Automatic role selection based on network
- Manual override configuration
- Mixed topology support
- Documentation for hybrid deployments

### Phase 4: Leaf Node Support (Week 8-9)

**T4.1: Leaf Node Configuration**
- Configure agent as leaf node (pkg/nats/leaf.go)
- Configure server NATS as leaf hub
- Configure remote URLs for leaf connection
- Configure credentials for leaf auth
- Configure subject imports/exports
- Validate leaf configuration

**T4.2: Leaf Node Connection**
- Implement leaf node connection logic
- Handle leaf reconnection
- Subject remapping (local ↔ remote)
- Selective subject bridging
- JetStream over leaf nodes
- Leaf connection metrics

**T4.3: Leaf Node Chains**
- Support multi-hop leaf chains
- Edge agent → Edge NATS → Regional → Central
- Configure hop timeout accumulation
- Subject propagation through chain
- Message deduplication across hops
- Latency monitoring per hop

**T4.4: Local Persistence During Outage**
- Enable JetStream on leaf nodes
- Buffer messages during upstream outage
- Replay messages on reconnection
- Configurable buffer limits
- Handle buffer overflow
- Message ordering guarantees

**T4.5: Leaf Node Testing**
- Unit tests for leaf configuration
- Integration tests for leaf connection
- Chain tests (3+ hops)
- Outage/recovery tests
- Performance benchmarks
- Chaos tests (random disconnections)

### Phase 5: Supercluster Support (Week 10-12)

**T5.1: Gateway Configuration**
- Configure NATS gateway connections (pkg/nats/gateway.go)
- Gateway cluster name configuration
- Gateway URL configuration
- Gateway credentials
- Gateway TLS configuration
- Interest-only mode configuration

**T5.2: Gateway Connection Manager**
- Manage gateway connections
- Handle gateway failures
- Gateway health monitoring
- Gateway latency tracking
- Gateway auto-discovery (optional)
- Configurable gateway topology

**T5.3: Subject Routing Across Gateways**
- Implement subject namespace with cluster prefix
- Route subjects across gateways correctly
- Interest propagation between clusters
- Avoid message duplication
- Optimize for locality (prefer local)
- Fallback to remote on local failure

**T5.4: Cross-Cluster Agent Management**
- Agent in cluster A managed by server in cluster B
- Command routing across gateways
- Response routing back
- Latency compensation
- Timeout adjustment for cross-cluster
- Locality preference configuration

**T5.5: Supercluster Failover**
- Detect local cluster failure
- Failover to remote cluster
- Maintain agent connectivity
- Failback when local recovers
- Split-brain handling
- Quorum across supercluster

**T5.6: Supercluster Testing**
- Multi-cluster integration tests
- Cross-cluster command execution
- Gateway failure tests
- Network partition tests (between clusters)
- Latency simulation tests
- Performance benchmarks

### Phase 6: WebSocket Transport (Week 13-14)

**T6.1: WebSocket Client**
- Implement NATS over WebSocket client (pkg/nats/websocket.go)
- TLS support (wss://)
- Certificate validation
- Custom CA support
- Proxy support (HTTP CONNECT)
- Proxy authentication

**T6.2: WebSocket Server**
- Enable WebSocket listener on embedded NATS
- Configure WebSocket port (default 443)
- TLS certificate configuration
- Path configuration (default /nats)
- CORS configuration (if browser clients)
- Connection upgrade handling

**T6.3: WebSocket Through Proxy**
- HTTP CONNECT tunnel support
- Proxy authentication (Basic, NTLM)
- Proxy auto-detection (PAC)
- Keep-alive through proxy
- Proxy timeout handling
- Fallback if proxy fails

**T6.4: WebSocket Performance**
- Benchmark WS vs native performance
- Optimize message framing
- Minimize latency overhead
- Connection pooling
- Compression support (permessage-deflate)
- Document performance characteristics

### Phase 7: Discovery & Auto-Configuration (Week 15-16)

**T7.1: DNS-Based Discovery**
- Implement SRV record lookup (pkg/nats/discovery.go)
- Parse SRV records for NATS endpoints
- Priority and weight handling
- TTL-based refresh
- Fallback to A/AAAA records
- Custom DNS resolver support

**T7.2: mDNS/Bonjour Discovery**
- Implement mDNS discovery for local network
- Service type: _nats._tcp
- Advertise agent embedded NATS
- Discover local NATS instances
- Automatic configuration on discovery
- LAN-only security consideration

**T7.3: Kubernetes Discovery**
- Kubernetes service discovery
- EndpointSlices watching
- Headless service support
- In-cluster automatic configuration
- Service account authentication
- Namespace scoping

**T7.4: Service Registry Discovery**
- Consul service discovery integration
- etcd service discovery integration
- HashiCorp Nomad integration
- Custom registry interface
- Health check integration
- Watch for endpoint changes

**T7.5: Auto-Configuration**
- Automatic strategy selection
- Network detection (NAT, firewall)
- Connectivity testing
- Fallback chain configuration
- Configuration caching
- Manual override support

### Phase 8: Reliability & Resilience (Week 17-18)

**T8.1: Message Buffering**
- Client-side message buffer (pkg/nats/buffer.go)
- Configurable buffer size
- Disk-backed buffer for large queues
- Buffer overflow policies (drop oldest, reject new, backpressure)
- Buffer drain on reconnection
- Buffer metrics

**T8.2: Delivery Guarantees**
- Implement at-least-once delivery
- JetStream for critical messages
- Message acknowledgment tracking
- Retry with exponential backoff
- Dead letter queue for failures
- Delivery metrics

**T8.3: Duplicate Detection**
- Message ID generation
- Deduplication window configuration
- Memory-efficient dedup storage
- Distributed dedup (JetStream)
- Dedup metrics
- Configurable per subject

**T8.4: Circuit Breaker**
- Implement circuit breaker for connections
- Failure threshold configuration
- Half-open state for recovery
- Per-endpoint circuit breakers
- Circuit breaker metrics
- Alerting on open circuits

**T8.5: Graceful Degradation**
- Continue operation during partial outage
- Queue commands when agents unreachable
- Retry commands on recovery
- Prioritize critical operations
- Rate limiting during recovery
- Recovery metrics

### Phase 9: Observability (Week 19-20)

**T9.1: Connection Metrics**
```
kscore_nats_connections_total{endpoint,strategy,status}
kscore_nats_connection_latency_seconds{endpoint}
kscore_nats_connection_errors_total{endpoint,error}
kscore_nats_messages_total{direction,subject_prefix}
kscore_nats_message_bytes_total{direction}
kscore_nats_buffer_size{type}
kscore_nats_buffer_overflow_total
kscore_nats_reconnections_total{endpoint}
kscore_nats_failovers_total{from,to}
```

**T9.2: Topology Visualization**
- Grafana dashboard for connection topology
- Node graph showing agents ↔ NATS ↔ servers
- Edge annotations (latency, errors)
- Real-time updates
- Drill-down to individual connections
- Historical topology changes

**T9.3: Connection Debugging**
- Connection state logging (debug level)
- Message flow tracing
- Latency breakdown (per hop)
- Packet capture integration (optional)
- Connection timeline visualization
- Diagnostic CLI commands

**T9.4: Alerting**
- Alert: High connection failure rate
- Alert: High message latency
- Alert: Buffer overflow
- Alert: Circuit breaker open
- Alert: Supercluster partition
- Alert: Leaf chain disconnection

### Phase 10: Testing & Validation (Week 21-22)

**T10.1: Unit Tests**
- Subject namespace tests
- Connection strategy tests
- Multi-endpoint selection tests
- Message buffer tests
- Duplicate detection tests
- Configuration validation tests

**T10.2: Integration Tests**
- End-to-end NATS-only communication
- Multi-endpoint failover
- Agent embedded NATS (reverse connection)
- Leaf node communication
- Gateway/supercluster communication
- WebSocket transport

**T10.3: Network Simulation Tests**
- NAT simulation (iptables)
- Firewall simulation (block ports)
- Latency injection (tc/netem)
- Packet loss simulation
- Connection reset simulation
- Bandwidth throttling

**T10.4: Chaos Tests**
- Random endpoint failures
- Random agent restarts
- Random gateway failures
- Network partition between clusters
- Simultaneous multi-failure
- Recovery time measurement

**T10.5: Performance Tests**
- Baseline: current implementation
- NATS-only: same workload
- Multi-endpoint overhead
- WebSocket overhead
- Leaf node overhead
- Supercluster latency

### Phase 11: Documentation (Week 23-24)

**T11.1: Architecture Documentation**
- NATS mesh architecture overview
- Subject namespace documentation
- Connection strategy documentation
- Topology diagrams for each mode
- Security model documentation
- Protocol specification

**T11.2: Deployment Guides**
- Simple deployment (embedded NATS)
- Production deployment (external cluster)
- Multi-region deployment (supercluster)
- Edge deployment (leaf nodes)
- Hybrid deployment (mixed topologies)
- Migration guide (from current architecture)

**T11.3: Operations Guides**
- Monitoring NATS mesh health
- Troubleshooting connectivity
- Capacity planning
- Performance tuning
- Disaster recovery
- Runbooks for common issues

**T11.4: API Reference**
- Configuration reference (all options)
- CLI reference (connection commands)
- Metrics reference
- Subject reference
- Environment variables

## NATS Deployment Patterns

### Pattern 1: Simple (Development)
```yaml
# Single embedded NATS on server
server:
  nats:
    mode: embedded
    listen: 0.0.0.0:4222

agent:
  nats:
    urls:
      - nats://server:4222
```

### Pattern 2: HA Cluster (Production)
```yaml
# External NATS cluster with multiple servers
server:
  nats:
    mode: external
    urls:
      - nats://nats-1:4222
      - nats://nats-2:4222
      - nats://nats-3:4222

agent:
  nats:
    urls:
      - nats://nats-1:4222
      - nats://nats-2:4222
      - nats://nats-3:4222
```

### Pattern 3: Edge Deployment (Leaf Nodes)
```yaml
# Edge agents connect to edge NATS, which connects to central
edge_nats:
  leafnodes:
    remotes:
      - url: leaf://central-nats:7422
        credentials: /path/to/edge.creds

agent:
  nats:
    urls:
      - nats://edge-nats:4222
```

### Pattern 4: Agent Hosted (Reverse Connection)
```yaml
# Agent hosts NATS, server connects to agent
agent:
  nats:
    mode: embedded
    listen: 0.0.0.0:4222
    advertise: public-ip:4222

server:
  nats:
    mode: external
    discover_agents: true
```

### Pattern 5: Multi-Region (Supercluster)
```yaml
# Region A cluster
nats_a:
  gateway:
    name: region-a
    gateways:
      - name: region-b
        urls:
          - nats://gateway-b:7222

# Region B cluster
nats_b:
  gateway:
    name: region-b
    gateways:
      - name: region-a
        urls:
          - nats://gateway-a:7222
```

### Pattern 6: Restrictive Firewall (WebSocket)
```yaml
# Agent behind strict firewall
agent:
  nats:
    urls:
      - wss://nats.company.com:443/nats
    proxy:
      url: http://corporate-proxy:3128
      auth:
        type: ntlm
        username: domain\user
```

## Security Considerations

### Transport Security
- TLS required for all production connections
- Certificate validation enabled by default
- Support for custom CA certificates
- Certificate rotation without downtime
- Minimum TLS version: 1.2

### Authentication
- Token-based authentication for agents
- Certificate-based authentication (mTLS)
- NKey-based authentication
- JWT-based authentication with claims
- Credential rotation support

### Authorization
- Per-subject authorization
- Deny by default, explicit allow
- Agent can only access own subjects
- Server has admin access
- Audit logging for all operations

### Network Security
- Embedded NATS binds to localhost by default
- External binding requires explicit configuration
- Firewall recommendations documented
- Rate limiting for connections
- DDoS protection recommendations

### Bootstrap Authentication (Agent Registration Security)

New agents connecting to NATS must follow a secure bootstrap process that limits their access until identity is verified:

**Bootstrap Credentials (Minimal Permissions)**:
```
# Bootstrap NATS account permissions (example)
bootstrap_account:
  permissions:
    publish:
      allow:
        - "kscore.{cluster}.agent.register"
    subscribe:
      allow:
        - "kscore.{cluster}.agent.{bootstrap_id}.register-response"
    deny:
      - ">"  # Deny all other subjects
```

**Registration Flow**:
```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Secure Agent Registration Flow                        │
│                                                                          │
│  1. Agent obtains bootstrap credentials (pre-provisioned or on-demand)  │
│     ┌─────────┐                                                         │
│     │  Agent  │ ← Bootstrap JWT/NKey (minimal permissions, short TTL)   │
│     └────┬────┘                                                         │
│          │                                                              │
│  2. Agent connects to NATS with bootstrap credentials                   │
│          │                                                              │
│          ▼                                                              │
│     ┌─────────────────────────────────────────────────────────────┐    │
│     │ NATS (Bootstrap Account)                                    │    │
│     │ - Can only publish: kscore.{cluster}.agent.register         │    │
│     │ - Can only subscribe: kscore.{cluster}.agent.{id}.reg-resp  │    │
│     │ - No access to other agents' subjects                       │    │
│     └──────────────────────┬──────────────────────────────────────┘    │
│                            │                                            │
│  3. Agent publishes registration request with identity proof            │
│          │                                                              │
│          ▼                                                              │
│     ┌─────────────────────────────────────────────────────────────┐    │
│     │ Control Plane Server                                        │    │
│     │ - Validates identity (PSK, cloud metadata, attestation)     │    │
│     │ - Generates agent-specific NATS credentials                 │    │
│     │ - Responds on agent's bootstrap response subject            │    │
│     └──────────────────────┬──────────────────────────────────────┘    │
│                            │                                            │
│  4. Server sends full credentials on bootstrap response subject         │
│          │                                                              │
│          ▼                                                              │
│     ┌─────────────────────────────────────────────────────────────┐    │
│     │ Agent receives credentials                                  │    │
│     │ - New JWT with agent-specific claims                        │    │
│     │ - Full permissions for agent's own subjects                 │    │
│     └──────────────────────┬──────────────────────────────────────┘    │
│                            │                                            │
│  5. Agent reconnects with full credentials                              │
│          │                                                              │
│          ▼                                                              │
│     ┌─────────────────────────────────────────────────────────────┐    │
│     │ NATS (Agent Account)                                        │    │
│     │ - Full access: kscore.{cluster}.agent.{agent_id}.*          │    │
│     │ - Subscribe to commands, publish responses                  │    │
│     └─────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────┘
```

**Identity Verification Options** (pluggable via IdentityVerifier interface):
- Pre-shared key (simple deployments)
- Cloud instance metadata (AWS IMDSv2, GCP metadata, Azure IMDS)
- TPM attestation (high-security environments)
- Future: SPIFFE SVID verification
- Future: Kubernetes service account token

**Credential Formats** (pluggable via CredentialIssuer interface):
- NATS JWT with custom claims (current)
- NATS NKey pairs (current)
- Future: SPIFFE SVID (X.509 or JWT-SVID)
- Future: Cloud IAM tokens

**Security Properties**:
- Bootstrap credentials have short TTL (default: 5 minutes)
- Bootstrap credentials can be bulk-revoked without affecting registered agents
- Full credentials are agent-specific (cannot impersonate other agents)
- Audit trail for all bootstrap and registration events
- Rate limiting on registration endpoint

### Server-to-Server Coordination Security

Server-to-server gRPC coordination channel operates independently of NATS:

**Authentication**:
- mTLS required (uses existing server TLS certificates)
- Certificate CN/SAN must match expected server identity
- Certificate rotation supported

**Authorization**:
- Only cluster member servers can connect
- Server identity verified against cluster membership (etcd)
- Operations logged for audit

**Use Cases**:
- NATS cluster health checking when NATS is degraded
- Leader election fallback when NATS is unavailable
- Coordinated NATS cluster recovery/restart
- Cluster quorum verification during network partitions

**Lightweight Design**:
- Minimal traffic when NATS is healthy (heartbeat only)
- Activated for coordination only when needed
- Does not replace NATS for normal operations

### Credential Migration (Epic 17 Integration)

This epic provides extensibility hooks for Epic 17 (SPIFFE Identity). During migration from static credentials to SPIFFE SVIDs:

**Dual-Mode Authentication Period**:
```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Credential Migration Timeline                         │
│                                                                          │
│  Phase 1: JWT/NKey Only (Pre-Epic 17)                                   │
│  ├── All agents use JWT or NKey authentication                          │
│  └── NATS configured for token/nkey auth                                │
│                                                                          │
│  Phase 2: Dual-Mode (Epic 17 Implementation)                            │
│  ├── NATS accepts BOTH JWT/NKey AND mTLS/SVID                          │
│  ├── New agents get SVIDs by default                                    │
│  ├── Existing agents continue with JWT/NKey                             │
│  ├── Metrics show migration progress                                    │
│  └── Warnings logged for legacy auth                                    │
│                                                                          │
│  Phase 3: SVID Required (Post-Migration)                                │
│  ├── JWT/NKey deprecated and disabled                                   │
│  ├── All agents must use SVID                                           │
│  └── Legacy agents fail to connect (planned cutover)                    │
└─────────────────────────────────────────────────────────────────────────┘
```

**NATS Configuration for Dual-Mode**:
```yaml
# NATS server supports both authentication methods simultaneously
tls:
  cert_file: /path/to/server.crt
  key_file: /path/to/server.key
  ca_file: /path/to/trust-bundle.pem  # For SVID verification
  verify_and_map: true                 # Map TLS CN to user

authorization:
  # Legacy JWT/NKey users (Phase 2 - deprecated after migration)
  users:
    - user: "agent-legacy-1"
      permissions: { ... }

  # SVID-authenticated agents mapped via TLS CN/SAN
  # spiffe://kscore.example.com/agent/{agent-id} → permissions
```

**Integration Points with Epic 17**:
- `IdentityVerifier` interface → Epic 17 attestation engine
- `CredentialIssuer` interface → Epic 17 SVID issuer
- Trust bundle distribution → Epic 17 trust bundle manager
- Credential rotation → Epic 17 SVID rotation

## Dependencies

- **NATS Server 2.10+**: Required for WebSocket, leaf nodes, superclusters
- **NATS Go Client**: Updated to latest version
- **Completed Epics**: Epic 1 (core), Epic 7 (metrics), Epic 11 (clustering)
- **External**: DNS server (for SRV discovery), Proxy server (optional)

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Breaking change for existing deployments | High | High | Migration guide, backward compatibility mode |
| Performance regression | Medium | Medium | Benchmark early, optimize hot paths |
| Complexity increase | Medium | High | Good documentation, sensible defaults |
| NATS server upgrade required | Medium | High | Document minimum version, check on startup |
| Supercluster configuration complexity | Medium | High | Validation, examples, templates |
| WebSocket overhead | Low | Medium | Benchmark, document limitations |
| Discovery service availability | Medium | Medium | Multiple discovery methods, manual fallback |
| Message loss during transition | High | Low | JetStream, client buffering, testing |

## Metrics & Monitoring

### Connection Metrics
```
kscore_nats_connection_state{agent_id,server_id,endpoint,state}
kscore_nats_connection_duration_seconds{agent_id,endpoint}
kscore_nats_reconnection_total{agent_id,endpoint,reason}
kscore_nats_failover_total{agent_id,from_endpoint,to_endpoint}
```

### Message Metrics
```
kscore_nats_messages_sent_total{subject_prefix,agent_id}
kscore_nats_messages_received_total{subject_prefix,server_id}
kscore_nats_message_latency_seconds{subject_prefix,direction}
kscore_nats_message_size_bytes{subject_prefix,direction}
```

### Topology Metrics
```
kscore_nats_leaf_nodes_total{hub}
kscore_nats_gateway_connections_total{local_cluster,remote_cluster}
kscore_nats_gateway_latency_seconds{local_cluster,remote_cluster}
```

### Buffer Metrics
```
kscore_nats_buffer_messages{agent_id,type}
kscore_nats_buffer_bytes{agent_id,type}
kscore_nats_buffer_overflow_total{agent_id,policy}
kscore_nats_buffer_drain_duration_seconds{agent_id}
```

### Server Coordination Metrics
```
kscore_server_coordination_connections_total{peer_server_id,status}
kscore_server_coordination_rpc_total{peer_server_id,method,status}
kscore_server_coordination_rpc_duration_seconds{peer_server_id,method}
kscore_server_coordination_nats_recovery_total{outcome}
kscore_server_coordination_leader_fallback_total
kscore_server_coordination_heartbeat_latency_seconds{peer_server_id}
```

### Bootstrap Authentication Metrics
```
kscore_agent_bootstrap_requests_total{status}           # success, rejected, expired
kscore_agent_bootstrap_duration_seconds                 # time from bootstrap to full registration
kscore_agent_bootstrap_identity_verification_total{method,status}  # psk, cloud_metadata, etc.
kscore_agent_credential_issued_total{type}              # jwt, nkey
kscore_agent_credential_rotation_total{agent_id,status}
kscore_agent_bootstrap_credentials_active               # current active bootstrap credentials
kscore_agent_bootstrap_rate_limited_total               # rate limiting triggered
```

## Testing Strategy

### Unit Tests
- Subject namespace parsing
- Connection strategy selection
- Multi-endpoint failover logic
- Message buffering
- Duplicate detection
- Configuration validation

### Integration Tests
- NATS-only communication end-to-end
- Multi-endpoint failover
- Leaf node communication
- Supercluster routing
- WebSocket transport
- Agent embedded NATS

### Network Tests
- NAT traversal (different NAT types)
- Firewall traversal (various configurations)
- Proxy traversal (HTTP, SOCKS)
- Latency resilience
- Packet loss resilience
- Connection reset recovery

### Chaos Tests
- Random NATS server failures
- Random network partitions
- Random agent restarts
- Simultaneous multi-failure
- Gateway failure between clusters
- Leaf chain disconnection

### Performance Tests
- Baseline comparison
- Throughput (messages/second)
- Latency (p50, p95, p99)
- Connection count scaling
- Memory usage
- CPU usage

## Definition of Done

- [ ] All 11 phases completed
- [ ] NATS-only agent↔server communication working
- [ ] Server↔server gRPC coordination channel implemented and tested
- [ ] Secure agent bootstrap registration with minimal permissions
- [ ] Bootstrap credential lifecycle (issue, validate, revoke, rotate)
- [ ] Identity verification extensibility points (future SPIFFE/SPIRE support)
- [ ] All 7 topologies tested and documented
- [ ] WebSocket transport working
- [ ] Leaf node chains working
- [ ] Supercluster working
- [ ] Discovery mechanisms implemented
- [ ] Message reliability verified (no loss)
- [ ] Performance benchmarks met
- [ ] Security review completed (including bootstrap flow)
- [ ] Documentation complete (including bootstrap and coordination)
- [ ] Migration guide available
- [ ] Chaos tests passing
- [ ] Production deployment tested

## Timeline

Total: **24 weeks** (6 months)

- **Weeks 1-3**: NATS-only communication refactor
- **Weeks 4-5**: Multi-endpoint support
- **Weeks 6-7**: Agent embedded NATS (reverse connection)
- **Weeks 8-9**: Leaf node support
- **Weeks 10-12**: Supercluster support
- **Weeks 13-14**: WebSocket transport
- **Weeks 15-16**: Discovery & auto-configuration
- **Weeks 17-18**: Reliability & resilience
- **Weeks 19-20**: Observability
- **Weeks 21-22**: Testing & validation
- **Weeks 23-24**: Documentation

## Success Metrics

- **Connectivity**: Agents connect successfully in all tested topologies
- **Reliability**: Zero message loss during planned failovers
- **Performance**: Latency within 20% of current implementation
- **Scalability**: Supercluster supports 10,000+ agents across 3+ regions
- **Availability**: 99.9% agent connectivity during network disturbances
- **Simplicity**: Default configuration works in 90% of environments

## Future Enhancements (Post-Epic)

- **QUIC Transport**: NATS over QUIC for mobile/lossy networks
- **Mesh Networking**: Peer-to-peer agent communication
- **Edge Computing**: Agent-to-agent work distribution
- **Multi-Tenancy**: Tenant isolation via NATS accounts
- **Traffic Shaping**: QoS for different message types
- **Geographic Routing**: Route commands to nearest agents
