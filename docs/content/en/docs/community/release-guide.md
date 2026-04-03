---
title: "Release Guide"
weight: 4
description: >
  Step-by-step guide for creating, signing, and publishing Keystone Core releases
---

{{% alert title="Authoritative Process" color="warning" %}}
The authoritative release process is defined in
[RELEASE-PLAYBOOK.md](https://github.com/shawnbutts/keystone-core/blob/main/RELEASE-PLAYBOOK.md).
All official releases **must** follow the offline multi-party signing ceremony
described there. The guide below provides supplementary tooling reference but
does not replace the playbook. Where this guide conflicts with the playbook,
the playbook takes precedence.
{{% /alert %}}

This guide provides tooling reference for creating a Keystone Core release. The
formal process — including quorum rules, air-gapped builds, multi-party
signing, and publication voting — is defined in `RELEASE-PLAYBOOK.md`.

## Prerequisites

### Required Tools

| Tool | Version | Installation |
|------|---------|--------------|
| Go | 1.25+ | `brew install go` or <https://go.dev/dl/> |
| goreleaser | 2.x | `go install github.com/goreleaser/goreleaser/v2@latest` |
| cosign | 2.x | `go install github.com/sigstore/cosign/v2/cmd/cosign@latest` |
| gpg | 2.x | `brew install gnupg` or `apt install gnupg` |
| gh | 2.x | `brew install gh` or <https://cli.github.com/> |
| Docker | 20.10+ | <https://docs.docker.com/get-docker/> |

### Required Access

- Write access to the GitHub repository
- `GITHUB_TOKEN` with `repo` and `write:packages` scopes
- GPG signing key (for package signing)
- Access to package repository hosting (if publishing to external repos)

### Environment Variables

```bash
# Required for goreleaser
export GITHUB_TOKEN="ghp_your_token_here"

# Optional: GPG key ID for signing
export GPG_KEY_ID="your_key_id"

# Optional: Cosign private key (if not using keyless)
export COSIGN_KEY="/path/to/cosign.key"
```

---

## Phase 1: Pre-Release Preparation

### 1.1 Verify Branch State

```bash
# Ensure you're on main branch and up to date
git checkout main
git pull origin main

# Check for uncommitted changes
git status

# Verify all tests pass
make test

# Run security checks
make security
```

### 1.2 Update Version References

Ensure all version references are correct:

```bash
# Search for version strings that might need updating
grep -r "v0\.[0-9]\+\.[0-9]\+" docs/ --include="*.md" | head -20

# Check blueprint versions
grep -r "version:" blueprints/*/blueprint.yaml
```

### 1.3 Update CHANGELOG

Edit `CHANGELOG.md` to document changes for this release:

```bash
# Review commits since last release
git log $(git describe --tags --abbrev=0)..HEAD --oneline

# Edit CHANGELOG.md
vim CHANGELOG.md
```

CHANGELOG format:

```markdown
## [0.1.0] - 2026-01-28

### Added
- Feature descriptions

### Changed
- Modification descriptions

### Fixed
- Bug fix descriptions

### Security
- Security-related changes
```

### 1.4 Verify Documentation

```bash
# Build and validate documentation
make docs-validate

# Check for broken links
make docs-validate-links

# Ensure examples work
make docs-validate-examples
```

### 1.5 Create Release Branch (Optional)

For major releases, consider a release branch:

```bash
# Create release branch
git checkout -b release/v0.1.0

# Push branch
git push origin release/v0.1.0
```

---

## Phase 2: Build Release Artifacts

### 2.1 Validate GoReleaser Configuration

```bash
# Check goreleaser config syntax
goreleaser check

# Dry run without publishing
make release-dry-run
```

### 2.2 Create Git Tag

```bash
# Create annotated tag
git tag -a v0.1.0 -m "Release v0.1.0"

# Verify tag
git show v0.1.0

# Push tag to remote
git push origin v0.1.0
```

### 2.3 Build with GoReleaser

```bash
# Build snapshot for testing (no publish)
make release-snapshot

# Verify artifacts in dist/
ls -la dist/

# Check package contents
dpkg-deb -c dist/kscore-server_0.1.0_linux_amd64.deb
rpm -qlp dist/kscore-server-0.1.0.x86_64.rpm
```

### 2.4 Test Artifacts Locally

```bash
# Test binary
./dist/kscore-server_linux_amd64/kscore-server version

# Test DEB package installation (in container)
docker run --rm -v $(pwd)/dist:/dist ubuntu:24.04 \
  dpkg -i /dist/kscore-server_0.1.0_linux_amd64.deb

# Test RPM package installation (in container)
docker run --rm -v $(pwd)/dist:/dist rockylinux:9 \
  rpm -i /dist/kscore-server-0.1.0.x86_64.rpm
```

---

## Phase 3: Sign Artifacts

### 3.1 Generate Checksums

GoReleaser automatically generates `checksums.txt`. Verify it:

```bash
# View checksums
cat dist/checksums.txt

# Verify a checksum
sha256sum -c dist/checksums.txt 2>/dev/null | grep kscore-server
```

### 3.2 Sign Checksums with GPG

{{% alert title="Multi-Party Signing Required" color="warning" %}}
Official releases require multi-party signing on an air-gapped machine per
`RELEASE-PLAYBOOK.md` Phase 6. The commands below are for reference only.
Do not sign official releases with a single signer or on a networked machine.
{{% /alert %}}

```bash
# Each authorized signer signs the checksums file independently
gpg --armor --detach-sign --output checksums.txt.sig.<SIGNER_ID> dist/checksums.txt

# Each participant verifies every other signer's signature
gpg --verify checksums.txt.sig.<SIGNER_ID> dist/checksums.txt
```

### 3.3 Sign Container Images with Cosign (Key-Based)

{{% alert title="No Keyless Signing" color="warning" %}}
Official releases use key-based Cosign signing, not keyless/OIDC mode.
See `RELEASE-PLAYBOOK.md` Phase 8 for the container signing ceremony.
{{% /alert %}}

```bash
# Sign with project-controlled key (not keyless)
cosign sign --key <KEY_REF> ghcr.io/kscore/kscore-server:<VERSION>@<DIGEST>
```

### 3.4 Generate SBOMs

```bash
# Generate SBOM for each binary
syft dist/keystone-core_0.1.0_linux_amd64.tar.gz \
  -o spdx-json=dist/sbom-linux-amd64.spdx.json

syft dist/keystone-core_0.1.0_darwin_amd64.tar.gz \
  -o spdx-json=dist/sbom-darwin-amd64.spdx.json

# Or use cyclonedx format
syft dist/keystone-core_0.1.0_linux_amd64.tar.gz \
  -o cyclonedx-json=dist/sbom-linux-amd64.cdx.json
```

### 3.5 Sign Blueprint Bundles

```bash
# Sign each official blueprint
for blueprint in blueprints/kscore/*/; do
  name=$(basename "$blueprint")
  cosign sign-blob --key cosign.key \
    --output-signature "blueprints/kscore/${name}/bundle.sig" \
    "blueprints/kscore/${name}/bundle.tar.gz"
done
```

---

## Phase 4: Generate Package Repositories

### 4.1 Generate All Repositories

```bash
# Generate all package repositories
make repos

# This creates:
# - build/repos/dnf/     (el8, el9 for x86_64, aarch64)
# - build/repos/apt/     (jammy, noble, bookworm, trixie)
# - build/repos/windows/ (x64, arm64)
# - build/repos/blueprints/
# - build/repos/modules/
```

### 4.2 Generate Individual Repositories

```bash
# DNF/YUM only
make repos-dnf

# APT only
make repos-apt

# Windows only
make repos-windows

# Blueprints only
make repos-blueprints

# Modules only
make repos-modules
```

### 4.3 Sign APT Repository

```bash
# Sign Release files
cd build/repos/apt
for dist in jammy noble bookworm trixie; do
  gpg --armor --detach-sign -o dists/${dist}/Release.gpg dists/${dist}/Release
  gpg --armor --clearsign -o dists/${dist}/InRelease dists/${dist}/Release
done
cd -
```

### 4.4 Sign DNF Repository

```bash
# Sign RPM packages
cd build/repos/dnf
for rpm in el*/*/*.rpm; do
  rpm --addsign "$rpm"
done
cd -
```

### 4.5 Verify Repository Structure

```bash
# Check APT repository
apt-ftparchive contents build/repos/apt/pool/

# Check DNF repository
find build/repos/dnf -name "*.rpm" -exec rpm -qip {} \; | head -50
```

---

## Phase 5: Build and Push Container Images

### 5.1 Build Multi-Arch Images

```bash
# Build server image
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag ghcr.io/kscore/kscore-server:0.1.0 \
  --tag ghcr.io/kscore/kscore-server:latest \
  --file deploy/docker/Dockerfile.server \
  --push .

# Build agent image
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag ghcr.io/kscore/kscore-agent:0.1.0 \
  --tag ghcr.io/kscore/kscore-agent:latest \
  --file deploy/docker/Dockerfile.agent \
  --push .
```

### 5.2 Sign Container Images

```bash
# Sign with cosign (keyless)
COSIGN_EXPERIMENTAL=1 cosign sign ghcr.io/kscore/kscore-server:0.1.0
COSIGN_EXPERIMENTAL=1 cosign sign ghcr.io/kscore/kscore-agent:0.1.0

# Or with a key
cosign sign --key cosign.key ghcr.io/kscore/kscore-server:0.1.0
cosign sign --key cosign.key ghcr.io/kscore/kscore-agent:0.1.0
```

### 5.3 Generate Container SBOMs

```bash
# Attach SBOM to image
syft ghcr.io/kscore/kscore-server:0.1.0 -o spdx-json | \
  cosign attach sbom --sbom /dev/stdin ghcr.io/kscore/kscore-server:0.1.0
```

---

## Phase 6: Publish Release

### 6.1 Publish GitHub Release

```bash
# Full release with goreleaser
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser release --clean

# Or manually create release
gh release create v0.1.0 \
  --title "Keystone Core v0.1.0" \
  --notes-file RELEASE_NOTES.md \
  dist/*.tar.gz \
  dist/*.zip \
  dist/*.deb \
  dist/*.rpm \
  dist/checksums.txt \
  dist/checksums.txt.asc
```

### 6.2 Upload Package Repositories

```bash
# Upload to your CDN/hosting (example with S3)
aws s3 sync build/repos/apt/ s3://packages.keystonecore.io/apt/ --acl public-read
aws s3 sync build/repos/dnf/ s3://packages.keystonecore.io/dnf/ --acl public-read
aws s3 sync build/repos/windows/ s3://packages.keystonecore.io/windows/ --acl public-read
aws s3 sync build/repos/blueprints/ s3://packages.keystonecore.io/blueprints/ --acl public-read

# Or use rsync to a web server
rsync -avz build/repos/ user@packages.keystonecore.io:/var/www/packages/
```

### 6.3 Publish Helm Charts

```bash
# Package Helm chart
helm package deploy/helm/keystone-core -d build/charts/

# Update chart repository index
helm repo index build/charts/ --url https://charts.keystonecore.io

# Upload to chart repository
aws s3 sync build/charts/ s3://charts.keystonecore.io/ --acl public-read
```

### 6.4 Update Documentation Site

```bash
# Build documentation
make docs

# Deploy documentation (example with Netlify CLI)
netlify deploy --prod --dir=docs/public

# Or trigger CI/CD deployment
git push origin main  # If docs auto-deploy on push
```

---

## Phase 7: Verify Release

### 7.1 Verify GitHub Release

```bash
# Check release exists
gh release view v0.1.0

# Download and verify an artifact
gh release download v0.1.0 --pattern "*.tar.gz" --dir /tmp/verify
cd /tmp/verify
sha256sum -c checksums.txt
```

### 7.2 Verify Package Installation

```bash
# Test APT installation
docker run --rm ubuntu:24.04 bash -c "
  apt-get update && apt-get install -y curl gnupg
  curl -fsSL https://packages.keystonecore.io/apt/gpg.key | gpg --dearmor -o /usr/share/keyrings/kscore.gpg
  echo 'deb [signed-by=/usr/share/keyrings/kscore.gpg] https://packages.keystonecore.io/apt noble main' > /etc/apt/sources.list.d/kscore.list
  apt-get update
  apt-get install -y kscore-server kscore-agent kscore-cli
  kscore-server version
"

# Test DNF installation
docker run --rm rockylinux:9 bash -c "
  dnf install -y dnf-plugins-core
  dnf config-manager --add-repo https://packages.keystonecore.io/dnf/el9/keystone-core.repo
  dnf install -y kscore-server kscore-agent kscore-cli
  kscore-server version
"
```

### 7.3 Verify Container Images

```bash
# Pull and verify
docker pull ghcr.io/kscore/kscore-server:0.1.0
docker run --rm ghcr.io/kscore/kscore-server:0.1.0 version

# Verify signature
cosign verify ghcr.io/kscore/kscore-server:0.1.0
```

### 7.4 Verify Bootstrap Flow

```bash
# Run VM bootstrap tests
make test-vm-demo

# Or manually test bootstrap
ssh testvm "
  curl -fsSL https://get.keystonecore.io | sudo bash -s -- \
    --mode demo \
    --cluster-name test-cluster
"
```

---

## Phase 8: Announce Release

### 8.1 Create Announcement

Use the template at `docs/content/en/docs/community/announcement-0.1.0.md` as a base.

### 8.2 Publish Announcements

```bash
# Post to GitHub Discussions (if enabled)
gh api repos/kscore/keystone-core/discussions \
  -f title="Keystone Core v0.1.0 Released" \
  -f body="$(cat ANNOUNCEMENT.md)" \
  -f category_id="announcements"
```

### 8.3 Notification Channels

- [ ] GitHub Release (automatic with goreleaser)
- [ ] Project mailing list
- [ ] Project blog (if applicable)
- [ ] Social media (Twitter/X, LinkedIn, Mastodon)
- [ ] Reddit (r/devops, r/sysadmin, r/kubernetes)
- [ ] Hacker News (for major releases)
- [ ] Slack/Discord communities

---

## Phase 9: Post-Release Tasks

### 9.1 Monitor for Issues

```bash
# Watch for new issues
gh issue list --label "regression" --state open

# Monitor discussions
gh api repos/kscore/keystone-core/discussions --jq '.[] | .title'
```

### 9.2 Update Version for Next Development

```bash
# Update version in code for next development cycle
# (if you maintain version in code)

# Create tracking issue for next release
gh issue create \
  --title "Release v0.2.0 Tracking" \
  --body "Tracking issue for v0.2.0 release" \
  --label "release"
```

### 9.3 Archive Release Artifacts

```bash
# Backup release artifacts
mkdir -p releases/v0.1.0
cp -r dist/* releases/v0.1.0/
tar -czvf releases/v0.1.0.tar.gz releases/v0.1.0/
```

### 9.4 Update Epic/Project Tracking

Mark the release readiness epic as complete and update any project tracking systems.

---

## Quick Reference: Complete Release Commands

```bash
# === PRE-RELEASE ===
git checkout main && git pull
make test && make security
# Edit CHANGELOG.md

# === BUILD ===
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
make release-dry-run  # Validate first

# === SIGN ===
gpg --armor --detach-sign dist/checksums.txt
# Sign containers with cosign

# === REPOSITORIES ===
make repos

# === PUBLISH ===
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser release --clean
# Upload repos to CDN

# === VERIFY ===
gh release view v0.1.0
make test-vm-demo

# === ANNOUNCE ===
# Post to channels
```

---

## Troubleshooting

### GoReleaser Fails

```bash
# Check configuration
goreleaser check

# Run with debug output
goreleaser release --debug --skip=publish
```

### GPG Signing Fails

```bash
# List available keys
gpg --list-secret-keys

# Test signing
echo "test" | gpg --clearsign

# If agent issues
gpgconf --kill gpg-agent
gpg-agent --daemon
```

### Container Build Fails

```bash
# Check buildx is available
docker buildx version

# Create builder if needed
docker buildx create --use --name kscore-builder

# Inspect builder
docker buildx inspect kscore-builder
```

### Package Installation Fails

```bash
# Check package dependencies
dpkg-deb -I package.deb

# Check RPM dependencies
rpm -qpR package.rpm

# Verbose installation
dpkg -i --debug=2 package.deb
```

---

## See Also

- [RELEASE-PLAYBOOK.md](https://github.com/shawnbutts/keystone-core/blob/main/RELEASE-PLAYBOOK.md) - Authoritative release process (multi-party signing ceremony)
- [Release Checklist]({{< relref "release-checklist" >}}) - Pre-release verification checklist
- [Release Notes]({{< relref "release-notes" >}}) - Version history and changes
- [Development Guide]({{< relref "development" >}}) - Development setup and workflows
