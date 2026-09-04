---
title: GitOps Integration
weight: 6
---

{{< clip name="docs-gitops-rollback" caption="A rollback created with `--require-approval` waits at `pending`; the decision and its transition are recorded." >}}

{{< clip name="docs-outbound-webhooks" caption="Subscribe to an event glob, then every delivery attempt is recorded with its HTTP status." >}}

Keystone Core integrates with a GitOps workflow at the **operational**
seam: it receives deployment webhooks from your VCS / CD system, runs
**verification workflows** to confirm a deployment is healthy, and can
trigger a **rollback** when it isn't.

{{< callout type="info" >}}
**v0.1 scope.** GitOps in v0.1 is the inbound webhook receiver plus the
`verify` and `rollback` commands — *not* a full "reconcile the cluster
from a Git repository" controller. Treat this as the hooks that let an
external GitOps pipeline (Argo CD, Flux, a CI job) drive Keystone Core,
rather than Keystone Core polling Git itself. Outbound webhooks and
repo-sync are on the roadmap.
{{< /callout >}}

This guide enables the webhook receiver, sends it an event, and runs a
verification workflow.

## Prerequisites

- A running `kscore-server`.
- `curl`.

## 1. Enable the webhook receiver

The receiver opens its own port and re-emits validated webhooks onto the
event bus. It's opt-in — add a `gitops` block and restart:

```yaml
gitops:
  webhook:
    enabled: true
    addr: ":8081"
    path: "/webhooks"
    sources:
      github:
        method: hmac   # none | hmac | bearer
        secret: ${KSCORE_GITOPS_WEBHOOK_SOURCES_GITHUB_SECRET}
```

`method` selects how the source is authenticated: `hmac` (GitHub's
`X-Hub-Signature-256`), `bearer` (a token), or `none` (for local
testing only). GitHub, GitLab, and Argo CD source shapes are supported.

## 2. Send an event

Point your VCS's webhook at `http://<host>:8081/webhooks`, or simulate
one locally. GitHub deployment events carry an `X-GitHub-Event` header:

```sh
curl -X POST http://localhost:8081/webhooks \
  -H "X-GitHub-Event: deployment_status" \
  -H "Content-Type: application/json" \
  -d '{"deployment_status":{"state":"success"},"repository":{"full_name":"acme/app"}}'
```

The receiver validates the source, normalizes the payload, and re-emits
it onto the event bus. Watch it arrive:

```sh
kscore-events watch
```

## 3. Run a verification workflow

A verification workflow is a YAML file of checks — most commonly HTTP
probes against the thing you just deployed. Save `post-deploy.yaml`:

```yaml
name: post-deploy-checks
on_failure: abort   # abort | continue
steps:
  - name: server-health
    type: http
    timeout: 5s
    retries: 2
    config:
      url: http://localhost:8080/health
      expect_status: 200
  - name: api-ready
    type: http
    config:
      url: http://localhost:8080/readyz
      expect_status: 200
      expect_body_contains: ready
```

Run it:

```sh
kscore-gitops verify post-deploy.yaml
```

```text
workflow=post-deploy-checks verdict=PASS duration=12ms steps=2
  [PASS] server-health (http) ...
  [PASS] api-ready (http) ...
```

`on_failure: abort` stops (and marks the rest skipped) on the first
failure; `continue` runs every step. `--parallel` and `--timeout`
override the file. A non-zero exit means the deployment failed
verification — wire that into your pipeline to gate a promotion.

## 4. Roll back

When verification fails, trigger a rollback of the last applied change:

```sh
kscore-gitops rollback
```

## Next steps

- [Querying the Audit Log & Policies](../audit-policy/) — webhook
  receipts and rollbacks are audited.
- [HA Cluster Topology](../ha-cluster/) — run the control plane itself
  clustered.
