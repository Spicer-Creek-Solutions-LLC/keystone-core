# security-baseline

CIS-aligned host hardening: sysctl tuning, a default-deny firewall
with an SSH allow rule, and an SSH `PermitRootLogin no` drop-in.

## Parameters

| Name | Type | Default | Description |
|---|---|---|---|
| `ssh_port` | integer | `22` | Port opened in the firewall allow rule. |

## Features

| Feature | Default | Effect |
|---|---|---|
| `disable_root_ssh` | on | Manage the no-root SSH drop-in. |
| `strict_firewall` | on | Apply the default-deny firewall policy. |

Disabling a feature drops its declarations from the applied set.

## Apply

```text
kscorectl blueprint apply security-baseline
kscorectl blueprint apply security-baseline --param ssh_port=2222
```
