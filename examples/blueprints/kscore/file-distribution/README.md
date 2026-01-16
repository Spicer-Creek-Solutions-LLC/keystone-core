# file-distribution Blueprint

Configure file distribution services for Keystone Core.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/file-distribution@0.1.0
    params:
      backends:
        - local
      s3_bucket: keystone-artifacts
```

## Parameters

See `blueprint.yaml` for parameter definitions.
