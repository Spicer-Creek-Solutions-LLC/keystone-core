---
name: Bug Report
about: Report a bug or unexpected behavior in Keystone Core
title: ''
labels: kind/bug
assignees: ''
---

<!--
Before filing: have you read SECURITY.md? Security vulnerabilities go
through the private disclosure channel — do not file them here.
-->

## Bug Description

<!-- A clear, concise description of what's broken. One paragraph. -->

## Steps to Reproduce

1.
2.
3.

## Expected Behavior

<!-- What you expected to happen -->

## Actual Behavior

<!-- What actually happened, including any error messages -->

## Environment

- **Keystone Core version**: <!-- `kscorectl --version` -->
- **OS**: <!-- e.g., Ubuntu 24.04, Debian 12, Rocky 9 -->
- **Install path**: <!-- .deb / .rpm package, built from source, container -->
- **Deployment mode**: <!-- single-node trial, multi-host, embedded NATS, external NATS -->

## Logs / Output

<!--
If applicable, attach relevant output from:
  sudo journalctl -u kscore-server -n 100 --no-pager
  sudo journalctl -u kscore-agent -n 100 --no-pager
Redact secrets (HMAC, API keys, etc.) before pasting.
-->

```text
<paste logs here>
```

## Additional Context

<!-- Any other context, screenshots, or related issues -->
