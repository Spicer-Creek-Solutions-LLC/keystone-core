---
title: Managing Secrets
weight: 4
---

{{< clip name="docs-secrets" caption="Write a secret and read it back — values are masked unless you ask for cleartext." >}}

Keystone Core's secrets subsystem stores secrets encrypted at rest and
serves them through the control plane. The simplest backend —
`encrypted_file` — keeps everything in a single AES-256-GCM file unlocked
by a master key you supply; a Vault backend is also available. This guide
enables the file backend and walks the `put` / `get` / `list` / `delete`
lifecycle with the `kscore-secrets` CLI.

## Prerequisites

- A running `kscore-server` you can reach (see the
  [quick start](/docs/reference/getting-started/)).
- `openssl` (to generate the master key).

## 1. Generate a master key

The `encrypted_file` backend is unlocked by a 32-byte master key. Generate
one and export it (the config reads it from an env var so the key never
lands in a file):

```sh
export KSCORE_SECRETS_MASTER=$(openssl rand -hex 32)
```

The master-key source is scheme-prefixed — `env:VAR`, `file:/path`, or
`inline:<hex|base64>` — the same resolver the identity CA encryption uses.
Keep this key safe: losing it means losing every secret in the file.

## 2. Enable the secrets backend

Add a `secrets` block to the server config and restart `kscore-server`:

```yaml
secrets:
  enabled: true
  default_backend: file
  backends:
    - name: file
      type: encrypted_file
      file:
        path: /var/lib/keystone/secrets.bin
        master_key: env:KSCORE_SECRETS_MASTER
```

Secrets are **opt-in**: without `secrets.enabled: true`, the rest of the
server runs unchanged and the SecretsService reports `Unavailable`.

## 3. Write a secret

```sh
kscore-secrets put kv/db/postgres \
  --data username=app \
  --data password=s3cr3t \
  --label env=prod
```

- The path (`kv/db/postgres`) is the secret's identity.
- `--data key=value` sets the secret payload (repeatable).
- `--label key=value` attaches non-secret metadata (repeatable).
- `--ttl 24h` optionally expires the secret.

The CLI talks to the server at `--server host:port` (default
`localhost:5397`) and authenticates with `--api-key` or the
`KSCORE_API_KEY` environment variable.

## 4. Read it back

```sh
kscore-secrets get kv/db/postgres
```

## 5. List + delete

List the metadata (never the values) under a prefix, then remove a
secret:

```sh
kscore-secrets list --prefix kv/db/
kscore-secrets delete kv/db/postgres
```

## Beyond static secrets

- **`kscore-secrets transit`** — encryption-as-a-service (encrypt/decrypt
  data without the key ever leaving the server), modeled on Vault's
  transit engine.
- **`kscore-secrets leases`** — list and manage the leases behind
  dynamic secrets.
- **Vault backend + routing** — route secret prefixes to different
  backends (`routing:` in the config) so, e.g., `secret/` resolves
  through Vault while `kv/` stays in the local file.

See the [configuration reference](/docs/reference/configuration-reference/)
for the full `secrets` schema.

## Next steps

- [Querying the Audit Log & Policies](../audit-policy/) — every secret
  read is audited.
