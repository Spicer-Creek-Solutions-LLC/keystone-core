---
title: DNS Records Module
description: Manage DNS records across multiple providers with declarative state definitions
weight: 30
---

## Overview

The DNS Records module (`dns_records`) enables declarative management of DNS records across multiple providers. It integrates with the Keystone state management system to provide idempotent operations, drift detection, and dry-run capabilities.

## Supported Providers

Keystone Core supports the following DNS providers via the [libdns](https://github.com/libdns/libdns) ecosystem:

| Provider | Configuration Key | Record Types | Notes |
|----------|------------------|--------------|-------|
| Cloudflare | `cloudflare` | A, AAAA, CNAME, TXT, MX, SRV, CAA, NS | Supports proxied records |
| Amazon Route 53 | `route53` | A, AAAA, CNAME, TXT, MX, SRV, CAA, NS, PTR, ALIAS | Supports alias records |
| Google Cloud DNS | `gcp` or `googleclouddns` | A, AAAA, CNAME, TXT, MX, SRV, CAA, NS, PTR | |
| Microsoft Azure DNS | `azure` | A, AAAA, CNAME, TXT, MX, SRV, CAA, NS, PTR | |
| DigitalOcean DNS | `digitalocean` | A, AAAA, CNAME, TXT, MX, SRV, NS | |
| DNSMadeEasy | `dnsmadeeasy` | A, AAAA, CNAME, TXT, MX, SRV, NS, PTR | |
| Hetzner DNS | `hetzner` | A, AAAA, CNAME, TXT, MX, SRV, CAA, NS | |

Additional providers can be added via the libdns ecosystem. See [Adding Custom Providers](#adding-custom-providers) below.

## State Definition

### Basic Structure

```yaml
dns:
  manage_zone_records:
    provider: cloudflare
    zone: example.com
    credentials:
      secret_ref: secret://dns/cloudflare
    state: present   # or: synced, absent
    records:
      - type: A
        name: www
        value: 203.0.113.10
        ttl: 300
      - type: CNAME
        name: api
        value: api.internal.example.com.
        ttl: 600
```

### States

| State | Description |
|-------|-------------|
| `present` | Ensure specified records exist (additive) |
| `synced` | Ensure zone matches exactly (removes extra records) |
| `absent` | Ensure specified records do not exist |

### Record Types

| Type | Fields | Description |
|------|--------|-------------|
| `A` | value (IPv4) | IPv4 address record |
| `AAAA` | value (IPv6) | IPv6 address record |
| `CNAME` | value (hostname) | Canonical name record |
| `TXT` | value (text) | Text record |
| `MX` | value (hostname), priority | Mail exchange record |
| `SRV` | value (hostname), priority, weight, port | Service record |
| `CAA` | value, tag | Certificate Authority Authorization |
| `NS` | value (hostname) | Name server record |
| `ALIAS` | value (hostname) | Alias record (provider-specific) |
| `PTR` | value (hostname) | Pointer record |

## Credentials Configuration

DNS credentials are resolved through Keystone's secret management system. Each provider has specific credential requirements.

### Cloudflare

```yaml
credentials:
  secret_ref: secret://dns/cloudflare
  # Or inline (not recommended for production):
  api_token: "your-api-token"
  # Or legacy API key:
  api_key: "your-api-key"
  api_email: "your-email@example.com"
```

Environment variables: `CLOUDFLARE_API_TOKEN` or `CLOUDFLARE_API_KEY` + `CLOUDFLARE_EMAIL`

### Amazon Route 53

```yaml
credentials:
  secret_ref: secret://dns/route53
  # Or inline:
  access_key_id: "AKIAIOSFODNN7EXAMPLE"
  secret_access_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  region: "us-east-1"  # optional
```

Environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`

### Google Cloud DNS

```yaml
credentials:
  secret_ref: secret://dns/gcp
  # Or inline:
  project_id: "my-project"
  service_account_json: |
    { "type": "service_account", ... }
```

Environment variables: `GOOGLE_PROJECT_ID`, `GOOGLE_APPLICATION_CREDENTIALS`

### Microsoft Azure DNS

```yaml
credentials:
  secret_ref: secret://dns/azure
  # Or inline:
  subscription_id: "your-subscription-id"
  resource_group: "your-resource-group"
  tenant_id: "your-tenant-id"
  client_id: "your-client-id"
  client_secret: "your-client-secret"
```

Environment variables: `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`

### DigitalOcean DNS

```yaml
credentials:
  secret_ref: secret://dns/digitalocean
  # Or inline:
  api_token: "your-do-token"
```

Environment variable: `DIGITALOCEAN_TOKEN`

### DNSMadeEasy

```yaml
credentials:
  secret_ref: secret://dns/dnsmadeeasy
  # Or inline:
  api_key: "your-api-key"
  secret_key: "your-secret-key"
```

Environment variables: `DNSMADEEASY_API_KEY`, `DNSMADEEASY_SECRET_KEY`

### Hetzner DNS

```yaml
credentials:
  secret_ref: secret://dns/hetzner
  # Or inline:
  api_token: "your-api-token"
```

Environment variable: `HETZNER_DNS_API_TOKEN`

## Usage Examples

### Managing Web Server Records

```yaml
# web-records.yaml
dns:
  web_endpoints:
    provider: cloudflare
    zone: example.com
    credentials:
      secret_ref: secret://dns/cloudflare
    state: present
    records:
      # Primary web server
      - type: A
        name: www
        value: 203.0.113.10
        ttl: 300
      # Root domain
      - type: A
        name: "@"
        value: 203.0.113.10
        ttl: 300
      # API endpoint
      - type: CNAME
        name: api
        value: api-lb.internal.example.com.
        ttl: 600
```

### Multi-Provider Setup

```yaml
# multi-provider.yaml
dns:
  # Primary zone on Cloudflare
  primary_zone:
    provider: cloudflare
    zone: example.com
    credentials:
      secret_ref: secret://dns/cloudflare
    state: synced
    records:
      - type: A
        name: www
        value: 203.0.113.10
        ttl: 300

  # Secondary zone on Route53
  internal_zone:
    provider: route53
    zone: internal.example.com
    credentials:
      secret_ref: secret://dns/route53
    state: present
    records:
      - type: A
        name: db
        value: 10.0.1.50
        ttl: 60
```

### Mail Records

```yaml
dns:
  mail_records:
    provider: cloudflare
    zone: example.com
    credentials:
      secret_ref: secret://dns/cloudflare
    state: present
    records:
      # MX records with priority
      - type: MX
        name: "@"
        value: mx1.mail.example.com.
        priority: 10
        ttl: 3600
      - type: MX
        name: "@"
        value: mx2.mail.example.com.
        priority: 20
        ttl: 3600
      # SPF record
      - type: TXT
        name: "@"
        value: "v=spf1 include:_spf.google.com ~all"
        ttl: 3600
      # DKIM record
      - type: TXT
        name: "google._domainkey"
        value: "v=DKIM1; k=rsa; p=MIGfMA0GCSqGS..."
        ttl: 3600
      # DMARC record
      - type: TXT
        name: "_dmarc"
        value: "v=DMARC1; p=reject; rua=mailto:dmarc@example.com"
        ttl: 3600
```

### Service Discovery with SRV Records

```yaml
dns:
  service_discovery:
    provider: route53
    zone: internal.example.com
    credentials:
      secret_ref: secret://dns/route53
    state: present
    records:
      # SIP service
      - type: SRV
        name: "_sip._tcp"
        value: sip.example.com.
        priority: 10
        weight: 60
        port: 5060
        ttl: 300
      # LDAP service
      - type: SRV
        name: "_ldap._tcp"
        value: ldap.example.com.
        priority: 0
        weight: 100
        port: 389
        ttl: 300
```

## CLI Operations

The DNS module integrates with `kscore-state` for all operations.

### Check for Drift

```bash
# Check all states including DNS
kscorectl state check states/

# Check specific DNS state file
kscorectl state check states/dns-records.yaml
```

### Preview Changes (Dry-Run)

```bash
# Preview what changes would be made
kscorectl state apply --dry-run states/dns-records.yaml
```

Example output:
```
DNS Records: manage-zone-records (example.com via cloudflare)
  + CREATE A www.example.com → 203.0.113.10 (TTL: 300)
  ~ UPDATE CNAME api.example.com → api-lb.internal.example.com. (TTL: 300→600)
  - DELETE A old.example.com

Summary: 1 to create, 1 to update, 1 to delete
```

### Apply Changes

```bash
# Apply DNS record changes
kscorectl state apply states/dns-records.yaml
```

### View Drift

```bash
# Show current drift from desired state
kscorectl state drift states/dns-records.yaml
```

## Rate Limiting and Best Practices

### Rate Limits by Provider

| Provider | Rate Limit | Recommendations |
|----------|-----------|-----------------|
| Cloudflare | 1200 req/5min | Batch changes, use API tokens |
| Route53 | 5 req/sec | Use batch change sets |
| Google Cloud DNS | 10 req/sec | Standard quota, request increase if needed |
| Azure DNS | 500 req/5min | Use managed identities |
| DigitalOcean | 250 req/min | Moderate batch sizes |
| DNSMadeEasy | 150 req/5min | Use HMAC authentication |
| Hetzner | 600 req/min | Standard API limits |

### Best Practices

1. **Use Secrets Management**: Never hardcode credentials in state files
2. **Prefer `present` Over `synced`**: Use `synced` only when you need to remove unknown records
3. **Set Appropriate TTLs**: Lower TTLs during migrations, higher for stable records
4. **Test with Dry-Run**: Always preview changes before applying
5. **Zone Isolation**: Use separate state files per zone for better control
6. **Version Control**: Keep DNS state files in version control with code review

### Compliance Considerations

- **Audit Logging**: All DNS changes are logged to the audit system
- **Change Tracking**: Use `--dry-run` for change approval workflows
- **CAA Records**: Consider adding CAA records to restrict certificate issuance
- **DNSSEC**: Enable DNSSEC at the provider level for supported zones

## Troubleshooting

### Common Issues

**Authentication Failures**
```
Error: provider authentication failed: invalid API token
```
- Verify credentials are correct and have DNS zone permissions
- Check environment variables are set if using env-based auth
- Ensure secret references point to valid secrets

**Record Conflicts**
```
Error: record already exists with different value
```
- Use `state: synced` to take ownership of existing records
- Or manually reconcile conflicts before applying

**Rate Limiting**
```
Error: rate limit exceeded, retry after 60s
```
- Reduce batch size in state definitions
- Use exponential backoff (automatic with retries)
- Request quota increase from provider

### Debug Mode

Enable verbose logging for DNS operations:

```bash
KSCORE_LOG_LEVEL=debug kscorectl state apply states/dns-records.yaml
```

## Metrics

The DNS module exports the following Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `keystone_dns_operations_total` | Counter | Total DNS operations by type and provider |
| `keystone_dns_operation_duration_seconds` | Histogram | Operation latency by type and provider |
| `keystone_dns_errors_total` | Counter | Total errors by type, provider, and error code |
| `keystone_dns_records_managed` | Gauge | Number of records currently managed |

## Adding Custom Providers

Keystone Core uses the [libdns](https://github.com/libdns/libdns) interface for DNS provider integrations. To add support for additional providers:

1. Find or create a libdns provider package (see [libdns providers](https://github.com/orgs/libdns/repositories))
2. Create a provider file in `internal/dns/providers/`
3. Define provider capabilities
4. Register the provider in `init()`

Example:

```go
package providers

import (
    "fmt"
    "github.com/libdns/myprovider"
    "github.com/shawnbutts/keystone-core/internal/dns"
)

var MyProviderCapabilities = dns.ProviderCapabilities{
    SupportedRecordTypes: []dns.RecordType{
        dns.RecordTypeA,
        dns.RecordTypeAAAA,
        // ...
    },
    MinTTL: 60,
    MaxTTL: 86400,
}

func NewMyProvider(creds dns.ResolvedCredentials) (dns.Provider, error) {
    apiToken := creds.APIToken
    if apiToken == "" {
        return nil, fmt.Errorf("myprovider: API token required")
    }

    provider := &myprovider.Provider{
        APIToken: apiToken,
    }

    return NewLibdnsAdapter(provider, MyProviderCapabilities), nil
}

func init() {
    dns.RegisterProvider("myprovider", NewMyProvider, MyProviderCapabilities)
}
```

## See Also

- [State Management Concepts](/docs/concepts/state-management/)
- [Secrets Management](/docs/operations/security/#secrets-management)
- [CLI Reference](/docs/reference/cli/)
