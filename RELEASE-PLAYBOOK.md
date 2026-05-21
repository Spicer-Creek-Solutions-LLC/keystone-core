# Keystone Core Release Playbook

This document defines the end-to-end process for building, signing, and
publishing an official Keystone Core release. Every release follows this
playbook. No exceptions.

The process is intentionally manual and offline. This is a deliberate
engineering decision, not an operational shortcoming. Keystone Core is a
runtime operations control plane that manages infrastructure. A compromised
release could give an attacker access to every system under management.
The cost of a slow release is hours. The cost of a compromised release is
unbounded.

This playbook is public so that users and operators can verify that we take
the integrity of what we ship as seriously as the code itself.

## Standards and References

This process is informed by, and aims to comply with, the following:

| Standard | Relevance |
|----------|-----------|
| [SLSA v1.0 Build Track](https://slsa.dev/spec/v1.0/) | Supply chain integrity framework; this process targets Level 3 (hardened build, signed provenance) |
| [The Update Framework (TUF)](https://theupdateframework.io/) | Threat model for software distribution; informs our multi-party signing and key hierarchy |
| [OpenSSF Secure Software Development Guide](https://best.openssf.org/) | Baseline secure development practices |
| [NIST SP 800-57 Part 1 Rev. 5](https://csrc.nist.gov/publications/detail/sp/800-57-part-1/rev-5/final) | Key management lifecycle guidance |
| [NIST SP 800-185](https://csrc.nist.gov/publications/detail/sp/800-185/final) | SHA-3 derived functions; informs our hash algorithm choices |
| [in-toto](https://in-toto.io/) | Supply chain layout and verification framework |
| [OpenPGP (RFC 9580)](https://www.rfc-editor.org/rfc/rfc9580) | Cryptographic signature format |
| [Reproducible Builds](https://reproducible-builds.org/) | Build reproducibility verification methodology |
| [CISA Secure Software Development Attestation](https://www.cisa.gov/secure-software-development-attestation) | Federal supply chain security requirements |

---

## Table of Contents

1. [Roles and Quorum Rules](#1-roles-and-quorum-rules)
2. [Key Hierarchy and Ceremony](#2-key-hierarchy-and-ceremony)
3. [Release Record](#3-release-record)
4. [Phase 1: Release Initiation](#4-phase-1-release-initiation)
5. [Phase 2: Source Verification](#5-phase-2-source-verification)
6. [Phase 3: Dependency Audit](#6-phase-3-dependency-audit)
7. [Phase 4: Build](#7-phase-4-build)
8. [Phase 5: Artifact Verification](#8-phase-5-artifact-verification)
9. [Phase 6: Signing](#9-phase-6-signing)
10. [Phase 7: Package Construction](#10-phase-7-package-construction)
11. [Phase 8: Container Images](#11-phase-8-container-images)
12. [Phase 9: Publication](#12-phase-9-publication)
13. [Phase 10: Post-Release Verification](#13-phase-10-post-release-verification)
14. [Emergency and Patch Releases](#14-emergency-and-patch-releases)
15. [Key Rotation and Revocation](#15-key-rotation-and-revocation)

---

## 1. Roles and Quorum Rules

### Roles

| Role | Description |
|------|-------------|
| **Release Manager** | Coordinates the ceremony. Maintains the release record. Does not have unilateral signing authority. |
| **Release Signer** | Holds a signing subkey. Participates in artifact signing. |
| **Release Witness** | Observes and verifies each phase. May or may not also be a signer. |

Every participant in a release ceremony holds at least one role. The Release
Manager may also be a Signer or Witness but cannot be the sole Signer.

### Quorum Requirements

Decisions during the release ceremony fall into two categories:

**Majority (>50% of participants) required for:**

- Confirming source commit matches expected tag
- Approving dependency audit results (when no new/changed dependencies)
- Confirming build output checksums match across independent builds
- Approving the changelog and release notes
- Approving publication to each distribution channel

**Unanimous (100% of participants) required for:**

- Proceeding when new or updated dependencies have been introduced since the last release
- Proceeding when any security scanner reports a finding not previously accepted
- Overriding a failed verification step for any reason
- Introducing any deviation from this playbook during a ceremony
- Key generation, rotation, or revocation decisions
- Releasing when the build environment has changed from the prior release

All votes are recorded in the release record with participant identity,
timestamp, and vote (approve/reject/abstain). Abstentions do not count toward
quorum.

---

## 2. Key Hierarchy and Ceremony

### Key Structure

The project maintains a two-tier key hierarchy, modeled on
[TUF's role-based structure](https://theupdateframework.io/metadata/):

```
Root Key (offline, air-gapped, never on a networked machine)
  |
  +-- Release Signing Subkey A (held by Signer 1)
  +-- Release Signing Subkey B (held by Signer 2)
  +-- Release Signing Subkey C (held by Signer 3)
  ...
```

- **Root key**: Ed25519 or RSA-4096. Generated on an air-gapped machine.
  Used only to certify signing subkeys. Stored encrypted on offline media
  in at least two geographically separate locations. See
  [NIST SP 800-57](https://csrc.nist.gov/publications/detail/sp/800-57-part-1/rev-5/final)
  for key management lifecycle.
- **Signing subkeys**: Ed25519. One per authorized signer. Stored on a
  hardware token (e.g., YubiKey with OpenPGP applet) or encrypted offline
  media. Subkeys have a maximum validity period of 26 months and must be
  rotated before expiry. This duration allows adopters two full annual
  planning cycles plus buffer to schedule the rotation ceremony, and is
  within the 1-3 year cryptoperiod recommended by
  [NIST SP 800-57](https://csrc.nist.gov/publications/detail/sp/800-57-part-1/rev-5/final)
  for digital signature keys.

### Initial Key Generation Ceremony

This is performed once and whenever the root key must be regenerated.

1. Convene all initial signers in person or via a verified secure channel.
2. Boot an air-gapped machine from a known-good live image
   (e.g., [Tails](https://tails.net/)).
3. Generate the root key pair:
   ```
   gpg --quick-generate-key "Keystone Core Release Authority <release@keystone-core.io>" ed25519 cert never
   ```
4. For each signer, generate a signing subkey:
   ```
   gpg --quick-add-key <ROOT_FINGERPRINT> ed25519 sign 26m
   ```
5. Export each subkey to the respective signer's hardware token or
   encrypted offline media.
6. Export the root public key and all subkey public keys.
7. Back up the root private key to at least two encrypted offline storage
   devices, held by different people in different locations.
8. Publish the root public key and signing subkey fingerprints:
   - In the repository at `keys/release-pubkey.asc`
   - On the project website
   - On at least one public keyserver
9. Record the full ceremony (participants, fingerprints, timestamps,
   storage locations) in a signed document stored alongside the keys.

**Vote**: Unanimous to finalize the key generation.

---

## 3. Release Record

Every release ceremony produces a release record: a plain-text, append-only
log that documents what happened, who was present, what was verified, and how
every vote went. This record is published alongside the release artifacts so
that anyone can audit the process.

### Format

```
====================================================================
KEYSTONE CORE RELEASE RECORD
Version:    v<X.Y.Z>
Date:       YYYY-MM-DD
Participants:
  - <Name> <Fingerprint> (Role: Release Manager, Signer)
  - <Name> <Fingerprint> (Role: Signer, Witness)
  - <Name> <Fingerprint> (Role: Witness)
====================================================================

[HH:MM UTC] PHASE 1 - RELEASE INITIATION
  Action: ...
  Verification: ...
  Vote: MAJORITY | UNANIMOUS
    <Name>: APPROVE | REJECT | ABSTAIN
    <Name>: APPROVE | REJECT | ABSTAIN
  Result: PASS | FAIL

...
```

The Release Manager maintains this record in real time. At the end of the
ceremony, every participant signs the record with their signing key. The
signed record is published as `release-record-vX.Y.Z.txt` and
`release-record-vX.Y.Z.txt.asc` (detached signatures from each signer).

---

## 4. Phase 1: Release Initiation

**Purpose**: Establish what is being released and who is present.

1. Release Manager identifies the target commit (tag or SHA) and states
   the intended version number.
2. All participants verify their identity and role.
3. Release Manager confirms quorum is met.
4. Release Manager creates the release record and records all participants.

**Vote**: Majority to confirm the target version and begin.

---

## 5. Phase 2: Source Verification

**Purpose**: Ensure every participant is working from identical, untampered
source code.

1. Each participant independently clones or fetches the repository and
   checks out the target commit:
   ```
   git clone <repo-url> keystone-core-release
   cd keystone-core-release
   git checkout <TAG_OR_SHA>
   ```
2. Each participant computes the tree hash:
   ```
   git rev-parse HEAD
   git ls-tree -r --name-only HEAD | sort | xargs sha256sum | sha256sum
   ```
3. Release Manager collects and compares all tree hashes. They must be
   identical.
4. If the commit is a signed tag, verify the tag signature:
   ```
   git verify-tag <TAG>
   ```

**Record**: Commit SHA, tree hash, tag signature verification result.

**Vote**: Majority to confirm source integrity.

**STOP condition**: If tree hashes do not match, the ceremony halts. Do not
proceed. Investigate the discrepancy.

---

## 6. Phase 3: Dependency Audit

**Purpose**: Verify that all dependencies are known, expected, and free of
known vulnerabilities.

### 6a. Dependency Inventory

1. Generate the full dependency list. Note: `go list -m all` may fail in
   repositories that transitively depend on `k8s.io/kubernetes` due to its
   internal `replace` directives. Use `go mod graph` to extract the resolved
   module set instead:
   ```
   # Extract unique modules from the dependency graph
   go mod graph | awk '{print $2}' | sort -u > deps-full.txt

   # Also capture the direct dependencies from go.mod for focused review
   go list -m -mod=mod all 2>/dev/null > deps-direct.txt || true
   ```
2. Diff against the dependency list from the previous release:
   ```
   diff deps-previous-release.txt deps-full.txt
   ```
3. For each new or changed dependency, record:
   - Module path and version
   - Reason for addition/change (link to commit or issue)
   - License
   - Maintainer/governance assessment

### 6b. Vulnerability Scan

Run all security scanners against the source tree:

```
govulncheck ./...
make security-vulns
make security-sast
make security-licenses
```

Record all output in the release record.

### 6c. Checksum Verification

Verify `go.sum` integrity:

```
go mod verify
```

This confirms that every downloaded module matches the checksum recorded in
`go.sum`. A failure here means a dependency has been tampered with since it
was added.

### 6d. SBOM Generation

Generate Software Bills of Materials for inclusion with the release:

```
make security-sbom
```

This produces both CycloneDX and SPDX format SBOMs.

**Record**: Dependency diff, vulnerability scan output, `go mod verify`
result, list of new/changed dependencies with justifications.

**Vote**:
- **Majority** if no new or changed dependencies and no new scanner findings.
- **Unanimous** if any new or changed dependencies exist.
- **Unanimous** if any scanner reports a finding not previously accepted.

**STOP condition**: If `go mod verify` fails, the ceremony halts.

---

## 7. Phase 4: Build

**Purpose**: Produce deterministic, reproducible binaries from the verified
source.

### Build Environment Requirements

- Air-gapped or network-isolated machine.
- Known-good Go toolchain, version-pinned and hash-verified. The exact
  Go version is recorded in `go.mod` and must match.
- Clean build environment (container or fresh chroot) with no prior build
  artifacts.
- GoReleaser version pinned and hash-verified.
- `-trimpath` flag must be set (already configured in `.goreleaser.yaml`
  via `CGO_ENABLED=0` and `-s -w` ldflags; verify `-trimpath` is present).

### Build Steps

1. Verify the Go toolchain:
   ```
   sha256sum $(which go)
   go version
   ```
   Compare against the published hash from
   [go.dev/dl](https://go.dev/dl/).

2. Verify GoReleaser:
   ```
   sha256sum $(which goreleaser)
   goreleaser --version
   ```

3. Run the build:
   ```
   make release-snapshot
   ```
   This produces all binaries, archives, and packages in `dist/` without
   publishing.

4. Generate checksums of all artifacts:
   ```
   cd dist/
   sha256sum *.tar.gz *.zip *.deb *.rpm checksums.txt > ceremony-checksums.txt
   ```

### Reproducibility Verification

At least two participants must build independently from the verified source
and compare output. Due to the use of `-trimpath`, `CGO_ENABLED=0`, and
identical Go toolchain versions, the binaries should be bit-identical.

```
diff participant-a/ceremony-checksums.txt participant-b/ceremony-checksums.txt
```

If the builds are not reproducible, investigate before proceeding. Common
causes: different Go patch versions, timezone differences in build date
ldflags, differing filesystem ordering. The build date ldflags in
`.goreleaser.yaml` use GoReleaser's `{{.Date}}` template -- for
reproducibility, all builders must use the same snapshot configuration or
the Release Manager must provide a fixed `--snapshot` build date.

**Record**: Go toolchain version and hash, GoReleaser version and hash,
build machine OS/arch, full `ceremony-checksums.txt` from each builder.

**Vote**: Majority to confirm builds are reproducible and checksums match.

**STOP condition**: If builds are not reproducible, halt and investigate.

---

## 8. Phase 5: Artifact Verification

**Purpose**: Verify the built artifacts function correctly before signing.

### 8a. Smoke Tests

Run against the built binaries (not a fresh `go test` -- use the actual
artifacts from `dist/`):

```
# Extract Linux amd64 archive
tar xzf dist/keystone-core_<VERSION>_linux_amd64.tar.gz -C /tmp/kscore-verify/

# Verify each binary runs and reports the correct version
for bin in /tmp/kscore-verify/kscore-*; do
  $bin version 2>/dev/null || $bin --version 2>/dev/null || echo "WARN: $bin has no version command"
done
```

### 8b. Package Verification

```
# DEB: inspect contents
dpkg-deb -c dist/kscore-server_<VERSION>_linux_amd64.deb
dpkg-deb -I dist/kscore-server_<VERSION>_linux_amd64.deb

# RPM: inspect contents
rpm -qilp dist/kscore-server-<VERSION>.x86_64.rpm

# Verify packages install cleanly in a disposable container/VM
```

### 8c. Vulnerability Scan on Artifacts

Run Trivy or Grype against the final binaries:

```
trivy fs dist/
```

**Record**: Smoke test output, package inspection output, vulnerability
scan results.

**Vote**: Majority to confirm artifacts are correct.

---

## 9. Phase 6: Signing

**Purpose**: Cryptographically bind the verified artifacts to the project's
release identity.

Signing is performed on an air-gapped machine. The artifacts are
transferred via read-only media (USB drive, SD card) from the build machine.

### 9a. Sign the Checksums File

The checksums file covers all artifacts. One signature over this file
provides transitive integrity for everything it lists.

Each signer independently verifies the checksums file matches the artifacts
they observed, then signs:

```
gpg --armor --detach-sign --output checksums.txt.sig.<SIGNER_ID> checksums.txt
```

### 9b. Verify All Signatures

Each participant verifies every other signer's signature:

```
gpg --verify checksums.txt.sig.<SIGNER_ID> checksums.txt
```

### 9c. Minimum Signature Threshold

A release requires signatures from a **majority of authorized signers**.
For example, with 3 signers, at least 2 must sign. The release record
must document why any authorized signer did not participate.

### 9d. Sign the SBOMs

```
gpg --armor --detach-sign --output sbom-cyclonedx.json.sig.<SIGNER_ID> sbom-cyclonedx.json
gpg --armor --detach-sign --output sbom-spdx.json.sig.<SIGNER_ID> sbom-spdx.json
```

### 9e. Sign the Release Record

After all other phases are complete, each participant signs the release
record itself:

```
gpg --armor --detach-sign --output release-record-v<VERSION>.txt.sig.<SIGNER_ID> release-record-v<VERSION>.txt
```

**Record**: Each signature file hash. Verification results of each signature
by each other participant.

**Vote**: Majority of authorized signers must have signed. This is verified
mechanically, not voted on.

---

## 10. Phase 7: Package Construction

**Purpose**: Assemble the final release artifact set.

The release directory structure:

```
keystone-core-v<VERSION>/
  checksums.txt
  checksums.txt.sig.<SIGNER_A>
  checksums.txt.sig.<SIGNER_B>
  keystone-core_<VERSION>_linux_amd64.tar.gz
  keystone-core_<VERSION>_linux_arm64.tar.gz
  keystone-core_<VERSION>_darwin_amd64.tar.gz
  keystone-core_<VERSION>_darwin_arm64.tar.gz
  keystone-core_<VERSION>_windows_amd64.zip
  kscore-server_<VERSION>_linux_amd64.deb
  kscore-server_<VERSION>_linux_arm64.deb
  kscore-server-<VERSION>.x86_64.rpm
  kscore-server-<VERSION>.aarch64.rpm
  kscore-agent_<VERSION>_linux_amd64.deb
  kscore-agent_<VERSION>_linux_arm64.deb
  kscore-agent-<VERSION>.x86_64.rpm
  kscore-agent-<VERSION>.aarch64.rpm
  kscore-cli_<VERSION>_linux_amd64.deb
  kscore-cli_<VERSION>_linux_arm64.deb
  kscore-cli-<VERSION>.x86_64.rpm
  kscore-cli-<VERSION>.aarch64.rpm
  sbom-cyclonedx.json
  sbom-cyclonedx.json.sig.<SIGNER_A>
  sbom-spdx.json
  sbom-spdx.json.sig.<SIGNER_A>
  release-record-v<VERSION>.txt
  release-record-v<VERSION>.txt.sig.<SIGNER_A>
  release-record-v<VERSION>.txt.sig.<SIGNER_B>
```

1. Verify every file in the directory is accounted for in `checksums.txt`.
2. Verify all signature files validate.
3. Compute a final SHA-256 hash of the entire directory listing for the
   release record.

**Record**: Complete file listing with sizes and hashes.

**Vote**: Majority to confirm the release artifact set is complete and correct.

---

## 11. Phase 8: Container Images

**Purpose**: Build, verify, and sign container images.

Container images are built from the same verified binaries produced in
Phase 4. They are not built from source independently -- the binary is
copied into the container image to ensure artifact identity.

### 11a. Build

```
# Using the exact binaries from the signed release
docker build \
  --build-arg VERSION=<VERSION> \
  --no-cache \
  --platform linux/amd64,linux/arm64 \
  -f deploy/gateway/Dockerfile \
  -t ghcr.io/kscore/kscore-server:<VERSION> \
  .
```

The Dockerfile base image must be pinned by digest
(e.g., `alpine:3.19@sha256:<digest>`). The digest used is recorded in the
release record.

### 11b. Scan

```
trivy image ghcr.io/kscore/kscore-server:<VERSION>
grype ghcr.io/kscore/kscore-server:<VERSION>
```

### 11c. Sign with Cosign (Key-Based, Not Keyless)

Container image signing uses [Cosign](https://docs.sigstore.dev/cosign/overview/)
with a project-controlled key, not keyless/OIDC mode. The Cosign private
key is stored on the same hardware token or offline media as the GPG
signing subkey.

```
cosign sign --key <KEY_REF> ghcr.io/kscore/kscore-server:<VERSION>@<DIGEST>
cosign sign --key <KEY_REF> ghcr.io/kscore/kscore-agent:<VERSION>@<DIGEST>
```

### 11d. Attach SBOM to Image

```
cosign attach sbom --sbom sbom-cyclonedx.json ghcr.io/kscore/kscore-server:<VERSION>@<DIGEST>
```

**Record**: Base image digest, build commands, scan results, image digests,
Cosign signature references.

**Vote**: Majority to approve container images for publication.

---

## 12. Phase 9: Publication

**Purpose**: Make the signed artifacts available to users.

Publication is the final irreversible step. Each distribution channel
requires a separate majority vote.

### Distribution Channels

| Channel | Artifacts | Vote Required |
|---------|-----------|---------------|
| Release page (e.g., GitHub/GitLab/Gitea Releases) | Archives, packages, checksums, signatures, SBOMs, release record | Majority |
| Container registry (e.g., ghcr.io, Docker Hub) | Signed container images | Majority |
| Package repository (APT/YUM if applicable) | .deb, .rpm packages | Majority |
| Project website | Release announcement, updated verification docs | Majority |

### Publication Steps

1. **Tag the release** (if not already tagged):
   ```
   git tag -s v<VERSION> -m "Release v<VERSION>" <COMMIT_SHA>
   git push origin v<VERSION>
   ```
   The tag must be signed by the Release Manager's key.

2. **Upload artifacts** to the release page. Include:
   - All archives and packages
   - `checksums.txt` and all `.sig` files
   - Both SBOMs and their signatures
   - The release record and its signatures
   - Release notes / changelog

3. **Push container images**:
   ```
   docker push ghcr.io/kscore/kscore-server:<VERSION>
   docker push ghcr.io/kscore/kscore-agent:<VERSION>
   # Also tag as latest if this is the newest stable release
   ```

4. **Update verification documentation** on the project website with
   instructions for users to verify signatures.

### Post-Publication Announcement

The release announcement must include:

- Version number and changelog summary
- Links to artifacts and verification instructions
- The release record (or link to it)
- Fingerprints of the signing keys used

**Record**: Timestamp of each publication action. URLs/references for each
published artifact.

**Vote**: Majority per channel, recorded separately.

---

## 13. Phase 10: Post-Release Verification

**Purpose**: Confirm published artifacts match what was signed.

Within 24 hours of publication, at least one participant (who was not the
one who performed the upload) must:

1. Download every artifact from every distribution channel.
2. Verify all checksums:
   ```
   sha256sum -c checksums.txt
   ```
3. Verify all signatures:
   ```
   gpg --verify checksums.txt.sig.<SIGNER_ID> checksums.txt
   ```
4. Pull container images and verify Cosign signatures:
   ```
   cosign verify --key <PUBLIC_KEY> ghcr.io/kscore/kscore-server:<VERSION>
   ```
5. Install a package (DEB or RPM) in a clean environment and run a smoke
   test.

**Record**: Verification results appended to the release record as an
addendum. Signed by the verifier.

If any verification fails, immediately notify all participants. Do not
issue a retraction until the cause is understood -- it may be a download
corruption rather than a compromise.

---

## 14. Emergency and Patch Releases

Security patches and critical fixes may require an expedited release. The
process is identical to a standard release with the following modifications:

- **Reduced quorum**: A minimum of 2 participants is acceptable for patch
  releases (one must be a signer, one a witness). The release record must
  document why the full quorum was not convened.
- **Dependency audit**: May be scoped to only the changed dependencies,
  but `go mod verify` must still pass against the full tree.
- **Reproducibility**: Still required. At least 2 independent builds must
  match.

A patch release does not relax any signing or verification requirements.
The only accommodation is a smaller quorum.

**Vote**: Unanimous among present participants to proceed with a reduced
quorum.

---

## 15. Key Rotation and Revocation

### Scheduled Rotation

Signing subkeys have a 26-month maximum validity. Rotation must occur before
expiry:

1. Convene a key ceremony (same requirements as initial generation).
2. Generate new subkeys under the existing root key.
3. Distribute to hardware tokens.
4. Publish updated public keys.
5. Revoke the old subkeys.

**Vote**: Unanimous.

### Emergency Revocation

If a signing key is suspected compromised:

1. Any participant may initiate revocation by notifying all other
   participants immediately.
2. Revoke the compromised subkey:
   ```
   gpg --edit-key <ROOT_FINGERPRINT>
   > key <SUBKEY_INDEX>
   > revkey
   ```
3. Publish the revocation certificate immediately to all channels where
   the public key was published.
4. Audit all releases signed with the compromised key.
5. Re-sign affected releases with a new key if the audit shows no evidence
   of tampering, or issue advisories if tampering is suspected.
6. Generate a replacement subkey via a key ceremony.

**Vote**: Unanimous to finalize revocation and replacement. The initial
revocation action (step 2-3) may be taken unilaterally by any participant
to minimize exposure window, but must be ratified unanimously afterward.

---

## Appendix A: Verification Instructions for Users

Include these instructions (or a link to them) with every release.

```bash
# 1. Import the Keystone Core release public key
curl -sSL https://keys.keystone-core.io/release-pubkey.asc | gpg --import

# 2. Download the release artifacts and checksums
# (from the release page of your chosen platform)

# 3. Verify the checksum signature
gpg --verify checksums.txt.sig.A checksums.txt
gpg --verify checksums.txt.sig.B checksums.txt

# 4. Verify artifact integrity
sha256sum -c checksums.txt

# 5. For container images
cosign verify --key release-cosign.pub ghcr.io/kscore/kscore-server:<VERSION>
```

---

## Appendix B: Checklist Summary

This is a condensed checklist for use during a ceremony. It does not replace
the full process descriptions above.

- [ ] Quorum confirmed, participants identified, release record started
- [ ] Target commit/tag agreed (Majority)
- [ ] Source tree hash verified by all participants
- [ ] Dependency diff reviewed
- [ ] `go mod verify` passed
- [ ] Vulnerability scans completed, findings reviewed
- [ ] Dependency audit approved (Majority or Unanimous per rules)
- [ ] SBOMs generated
- [ ] Build completed on air-gapped/isolated machine
- [ ] Reproducibility verified (2+ independent builds match)
- [ ] Build approved (Majority)
- [ ] Artifact smoke tests passed
- [ ] Package contents inspected
- [ ] Artifact vulnerability scan clean
- [ ] Artifacts approved (Majority)
- [ ] Checksums file signed by majority of signers
- [ ] All signatures cross-verified
- [ ] SBOMs signed
- [ ] Container images built from signed binaries
- [ ] Container images scanned
- [ ] Container images signed with Cosign
- [ ] Container images approved (Majority)
- [ ] Release record completed and signed by all participants
- [ ] Publication to each channel approved (Majority per channel)
- [ ] Artifacts published
- [ ] Post-release verification completed within 24 hours

---

## Appendix C: Threat Model

This process is designed to defend against the following threats, mapped to
the [SLSA threat taxonomy](https://slsa.dev/spec/v1.0/threats):

| Threat | Mitigation |
|--------|------------|
| **Source tampering** (unauthorized commit) | Multi-party source verification, signed tags, tree hash comparison |
| **Dependency compromise** (malicious upstream) | `go mod verify`, dependency diff review, vulnerability scanning, unanimous vote on new deps |
| **Build process compromise** (CI/CD takeover) | Air-gapped build, no CI in the signing/release path, reproducible builds |
| **Artifact tampering** (modify after build) | Checksums, multi-party signatures, post-publication verification |
| **Key compromise** (stolen signing key) | Hardware tokens, subkey architecture, revocation procedures, multi-signer threshold |
| **Single actor compromise** (insider threat) | Multi-party ceremony, quorum requirements, no unilateral signing authority |
| **Registry compromise** (tampered distribution) | Post-release verification from independent download, Cosign for containers |

---

*This document is versioned alongside the code. Changes to this playbook
require a pull request reviewed by all authorized signers.*
