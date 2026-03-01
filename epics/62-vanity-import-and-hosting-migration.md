# Epic 62: Vanity Import Path and Hosting Migration

**Status**: PLANNED

## Overview

Decouple the project's Go module path and hosting infrastructure from GitHub. Adopt a vanity Go import path so the repository can be hosted anywhere (Codeberg, self-hosted Gitea, etc.) without breaking downstream consumers. Migrate CI/CD, container registry, and release infrastructure away from GitHub-specific services. Maintain a read-only GitHub mirror.

**Goal**: The project's identity is its own domain, not a hosting provider. `go get <vanity-domain>/keystone-core` works regardless of where the canonical repo lives.

> **IMPORTANT**: Before starting Phase 1, confirm the vanity import domain with the user. The Go module path is permanent once published — it cannot be changed without breaking all consumers.

## Inventory (Blast Radius)

Research across the full codebase reveals:

| Category | Files | Occurrences | Notes |
|----------|-------|-------------|-------|
| Go imports (`github.com/shawnbutts/keystone-core`) | 795 | 1,370 | cmd/, internal/, pkg/, test/ |
| go.mod module path | 1 | 1 | Root declaration |
| Makefile LDFLAGS | 1 | 3 | Version injection path |
| .goreleaser.yaml | 1 | 6 | LDFLAGS + homepage URLs |
| Documentation (.md) | 40 | 114 | Install guides, API refs, community docs |
| Helm Charts (.yaml) | 3 | 6 | Chart.yaml home/sources fields |
| CI workflows (.yml) | 1 | 1 | buf breaking check |
| OpenAPI spec | 1 | 1 | Server URL reference |
| Dockerfiles | 2 | 2 | E2E test container LDFLAGS |
| Issue/PR templates | 5 | — | .github/ directory |
| ghcr.io references | ~30 | 61 | Go code, K8s manifests, Helm values, docs |
| google/go-github dependency | 1 | — | internal/gitops/github/ |
| GitHub webhook handler | 1 | — | internal/gitops/webhook/github.go |
| GitHub Actions workflows | 6 | — | CI, release, e2e, benchmark, bootstrap, docs |

**Total estimated changes**: ~1,500 import rewrites + ~200 non-Go file edits + 6 CI workflow rewrites.

## Success Criteria

- [ ] Vanity Go import path resolves and `go get` works
- [ ] All Go imports rewritten; build and tests pass
- [ ] CI/CD runs on new platform (all 6 workflows ported)
- [ ] Container images published to non-GitHub registry
- [ ] Releases published to new platform
- [ ] GitHub API client replaced with Gitea SDK (or abstracted)
- [ ] GitHub webhook handler supports Gitea/Codeberg payloads
- [ ] Documentation updated with new URLs
- [ ] Read-only GitHub mirror syncs automatically
- [ ] No remaining hard dependency on GitHub for core operations

## Phases

### Phase 1: Vanity Import Path

Rewrite the Go module path to a domain-based vanity path. This is the foundational change — everything else depends on it.

> **Ask user**: What vanity import domain should we use? (e.g., `go.keystone-core.io/keystone-core`, `keystone-core.dev/core`, etc.)

**Prerequisites** (user must complete before we start):
- Domain registered and DNS configured
- Static hosting for `<meta name="go-import">` tags (can be a single HTML page, Caddy, or Cloudflare Worker)

**Changes**:

1. **Vanity import server setup** — Set up `<vanity-domain>` with `<meta name="go-import">` tag pointing to the canonical Git URL:
   ```html
   <meta name="go-import" content="<vanity-domain>/keystone-core git https://<repo-host>/<org>/keystone-core">
   ```

2. **go.mod** — Rewrite module path:
   ```
   module <vanity-domain>/keystone-core
   ```

3. **Bulk import rewrite** — All 795 Go files (~1,370 import lines):
   - `sed -i 's|github.com/shawnbutts/keystone-core|<vanity-domain>/keystone-core|g'` across cmd/, internal/, pkg/, test/
   - Verify with `go build ./...` and `go test ./...`

4. **Makefile** — Update 3 LDFLAGS references:
   - `MODULE` variable: `github.com/shawnbutts/keystone-core/pkg/version` → `<vanity-domain>/keystone-core/pkg/version`

5. **.goreleaser.yaml** — Update 6 references:
   - LDFLAGS module path (4 occurrences)
   - Homepage URLs (2 occurrences)

6. **Verification**:
   - `go build ./...` passes
   - `go test ./...` passes
   - `go get <vanity-domain>/keystone-core@latest` resolves correctly

**Tests**: Full build + test suite. No new tests needed — this is a mechanical rewrite.

---

### Phase 2: CI/CD Migration

Port all 6 GitHub Actions workflows to the new platform. The choice of CI platform affects the specifics.

**Decision needed**: Woodpecker CI (Codeberg-native), Gitea Actions (GitHub Actions-compatible), or self-hosted (Drone, Jenkins, etc.)

**Workflows to port** (6 total):

| Workflow | Jobs | Complexity | Notes |
|----------|------|------------|-------|
| ci.yml | lint, test, build, security (6 tools), license, semgrep, SBOM, fuzz, protobuf | HIGH | Most complex; 10+ jobs |
| release.yml | build, docker multi-arch, GitHub release | HIGH | Registry login, artifact publishing |
| e2e.yml | all-in-one, HA cluster, performance, integration | MEDIUM | Docker Compose orchestration |
| benchmark.yml | perf benchmarks, baseline comparison, PR comments | MEDIUM | Needs baseline storage |
| bootstrap-tests.yml | 5-platform Docker tests, VM tests | LOW | Mostly Docker-based |
| docs-validation.yml | doc inventory, links, examples, drift | LOW | Single job |

**Changes per workflow**:
- Replace `actions/checkout@v4` → platform equivalent
- Replace `actions/setup-go@v5` → manual Go install or platform action
- Replace `actions/cache@v4` → platform cache or manual
- Replace `codecov/codecov-action@v4` → codecov CLI upload
- Replace `github.repository`, `secrets.GITHUB_TOKEN` → platform equivalents
- Replace `github-script` PR comments → platform API calls
- Configure platform secrets (registry credentials, tokens)

**If Gitea Actions chosen**: ~70% of workflow YAML is reusable (compatible syntax). Main changes are context variables and marketplace actions.

**If Woodpecker CI chosen**: Full rewrite of all workflows to Woodpecker pipeline format.

---

### Phase 3: Container Registry Migration

Move container images from `ghcr.io` to a new registry.

**Decision needed**: Docker Hub, Quay.io, Codeberg Packages, or self-hosted (Harbor).

**Files to update** (61 ghcr.io references):

| Location | Files | What Changes |
|----------|-------|-------------|
| Go source code | 3 | `internal/selfmgmt/kscore_agent.go`, `kscore_server.go`, `internal/bootstrap/installer.go` |
| Registry auth detection | 1 | `internal/registry/auth.go` — add new registry type or generalize |
| K8s deployments | 4 | `deploy/kubernetes/*/deployment.yaml` and `kustomization.yaml` |
| Helm values | 3 | `deploy/helm/*/values.yaml` — `image.repository` field |
| Docker Compose | 1 | `deploy/gateway/docker-compose.minimal.yml` |
| GoReleaser | 1 | `.goreleaser.yaml` — docker image names |
| Release workflow | 1 | Docker login + push targets |
| Documentation | ~8 | Install guides, deployment docs, release guide |
| Tests | 4 | Registry auth tests, OCI client tests, bootstrap tests |

**Changes**:
- Bulk-replace `ghcr.io/shawnbutts/keystone-core` → `<new-registry>/<org>/keystone-core`
- Update `internal/registry/auth.go` to support new registry type detection
- Update `internal/bootstrap/installer.go` image URL construction
- Update CI release workflow for new registry login

---

### Phase 4: GitHub API → Gitea API

Replace the GitHub-specific GitOps integration with a Gitea/Codeberg-compatible client.

**Files to modify**:

| File | Change |
|------|--------|
| `go.mod` | Replace `github.com/google/go-github/v57` with `code.gitea.io/sdk/gitea` |
| `internal/gitops/github/client.go` | Rewrite to use Gitea SDK (or create abstraction layer) |
| `internal/gitops/github/types.go` | Update default BaseURL and config types |
| `internal/gitops/github/client_test.go` | Rewrite tests for Gitea API |
| `internal/gitops/webhook/github.go` | Add Gitea/Codeberg webhook payload parsing |
| `internal/config/config.go` | Update webhook handler list (keep "github" for backwards compat, add "gitea") |
| `pkg/api/webhooks/handlers.go` | Add "gitea" to supported webhook types |
| CLI commands | Update examples in kscore-webhook and kscore-gitops |
| Documentation | Update webhook setup guides |

**Design choice**: Either:
- (A) Replace GitHub client entirely with Gitea client (simpler, breaking)
- (B) Abstract behind an interface supporting both (more work, non-breaking)

Option B is recommended since users may use GitHub, GitLab, or Gitea forges.

**New tests**: Gitea API client tests, Gitea webhook payload parsing tests.

---

### Phase 5: Release Infrastructure

Update GoReleaser and release workflow to publish to the new platform.

**Changes**:

1. **.goreleaser.yaml**:
   - Change release target from GitHub to Gitea (or generic HTTP upload)
   - Update changelog generation (GitHub API → git log or Gitea API)
   - Update package metadata homepage URLs (3 locations)
   - Update Docker image references (covered in Phase 3)

2. **Release workflow**:
   - Login to new container registry
   - Upload release artifacts to new platform
   - Generate changelog from Gitea API or git log
   - Sign artifacts (cosign — registry-agnostic)

3. **Makefile**:
   - Update `release` target if needed
   - Ensure `GITHUB_TOKEN` references are replaced

4. **Verification**:
   - `make release-dry-run` succeeds
   - Snapshot release builds correctly

---

### Phase 6: Documentation and Templates

Update all user-facing documentation and project templates.

**Documentation** (40 files, 114 occurrences):

| Category | Files | Key Changes |
|----------|-------|-------------|
| Installation guides | 3 | Download URLs, `go install` path, package repo |
| API references | 3 | OpenAPI spec URL, module import examples |
| Operations guides | 6 | Deployment examples, webhook setup, troubleshooting |
| Community docs | 10 | Contributing, governance, support, roadmap — all GitHub URLs |
| Concept docs | 4 | GitOps, K8s, observability, identity |
| Module SDK docs | 2 | Go import path examples |
| Other | 12 | Various references |

**Templates**:
- Move `.github/ISSUE_TEMPLATE/` → `.gitea/issue_template/` (if Gitea/Codeberg)
- Move `.github/PULL_REQUEST_TEMPLATE.md` → `.gitea/pull_request_template.md`
- Keep `.github/` directory for GitHub mirror (different content: "This is a mirror" notice)

**Bulk operations**:
- Replace `github.com/shawnbutts/keystone-core` URLs in all .md files
- Replace GitHub-specific feature references (Discussions → Codeberg equivalent)
- Update community links (Issues, PRs, wiki)

---

### Phase 7: GitHub Mirror

Set up read-only mirror on GitHub.

**Changes**:
1. Configure push mirror from canonical repo to GitHub (Codeberg settings or git hook)
2. Add mirror notice to GitHub repo README
3. Optionally disable GitHub Issues/PRs to direct contributors to canonical repo
4. Keep `.github/` directory with minimal workflows (redirect notice, mirror sync)

**Verification**:
- Push to canonical repo → appears on GitHub mirror
- GitHub README shows mirror notice with link to canonical repo

---

## Dependencies

- Vanity import domain must be registered and serving `<meta>` tags before Phase 1
- CI platform choice must be made before Phase 2
- Container registry choice must be made before Phase 3
- Phases 1-3 are sequential (import path → CI → registry)
- Phases 4-5 can run in parallel after Phase 3
- Phase 6 can start after Phase 1 (doesn't depend on CI/registry)
- Phase 7 is last (needs everything else complete)

## Risks

- **Import path is permanent**: Once published to a proxy (proxy.golang.org), the vanity path must be maintained forever. Choose carefully.
- **CI migration complexity**: 6 workflows with ~10+ jobs total. GitHub Actions marketplace actions have no direct equivalents on other platforms.
- **Mirror drift**: If GitHub mirror setup breaks silently, downstream users on GitHub won't get updates.
- **Webhook backwards compatibility**: Existing users with GitHub webhooks configured will need to update if the handler path changes.
- **DNS dependency**: Vanity imports add a DNS dependency. If the domain lapses, `go get` breaks for all consumers.
