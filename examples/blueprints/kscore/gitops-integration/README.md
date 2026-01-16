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

| Parameter | Type | Default | Description |
| --- | --- | --- | --- |
| `argocd_enabled` | boolean | true | Enable ArgoCD integration |
| `flux_enabled` | boolean | true | Enable Flux integration |
| `verification_enabled` | boolean | true | Enable deployment verification |
| `rollback_enabled` | boolean | true | Enable automatic rollback |
| `git_repos` | array | [] | Git repositories to sync |
