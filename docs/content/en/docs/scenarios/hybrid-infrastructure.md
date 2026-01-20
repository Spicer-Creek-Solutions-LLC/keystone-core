---
title: "Hybrid Cloud Infrastructure"
weight: 3
description: >
  Manage resources across Kubernetes, VMs, and cloud providers
---

## Overview

This scenario demonstrates managing hybrid infrastructure:
- **Kubernetes Clusters**: EKS, GKE, AKS, and on-premises
- **Virtual Machines**: AWS EC2, Azure VMs, GCP Compute, VMware
- **Bare Metal**: Physical servers in data centers
- **Edge Devices**: IoT and edge computing nodes

### Business Context

Hybrid infrastructure enables:
- Best-of-breed technology selection
- Regulatory compliance (data residency)
- Cost optimization
- Disaster recovery across providers

## Implementation

### Multi-Cloud Agent Deployment

```yaml
# blueprints/hybrid-infrastructure/blueprint.yaml
name: hybrid-infrastructure
version: "1.0.0"
description: Deploy Keystone Core across hybrid infrastructure

parameters:
  aws_regions:
    type: array
    default: [us-east-1, us-west-2]
  gcp_regions:
    type: array
    default: [us-central1]
  azure_regions:
    type: array
    default: [eastus]
  on_prem_datacenters:
    type: array
    default: [dc1, dc2]

resources:
  # AWS EC2 Instances
  aws_agents:
    for_each: "{{ .parameters.aws_regions }}"
    module: cloud/aws
    resource_type: ec2_instance
    properties:
      instance_type: t3.medium
      ami: ami-kscore-agent
      region: "{{ .each.value }}"
      tags:
        role: kscore-agent
        provider: aws
        region: "{{ .each.value }}"
      user_data: |
        #!/bin/bash
        curl -fsSL https://get.kscore.io | bash -s -- \
          --control-plane {{ .pillar.control_plane_url }} \
          --token {{ .pillar.bootstrap_token }}

  # GCP Compute Instances
  gcp_agents:
    for_each: "{{ .parameters.gcp_regions }}"
    module: cloud/gcp
    resource_type: compute_instance
    properties:
      machine_type: n2-standard-2
      image: kscore-agent
      zone: "{{ .each.value }}-a"
      labels:
        role: kscore-agent
        provider: gcp
        region: "{{ .each.value }}"

  # Azure VMs
  azure_agents:
    for_each: "{{ .parameters.azure_regions }}"
    module: cloud/azure
    resource_type: virtual_machine
    properties:
      size: Standard_B2s
      image: kscore-agent
      location: "{{ .each.value }}"
      tags:
        role: kscore-agent
        provider: azure
        region: "{{ .each.value }}"

  # On-premises VMware
  vmware_agents:
    for_each: "{{ .parameters.on_prem_datacenters }}"
    module: vmware
    resource_type: virtual_machine
    properties:
      datacenter: "{{ .each.value }}"
      template: kscore-agent-template
      folder: /infrastructure/kscore
      cpu: 2
      memory: 4096
```

### Cross-Provider State Management

```yaml
# states/hybrid/webserver.yaml
metadata:
  name: webserver
  description: Web server configuration for hybrid infrastructure

# Provider-specific implementations
provider_overrides:
  aws:
    nginx_install:
      module: package
      state: installed
      name: nginx
      provider: amazon-linux

  gcp:
    nginx_install:
      module: package
      state: installed
      name: nginx
      provider: apt

  azure:
    nginx_install:
      module: package
      state: installed
      name: nginx
      provider: apt

  vmware:
    nginx_install:
      module: package
      state: installed
      name: nginx
      provider: yum

# Common configuration
states:
  nginx_config:
    module: file
    state: present
    path: /etc/nginx/nginx.conf
    template: nginx.conf.tmpl
    vars:
      worker_processes: auto
      upstream_servers: "{{ .pillar.upstream_servers }}"

  nginx_service:
    module: service
    state: running
    name: nginx
    enable: true
    require:
      - nginx_install
      - nginx_config
```

### Unified Targeting

```bash
# Target by cloud provider
kscorectl exec "provider:aws" --cmd "uptime"
kscorectl exec "provider:gcp" --cmd "uptime"

# Target by region across providers
kscorectl exec "region:us-east*" --cmd "hostname"

# Target Kubernetes workloads
kscorectl exec "kubernetes:true and namespace:production" --cmd "kubectl get pods"

# Target on-premises only
kscorectl exec "datacenter:dc1 or datacenter:dc2" --cmd "dmidecode -s system-product-name"

# Combined targeting
kscorectl exec "(provider:aws and region:us-east-1) or datacenter:dc1" --cmd "date"
```

### Cross-Provider Load Balancing

```yaml
# states/hybrid/global-lb.yaml
global_load_balancer:
  module: dns
  state: present
  type: weighted
  name: app.example.com
  records:
    - target: alb-us-east-1.amazonaws.com
      weight: 40
      health_check: https://app-us-east-1.example.com/health
    - target: us-central1-lb.example.com
      weight: 30
      health_check: https://app-us-central1.example.com/health
    - target: dc1-vip.internal.example.com
      weight: 30
      health_check: https://app-dc1.example.com/health
```

## Verification

```bash
# Check agents by provider
kscorectl agents list --group-by provider

# Output:
# PROVIDER    CONNECTED    TOTAL    HEALTHY
# aws         45           45       45
# gcp         20           20       19
# azure       15           15       15
# vmware      30           30       30
# bare_metal  10           10       10

# Check cross-provider connectivity
kscorectl connectivity test --from "provider:aws" --to "provider:gcp"
```

## Troubleshooting

### Provider-Specific Issues

```bash
# Check AWS metadata service
kscorectl exec "provider:aws" --cmd "curl -s http://169.254.169.254/latest/meta-data/"

# Check GCP metadata
kscorectl exec "provider:gcp" --cmd "curl -s -H 'Metadata-Flavor: Google' http://metadata.google.internal/"

# Check on-prem connectivity
kscorectl exec "datacenter:dc1" --cmd "traceroute control-plane.example.com"
```
