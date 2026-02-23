---
title: "Outbound Webhooks"
linkTitle: "Outbound Webhooks"
weight: 26
description: >
  Manage outbound webhook subscriptions, delivery tracking, and HMAC signature verification.
---

## Overview

Outbound webhooks deliver internal Keystone Core events to external HTTP endpoints in real time. Each delivery is HMAC-SHA256 signed, retried on failure with exponential backoff, and tracked in a persistent delivery log.

```
Event Bus (NATS) → Dispatcher → HTTP POST → External Endpoint
                       ↓
                  SQLite Store (subscriptions + delivery history)
```

## Creating Subscriptions

A subscription binds one or more event patterns to a target URL:

```bash
# Subscribe to all agent events
kscorectl webhook outbound create \
  --name "agent-alerts" \
  --url "https://hooks.example.com/kscore" \
  --events "agent.*" \
  --secret "your-hmac-secret"

# Subscribe to specific event types
kscorectl webhook outbound create \
  --name "security-events" \
  --url "https://siem.example.com/ingest" \
  --events "auth.failed,policy.violation" \
  --secret "siem-secret" \
  --max-retries 5
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | required | Human-readable subscription name |
| `--url` | required | Target HTTP endpoint |
| `--events` | required | Comma-separated event patterns (glob supported) |
| `--secret` | `""` | HMAC-SHA256 signing secret |
| `--max-retries` | `3` | Max delivery retry attempts |
| `--timeout` | `10` | HTTP request timeout in seconds |

## Managing Subscriptions

```bash
# List all subscriptions
kscorectl webhook outbound list

# Show subscription details
kscorectl webhook outbound show sub_abc123

# Delete a subscription
kscorectl webhook outbound delete sub_abc123
```

## Delivery Tracking

Every delivery attempt is recorded with status, HTTP status code, attempt number, and any error message.

```bash
# View recent deliveries for a subscription
kscorectl webhook outbound history sub_abc123 --limit 20
```

Delivery statuses:

| Status | Meaning |
|--------|---------|
| `pending` | Queued, not yet attempted |
| `success` | Delivered successfully (2xx response) |
| `retrying` | Failed, will retry |
| `failed` | All retry attempts exhausted |

Old delivery records are automatically purged based on the `webhook.outbound.delivery_retention` setting (default: 7 days).

## Testing a Subscription

Send a test event to verify the endpoint is reachable and signature verification works:

```bash
kscorectl webhook outbound test sub_abc123
```

This sends a synthetic event with `event_type: test` to the subscription's URL.

## HMAC Signature Verification

When a subscription has a `secret` configured, every delivery includes an HMAC-SHA256 signature in the `X-Signature-256` header. The format is GitHub-compatible:

```
X-Signature-256: sha256=<hex-encoded-hmac>
```

Additional headers on each delivery:

| Header | Description |
|--------|-------------|
| `X-Signature-256` | HMAC-SHA256 of the request body |
| `X-Webhook-ID` | Unique delivery ID |
| `X-Webhook-Timestamp` | Unix timestamp of the delivery |
| `Content-Type` | `application/json` |

### Verifying Signatures

To verify a delivery on the receiving side, compute the HMAC-SHA256 of the raw request body using your shared secret and compare it to the signature header.

**Go:**

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "strings"
)

func verifySignature(secret, body []byte, header string) bool {
    sig := strings.TrimPrefix(header, "sha256=")
    sigBytes, err := hex.DecodeString(sig)
    if err != nil {
        return false
    }
    mac := hmac.New(sha256.New, secret)
    mac.Write(body)
    return hmac.Equal(sigBytes, mac.Sum(nil))
}
```

**Python:**

```python
import hmac
import hashlib

def verify_signature(secret: bytes, body: bytes, header: str) -> bool:
    expected = "sha256=" + hmac.new(secret, body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, header)
```

## Retry Behavior

Failed deliveries are retried with exponential backoff:

- **Base interval**: Configured via `webhook.outbound.retry_backoff` (default: 1s)
- **Backoff multiplier**: 2x per attempt
- **Max retries**: Per-subscription `max_retries` or the global default (3)

Example with defaults: retry at 1s, 2s, 4s, then mark as `failed`.

A delivery is considered failed if:
- The HTTP response status is not 2xx
- The connection times out
- DNS resolution fails

## Configuration

Outbound webhooks are configured in `server.yaml`:

```yaml
webhook:
  outbound:
    enabled: true
    max_retries: 3
    retry_backoff: 1s
    timeout: 10s
    max_payload_size: 1048576    # 1 MB
    delivery_retention: 168h     # 7 days
```

Set `webhook.outbound.enabled: true` to activate the outbound webhook system. See [Configuration Reference](../../reference/configuration/#outbound-webhooks) for all settings and environment variable overrides.

## Monitoring

Key indicators to watch:

- **Delivery success rate**: A sustained drop indicates endpoint issues or misconfiguration
- **Retry volume**: High retry counts suggest the target endpoint is struggling
- **Failed deliveries**: Alerts should fire on sustained delivery failures
- **Delivery latency**: Time from event emission to successful delivery

Use `webhook outbound history` to investigate delivery failures for a specific subscription. Check the `error` field for connection errors and `status_code` for HTTP-level failures.

## Common Patterns

### Slack Notifications

```bash
kscorectl webhook outbound create \
  --name "slack-alerts" \
  --url "https://hooks.slack.com/services/T.../B.../..." \
  --events "agent.offline,policy.violation"
```

Note: Slack webhooks don't support HMAC verification, so omit `--secret`.

### SIEM Integration

```bash
kscorectl webhook outbound create \
  --name "siem-feed" \
  --url "https://siem.internal/api/v1/events" \
  --events "*" \
  --secret "siem-hmac-key" \
  --max-retries 5
```

### CI/CD Pipeline Triggers

```bash
kscorectl webhook outbound create \
  --name "deploy-trigger" \
  --url "https://ci.example.com/api/webhooks/kscore" \
  --events "state.drift_detected" \
  --secret "ci-webhook-secret"
```

## See Also

- [API Reference](../../reference/api/#outbound-webhooks) - REST API endpoints
- [CLI Reference](../../reference/cli/) - Full command documentation
- [Configuration Reference](../../reference/configuration/#outbound-webhooks) - All configuration options
