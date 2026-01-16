# gitops-integration Blueprint

Configure GitOps integrations for Keystone Core.

## Quick Start

```yaml
include:
  - blueprint: blueprints/kscore/gitops-integration@0.1.0
    params:
      argocd_enabled: true
      flux_enabled: true
      git_repos:
        - https://github.com/example/keystone-states.git
```

## Parameters

See `blueprint.yaml` for parameter definitions.
