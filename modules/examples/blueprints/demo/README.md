# demo

Single-node demo deployment: install a package, run it as a service,
and write a managed marker file. The simplest end-to-end blueprint —
use it to learn the apply/rollback flow.

## Parameters

| Name | Type | Default | Description |
|---|---|---|---|
| `app_name` | string | `keystone-demo` | Rendered into the marker file content. |
| `package_name` | string | `nginx` | Package installed and run as a service. |

## Features

| Feature | Default | Effect |
|---|---|---|
| `marker_file` | on | Manage `/etc/keystone-demo.deployed`. Disable to skip it. |

## Apply

```text
kscorectl blueprint apply demo --target id:agent-1
kscorectl blueprint apply demo --param package_name=caddy
```

`rollback.yaml` removes the marker file and stops the service.
