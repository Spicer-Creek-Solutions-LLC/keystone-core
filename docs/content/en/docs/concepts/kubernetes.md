---
title: "Kubernetes Integration"
weight: 12
description: >
  Native Kubernetes support with CRDs, operators, and seamless integration for container orchestration environments.
---

## Overview

Keystone Core provides deep Kubernetes integration, enabling unified management of containerized workloads alongside traditional infrastructure. The integration includes Custom Resource Definitions (CRDs), operator controllers, and a comprehensive client wrapper for interacting with Kubernetes clusters.

## Architecture

```mermaid
flowchart TB
    subgraph CP["Keystone Core Control Plane"]
        KC["K8s Client<br>Wrapper"]
        CT["Controllers<br>(Operators)"]
        KC --> API
        CT --> API
        API["Kubernetes API Server"]
        API --> RE["RemoteExecution<br>CRD"]
        API --> SC["StateConfig<br>CRD"]
    end
```

## Key Components

### Kubernetes Client Wrapper

The client wrapper (`internal/k8s/client.go`) provides a unified interface for Kubernetes operations. Note that this is an internal API; for external integrations, use the REST or gRPC endpoints.

- **Multi-cluster support**: Connect to multiple Kubernetes clusters simultaneously
- **Context switching**: Seamlessly switch between cluster contexts
- **Resource management**: Create, get, list, update, and delete Kubernetes resources
- **Pod execution**: Execute commands directly in pods

```go
// Create a new Kubernetes client
client, err := k8s.NewClient(k8s.ClusterConfig{
    Kubeconfig: "/path/to/kubeconfig",
    Context:    "my-cluster",
})

// Execute a command in a pod
result, err := client.ExecInPod(k8s.PodExecOptions{
    Namespace: "my-namespace",
    PodName:   "my-pod",
    Container: "my-container",
    Command:   []string{"ls", "-la"},
})
```

### Custom Resource Definitions

Keystone Core defines two primary CRDs for Kubernetes-native operations:

#### RemoteExecution CRD

Enables distributed command execution across Kubernetes clusters:

```yaml
apiVersion: keystone.io/v1alpha1
kind: RemoteExecution
metadata:
  name: update-config
  namespace: default
spec:
  target:
    labelSelector:
      matchLabels:
        app: web-server
    namespaces:
      - production
  command:
    shell: /bin/bash
    script: |
      kubectl rollout restart deployment/web-app
  timeout: 300s
  retries: 3
status:
  phase: Completed
  completedAt: "2024-01-15T10:30:00Z"
  results:
    - pod: web-server-abc123
      exitCode: 0
      output: "deployment.apps/web-app restarted"
```

#### StateConfig CRD

Declarative state management for Kubernetes resources:

```yaml
apiVersion: keystone.io/v1alpha1
kind: StateConfig
metadata:
  name: nginx-config
  namespace: default
spec:
  states:
    - module: k8s_namespace
      id: web-namespace
      state: present
      parameters:
        name: web-apps
        labels:
          team: platform
    - module: k8s_deployment
      id: nginx-deployment
      state: present
      parameters:
        name: nginx
        namespace: web-apps
        replicas: 3
        image: nginx:1.25
  driftDetection:
    enabled: true
    interval: 5m
status:
  phase: Applied
  lastApplied: "2024-01-15T10:00:00Z"
  drift:
    detected: false
```

### Controllers (Operators)

The Keystone Core operators reconcile CRD states:

**RemoteExecutionController**:

- Watches RemoteExecution resources
- Dispatches commands to target pods/nodes
- Aggregates results and updates status
- Handles retries and timeouts

**StateConfigController**:

- Watches StateConfig resources
- Applies state declarations to the cluster
- Monitors for drift
- Reports state compliance

## Execution Modes

### Pod Execution

Execute commands inside running pods:

```yaml
execution:
  mode: pod
  pod:
    name: my-app-pod
    namespace: default
    container: app
  command: ["./health-check.sh"]
```

### Job Execution

Create Kubernetes Jobs for one-off tasks:

```yaml
execution:
  mode: job
  job:
    name: backup-job
    namespace: default
    image: backup-tools:latest
    command: ["backup.sh", "--full"]
    ttlSecondsAfterFinished: 3600
```

### Node Execution

Execute commands on Kubernetes nodes via DaemonSet:

```yaml
execution:
  mode: node
  node:
    selector:
      node-role.kubernetes.io/worker: ""
  command: ["sysctl", "-a"]
```

## Targeting

### Label Selectors

Target workloads by labels:

```yaml
target:
  labelSelector:
    matchLabels:
      app: my-app
      environment: production
    matchExpressions:
      - key: tier
        operator: In
        values: ["frontend", "backend"]
```

### Field Selectors

Target by resource fields:

```yaml
target:
  fieldSelector:
    status.phase: Running
    spec.nodeName: worker-01
```

### Namespace Filtering

Scope operations to specific namespaces:

```yaml
target:
  namespaces:
    - production
    - staging
  excludeNamespaces:
    - kube-system
```

## State Modules

Keystone Core includes Kubernetes-specific state modules:

### k8s_namespace

Manage Kubernetes namespaces:

```yaml
- module: k8s_namespace
  id: /namespaces/my-app
  state: present
  parameters:
    name: my-app
    labels:
      team: platform
    annotations:
      description: "Application namespace"
```

### k8s_deployment

Manage Deployments:

```yaml
- module: k8s_deployment
  id: /deployments/nginx
  state: present
  parameters:
    name: nginx
    namespace: web
    replicas: 3
    image: nginx:1.25
    ports:
      - containerPort: 80
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
```

## Configuration

### Control Plane Configuration

```yaml
kubernetes:
  # Default kubeconfig path
  kubeconfig: ~/.kube/config

  # Default context (empty = current context)
  context: ""

  # Multi-cluster configuration
  clusters:
    - name: production
      kubeconfig: /etc/keystone/kubeconfig-prod
      context: prod-cluster
    - name: staging
      kubeconfig: /etc/keystone/kubeconfig-staging
      context: staging-cluster

  # CRD installation
  installCRDs: true

  # Controller settings
  controllers:
    remoteExecution:
      enabled: true
      concurrency: 10
    stateConfig:
      enabled: true
      syncPeriod: 5m
```

### Agent Configuration (Kubernetes Mode)

When running as a DaemonSet:

```yaml
agent:
  mode: kubernetes
  kubernetes:
    # Node name (auto-detected if empty)
    nodeName: ""

    # Namespace for agent resources
    namespace: keystone-system

    # Service account
    serviceAccount: keystone-agent

    # Pod execution settings
    podExec:
      timeout: 60s
      tty: false
```

## Deployment

### Helm Chart

Deploy Keystone Core to Kubernetes using Helm:

```bash
# Add the Keystone Core Helm repository
helm repo add keystone https://charts.keystone-core.io
helm repo update

# Install the control plane
helm install keystone-server keystone/kscore-server \
  --namespace keystone-system \
  --create-namespace \
  --set kubernetes.installCRDs=true

# Install agents as DaemonSet
helm install keystone-agent keystone/kscore-agent \
  --namespace keystone-system \
  --set agent.mode=kubernetes
```

### Kustomize

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - https://github.com/shawnbutts/keystone-core/deploy/kubernetes/base

namespace: keystone-system

patches:
  - patch: |-
      - op: replace
        path: /spec/replicas
        value: 3
    target:
      kind: Deployment
      name: keystone-server
```

## Best Practices

### Resource Limits

Always set resource limits for Keystone Core components:

```yaml
resources:
  requests:
    cpu: 100m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi
```

### RBAC

Use minimal RBAC permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: keystone-agent
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/exec"]
    verbs: ["get", "list", "create"]
  - apiGroups: ["keystone.io"]
    resources: ["remoteexecutions", "stateconfigs"]
    verbs: ["get", "list", "watch", "update", "patch"]
```

### Multi-Cluster Management

When managing multiple clusters:

1. Use separate kubeconfig files per cluster
2. Configure cluster-specific contexts
3. Use cluster labels for targeting
4. Implement network policies for cross-cluster communication

### Drift Detection

Enable drift detection for critical resources:

```yaml
driftDetection:
  enabled: true
  interval: 5m
  severity:
    critical:
      - deployments
      - configmaps
    high:
      - services
    medium:
      - namespaces
```

## Troubleshooting

### CRD Not Found

If CRDs are not installed:

```bash
# Check if CRDs exist
kubectl get crd remoteexecutions.keystone.io stateconfigs.keystone.io

# Install CRDs manually
kubectl apply -f https://github.com/shawnbutts/keystone-core/deploy/kubernetes/crds/
```

### Controller Not Reconciling

Check controller logs:

```bash
kubectl logs -n keystone-system deployment/keystone-server -c controller
```

### Pod Execution Failures

Verify RBAC permissions:

```bash
kubectl auth can-i create pods/exec --as=system:serviceaccount:keystone-system:keystone-agent
```

### Multi-Cluster Connection Issues

Test cluster connectivity:

```bash
# Test with kubectl
KUBECONFIG=/etc/keystone/kubeconfig-prod kubectl cluster-info

# Check Keystone Core cluster status
kscorectl cluster status
```

## See Also

- [Agents](/docs/concepts/agents/) - Agent architecture and deployment
- [State Management](/docs/concepts/state-management/) - Declarative configuration
- [Remote Execution](/docs/concepts/remote-execution/) - Command execution system
- [Multi-Environment Support](/docs/scenarios/multi-environment/) - Cross-platform operations
