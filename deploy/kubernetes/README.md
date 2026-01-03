# Keystone Core Kubernetes Manifests

Raw Kubernetes manifests for deploying Keystone Core without Helm.

## Structure

```
deploy/kubernetes/
├── kustomization.yaml      # Deploy everything
├── kscore-server/          # Control plane
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── serviceaccount.yaml
│   ├── pvc.yaml
│   ├── deployment.yaml
│   └── service.yaml
└── kscore-agent/           # Agent DaemonSet
    ├── kustomization.yaml
    ├── configmap.yaml
    ├── serviceaccount.yaml
    ├── daemonset.yaml
    └── service.yaml
```

## Quick Start

### Deploy Everything

```bash
# Deploy server and agents
kubectl apply -k deploy/kubernetes/

# Check status
kubectl -n kscore-system get pods
```

### Deploy Server Only

```bash
kubectl apply -k deploy/kubernetes/kscore-server/
```

### Deploy Agents Only

```bash
# Agents require the server to be running first
kubectl apply -k deploy/kubernetes/kscore-agent/
```

## Customization with Kustomize

### Change Image Tag

Create an overlay:

```bash
mkdir -p deploy/kubernetes/overlays/production
```

```yaml
# deploy/kubernetes/overlays/production/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../

images:
  - name: ghcr.io/keystone-core/kscore-server
    newTag: v0.10.0
  - name: ghcr.io/keystone-core/kscore-agent
    newTag: v0.10.0
```

### Change Namespace

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../

namespace: my-namespace
```

### Add Resource Limits

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../

patches:
  - patch: |-
      - op: replace
        path: /spec/template/spec/containers/0/resources/limits/memory
        value: 2Gi
    target:
      kind: Deployment
      name: kscore-server
```

## Configuration

### Server Configuration

Edit `kscore-server/configmap.yaml` to customize:

- NATS mode (embedded/external)
- Storage backend (SQLite/PostgreSQL)
- Authentication settings
- Logging and telemetry

### Agent Configuration

Edit `kscore-agent/configmap.yaml` to customize:

- Server address
- Agent labels
- Heartbeat interval
- Execution settings

## Accessing the Server

### Port Forward

```bash
# gRPC API
kubectl -n kscore-system port-forward svc/kscore-server 9090:9090

# HTTP API
kubectl -n kscore-system port-forward svc/kscore-server 8080:8080
```

### Create Ingress

Add an Ingress resource for external access:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: kscore-server
  namespace: kscore-system
spec:
  rules:
    - host: kscore.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: kscore-server
                port:
                  name: http
```

## Monitoring

### Prometheus

The manifests include Prometheus annotations. Add a ServiceMonitor for
Prometheus Operator:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kscore-server
  namespace: kscore-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: kscore-server
  endpoints:
    - port: http
      path: /metrics
```

## Uninstall

```bash
kubectl delete -k deploy/kubernetes/
```

## For Production

For production deployments, consider using the Helm charts instead:

```bash
helm install kscore ./deploy/helm/kscore-server -f values-ha.yaml
```

The Helm charts provide:
- High availability with StatefulSet
- External NATS/PostgreSQL/etcd dependencies
- PrometheusRule alerts
- Grafana dashboards
- PodDisruptionBudget
- HorizontalPodAutoscaler
