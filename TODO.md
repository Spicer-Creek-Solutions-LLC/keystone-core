# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Resolution Tags

Each TODO item includes a `Resolution:` line to indicate how it should be addressed:

- `doc` — update documentation to match current code behavior.
- `code` — update code to add the documented behavior and update documents to new behavior.
- `both` — update both docs and code.
- `decide` — needs triage to choose a direction.

---

## Open Items

### Re-enable gosec G115 after golangci-lint upgrades gosec

**Resolution:** code

`gosec v2.23.0` (bundled in `golangci-lint v2.10.1`) panics on float-to-int casts (`"10000 not an Int"` in `internal/files/mirror/geo.go:271`). Fixed upstream in gosec v2.24.0 ([securego/gosec#1229](https://github.com/securego/gosec/issues/1229), [#1501](https://github.com/securego/gosec/issues/1501)), but golangci-lint v2.10.1 still bundles v2.23.0.

G115 is excluded in `.golangci.yml`. Re-enable when golangci-lint ships a version bundling gosec >= v2.24.0.

Taint analysis rules (G117, G702-G706) are **permanently excluded** — they flag legitimate infrastructure operations by design. See `.golangci.yml` comment.

**Tasks:**

- [x] Monitor gosec releases for G115 panic fix — fixed in gosec v2.24.0 (Feb 2026)
- [ ] Re-enable G115 when golangci-lint bundles gosec >= v2.24.0
- [x] Re-evaluate taint analysis rules (G117, G702-G706) — permanently excluded; these flag legitimate infrastructure operations

### Migrate primary hosting to Codeberg with GitHub mirror

**Resolution:** both

Move the canonical repository from GitHub to Codeberg, adopt a vanity Go import path (`go.keystone-core.io/keystone-core`), and maintain a read-only mirror on GitHub.

**Phase 1: Vanity import path**

- [ ] Register/configure `keystone-core.io` domain
- [ ] Set up `go.keystone-core.io` with `<meta name="go-import">` tags pointing to Codeberg
- [ ] Rewrite `go.mod` module path to `go.keystone-core.io/keystone-core`
- [ ] Bulk-rewrite all Go imports across the codebase (~1000+ files)
- [ ] Update LDFLAGS module path in `Makefile` and `.goreleaser.yaml`
- [ ] Verify build, tests, and `go get` resolution all pass

**Phase 2: CI/CD migration**

- [ ] Evaluate Woodpecker CI vs Gitea Actions for Codeberg CI
- [ ] Rewrite 6 GitHub Actions workflows (ci, release, benchmark, bootstrap-tests, docs-validation, e2e) for chosen CI
- [ ] Configure CI secrets and context variables on Codeberg
- [ ] Verify all CI pipelines pass on Codeberg

**Phase 3: Container registry migration**

- [ ] Choose replacement registry (Codeberg Packages, Docker Hub, Quay.io, or self-hosted)
- [ ] Update 61 `ghcr.io` references across K8s manifests, Helm values, docker-compose, Go code, goreleaser
- [ ] Update `internal/bootstrap/installer.go` image URL construction
- [ ] Update `internal/registry/auth.go` registry authentication logic
- [ ] Verify image build, push, and pull with new registry

**Phase 4: GitHub API → Gitea API**

- [ ] Replace `google/go-github` with `code.gitea.io/sdk/gitea` in `internal/gitops/github/client.go`
- [ ] Rewrite webhook handler in `internal/gitops/webhook/github.go` for Gitea/Codeberg payloads
- [ ] Update webhook endpoint path and documentation
- [ ] Add tests for new Gitea API client and webhook handler

**Phase 5: Release infrastructure**

- [ ] Update `.goreleaser.yaml`: change `use: github` → `use: gitea`, update release owner/repo
- [ ] Update package metadata homepage URLs (3 locations) to Codeberg
- [ ] Verify release pipeline publishes to Codeberg

**Phase 6: Documentation and templates**

- [ ] Bulk-replace `github.com/shawnbutts/keystone-core` URLs across ~539 doc files
- [ ] Update references to GitHub-specific features (Discussions, Actions) with Codeberg equivalents
- [ ] Move issue/PR templates from `.github/` to `.gitea/` format (keep `.github/` for mirror)
- [ ] Update `README.md`, `AGENTS.md`, and community docs with Codeberg links

**Phase 7: GitHub mirror**

- [ ] Create Codeberg push mirror to GitHub (repo settings → mirror)
- [ ] Verify code syncs to GitHub on push
- [ ] Add note to GitHub repo README indicating Codeberg is the canonical source
- [ ] Optionally disable GitHub Issues/PRs to direct contributors to Codeberg
