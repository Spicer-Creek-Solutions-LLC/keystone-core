# Keystone Core package repositories

Tooling and install-side templates for the hosted APT and DNF/YUM
repositories at **`https://repos.keystone-core.io`** (self-hosted,
published by `scp`).

This directory holds the operator-facing templates
([`kscore.list`](kscore.list), [`kscore.repo`](kscore.repo)). The
repository *build* tooling lives under [`scripts/repo/`](../../scripts/repo/)
and is driven by the `make repo-*` targets.

> **Status (v0.x).** v0.1.0 shipped `.deb` / `.rpm` files as direct
> downloads on the Codeberg release page. The hosted repositories are
> the convenience layer on top of those same artifacts. Tracked in
> `docs/project/ROADMAP.md` → "Native package repositories — APT,
> DNF/YUM".

## What the build produces

`make repo-build` lays out a host-agnostic static tree under
`dist/repos/` that any web server can serve:

```
dist/repos/
  apt/
    dists/stable/{Release,Release.gpg,InRelease,main/binary-{amd64,arm64}/Packages[.gz]}
    pool/main/<all .deb files>
  rpm/stable/
    x86_64/{<.rpm files>,repodata/{repomd.xml,repomd.xml.asc,...}}
    aarch64/{<.rpm files>,repodata/{repomd.xml,repomd.xml.asc,...}}
  keystone-core-archive-keyring.asc      # the signing public key
```

The web root maps directly to the URLs in the templates:
`apt/` → `https://repos.keystone-core.io/apt`,
`rpm/stable/$basearch` → `https://repos.keystone-core.io/rpm/stable/x86_64`.

## Signing

Both repositories are signed with the **same GPG key as the release
ceremony** (shared key-onboarding — see
[`RELEASE-PLAYBOOK.md`](../../RELEASE-PLAYBOOK.md) §2). The signature
covers the repository metadata, which in turn carries the checksum of
every package:

- **APT** — a signed `InRelease` (and detached `Release.gpg`) covers the
  per-arch `Packages` indices, which carry each `.deb`'s SHA256. APT has
  no per-package GPG signatures; the `Release` signature secures the
  whole repo.
- **DNF** — a detached `repomd.xml.asc` (verified by `repo_gpgcheck=1`)
  covers `primary.xml.gz`, which carries each `.rpm`'s checksum.
  Per-package RPM header signing (`gpgcheck=1`) is a follow-up that
  lands with the release-signing ceremony.

### Build modes

```sh
make release-snapshot              # produce dist/*.deb + dist/*.rpm first

make repo-build REPO_SIGN=key:ABCD1234   # sign with a real gpg key (the publish flow)
make repo-build REPO_SIGN=test           # ephemeral throwaway key (local validation; default)
make repo-build REPO_SIGN=skip           # unsigned (dev only)
```

`REPO_SIGN=test` writes a `TESTKEY-DO-NOT-PUBLISH` marker into the tree.
The publish step (Phase 2) refuses to push a tree carrying that marker.

### Running from macOS (or any host without the Debian/rpm tools)

The repository index tools (`apt-ftparchive`, `dpkg-scanpackages`,
`createrepo_c`) are Linux-only. When they are not on the host — the
release-from-a-Mac case — `repo-build` generates the metadata inside
`debian:12-slim` / `rockylinux:9` via **docker or podman** automatically;
only `gpg` and a container engine are needed on the host. **Signing
always runs on the host, so the GPG key never enters a container.**

- The container engine is resolved as `CONTAINER_ENGINE` (if set) → else
  `docker` → else `podman`.
- `make repo-smoke` mounts the tree and installs over `file://` (no host
  HTTP server), so it works the same on macOS, where the engine runs a
  Linux VM and a host-side server is not reachable from a container.
- Set `KSCORE_REPO_CONTAINER=1` to force the container path on a Linux
  host (used to validate the macOS code path).

## Verify locally

```sh
make repo-smoke      # serves dist/repos/ and installs kscore-cli in
                     # debian:12-slim (apt) + rockylinux:9 (dnf)
```

With a test- or real-key-signed tree this exercises signature
verification end-to-end (`signed-by` for apt, `repo_gpgcheck=1` for dnf).

## Publish (Phase 2 — not yet automated)

The production repositories are self-hosted and published by `scp`-ing
the built tree to the web root on the server behind
`repos.keystone-core.io`. The scp publish script, the live signing key,
and flipping [`GETTING-STARTED.md`](../../docs/project/GETTING-STARTED.md)
to lead with the repo-install path are tracked as the Phase-2 follow-up
in the ROADMAP entry. Until then, build with a real key and copy
`dist/repos/` to the web root manually; the `keystone-core-archive-keyring.asc`
at the web root is what the install snippets below import.

## Operator install

### Debian / Ubuntu

```sh
sudo install -d /etc/apt/keyrings
curl -fsSL https://repos.keystone-core.io/keystone-core-archive-keyring.asc \
  | sudo gpg --dearmor -o /etc/apt/keyrings/keystone-core.gpg
sudo curl -fsSL https://repos.keystone-core.io/kscore.list \
  -o /etc/apt/sources.list.d/keystone-core.list
sudo apt-get update
sudo apt-get install kscore-cli kscore-agent kscore-server
```

### Rocky / RHEL / Fedora

```sh
sudo curl -fsSL https://repos.keystone-core.io/kscore.repo \
  -o /etc/yum.repos.d/keystone-core.repo
sudo dnf install kscore-cli kscore-agent kscore-server
```
