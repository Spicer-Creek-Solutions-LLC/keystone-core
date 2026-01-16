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

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `backends` | array | [local] | Storage backends |
| `s3_bucket` | string |  | S3 bucket name |
| `s3_region` | string |  | S3 region |
| `gcs_bucket` | string |  | GCS bucket name |
| `azure_container` | string |  | Azure container name |
| `mirror_groups` | array | [] | Mirror group configurations |
