# Epic 25: Release & Distribution

## Overview

Establish a comprehensive release and distribution strategy for Keystone Core, including release processes, package repositories, documentation hosting, and update mechanisms. This epic covers the research, setup, and automation needed to make Keystone Core easily installable and updatable for end users.

**Goal**: Enable users to easily install, update, and stay current with Keystone Core releases through standard package managers, container registries, and a public documentation site.

## Success Criteria

- [ ] Automated release process with semantic versioning
- [ ] Public documentation site hosted and accessible
- [ ] APT repository for Debian/Ubuntu users
- [ ] DNF/YUM repository for RHEL/Fedora/Rocky users
- [ ] Homebrew tap for macOS users
- [ ] Windows package (Chocolatey, winget, or MSI downloads)
- [ ] Container images on public registry (Docker Hub, GHCR, or Quay)
- [ ] Helm charts published to chart repository
- [ ] Binary downloads available (GitHub Releases or dedicated CDN)
- [ ] Update notification mechanism for agents
- [ ] Changelog and release notes automation
- [ ] Signing and verification for all artifacts

## Research Areas

### Documentation Hosting Options

| Option | Pros | Cons | Cost |
|--------|------|------|------|
| **GitHub Pages** | Free, integrated with repo, custom domain | Limited to static sites | Free |
| **Netlify** | Free tier, preview deploys, custom domain | Bandwidth limits on free tier | Free-$19/mo |
| **Cloudflare Pages** | Free, fast CDN, unlimited bandwidth | Newer platform | Free |
| **Vercel** | Free tier, great DX, preview deploys | Bandwidth limits | Free-$20/mo |
| **ReadTheDocs** | Free for OSS, versioning built-in | Less customizable | Free |
| **Self-hosted** | Full control | Maintenance burden | Varies |

**Recommendation**: Cloudflare Pages or Netlify for free tier with good performance.

### Package Repository Hosting

#### APT Repository Options

| Option | Pros | Cons | Cost |
|--------|------|------|------|
| **Packagecloud** | Easy setup, multiple formats | Cost scales with downloads | Free-$150/mo |
| **Cloudsmith** | Modern, good API, multiple formats | Cost | Free-$100/mo |
| **Aptly + S3** | Self-managed, low cost | Setup complexity | S3 costs only |
| **GitHub Releases + script** | Simple, free | Not native apt | Free |
| **Launchpad PPA** | Free, Ubuntu-native | Ubuntu only, slow builds | Free |

#### DNF/YUM Repository Options

| Option | Pros | Cons | Cost |
|--------|------|------|------|
| **Packagecloud** | Easy setup, RPM support | Cost | Free-$150/mo |
| **Cloudsmith** | Modern platform | Cost | Free-$100/mo |
| **Copr** | Free, Fedora-native | Fedora ecosystem only | Free |
| **Self-hosted + S3** | Full control | Setup complexity | S3 costs |

#### macOS Options

| Option | Pros | Cons | Cost |
|--------|------|------|------|
| **Homebrew Tap** | Standard for macOS, easy updates | Requires GitHub repo | Free |
| **MacPorts** | Alternative to Homebrew | Less popular | Free |
| **PKG installer** | Native macOS | Manual updates | Free |

#### Windows Options

| Option | Pros | Cons | Cost |
|--------|------|------|------|
| **Chocolatey** | Popular, easy updates | Community repo moderation | Free |
| **winget** | Microsoft native | Newer, less adoption | Free |
| **Scoop** | Developer friendly | Less mainstream | Free |
| **MSI + downloads** | Native Windows | Manual updates | Free |

### Container Registry Options

| Option | Pros | Cons | Cost |
|--------|------|------|------|
| **GitHub Container Registry** | Integrated with GitHub, free for public | Tied to GitHub | Free |
| **Docker Hub** | Most recognized, default | Rate limits, cost for teams | Free-$9/mo |
| **Quay.io** | Red Hat backed, security scanning | Less mainstream | Free |
| **Amazon ECR Public** | Fast, reliable | AWS ecosystem | Free |

### Helm Chart Repository Options

| Option | Pros | Cons | Cost |
|--------|------|------|------|
| **GitHub Pages** | Free, simple | Basic features | Free |
| **ChartMuseum** | Feature rich | Self-hosted | Free |
| **Artifact Hub** | Discovery, verification | Listing only | Free |
| **Harbor** | Enterprise features | Self-hosted complexity | Free |

## Release Process

### Versioning Strategy

Follow [Semantic Versioning 2.0.0](https://semver.org/):
- **MAJOR**: Breaking changes to CLI, API, or configuration
- **MINOR**: New features, backward compatible
- **PATCH**: Bug fixes, security patches

### Release Channels

| Channel | Purpose | Stability | Update Frequency |
|---------|---------|-----------|------------------|
| **stable** | Production use | Highest | Monthly |
| **beta** | Preview features | Medium | Bi-weekly |
| **nightly** | Latest development | Low | Daily |

### Release Artifacts

For each release, produce:
1. **Binaries**: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64)
2. **Packages**: .deb, .rpm, .pkg, .msi
3. **Containers**: Multi-arch images (amd64, arm64)
4. **Helm Charts**: Versioned charts
5. **Checksums**: SHA256 for all files
6. **Signatures**: GPG/cosign for verification

### Release Checklist

```markdown
## Pre-Release
- [ ] All tests passing
- [ ] Version bumped in code
- [ ] CHANGELOG.md updated
- [ ] Documentation updated
- [ ] Migration guide (if breaking changes)

## Build
- [ ] Build all binaries
- [ ] Build all packages
- [ ] Build container images
- [ ] Package Helm charts
- [ ] Generate checksums
- [ ] Sign artifacts

## Publish
- [ ] Create GitHub Release
- [ ] Upload binaries to release
- [ ] Push packages to repositories
- [ ] Push container images
- [ ] Push Helm charts
- [ ] Update documentation site

## Announce
- [ ] Release notes published
- [ ] Blog post (major releases)
- [ ] Social media (major releases)
- [ ] Mailing list notification
```

## User Stories

### US25.1: Automated Release Pipeline
**As a** maintainer
**I want** releases to be automated
**So that** releases are consistent and low-effort

**Acceptance Criteria**:
- GitHub Actions workflow for releases
- Tag-triggered release process
- Automated changelog generation
- Multi-platform binary builds
- Package builds for all formats
- Container image builds and push
- Helm chart packaging and push

### US25.2: Public Documentation Site
**As a** user
**I want** documentation available on a public website
**So that** I can easily find information

**Acceptance Criteria**:
- Hugo site deployed to hosting provider
- Custom domain configured
- HTTPS enabled
- Search functionality
- Version selector for docs
- Automatic deployment on merge to main

### US25.3: APT Package Repository
**As a** Debian/Ubuntu user
**I want** to install via apt
**So that** I can use standard package management

**Acceptance Criteria**:
- APT repository hosted and accessible
- GPG key for package signing
- Repository added via standard apt commands
- Packages for amd64 and arm64
- Automatic updates via apt upgrade

### US25.4: DNF/YUM Package Repository
**As a** RHEL/Fedora user
**I want** to install via dnf/yum
**So that** I can use standard package management

**Acceptance Criteria**:
- DNF/YUM repository hosted and accessible
- GPG key for package signing
- Repository added via standard dnf commands
- Packages for x86_64 and aarch64
- Automatic updates via dnf upgrade

### US25.5: Homebrew Installation
**As a** macOS user
**I want** to install via Homebrew
**So that** I can use the standard macOS package manager

**Acceptance Criteria**:
- Homebrew tap repository created
- Formula for all binaries
- Cask for GUI tools (if any)
- `brew install kscore/tap/kscore` works
- `brew upgrade` updates to latest

### US25.6: Windows Installation
**As a** Windows user
**I want** easy installation options
**So that** I can install without manual steps

**Acceptance Criteria**:
- Chocolatey package available
- winget package available
- MSI installer downloadable
- Silent install support
- Automatic updates (where supported)

### US25.7: Container Images
**As a** Kubernetes user
**I want** official container images
**So that** I can deploy in containers

**Acceptance Criteria**:
- Images on public registry
- Multi-arch support (amd64, arm64)
- Versioned tags (v1.0.0, v1.0, v1, latest)
- Minimal base images (distroless or alpine)
- Signed with cosign

### US25.8: Helm Charts
**As a** Kubernetes administrator
**I want** official Helm charts
**So that** I can deploy using Helm

**Acceptance Criteria**:
- Charts for server and agent
- Published to chart repository
- Listed on Artifact Hub
- Configurable values
- Upgrade support

### US25.9: Update Notifications
**As an** operator
**I want** to be notified of available updates
**So that** I know when to upgrade

**Acceptance Criteria**:
- Agent checks for updates periodically
- Server shows available updates in dashboard
- CLI command to check for updates
- Optional automatic update (agent only)
- Release notes accessible

### US25.10: Artifact Signing
**As a** security-conscious user
**I want** all artifacts signed
**So that** I can verify authenticity

**Acceptance Criteria**:
- GPG signing for packages
- Cosign signing for containers
- Checksums for all downloads
- Public keys published
- Verification instructions documented

## Technical Tasks

### Phase 1: Research & Decision (Week 1)

**T1.1: Documentation Hosting Research**
- Evaluate hosting options
- Test deployment to top 2-3 options
- Measure performance and features
- Make recommendation

**T1.2: Package Repository Research**
- Evaluate APT/DNF hosting options
- Cost analysis for expected traffic
- Test setup for top options
- Make recommendation

**T1.3: Container Registry Research**
- Evaluate registry options
- Test push/pull performance
- Consider rate limits
- Make recommendation

**T1.4: Decision Document**
- Document decisions with rationale
- Budget estimation
- Timeline for implementation

### Phase 2: Documentation Site (Weeks 2-3)

**T2.1: Hosting Setup**
- Create account on chosen provider
- Configure custom domain
- Set up HTTPS
- Configure build settings

**T2.2: Deployment Automation**
- GitHub Actions workflow for docs
- Preview deployments for PRs
- Production deployment on merge
- Cache invalidation

**T2.3: Search Setup**
- Integrate search (Algolia DocSearch or Pagefind)
- Index configuration
- Search UI integration

**T2.4: Version Support**
- Version selector implementation
- Archived docs for old versions
- Latest redirect

### Phase 3: Package Repositories (Weeks 4-6)

**T3.1: APT Repository Setup**
- Create repository on chosen host
- Generate GPG key pair
- Configure repository structure
- Write repository setup script

**T3.2: DNF Repository Setup**
- Create repository on chosen host
- Generate GPG key pair
- Configure repository structure
- Write repository setup script

**T3.3: Homebrew Tap**
- Create tap repository (kscore/homebrew-tap)
- Write formula for binaries
- Test installation
- Document usage

**T3.4: Windows Packages**
- Create Chocolatey package
- Submit to Chocolatey community
- Create winget manifest
- Submit to winget-pkgs
- Create MSI installer

**T3.5: Package Build Automation**
- GitHub Actions for .deb builds
- GitHub Actions for .rpm builds
- Automated upload to repositories
- Version synchronization

### Phase 4: Container & Helm (Weeks 7-8)

**T4.1: Container Registry Setup**
- Create organization on registry
- Configure access tokens
- Set up multi-arch builds
- Configure signing

**T4.2: Container Build Automation**
- Multi-arch Dockerfile optimization
- GitHub Actions for container builds
- Automated push on release
- Tag management (latest, version)

**T4.3: Helm Repository Setup**
- Create chart repository
- Configure GitHub Pages or ChartMuseum
- Register on Artifact Hub
- Configure signing

**T4.4: Helm Chart Automation**
- GitHub Actions for chart packaging
- Automated index update
- Version management

### Phase 5: Release Automation (Weeks 9-10)

**T5.1: Release Workflow**
- Tag-triggered GitHub Actions workflow
- Version extraction from tag
- Parallel build jobs
- Release creation

**T5.2: Binary Builds**
- Cross-compilation for all platforms
- CGO_ENABLED=0 builds
- UPX compression (optional)
- Checksum generation

**T5.3: Package Builds**
- Debian package builds
- RPM package builds
- macOS package builds
- Windows MSI builds

**T5.4: Artifact Publishing**
- Upload to GitHub Releases
- Push to package repositories
- Push to container registry
- Push to Helm repository

**T5.5: Changelog Automation**
- Conventional commits parsing
- Automated changelog generation
- Release notes template
- Breaking change highlighting

### Phase 6: Signing & Security (Week 11)

**T6.1: GPG Key Management**
- Generate release signing key
- Secure key storage (GitHub Secrets or Vault)
- Public key publication
- Key rotation plan

**T6.2: Package Signing**
- Sign .deb packages
- Sign .rpm packages
- Verification documentation

**T6.3: Container Signing**
- Cosign setup
- Keyless signing with GitHub OIDC
- Signature verification documentation
- SBOM generation

**T6.4: Binary Signing**
- GPG signing for binaries
- Checksum file signing
- Verification script

### Phase 7: Update Mechanism (Week 12)

**T7.1: Version Check API**
- Endpoint for latest version
- Channel support (stable, beta, nightly)
- Platform-specific versions
- Cache headers

**T7.2: Agent Update Check**
- Periodic version check
- Configurable check interval
- Update available event
- Disable option

**T7.3: CLI Update Command**
- `kscorectl version check` command
- Show current vs latest
- Download link/instructions
- Changelog summary

**T7.4: Self-Update (Optional)**
- Agent self-update capability
- Staged rollout support
- Rollback on failure
- Requires Epic 23 completion

### Phase 8: Documentation & Testing (Weeks 13-14)

**T8.1: Installation Documentation**
- APT installation guide
- DNF installation guide
- Homebrew installation guide
- Windows installation guide
- Container deployment guide
- Helm deployment guide
- Binary installation guide

**T8.2: Verification Documentation**
- GPG signature verification
- Container signature verification
- Checksum verification
- Security best practices

**T8.3: Release Process Documentation**
- Internal release runbook
- Emergency release process
- Rollback procedures
- Post-release checklist

**T8.4: Testing**
- Test installation on all platforms
- Test upgrades on all platforms
- Test signature verification
- Test update notifications

## Dependencies

- **Epic 10** (Documentation) - Hugo site to deploy
- **Epic 12** (E2E Testing) - Test infrastructure for release testing
- **Epic 13** (CGO Removal) - Cross-compilation capability
- **Epic 20** (Windows Support) - Windows packages
- **Epic 23** (Self-Management) - Self-update capability (optional)

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Package repository costs escalate | Medium | Medium | Start with free tiers, monitor usage |
| Signing key compromise | Critical | Low | Hardware key, limited access, rotation plan |
| Repository downtime | High | Low | Multiple mirrors, CDN, fallback instructions |
| Breaking package manager policies | Medium | Medium | Follow guidelines, automated checks |
| Update mechanism causes issues | High | Low | Opt-in, staged rollout, easy disable |

## Cost Estimates

### Monthly Costs (Estimated)

| Service | Free Tier | Estimated Need | Cost |
|---------|-----------|----------------|------|
| Documentation hosting | Yes | Within free tier | $0 |
| Package repository | Limited | May need paid | $0-50 |
| Container registry | Yes | Within free tier | $0 |
| Helm repository | Yes | GitHub Pages | $0 |
| CDN (if needed) | Limited | Depends on traffic | $0-20 |
| **Total** | | | **$0-70/mo** |

### One-Time Costs

| Item | Cost |
|------|------|
| Domain name (if custom) | $10-15/year |
| Code signing certificate (Windows) | $0-200/year |
| Hardware security key (YubiKey) | $50-100 |

## Deliverables

1. **Documentation Site** - Public Hugo site with docs
2. **APT Repository** - Debian/Ubuntu packages
3. **DNF Repository** - RHEL/Fedora packages
4. **Homebrew Tap** - macOS packages
5. **Windows Packages** - Chocolatey, winget, MSI
6. **Container Images** - Multi-arch on public registry
7. **Helm Charts** - Published charts
8. **Release Automation** - GitHub Actions workflows
9. **Signing Infrastructure** - Keys and processes
10. **Installation Documentation** - Guides for all platforms

## Definition of Done

- [ ] Documentation site live on custom domain
- [ ] APT repository working with test install
- [ ] DNF repository working with test install
- [ ] Homebrew tap working with test install
- [ ] At least one Windows option working
- [ ] Container images on public registry
- [ ] Helm charts published
- [ ] Release workflow tested end-to-end
- [ ] All artifacts signed and verifiable
- [ ] Installation docs for all platforms
- [ ] Update check mechanism working
- [ ] Cost monitoring in place
