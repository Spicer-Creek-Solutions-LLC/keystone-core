---
title: "MCP Security Considerations"
linkTitle: "MCP Security"
weight: 61
description: >
  Security model, RBAC enforcement, and audit attribution for MCP-based AI operations.
---

## Security Model

The MCP server uses **credential pass-through** — it connects to `kscore-server` using the operator's own credentials. This means:

1. **Permission parity**: MCP users get exactly the same permissions as CLI/API users
2. **No service accounts**: No shared or elevated credentials
3. **Server-side RBAC is authoritative**: Client-side profiles are defense-in-depth, not authorization

```
AI Client --> kscore-mcp (operator's creds) --> kscore-server --> RBAC check
```

## Defense in Depth

### Layer 1: Capability Profiles (Client-Side)

Profiles control which tools the MCP server registers. Tools outside the profile simply don't exist for the AI client.

```yaml
features:
  default_profile: read_only  # AI can only see read-only tools
```

This prevents the AI from even attempting dangerous operations, but is not a security boundary.

### Layer 2: Server-Side RBAC (Authoritative)

Even if a tool is registered, `kscore-server` enforces RBAC based on the principal's role. A `readonly` API key cannot execute commands regardless of the MCP profile.

### Layer 3: Audit Attribution

Every MCP operation is logged with full attribution:

```
principal=alice role=operator client_type=mcp mcp_tool=exec_run mcp_session=abc123 mcp_ai_client=claude-desktop
```

This allows you to distinguish between direct CLI usage and AI-assisted operations in audit logs.

## Audit Attribution

The MCP server injects metadata headers on every gRPC call:

| Header | Description |
|--------|-------------|
| `x-kscore-client-type` | Always `mcp` |
| `x-kscore-mcp-tool` | The tool being invoked (e.g., `agent_list`) |
| `x-kscore-mcp-session` | UUID identifying this MCP session |
| `x-kscore-mcp-ai-client` | AI client identifier (e.g., `claude-desktop`) |

These are captured into the principal's metadata after authentication and included in audit log entries.

## Recommendations

### Use the Most Restrictive Profile

Start with `read_only` and escalate only when needed:

```yaml
features:
  default_profile: read_only
```

### Use Scoped API Keys

Create API keys with the minimum required role:

```yaml
# For monitoring/investigation
auth:
  method: apikey
  api_key: "readonly-key"

features:
  default_profile: read_only
```

### Limit Target Scope

Set `max_target_count` to prevent accidental mass operations:

```yaml
features:
  max_target_count: 10  # Limit exec_run to 10 agents at a time
```

### Monitor Audit Logs

Filter audit logs for MCP activity:

```bash
# Find all MCP operations
kscore-audit list --filter 'client_type=mcp'

# Find operations by a specific AI session
kscore-audit list --filter 'mcp_session=abc123'
```

### Use TLS in Production

Always use TLS for the gRPC connection in production:

```yaml
auth:
  method: apikey
  api_key: "your-key"
  tls_ca_cert: "/path/to/ca.crt"
```

## Untrusted Output

Command execution output (`exec_run`, `state_apply`) is tagged as untrusted content using MCP annotations. This signals to the AI client that the content comes from an external source and should not be treated as instructions.

## Stdio Transport

The MCP server currently uses stdio transport only, meaning each MCP server instance serves a single AI client session. This simplifies the security model — there is no multi-tenant concern. HTTP transport for multi-user deployments is planned for a future release.
