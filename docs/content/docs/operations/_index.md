---
title: Operations
weight: 2
cascade:
  # Order the runbooks by operational lifecycle (cluster bring-up through
  # incident response) rather than alphabetically; hide the mounted README.
  - {_target: {path: '/docs/operations/bootstrap-new-cluster'}, title: "Bootstrap a New Cluster", weight: 11}
  - {_target: {path: '/docs/operations/upgrade-cluster'}, title: "Upgrade a Cluster", weight: 12}
  - {_target: {path: '/docs/operations/scheduled-maintenance'}, title: "Scheduled Maintenance", weight: 13}
  - {_target: {path: '/docs/operations/capacity-scaling'}, title: "Capacity Scaling", weight: 14}
  - {_target: {path: '/docs/operations/certificate-rotation'}, title: "Certificate Rotation", weight: 15}
  - {_target: {path: '/docs/operations/backup-restore'}, title: "Backup & Restore", weight: 16}
  - {_target: {path: '/docs/operations/disaster-recovery'}, title: "Disaster Recovery", weight: 17}
  - {_target: {path: '/docs/operations/emergency-rollback'}, title: "Emergency Rollback", weight: 18}
  - {_target: {path: '/docs/operations/performance-degradation'}, title: "Performance Degradation", weight: 19}
  - {_target: {path: '/docs/operations/troubleshooting'}, title: "Troubleshooting", weight: 20}
  - {_target: {path: '/docs/operations/security-incident'}, title: "Security Incident", weight: 21}
  - {_target: {path: '/docs/operations/readme'}, sidebar: {exclude: true}, weight: 99}
---

Day-2 operational runbooks: cluster bring-up and upgrades, scheduled
maintenance, capacity, certificate rotation, backup and restore,
disaster recovery, and incident response.
