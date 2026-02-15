---
title: "MCP Server Setup"
linkTitle: "MCP Server"
weight: 60
description: >
  Set up kscore-mcp to control Keystone Core from AI clients like Claude Desktop and Cursor.
---

## Overview

`kscore-mcp` is a Model Context Protocol (MCP) server that exposes Keystone Core operations to AI clients. It translates MCP JSON-RPC requests into gRPC calls to your running `kscore-server`, using your existing credentials for authentication.

```
AI Client --> (MCP/stdio) --> kscore-mcp --> (gRPC + your creds) --> kscore-server
```

## Prerequisites

- A running `kscore-server` with gRPC enabled
- Valid credentials (API key, JWT token, or mTLS certificates)
- `kscore-mcp` binary in your PATH

## Configuration

Create an MCP configuration file (e.g., `mcp.yaml`):

### API Key Authentication

```yaml
server:
  address: "localhost:50051"
  rest_base_url: "http://localhost:8080"

auth:
  method: apikey
  api_key: "your-api-key-here"

features:
  default_profile: ops_safe
  max_target_count: 50
```

### JWT Authentication

```yaml
server:
  address: "localhost:50051"

auth:
  method: jwt
  jwt_token: "your-jwt-token"
```

### mTLS Authentication

```yaml
server:
  address: "localhost:50051"

auth:
  method: mtls
  tls_cert: "/path/to/client.crt"
  tls_key: "/path/to/client.key"
  tls_ca_cert: "/path/to/ca.crt"
```

## Validate Configuration

```bash
kscore-mcp validate --config mcp.yaml
```

## Claude Desktop Integration

Add to your Claude Desktop configuration (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "keystone": {
      "command": "kscore-mcp",
      "args": ["--config", "/path/to/mcp.yaml"]
    }
  }
}
```

Restart Claude Desktop. The Keystone tools will appear in the tool list.

## Cursor Integration

In Cursor settings, add an MCP server:

- **Name**: keystone
- **Command**: `kscore-mcp --config /path/to/mcp.yaml`

## Available Tools

### Read-Only (all profiles)

| Tool | Description |
|------|-------------|
| `agent_list` | List registered agents with optional status filter |
| `agent_show` | Show details for a specific agent |
| `agent_health` | Cluster-wide agent health summary |
| `exec_status` | Check batch job status |
| `exec_history` | List recent command history |
| `state_check` | Dry-run state check |
| `state_drift` | Detect configuration drift |
| `state_history` | State application history |
| `event_query` | Query events with filters |
| `event_stats` | Event statistics by type |
| `runbook_list` | List available runbooks |
| `runbook_status` | Check runbook execution status |
| `cluster_status` | Cluster health overview |

### Operations (ops_safe, ops_admin)

| Tool | Description | Profile |
|------|-------------|---------|
| `exec_run` | Execute commands on targeted agents | ops_safe+ |
| `runbook_execute` | Execute a runbook | ops_safe+ |
| `state_apply` | Apply state to targeted agents | ops_admin |

### Resources

| URI | Description |
|-----|-------------|
| `keystone://agents` | Current agent inventory |
| `keystone://cluster/status` | Cluster health data |
| `keystone://events/recent` | Last 50 events |

## Capability Profiles

Profiles control which tools the AI client can see. Server-side RBAC still enforces permissions regardless of profile.

| Profile | Use Case |
|---------|----------|
| `read_only` | Monitoring, investigation, read-only queries |
| `ops_safe` | Day-to-day operations including command execution |
| `ops_admin` | Full access including state application |

Set the profile in your config:

```yaml
features:
  default_profile: read_only
```

## Example Interactions

After setup, ask your AI client:

- "List all online agents"
- "Show the cluster status"
- "What commands were run recently?"
- "Check for configuration drift on Linux hosts"
- "Run `uptime` on all agents matching os:linux"
- "Execute the deploy runbook"

## Troubleshooting

### Connection Refused

Verify `kscore-server` is running and the address in your config is correct:

```bash
grpcurl -plaintext localhost:50051 list
```

### Authentication Failed

Verify your credentials work with the CLI:

```bash
kscore-agents list --api-key your-key --server localhost:50051
```

### Tool Not Available

Check your profile allows the tool. Use `ops_admin` for full access, or check the profile table above.
