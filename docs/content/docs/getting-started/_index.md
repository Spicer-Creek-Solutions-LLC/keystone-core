---
title: Getting Started
weight: 1
---

{{< clip name="docs-fleet" caption="`kscorectl agent list` — every registered agent and its last heartbeat." >}}

{{< clip name="docs-remote-exec" caption="Resolve a target selector first, then dispatch across the fleet." >}}

Hands-on, runnable tutorials for the day-2 tasks an operator or author
takes on after the [30-minute quick start](/docs/reference/getting-started/)
(install → agent online → first command → first state apply → audit).

Each guide is self-contained and runnable on a fresh Ubuntu host with
Keystone Core installed.

{{< cards >}}
  {{< card link="blueprint-authoring/" title="Authoring a Blueprint" subtitle="Package a parameterized, rollback-able deployment from state files." >}}
  {{< card link="module-authoring/" title="Authoring & Publishing a Module" subtitle="Write a Starlark module, test it, sign it, and publish it to a registry." >}}
  {{< card link="secrets/" title="Managing Secrets" subtitle="Enable the encrypted-file backend and put / get / list / delete secrets." >}}
  {{< card link="audit-policy/" title="Audit Log & Policies" subtitle="Query the audit log and write, validate, and evaluate a Rego policy." >}}
  {{< card link="gitops/" title="GitOps Integration" subtitle="Receive deployment webhooks, run verification workflows, and roll back." >}}
  {{< card link="ha-cluster/" title="HA Cluster Topology" subtitle="Bring up a clustered control plane and operate it with kscore-cluster." >}}
{{< /cards >}}
