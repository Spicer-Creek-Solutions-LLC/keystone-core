---
title: "Visualization API Reference"
weight: 9
description: >
  Infrastructure topology visualization and real-time updates
---

The Visualization API provides infrastructure topology visualization through HTTP and WebSocket endpoints. It enables dashboards and UIs to display agent relationships, hierarchical views, and real-time status updates.

## Overview

The Visualization API offers:

- **Agent List**: Filtered list of all agents with metadata
- **Topology Tree**: Hierarchical view (datacenter → environment → role → agent)
- **Graph View**: Graph representation with nodes and edges
- **Real-time Updates**: WebSocket stream for live topology changes

```
                    ┌─────────────────────────────────────────┐
                    │         Visualization Server            │
                    │  ┌──────────┬──────────┬─────────────┐  │
                    │  │  /api/*  │ /ws/*    │   Provider  │  │
                    │  │  (HTTP)  │(WebSocket)│  Interface  │  │
                    │  └────┬─────┴─────┬────┴──────┬──────┘  │
                    └───────┼───────────┼───────────┼─────────┘
                            │           │           │
                    ┌───────▼───────────▼───────────▼─────────┐
                    │           Agent Data Store              │
                    └─────────────────────────────────────────┘
```

## Server Configuration

### VisualizationConfig

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `listen_addr` | `string` | Address to listen on | `localhost:8080` |
| `enable_websocket` | `bool` | Enable WebSocket support | `true` |
| `update_interval` | `time.Duration` | Broadcast interval | `5s` |

### Creating a Server

```go
import "github.com/keystone-core/pkg/visualization"

// Create with default configuration
config := visualization.DefaultConfig()

// Or customize
config := &visualization.VisualizationConfig{
    ListenAddr:      ":8080",
    EnableWebSocket: true,
    UpdateInterval:  5 * time.Second,
}

// Create server with an agent provider
server := visualization.NewServer(config, agentProvider)

// Start the server
ctx := context.Background()
if err := server.Start(ctx); err != nil {
    log.Fatal(err)
}

// Stop when done
defer server.Stop(ctx)
```

### AgentProvider Interface

The server requires an `AgentProvider` implementation to get agent data:

```go
type AgentProvider interface {
    // GetAgents returns all agents
    GetAgents() []*Agent

    // GetAgent returns a specific agent by ID
    GetAgent(id string) (*Agent, error)
}
```

## HTTP API Endpoints

### GET /api/agents

List all agents with optional filtering.

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `datacenter` | `string` | Filter by datacenter |
| `environment` | `string` | Filter by environment (prod, staging, dev) |
| `role` | `string` | Filter by role (web, db, cache, etc.) |
| `status` | `string` | Filter by status (healthy, degraded, offline, unknown) |

#### Request

```bash
# Get all agents
curl http://localhost:8080/api/agents

# Filter by datacenter and environment
curl "http://localhost:8080/api/agents?datacenter=us-west-2&environment=prod"

# Filter by status
curl "http://localhost:8080/api/agents?status=healthy"
```

#### Response

```json
{
  "agents": [
    {
      "id": "agent-001",
      "hostname": "web-server-1",
      "status": "healthy",
      "last_seen": "2026-01-10T12:00:00Z",
      "metadata": {
        "os": "linux",
        "arch": "amd64"
      },
      "tags": ["web", "frontend"],
      "datacenter": "us-west-2",
      "environment": "prod",
      "role": "web",
      "version": "1.2.3"
    }
  ],
  "total": 1
}
```

### GET /api/agents/{id}

Get a specific agent by ID.

#### Request

```bash
curl http://localhost:8080/api/agents/agent-001
```

#### Response

```json
{
  "id": "agent-001",
  "hostname": "web-server-1",
  "status": "healthy",
  "last_seen": "2026-01-10T12:00:00Z",
  "metadata": {
    "os": "linux",
    "arch": "amd64",
    "cpu_cores": "8",
    "memory_gb": "32"
  },
  "tags": ["web", "frontend"],
  "datacenter": "us-west-2",
  "environment": "prod",
  "role": "web",
  "version": "1.2.3"
}
```

#### Error Response

```json
// 404 Not Found
{
  "error": "Agent not found"
}
```

### GET /api/topology

Get hierarchical topology tree organized by datacenter, environment, and role.

#### Request

```bash
curl http://localhost:8080/api/topology
```

#### Response

```json
{
  "id": "root",
  "name": "Infrastructure",
  "type": "root",
  "status": "healthy",
  "stats": {
    "total": 10,
    "healthy": 8,
    "degraded": 1,
    "offline": 1
  },
  "children": [
    {
      "id": "us-west-2",
      "name": "us-west-2",
      "type": "datacenter",
      "status": "healthy",
      "stats": {
        "total": 6,
        "healthy": 5,
        "degraded": 1,
        "offline": 0
      },
      "children": [
        {
          "id": "us-west-2/prod",
          "name": "prod",
          "type": "environment",
          "status": "healthy",
          "stats": {
            "total": 4,
            "healthy": 4,
            "degraded": 0,
            "offline": 0
          },
          "children": [
            {
              "id": "us-west-2/prod/web",
              "name": "web",
              "type": "role",
              "status": "healthy",
              "stats": {
                "total": 2,
                "healthy": 2,
                "degraded": 0,
                "offline": 0
              },
              "children": [
                {
                  "id": "agent-001",
                  "name": "web-server-1",
                  "type": "agent",
                  "status": "healthy",
                  "agent": {
                    "id": "agent-001",
                    "hostname": "web-server-1",
                    "status": "healthy",
                    "datacenter": "us-west-2",
                    "environment": "prod",
                    "role": "web"
                  }
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

### GET /api/graph

Get graph representation with nodes and edges for visualization libraries.

#### Request

```bash
curl http://localhost:8080/api/graph
```

#### Response

```json
{
  "nodes": [
    {
      "id": "agent-001",
      "label": "web-server-1",
      "type": "agent",
      "status": "healthy",
      "metadata": {
        "datacenter": "us-west-2",
        "environment": "prod",
        "role": "web"
      }
    },
    {
      "id": "agent-002",
      "label": "web-server-2",
      "type": "agent",
      "status": "healthy",
      "metadata": {
        "datacenter": "us-west-2",
        "environment": "prod",
        "role": "web"
      }
    }
  ],
  "edges": [
    {
      "source": "agent-001",
      "target": "agent-002",
      "type": "same_datacenter"
    }
  ]
}
```

## WebSocket API

### WS /ws/topology

Real-time topology updates via WebSocket.

#### Connection

```javascript
const ws = new WebSocket('ws://localhost:8080/ws/topology');

ws.onopen = () => {
    console.log('Connected to topology stream');
};

ws.onmessage = (event) => {
    const update = JSON.parse(event.data);
    console.log('Received update:', update);
};

ws.onclose = () => {
    console.log('Disconnected from topology stream');
};
```

#### Initial Message

On connection, the server sends the current topology:

```json
{
  "type": "initial",
  "data": {
    "id": "root",
    "name": "Infrastructure",
    "type": "root",
    "status": "healthy",
    "children": [...]
  }
}
```

#### Update Messages

Subsequent messages contain topology updates:

```json
{
  "type": "agent_added",
  "agent_id": "agent-003",
  "agent": {
    "id": "agent-003",
    "hostname": "web-server-3",
    "status": "healthy",
    "datacenter": "us-west-2",
    "environment": "prod",
    "role": "web"
  },
  "timestamp": "2026-01-10T12:00:00Z"
}
```

#### Update Types

| Type | Description |
|------|-------------|
| `initial` | Initial topology on connection |
| `agent_added` | New agent registered |
| `agent_removed` | Agent was removed |
| `agent_updated` | Agent metadata changed |
| `status_changed` | Agent status changed |
| `periodic_update` | Scheduled topology refresh |

### Broadcasting Updates

From the server side, broadcast updates to all connected clients:

```go
server.BroadcastUpdate(visualization.TopologyUpdate{
    Type:      "agent_added",
    AgentID:   "agent-003",
    Agent:     newAgent,
    Timestamp: time.Now(),
})
```

## Data Types

### Agent

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | Unique agent identifier |
| `hostname` | `string` | Agent hostname |
| `status` | `AgentStatus` | Current status |
| `last_seen` | `time.Time` | Last heartbeat time |
| `metadata` | `map[string]string` | Agent metadata |
| `tags` | `[]string` | Agent tags |
| `datacenter` | `string` | Datacenter location |
| `environment` | `string` | Environment (prod, staging, dev) |
| `role` | `string` | Agent role (web, db, cache) |
| `version` | `string` | Agent version |

### AgentStatus

| Value | Description |
|-------|-------------|
| `healthy` | Agent is functioning normally |
| `degraded` | Agent has issues but is operational |
| `offline` | Agent is not responding |
| `unknown` | Status cannot be determined |

### TopologyNode

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | Node identifier |
| `name` | `string` | Display name |
| `type` | `string` | Node type (root, datacenter, environment, role, agent) |
| `status` | `AgentStatus` | Aggregated status |
| `children` | `[]*TopologyNode` | Child nodes |
| `agent` | `*Agent` | Agent data (if type is "agent") |
| `stats` | `*NodeStats` | Statistics for this node |

### NodeStats

| Field | Type | Description |
|-------|------|-------------|
| `total` | `int` | Total agents under this node |
| `healthy` | `int` | Healthy agent count |
| `degraded` | `int` | Degraded agent count |
| `offline` | `int` | Offline agent count |

### GraphNode

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | Node identifier |
| `label` | `string` | Display label |
| `type` | `string` | Node type |
| `status` | `AgentStatus` | Node status |
| `metadata` | `map[string]interface{}` | Additional metadata |

### GraphEdge

| Field | Type | Description |
|-------|------|-------------|
| `source` | `string` | Source node ID |
| `target` | `string` | Target node ID |
| `label` | `string` | Edge label |
| `type` | `string` | Edge type (e.g., "same_datacenter", "depends_on") |

### FilterOptions

| Field | Type | Description |
|-------|------|-------------|
| `datacenter` | `string` | Filter by datacenter |
| `environment` | `string` | Filter by environment |
| `role` | `string` | Filter by role |
| `status` | `AgentStatus` | Filter by status |
| `tags` | `[]string` | Filter by tags (all must match) |

### TopologyUpdate

| Field | Type | Description |
|-------|------|-------------|
| `type` | `string` | Update type |
| `agent_id` | `string` | Affected agent ID |
| `agent` | `*Agent` | Updated agent data |
| `timestamp` | `time.Time` | When the update occurred |

## Integration Examples

### React Component

```jsx
import React, { useState, useEffect } from 'react';

function TopologyView() {
    const [topology, setTopology] = useState(null);

    useEffect(() => {
        // Initial fetch
        fetch('/api/topology')
            .then(res => res.json())
            .then(data => setTopology(data));

        // WebSocket for live updates
        const ws = new WebSocket('ws://localhost:8080/ws/topology');

        ws.onmessage = (event) => {
            const update = JSON.parse(event.data);
            if (update.type === 'initial') {
                setTopology(update.data);
            } else {
                // Handle incremental updates
                setTopology(prev => updateTopology(prev, update));
            }
        };

        return () => ws.close();
    }, []);

    return (
        <div className="topology">
            {topology && <TopologyTree node={topology} />}
        </div>
    );
}
```

### D3.js Graph Visualization

```javascript
async function renderGraph() {
    const response = await fetch('/api/graph');
    const graph = await response.json();

    const svg = d3.select('#graph')
        .append('svg')
        .attr('width', 800)
        .attr('height', 600);

    // Create force simulation
    const simulation = d3.forceSimulation(graph.nodes)
        .force('link', d3.forceLink(graph.edges).id(d => d.id))
        .force('charge', d3.forceManyBody().strength(-100))
        .force('center', d3.forceCenter(400, 300));

    // Draw edges
    const links = svg.selectAll('.link')
        .data(graph.edges)
        .enter().append('line')
        .attr('class', 'link');

    // Draw nodes
    const nodes = svg.selectAll('.node')
        .data(graph.nodes)
        .enter().append('circle')
        .attr('class', d => `node status-${d.status}`)
        .attr('r', 10);

    // Update positions on tick
    simulation.on('tick', () => {
        links
            .attr('x1', d => d.source.x)
            .attr('y1', d => d.source.y)
            .attr('x2', d => d.target.x)
            .attr('y2', d => d.target.y);

        nodes
            .attr('cx', d => d.x)
            .attr('cy', d => d.y);
    });
}
```

### Go Client

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
)

func main() {
    // Get topology
    resp, err := http.Get("http://localhost:8080/api/topology")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    var topology map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&topology)

    fmt.Printf("Topology: %+v\n", topology)

    // Get filtered agents
    resp, err = http.Get("http://localhost:8080/api/agents?datacenter=us-west-2&status=healthy")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)

    fmt.Printf("Healthy agents in us-west-2: %v\n", result["total"])
}
```

## Best Practices

1. **Use WebSocket for dashboards**: For real-time dashboards, prefer WebSocket connections over polling HTTP endpoints
2. **Filter early**: Use query parameters to filter agents at the server rather than client-side filtering
3. **Handle reconnection**: Implement WebSocket reconnection logic with exponential backoff
4. **Cache topology**: Cache the full topology and apply incremental updates for better performance
5. **Monitor connection count**: Track WebSocket connections to prevent resource exhaustion

## See Also

- [Observability Concepts](../../concepts/observability/) - Architecture overview
- [Agents Concept](../../concepts/agents/) - Agent architecture
- [TUI Monitor](../cli/#kscore-monitor) - Terminal-based monitoring
