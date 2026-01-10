# Configuration Documentation Validation Report

## Overview

This report compares the configuration reference documentation (`docs/content/en/docs/reference/configuration.md`) with the actual implementation in `pkg/config/config.go`.

## Key Discrepancies Found

### 1. Logging File Output

**Documentation (line 82-83):**
```yaml
logging:
  output: "stdout"                  # stdout, file
  file: "/var/log/kscore/server.log"
```

**Implementation (`pkg/config/config.go` line 52-54):**
```go
// Output: stdout (default), syslog
// Note: file output is intentionally not supported - use journald or container log drivers
Output string
```

**Issue:** Documentation mentions `file` as a valid output option, but the implementation explicitly states file output is not supported. Documentation should only list `stdout` and `syslog`.

**Recommendation:** Update documentation to:
```yaml
logging:
  output: "stdout"                  # stdout, syslog (file output not supported - use journald/container log drivers)
```

### 2. Struct Name Mapping

The documentation uses different field names than the Go struct:

| Documentation | Implementation | Notes |
|---------------|----------------|-------|
| `api:` | `Server:` (ServerConfig) | Different naming convention |
| `nats.listen:` | `NATS.Embedded.Host` + `NATS.Embedded.Port` | Split into host/port |
| `storage.type:` | `Storage.Backend` | Different field name |
| `storage.sqlite.max_connections:` | Not present | SQLite uses `BusyTimeout`, not max connections |

### 3. Missing Documentation

Fields in code that are not documented:
- `LoggingConfig.Syslog` - Syslog configuration options
- `AuthConfig` - Full authentication configuration
- `AgentConfig.AddressFamily` - IPv6 address family preference
- `AgentConfig.AdvertiseAddrs` - Address advertisement

### 4. Documented but Not Implemented

Options documented but may not be implemented:
- `logging.max_size`, `logging.max_backups`, `logging.max_age`, `logging.compress` - Log rotation (file output not supported)
- `api.cors` - CORS configuration
- `api.rate_limit` - Rate limiting configuration

## Validation Status

| Section | Status | Notes |
|---------|--------|-------|
| NATS Configuration | ⚠️ Partial | Mode values correct, but listen format differs |
| Storage Configuration | ⚠️ Partial | SQLite path correct, PostgreSQL needs review |
| Logging Configuration | ❌ Incorrect | File output documented but not supported |
| Agent Configuration | ✅ Correct | Heartbeat and timeout values match |
| TLS Configuration | ✅ Correct | Fields match implementation |

## Recommendations

1. **Update logging section** to remove file output option
2. **Add syslog configuration** documentation
3. **Add authentication section** documentation
4. **Review and update** field mappings between documentation and implementation
5. **Add IPv6 configuration** documentation to match Epic 18 implementation

## Generated

Date: 2026-01-10
Tool: test/doc-examples/extract_examples.go (T5.2)
