---
title: "Microservices Platform"
weight: 15
description: >
  Deploy and manage a microservices-based architecture with service mesh integration
---

This scenario demonstrates deploying a complete microservices platform with Keystone Core, including service discovery, load balancing, and observability.

## Overview

Modern microservices architectures require sophisticated infrastructure management. This scenario covers:

- **Service Mesh Integration**: Istio/Linkerd deployment and configuration
- **Service Discovery**: Automatic service registration and DNS
- **Traffic Management**: Load balancing, circuit breakers, and retries
- **Observability**: Distributed tracing, metrics, and logging

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                   Service Mesh                       │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐          │    │
│  │  │ Frontend │  │   API    │  │ Backend  │          │    │
│  │  │ Service  │──│ Gateway  │──│ Services │          │    │
│  │  └──────────┘  └──────────┘  └──────────┘          │    │
│  │       │              │              │               │    │
│  │  ┌──────────────────────────────────────┐          │    │
│  │  │         Sidecar Proxies (Envoy)      │          │    │
│  │  └──────────────────────────────────────┘          │    │
│  └─────────────────────────────────────────────────────┘    │
│                           │                                  │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Observability Stack                     │    │
│  │   Prometheus │ Jaeger │ Grafana │ Loki              │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

- Kubernetes cluster (1.25+)
- Keystone Core control plane
- Agents deployed on cluster nodes
- Service mesh (Istio or Linkerd) installed

## Implementation

### 1. Namespace Setup

```yaml
# microservices-namespaces.yaml
apiVersion: v1
kind: state
metadata:
  name: microservices-namespaces

resources:
  - type: kubernetes.namespace
    name: microservices
    properties:
      labels:
        istio-injection: enabled
        environment: "{{ pillar.environment }}"

  - type: kubernetes.namespace
    name: observability
    properties:
      labels:
        environment: "{{ pillar.environment }}"
```

### 2. Service Deployment

```yaml
# api-gateway.yaml
apiVersion: v1
kind: state
metadata:
  name: api-gateway

parameters:
  replicas:
    type: integer
    default: 3
  image_tag:
    type: string
    default: "latest"

resources:
  - type: kubernetes.deployment
    name: api-gateway
    namespace: microservices
    properties:
      replicas: "{{ parameters.replicas }}"
      selector:
        matchLabels:
          app: api-gateway
      template:
        spec:
          containers:
            - name: gateway
              image: "myregistry/api-gateway:{{ parameters.image_tag }}"
              ports:
                - containerPort: 8080
              resources:
                requests:
                  cpu: 100m
                  memory: 128Mi
                limits:
                  cpu: 500m
                  memory: 512Mi
              livenessProbe:
                httpGet:
                  path: /health
                  port: 8080
              readinessProbe:
                httpGet:
                  path: /ready
                  port: 8080

  - type: kubernetes.service
    name: api-gateway
    namespace: microservices
    properties:
      selector:
        app: api-gateway
      ports:
        - port: 80
          targetPort: 8080
```

### 3. Traffic Management

```yaml
# traffic-policies.yaml
apiVersion: v1
kind: state
metadata:
  name: traffic-policies

resources:
  - type: kubernetes.manifest
    name: virtual-service
    properties:
      apiVersion: networking.istio.io/v1beta1
      kind: VirtualService
      metadata:
        name: api-gateway
        namespace: microservices
      spec:
        hosts:
          - api-gateway
        http:
          - match:
              - headers:
                  x-canary:
                    exact: "true"
            route:
              - destination:
                  host: api-gateway-canary
                  port:
                    number: 80
          - route:
              - destination:
                  host: api-gateway
                  port:
                    number: 80
                weight: 100

  - type: kubernetes.manifest
    name: destination-rule
    properties:
      apiVersion: networking.istio.io/v1beta1
      kind: DestinationRule
      metadata:
        name: api-gateway
        namespace: microservices
      spec:
        host: api-gateway
        trafficPolicy:
          connectionPool:
            tcp:
              maxConnections: 100
            http:
              h2UpgradePolicy: UPGRADE
              http1MaxPendingRequests: 100
              http2MaxRequests: 1000
          outlierDetection:
            consecutive5xxErrors: 5
            interval: 30s
            baseEjectionTime: 30s
```

## Verification

```bash
# Check service mesh status
kscorectl exec run "role:k8s-master" -- istioctl analyze

# Verify services are registered
kscorectl exec run "role:k8s-master" -- kubectl get pods -n microservices

# Test service connectivity
kscorectl exec run "role:k8s-master" -- \
  kubectl exec -n microservices deploy/api-gateway -- \
  curl -s http://backend-service/health
```

## Troubleshooting

### Sidecar Not Injecting

Ensure namespace has the injection label:

```bash
kubectl label namespace microservices istio-injection=enabled
```

### Circuit Breaker Triggering

Check outlier detection settings and service health:

```bash
istioctl proxy-config cluster deploy/api-gateway -n microservices
```

## Next Steps

- [Event-Driven Automation]({{< relref "event-driven-automation" >}}) - Add reactive automation
- [Compliance Automation]({{< relref "compliance-automation" >}}) - Implement security policies
