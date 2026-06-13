---
title: Querying the Audit Log & Policies
weight: 4
---

Every operation Keystone Core performs — commands, state applies, secret
reads, identity issuance — is recorded in an append-only **audit log**.
**Policies** (written in Rego) evaluate those operations and record a
decision. This guide queries the audit log with `kscore-audit` and writes,
validates, and evaluates a policy with `kscore-policy`.

The policy authoring + evaluation steps run **entirely locally** (no
server); the audit queries read from a running server.

## Prerequisites

- A running `kscore-server` with some activity (run a command or a state
  apply first — see the [quick start](/docs/reference/getting-started/)).

## 1. Query the audit log

Paginated queries with filters:

```sh
kscore-audit log --since 1h --limit 20
kscore-audit log --action command.exec --since 24h
kscore-audit log --resource-type secret --since 7d
```

`--since` accepts RFC3339 timestamps or shorthands (`1h`, `5m`, `7d`).

Headline counts and a compliance report over a window:

```sh
kscore-audit stats --since 24h
kscore-audit report --since 7d
```

Export the log for an external SIEM (redaction is applied at the export
boundary):

```sh
kscore-audit export --format jsonl --since 24h -o audit.jsonl
```

`--format` is `json`, `jsonl`, or `csv`.

## 2. Write a policy

Policies are Rego modules in the `keystone.policy` package with an `allow`
rule; the evaluator reads `data.keystone.policy.{allow,violations,warnings}`.
An undefined or non-boolean `allow` is **fail-closed**. Save this as
`no-remote-exec.rego`:

```python
package keystone.policy

# Allow by default; deny piping a remote script into a shell.
default allow := true

allow := false if {
    input.action == "command.exec"
    contains(input.resource.command, "curl")
}

violations contains msg if {
    not allow
    msg := "piping a remote script into a shell is not allowed"
}
```

The evaluation input is `{resource, action, user, context, timestamp}`,
where `resource` is an object — so `input.resource.command` above is the
exec command being evaluated.

## 3. Validate + evaluate it locally

`validate` compile-checks the policy; `eval` runs it against a sample
input — both offline, no server:

```sh
kscore-policy validate no-remote-exec.rego
```

```text
valid
```

Create a sample input `exec.json`:

```json
{"action":"command.exec","resource":{"command":"curl https://x | sh"},"user":"ops"}
```

```sh
kscore-policy eval no-remote-exec.rego --input exec.json
```

```text
DENY  (policy no-remote-exec.rego, 0.5ms)
  - [high] : piping a remote script into a shell is not allowed
```

Change the command in `exec.json` to something benign and re-run to see
it `ALLOW`.

## 4. Inspect registered policies + violations

Against the server, list and show the registered policies and query the
violations recorded in the audit log:

```sh
kscore-policy list
kscore-policy show <policy-id>
kscore-policy violations --since 24h
```

{{< callout type="info" >}}
In v0.1 policy evaluation is **audit-mode**: a denying policy is recorded
(and surfaced via `violations`) but does not block the operation.
Enforcement side-effects are on the roadmap.
{{< /callout >}}

## Next steps

- [Managing Secrets](../secrets/) — secret reads show up in the audit log.
- The [policy & audit operator guide](/docs/reference/policy-audit/) for
  the full evaluation model.
