---
title: "Keystone Core"
---

{{< blocks/cover title="Keystone Core" image_anchor="top" height="full" >}}
<p class="lead mt-5">Cloud-Native Runtime Infrastructure Control Plane</p>
<a class="btn btn-lg btn-primary me-3 mb-4" href="/docs/">
  Learn More <i class="fas fa-arrow-alt-circle-right ms-2"></i>
</a>
<a class="btn btn-lg btn-secondary me-3 mb-4" href="https://github.com/kscore/keystone-core">
  Download <i class="fab fa-github ms-2 "></i>
</a>
<p class="lead mt-5">GitOps deploys it. We keep it running.</p>
{{< /blocks/cover >}}

{{% blocks/lead color="primary" %}}
Keystone Core is the operational layer between GitOps/IaC deployments and runtime infrastructure.

It bridges the gap between declarative GitOps tools and the dynamic reality of production operations.
{{% /blocks/lead %}}

{{% blocks/section color="dark" type="row" %}}

{{% blocks/feature icon="fa-lightbulb" title="Declarative State Management" %}}
Define infrastructure as code with idempotent state modules. Detect and fix configuration drift automatically.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-code-branch" title="GitOps Integration" %}}
Deep integration with ArgoCD and Flux. Automated deployment verification, rollback, and promotion pipelines.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-shield-alt" title="Policy Enforcement" %}}
Continuous compliance with OPA and CEL policy engines. Audit logging and compliance reporting built-in.
{{% /blocks/feature %}}

{{% /blocks/section %}}

{{% blocks/section type="row" %}}

{{% blocks/feature icon="fab fa-app-store-ios" title="Event-Driven Automation" %}}
React to infrastructure events in real-time with powerful filtering, routing, and reactor system.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-plug" title="Extensible Plugin System" %}}
Extend Keystone Core with custom modules written in Starlark or WASM (Rust/Go/C++). Sandboxed execution with capability-based security.
{{% /blocks/feature %}}

{{% blocks/feature icon="fa-chart-line" title="Full Observability" %}}
Prometheus metrics, structured logging, and distributed tracing. Grafana dashboards and real-time TUI monitor included.
{{% /blocks/feature %}}

{{% /blocks/section %}}
