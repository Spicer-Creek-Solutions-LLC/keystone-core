# Keystone Core — project Makefile.
#
# Targets are grouped by purpose. Run `make` or `make help` to list them.
# Additional targets (proto*, dev*, release*, e2e*) are added by their
# owning tasks/epics — see the comment block at the bottom of this file.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ---- Module + binaries ----------------------------------------------------

MODULE   := go.keystone-core.io/keystone-core

# Binaries are auto-detected from cmd/. Drop a new dir under cmd/ and it
# joins the build set automatically — no Makefile edit required.
BINARIES := $(notdir $(wildcard cmd/*))

# ---- Build metadata (consumed by pkg/version via -ldflags -X) -------------

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS    := -X $(MODULE)/pkg/version.Version=$(VERSION) \
              -X $(MODULE)/pkg/version.GitCommit=$(GIT_COMMIT) \
              -X $(MODULE)/pkg/version.BuildDate=$(BUILD_DATE)

# ---- Cross-compile matrix --------------------------------------------------

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# ---- Project-wide flags ----------------------------------------------------

# Production builds are pure-Go (PROJECT-DETAILS §3.1). Tests override this
# to CGO_ENABLED=1 so the race detector works.
export CGO_ENABLED := 0

# ---- Phony declarations ---------------------------------------------------

.PHONY: help \
        build build-all-platforms clean clean-all clean-check deps install-tools install-lychee install-hugo docs-site docs-links-site \
        test test-verbose test-coverage coverage-gate race-policy goleak-policy docs-sync docs-sync-check test-integration slo profile test-cross-distro check deps deps-outdated deps-outdated-issue \
        fmt lint lint-fix smoke test-packaging \
        proto proto-lint proto-breaking \
        openapi-lint \
        docs-lint docs-lint-fix docs-lint-container docs-links docs-links-online \
        changelog-new changelog-preview changelog-batch \
        dev dev-server dev-agent \
        e2e-build e2e-up e2e-down e2e-logs e2e-test e2e-test-docker \
        release-snapshot release release-dry-run release-config-check release-smoke \
        security-secrets security-vulns security-sast security-licenses

# ---- Help (default) -------------------------------------------------------

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---- Build ----------------------------------------------------------------

build: ## Build all binaries for the host platform
	@if [ -z "$(BINARIES)" ]; then \
		echo "no binaries in cmd/ yet (added by task 13)"; \
		exit 0; \
	fi
	@for bin in $(BINARIES); do \
		os=$$(go env GOOS); arch=$$(go env GOARCH); \
		echo ">>> build $$os/$$arch $$bin"; \
		go build -ldflags="$(LDFLAGS)" \
			-o build/bin/$$os/$$arch/$$bin ./cmd/$$bin || exit $$?; \
	done

build-all-platforms: ## Cross-compile all binaries for the v1.0 platform matrix
	@if [ -z "$(BINARIES)" ]; then \
		echo "no binaries in cmd/ yet (added by task 13)"; \
		exit 0; \
	fi
	@for plat in $(PLATFORMS); do \
		os=$${plat%/*}; arch=$${plat#*/}; \
		ext=""; if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		for bin in $(BINARIES); do \
			echo ">>> build $$os/$$arch $$bin$$ext"; \
			GOOS=$$os GOARCH=$$arch go build -ldflags="$(LDFLAGS)" \
				-o build/bin/$$os/$$arch/$$bin$$ext ./cmd/$$bin || exit $$?; \
		done; \
	done

clean: ## Remove build artifacts and runtime state
	# Build outputs (make build, make release, goreleaser).
	rm -rf build/ dist/
	# Root strays from ad-hoc `go build ./cmd/...` (gitignored as
	# /kscore-* + /kscorectl + /trackerctl). `make build` writes to
	# build/bin/$$GOOS/$$GOARCH/ — these only appear when someone
	# bypasses the Makefile. See docs/project/DEVELOPMENT.md.
	rm -f kscore-agent kscore-backup kscore-migrate kscore-server kscorectl trackerctl
	# Test artifacts.
	rm -f coverage.out *.test
	rm -rf reports/ test/e2e/performance/reports/ internal/loadtest/reports/
	# Per-tool binaries (gitignored).
	rm -f tools/moddoc/moddoc scripts/docvalidation/docvalidation docvalidation
	# Runtime state (integration tests, dev mode).
	rm -rf data/ tmp/ temp/
	rm -f *.db *.db-shm *.db-wal
	rm -f kscore-agent.yaml *.creds
	# Editor/OS junk.
	rm -f *.bak *.tmp .DS_Store
	# Hugo (post-v1.0, but already gitignored).
	rm -rf docs/.hugo_build.lock docs/resources docs/node_modules docs/package-lock.json

clean-all: clean ## clean + scan caches (slow re-download on next security scan)
	# .cache/ holds grype / trivy / semgrep scan DBs etc. Containerized
	# scan tooling may write it as root (via `docker run -v $PWD/.cache:...`);
	# in that case rm cannot remove the files even with chmod (you can't
	# chmod what you don't own). Detect up front and fail with one clear
	# line instead of hundreds of `Permission denied` errors.
	@if [ -d .cache ] && [ -n "$$(find .cache -not -user "$$(id -un)" -print -quit 2>/dev/null)" ]; then \
	  echo "clean-all: .cache/ contains files not owned by $$(id -un) (likely from a Docker-run scan)."; \
	  echo "  Run 'sudo rm -rf .cache/' to remove."; \
	  exit 1; \
	fi
	# All entries are current-user-owned; chmod handles any read-only
	# bits, then rm -rf wipes the tree.
	@chmod -R u+w .cache/ 2>/dev/null || true
	rm -rf .cache/

clean-check: ## Assert repo is free of stray build artifacts (CI lint gate)
	# Catches the "I ran 'go build ./cmd/foo' from repo root" footgun.
	# `make build` writes to build/bin/$$GOOS/$$GOARCH/$$bin — anything
	# at repo root means someone bypassed the Makefile. See
	# docs/project/DEVELOPMENT.md "Build artifact discipline".
	@strays=""; \
	for f in kscore-agent kscore-backup kscore-migrate kscore-server kscorectl trackerctl coverage.out; do \
	  [ -e "$$f" ] && strays="$$strays $$f"; \
	done; \
	for t in tools/moddoc/moddoc scripts/docvalidation/docvalidation docvalidation; do \
	  [ -e "$$t" ] && strays="$$strays $$t"; \
	done; \
	for f in *.test; do \
	  [ -e "$$f" ] && strays="$$strays $$f"; \
	done; \
	if [ -n "$$strays" ]; then \
	  echo "clean-check: FAIL — stray build artifacts present (run 'make clean'):"; \
	  for s in $$strays; do echo "  $$s"; done; \
	  echo ""; \
	  echo "Hint: do not run 'go build ./cmd/foo' from repo root."; \
	  echo "Use 'make build' (writes to build/bin/\$$GOOS/\$$GOARCH/)"; \
	  echo "or 'go build -o build/bin/\$$(go env GOOS)/\$$(go env GOARCH)/foo ./cmd/foo'."; \
	  echo "See docs/project/DEVELOPMENT.md."; \
	  exit 1; \
	fi; \
	echo "clean-check: ok"

deps: ## Download and verify Go module dependencies
	go mod download
	go mod verify

deps-outdated: ## Report direct deps with newer releases (informational; runs nightly in ci-full)
	@go run ./tools/depsoutdated

deps-outdated-issue: ## deps-outdated + sync a tracking issue via the Forgejo API (ci-full nightly; needs GITHUB_* env)
	@go run ./tools/depsoutdated --issue

install-tools: ## Install dev tools (Go-installable + lychee binary)
	@command -v golangci-lint >/dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@command -v gosec >/dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	@command -v govulncheck >/dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	@command -v buf >/dev/null || go install github.com/bufbuild/buf/cmd/buf@latest
	@# Pin the proto generators to specific versions: their output is
	@# committed under pkg/api/v1/, so a moving @latest would drift the
	@# version-stamp comment in *_grpc.pb.go / *.pb.go and trip the
	@# proto-job's `git diff --exit-code` gate on fresh CI runs.
	@command -v protoc-gen-go >/dev/null || go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	@command -v protoc-gen-go-grpc >/dev/null || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1
	@command -v goreleaser >/dev/null || go install github.com/goreleaser/goreleaser/v2@latest
	@command -v gitleaks >/dev/null || go install github.com/zricethezav/gitleaks/v8@latest
	@# Pinned to go-licenses v2 (the /v2 module path): the v1 @latest
	@# (v1.6.0) fails to load a Go 1.26 project — every stdlib package
	@# reports "does not have module info" (google/go-licenses#128) and the
	@# check aborts. CI is green only because its runners cache an older v1
	@# binary; a fresh runner would pull the broken @latest. v2.0.1 is the
	@# current release and loads cleanly under Go 1.26 with the same check
	@# flags (the binary is still named go-licenses).
	@command -v go-licenses >/dev/null || go install github.com/google/go-licenses/v2@v2.0.1
	@# Syft is pinned because SBOM output drift across syft versions
	@# would change the byte-content of `sbom-vX.Y.Z.{spdx,cyclonedx}.json`
	@# release artifacts even when the dep tree is unchanged. Pin matches
	@# the RELEASE-PLAYBOOK §3 v0.1.0 release record's syft version.
	@command -v syft >/dev/null || go install github.com/anchore/syft/cmd/syft@v1.44.0
	@command -v vangen >/dev/null || go install 4d63.com/vangen@latest
	@command -v changie >/dev/null || go install github.com/miniscruff/changie@latest
	@command -v lychee >/dev/null || $(MAKE) --no-print-directory install-lychee
	@# Hugo Extended for the docs site. Check $(GOPATH)/bin/hugo
	@# specifically (a non-extended `hugo` may shadow it on PATH).
	@hb="$$(go env GOPATH)/bin/hugo"; { [ -x "$$hb" ] && "$$hb" version 2>/dev/null | grep -q extended; } || $(MAKE) --no-print-directory install-hugo

# Lychee is a Rust binary, not Go-installable. Pull the prebuilt release for
# the current GOOS/GOARCH so docs-links can run without docker — the Forgejo
# runner image lacks docker; ubuntu-latest GitHub runners ship it but pulling
# the small native binary is faster than the docker image either way.
LYCHEE_VERSION ?= lychee-v0.24.2

install-lychee: ## Install pinned lychee binary into $(GOPATH)/bin
	@goos="$$(go env GOOS)"; goarch="$$(go env GOARCH)"; \
	case "$$goos-$$goarch" in \
	  linux-amd64)  target="x86_64-unknown-linux-musl" ;; \
	  linux-arm64)  target="aarch64-unknown-linux-musl" ;; \
	  darwin-amd64) target="x86_64-apple-darwin" ;; \
	  darwin-arm64) target="aarch64-apple-darwin" ;; \
	  *) echo "lychee: unsupported $$goos-$$goarch (manual install required)"; exit 1 ;; \
	esac; \
	url="https://github.com/lycheeverse/lychee/releases/download/$(LYCHEE_VERSION)/lychee-$$target.tar.gz"; \
	dest="$$(go env GOPATH)/bin"; mkdir -p "$$dest"; \
	tmp="$$(mktemp -d)"; trap "rm -rf $$tmp" EXIT; \
	echo "install-lychee: downloading $(LYCHEE_VERSION) for $$target"; \
	curl -sSfL "$$url" -o "$$tmp/lychee.tar.gz"; \
	tar -xzf "$$tmp/lychee.tar.gz" -C "$$tmp"; \
	bin="$$(find "$$tmp" -name lychee -type f -perm -u+x | head -1)"; \
	if [ -z "$$bin" ]; then echo "install-lychee: binary not found in tarball"; exit 1; fi; \
	mv "$$bin" "$$dest/lychee"; \
	echo "install-lychee: installed $$dest/lychee"; \
	"$$dest/lychee" --version

# Hugo Extended is required to build the docs site (the Hextra theme
# compiles SCSS, which the standard Hugo build can't do). Pinned so the
# rendered site is reproducible across dev + CI. Installed into
# $(GOPATH)/bin so `make docs-site` finds it ahead of any non-extended
# `hugo` elsewhere on PATH.
HUGO_VERSION ?= 0.154.5

install-hugo: ## Install pinned Hugo Extended into $(GOPATH)/bin
	@goos="$$(go env GOOS)"; goarch="$$(go env GOARCH)"; \
	case "$$goos-$$goarch" in \
	  linux-amd64)  target="linux-amd64" ;; \
	  linux-arm64)  target="linux-arm64" ;; \
	  darwin-amd64) target="darwin-universal" ;; \
	  darwin-arm64) target="darwin-universal" ;; \
	  *) echo "hugo: unsupported $$goos-$$goarch (manual install required)"; exit 1 ;; \
	esac; \
	url="https://github.com/gohugoio/hugo/releases/download/v$(HUGO_VERSION)/hugo_extended_$(HUGO_VERSION)_$$target.tar.gz"; \
	dest="$$(go env GOPATH)/bin"; mkdir -p "$$dest"; \
	tmp="$$(mktemp -d)"; trap "rm -rf $$tmp" EXIT; \
	echo "install-hugo: downloading hugo_extended $(HUGO_VERSION) for $$target"; \
	curl -sSfL "$$url" -o "$$tmp/hugo.tar.gz"; \
	tar -xzf "$$tmp/hugo.tar.gz" -C "$$tmp"; \
	if [ ! -f "$$tmp/hugo" ]; then echo "install-hugo: binary not found in tarball"; exit 1; fi; \
	mv "$$tmp/hugo" "$$dest/hugo"; \
	echo "install-hugo: installed $$dest/hugo"; \
	"$$dest/hugo" version

# ---- Test -----------------------------------------------------------------

test: ## Run unit tests with -race
	CGO_ENABLED=1 go test -race ./...

test-verbose: ## Run unit tests with -race -v
	CGO_ENABLED=1 go test -race -v ./...

test-coverage: ## Run tests with coverage profile and per-function output
	CGO_ENABLED=1 go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out

coverage-gate: ## Enforce per-package coverage gates (critical >=70%, CLI >=40%)
	go run ./tools/covgate --profile=coverage.out

race-policy: ## Enforce -race on every `go test` (docs/project/TEST-POLICY.md)
	go run ./tools/racegate

goleak-policy: ## Enforce TestMain-with-goleak in every integration test package (docs/project/TEST-POLICY.md)
	go run ./tools/goleakgate

docs-sync: ## Regenerate auto-generated reference docs (CLI / config / API)
	# Sources of truth: cmd/kscore-* --help, internal/config/*.go,
	# api/proto/keystone/core/v1/*.proto + api/openapi/openapi-spec.yaml.
	# Run after touching any of those; CI fails if the checked-in
	# output drifts from the regenerated version.
	go run ./tools/gendocs/cli    > docs/project/CLI-REFERENCE.md
	go run ./tools/gendocs/config > docs/project/CONFIGURATION-REFERENCE.md
	go run ./tools/gendocs/api    > docs/project/API-REFERENCE.md

docs-sync-check: ## Assert auto-generated reference docs are in sync with the generators
	@tmpdir=$$(mktemp -d) && \
		go run ./tools/gendocs/cli    > $$tmpdir/CLI.md && \
		go run ./tools/gendocs/config > $$tmpdir/CONFIGURATION.md && \
		go run ./tools/gendocs/api    > $$tmpdir/API.md && \
		diff -q docs/project/CLI-REFERENCE.md           $$tmpdir/CLI.md && \
		diff -q docs/project/CONFIGURATION-REFERENCE.md $$tmpdir/CONFIGURATION.md && \
		diff -q docs/project/API-REFERENCE.md           $$tmpdir/API.md && \
		echo "docs-sync-check: ok"

docs-site: ## Build the Hugo documentation site (Hextra) to docs/public/
	# Renders the canonical Markdown (docs/project, docs/runbooks,
	# docs/adr — mounted in place via docs/hugo.toml) into a searchable
	# static site. Needs Hugo Extended (run `make install-hugo`). The
	# Hextra theme is fetched as a Hugo module per docs/go.mod.
	@hugo="$$(go env GOPATH)/bin/hugo"; \
	[ -x "$$hugo" ] || hugo=hugo; \
	"$$hugo" version 2>/dev/null | grep -q extended || { \
	  echo "docs-site: Hugo Extended required — run 'make install-hugo'"; exit 1; }; \
	cd docs && "$$hugo" --gc --minify && \
	echo "docs-site: built docs/public/"

docs-links-site: ## Link-check the rendered Hugo site (lychee over docs/public/)
	# Builds the site, then checks the rendered links offline. A render
	# hook rewrites the canonical docs' relative links to in-site URLs or
	# absolute Codeberg source URLs, so the rendered site is self-consistent
	# (the external Codeberg/GitHub URLs are skipped in offline mode).
	# Not --config .lychee.toml: that file excludes docs/public (so the
	# source `docs-links` gate skips the build output). --offline already
	# skips every external URL (the config's excludes are all external
	# domains), so the rendered-site check only verifies local links.
	@$(MAKE) --no-print-directory docs-site >/dev/null
	@if command -v lychee >/dev/null 2>&1; then \
		lychee --no-progress --offline --root-dir "$(CURDIR)/docs/public" "docs/public/**/*.html"; \
	else \
		echo "docs-links-site: needs lychee on PATH (run 'make install-tools')"; exit 1; \
	fi

vanity-regen: ## Regenerate the Go-vanity-import static HTML under deploy/vanity/site/
	# Source of truth: deploy/vanity/vangen.json. Output lands under
	# deploy/vanity/site/ and is committed alongside the config so deploys
	# don't need vangen installed. See deploy/vanity/README.md for the
	# end-to-end story.
	vangen -config deploy/vanity/vangen.json -out deploy/vanity/site/

vanity-regen-check: ## Assert committed vanity HTML matches what vangen would produce now
	@tmpdir=$$(mktemp -d) && \
		vangen -config deploy/vanity/vangen.json -out $$tmpdir/dist/ && \
		diff -ruq deploy/vanity/site/ $$tmpdir/dist/ && \
		echo "vanity-regen-check: ok"

test-integration: ## Run integration tests (-tags=integration)
	# -p=1 forces test binaries to run sequentially. Integration tests
	# in different packages share the KSCORE_TEST_POSTGRES_DSN target
	# and would otherwise race on TRUNCATE / seed / read.
	CGO_ENABLED=1 go test -race -tags=integration -p=1 ./...

slo: ## Verify v1.0 performance SLOs (-tags=slo, NO -race)
	# Two suites under one gate:
	#   - test/e2e/ha/   Epic 13 task 18: cluster-formation SLOs
	#                    (first leader <3s, failover <5s/10s,
	#                    minority-block <1s, recovery <15s, ...).
	#   - test/e2e/perf/ Epic 19 task 3: command/event/batch SLOs
	#                    (single-agent command latency <100ms,
	#                    event throughput >10k/s, 10-agent batch
	#                    exec <2s).
	# Both run NOT -race: race instrumentation inflates wall-clock
	# 2-10x, which would make the asserted numbers meaningless. The
	# functional in--race smoke lives in the per-domain integration
	# tests (make test-integration).
	CGO_ENABLED=1 go test -tags=slo -count=1 -timeout=300s ./test/e2e/ha/... ./test/e2e/perf/...

profile: ## pprof against the perf SLO workload — captures CPU + heap, reports top 20 each (docs/project/PROFILING-BASELINE.md)
	# Epic 19 task 8 hardening pass. The perf SLO tests are the
	# repeatable, representative workload — command latency, event
	# throughput, batch fan-out. Profiles land at /tmp so they don't
	# pollute the workspace; rerun + analyse via `go tool pprof`.
	CGO_ENABLED=1 go test -tags=slo -count=1 -timeout=300s \
		-cpuprofile=/tmp/kscore-profile.cpu \
		-memprofile=/tmp/kscore-profile.mem \
		./test/e2e/perf/...
	@echo ""
	@echo "Top 20 cumulative CPU consumers:"
	@go tool pprof -top -cum /tmp/kscore-profile.cpu 2>/dev/null | head -25
	@echo ""
	@echo "Top 20 allocation sites:"
	@go tool pprof -top -alloc_space /tmp/kscore-profile.mem 2>/dev/null | head -25
	@echo ""
	@echo "Interactive: go tool pprof /tmp/kscore-profile.{cpu,mem}"

test-cross-distro: ## Run the state stdlib smoke across the distro matrix (privileged Docker; gated)
	# Epic 08 cross-distro matrix — boots each distro (Debian 12,
	# Ubuntu 22.04/24.04, Rocky 9, Alpine 3.19) in a privileged
	# container with its real init system and applies the state smoke
	# fixtures twice, exercising the package (apt/dnf/apk) and service
	# (systemd/OpenRC) backends live + asserting idempotency. Skips
	# cleanly without Docker. Manual / Docker-host gate — NOT wired into
	# CI (privileged). Pass distro names to narrow, e.g.
	# `bash test/e2e/state/run.sh alpine-3-19`.
	@bash test/e2e/state/run.sh

check: lint docs-lint test ## Run lint + docs-lint + tests

smoke: ## Run quick smoke checks (compile + SQLite pragmas)
	scripts/smoke-test.sh quick

test-packaging: ## Run the postinst-scriptlet unit tests (no root needed)
	bash deploy/packaging/postinst_test.sh

# ---- Lint / Format --------------------------------------------------------

fmt: ## Format Go code and tidy go.mod
	@dirs="$(wildcard pkg internal cmd api tools)"; \
	if [ -n "$$dirs" ]; then \
		find $$dirs -name '*.go' -print0 | xargs -0 -r gofmt -w; \
	fi
	go mod tidy

lint: ## Run golangci-lint
	golangci-lint run ./...

lint-fix: ## Auto-fix lint issues where possible
	golangci-lint run --fix ./...

# ---- Proto ----------------------------------------------------------------

proto: ## Generate Go + gRPC stubs from proto files
	buf generate

proto-lint: ## Lint proto files (buf STANDARD)
	buf lint

proto-breaking: ## Check protos for breaking changes vs. previous commit
	# Compare against HEAD~1 rather than `branch=main`: this project
	# pushes directly to main, so on push `branch=main` resolves to
	# the just-pushed commit (i.e., self vs. self — no diff). HEAD~1
	# is the right semantic for "what was this before this push?".
	buf breaking --against '.git#ref=HEAD~1'

# ---- OpenAPI --------------------------------------------------------------

openapi-lint: ## Lint api/openapi/openapi-spec.yaml via redocly
	@command -v npx >/dev/null || { \
		echo "ERROR: openapi-lint needs npm/npx (install Node.js)"; \
		exit 1; \
	}
	npx --yes @redocly/cli@latest lint api/openapi/openapi-spec.yaml

# ---- Docs -----------------------------------------------------------------

# markdownlint-cli2 reads .markdownlint-cli2.yaml (rules + the docs/**/*.md
# glob). docs-lint needs Node locally; docs-lint-container runs the same thing
# in node:22-alpine for hosts without Node (CI runs docs-lint directly).

docs-lint: ## Lint Markdown docs via markdownlint-cli2 (.markdownlint-cli2.yaml)
	@command -v npx >/dev/null || { \
		echo "ERROR: docs-lint needs npm/npx (install Node.js), or run 'make docs-lint-container'"; \
		exit 1; \
	}
	npx --yes markdownlint-cli2

docs-lint-fix: ## Auto-fix Markdown lint issues where possible
	@command -v npx >/dev/null || { \
		echo "ERROR: docs-lint-fix needs npm/npx (install Node.js)"; \
		exit 1; \
	}
	npx --yes markdownlint-cli2 --fix

docs-lint-container: ## Lint Markdown docs in a node:22-alpine container (no local Node needed)
	@cre="$$(command -v docker || command -v podman)"; \
	if [ -z "$$cre" ]; then \
		echo "ERROR: docs-lint-container needs docker or podman; use 'make docs-lint' if you have Node"; \
		exit 1; \
	fi; \
	"$$cre" run --rm -v "$(CURDIR)":/repo:ro -w /repo node:22-alpine npx --yes markdownlint-cli2

# Markdown link-health check (Epic 19 task 13 follow-up — Phase A3 of
# the public-launch checklist). Runs lychee inside its official
# container against every .md file using .lychee.toml. Offline mode
# checks only local + relative refs (CI gate, deterministic).
# docs-links-online additionally checks external URLs — slower, can
# flake on rate limits, so it's not a CI gate; run manually.
docs-links: ## Check internal/relative .md links via lychee (offline; CI gate)
	@if command -v lychee >/dev/null 2>&1; then \
		lychee --config .lychee.toml --no-progress --offline "**/*.md"; \
	elif cre="$$(command -v docker || command -v podman)"; [ -n "$$cre" ]; then \
		"$$cre" run --rm -v "$(CURDIR)":/workspace:ro \
			lycheeverse/lychee:latest \
			--config /workspace/.lychee.toml \
			--no-progress --offline \
			"/workspace/**/*.md"; \
	else \
		echo "ERROR: docs-links needs lychee on PATH (run 'make install-tools') or docker/podman"; \
		exit 1; \
	fi

# ---- Changelog (changie fragments) ---------------------------------------
#
# Per-PR entries live as YAML fragments under .changes/unreleased/ instead of
# direct edits to CHANGELOG.md's [Unreleased] section. This eliminates the
# mechanical merge conflicts that hit every concurrent PR touching the same
# section anchor. See .changie.yaml + CONTRIBUTING.md § Changelog entries.

changelog-new: ## Draft a new changelog fragment under .changes/unreleased/
	@command -v changie >/dev/null || { \
		echo "ERROR: changie not installed; run 'make install-tools'"; exit 1; }
	@changie new

changelog-preview: ## Preview the accumulated unreleased section (dry-run)
	@command -v changie >/dev/null || { \
		echo "ERROR: changie not installed; run 'make install-tools'"; exit 1; }
	@# `minor` is the version-increment keyword for preview only — actual
	@# release version is supplied via `make changelog-batch VERSION=...`.
	@changie batch minor --dry-run

# changelog-batch: invoked at release time. VERSION must be supplied
# (e.g., make changelog-batch VERSION=v0.1.0). Writes the aggregated
# section into CHANGELOG.md and archives the fragments under
# .changes/<version>/. Re-stage CHANGELOG.md and the archived fragments
# in the release-prep commit.
changelog-batch: ## Aggregate fragments into CHANGELOG.md for a release. Requires VERSION=v0.x.y
	@command -v changie >/dev/null || { \
		echo "ERROR: changie not installed; run 'make install-tools'"; exit 1; }
	@if [ -z "$(VERSION)" ]; then \
		echo "ERROR: VERSION is required, e.g. make changelog-batch VERSION=v0.1.0"; \
		exit 2; \
	fi
	@changie batch $(VERSION)
	@changie merge

docs-links-online: ## Check internal + external .md links via lychee (slow; not a CI gate)
	@if command -v lychee >/dev/null 2>&1; then \
		lychee --config .lychee.toml --no-progress "**/*.md"; \
	elif cre="$$(command -v docker || command -v podman)"; [ -n "$$cre" ]; then \
		"$$cre" run --rm -v "$(CURDIR)":/workspace:ro \
			lycheeverse/lychee:latest \
			--config /workspace/.lychee.toml \
			--no-progress \
			"/workspace/**/*.md"; \
	else \
		echo "ERROR: docs-links-online needs lychee on PATH (run 'make install-tools') or docker/podman"; \
		exit 1; \
	fi

# ---- Dev run --------------------------------------------------------------

# `make dev` runs the binary named by DEV_BIN (default: kscore-server) against
# testdata/dev.yaml. Override with: make dev DEV_BIN=kscorectl
DEV_BIN ?= kscore-server

dev: ## Run a binary in dev mode (DEV_BIN=kscore-server by default)
	go run ./cmd/$(DEV_BIN) --config testdata/dev.yaml

dev-server: ## Run kscore-server against testdata/dev.yaml
	go run ./cmd/kscore-server --config testdata/dev.yaml

dev-agent: ## Run kscore-agent against testdata/dev.yaml
	go run ./cmd/kscore-agent --config testdata/dev.yaml

# ---- E2E (single-topology) ------------------------------------------------

# Epic 19 task 1 — single-topology E2E. 1× kscore-server + 2× kscore-agent
# + Postgres + NATS. Two execution modes:
#
#   e2e-test         — native: builds binaries, embeds NATS, uses
#                      $KSCORE_TEST_POSTGRES_DSN. No docker required.
#                      Used by CI; the default for most local dev.
#   e2e-test-docker  — docker-compose: builds Dockerfile.kscore + brings
#                      up the production-shaped image stack. For when
#                      you want container-image coverage.
#
# The compose-driven e2e-up / e2e-down / e2e-logs / e2e-build helpers
# stay docker-only.

E2E_COMPOSE := docker compose -f test/e2e/single/docker-compose.yml

e2e-build: ## Build kscore-server + kscore-agent images for the single-topology E2E (docker)
	$(E2E_COMPOSE) build

e2e-up: ## Bring the single-topology E2E up + wait for healthy (docker)
	$(E2E_COMPOSE) up -d --wait

e2e-down: ## Tear down the single-topology E2E (docker; removes volumes)
	$(E2E_COMPOSE) down -v

e2e-logs: ## Follow logs from the single-topology E2E (docker)
	$(E2E_COMPOSE) logs -f

e2e-test: ## Native single-topology E2E (no docker; needs KSCORE_TEST_POSTGRES_DSN)
	# Native scaffold builds the binaries, embeds nats-server, and
	# uses the postgres pointed to by KSCORE_TEST_POSTGRES_DSN. Race
	# instrumentation on the Go-side test code per TEST-POLICY.md;
	# host subprocesses are out of scope for the race detector.
	# 600s budget: race overhead + binary build + 11 scenarios.
	CGO_ENABLED=1 go test -race -tags=e2e -count=1 -timeout=600s ./test/e2e/single/...

e2e-test-docker: ## Full cycle docker-compose E2E: build, up, run e2e tests, down
	$(E2E_COMPOSE) up -d --wait --build
	# KSCORE_E2E_USE_DOCKER tells the scaffold to pin docker-mode
	# addresses; KSCORE_E2E_NO_COMPOSE tells it not to re-up compose
	# (this target owns the lifecycle).
	KSCORE_E2E_USE_DOCKER=1 KSCORE_E2E_NO_COMPOSE=1 CGO_ENABLED=1 \
		go test -race -tags=e2e -count=1 -timeout=600s ./test/e2e/single/... ; \
	rc=$$? ; \
	$(E2E_COMPOSE) down -v ; \
	exit $$rc

# ---- Release --------------------------------------------------------------

release-snapshot: ## Build multi-arch snapshot tarballs to dist/ (no tag required)
	goreleaser release --snapshot --clean

release-config-check: ## Validate the goreleaser config without building
	goreleaser check

release-smoke: ## Run the artifact-shape smoke tests against dist/ (Epic 19 task 13)
	# Set RELEASE_SMOKE_CONTAINERS=1 to also install one .deb in
	# debian:12-slim and one .rpm in rockylinux:9. See
	# scripts/release-smoke.sh for the full list of checks.
	scripts/release-smoke.sh dist/

release-dry-run: release-config-check release-snapshot release-smoke ## Full local release dry-run (config + build + smoke)
	@echo ""
	@echo "release-dry-run: ok"

release: ## Build the full release artifact set into dist/ WITHOUT publishing.
	# Used by the offline-workstation step of RELEASE-PLAYBOOK.md
	# §Phase 4. Requires a v0.x.y / v1.x.y tag on HEAD. The
	# --skip=publish flag keeps goreleaser from touching any forge;
	# the playbook's manual signing + upload steps take it from
	# here.
	goreleaser release --skip=publish --clean

# ---- Security -------------------------------------------------------------

security-secrets: ## Scan for committed secrets (gitleaks)
	gitleaks detect --source . -v

security-vulns: ## Scan deps for known CVEs (govulncheck)
	govulncheck ./...

security-sast: ## Static analysis (gosec) — G115 globally excluded, HIGH+ severity gate; see docs/project/SECURITY-GOVERNANCE.md
	# Two posture choices documented in SECURITY-GOVERNANCE.md:
	#   - G115 (integer overflow conversion) is excluded project-wide
	#     — standard Go-project posture for noisy bounded conversions
	#     at proto<->Go and parser boundaries.
	#   - `-severity=high` per the epic-19 acceptance ("no high/
	#     critical findings"). MEDIUM + LOW are reported in the
	#     verbose run via `make security-sast-verbose` (v1.x).
	gosec -exclude-dir=.cache -exclude=G115 -severity=high ./...

security-licenses: ## Verify dep licenses are Apache-2.0 / MIT / BSD-compatible
	# Strict per epic 19 acceptance: forbidden, restricted (LGPL),
	# and unknown all fail. Each --ignore entry needs a comment
	# below naming why the dep is safe.
	# modernc.org/mathutil — BSD-3-Clause (confirmed in the LICENSE
	# file); go-licenses can't auto-classify because the file lacks
	# a SPDX header.
	go-licenses check \
		--disallowed_types=forbidden,restricted,unknown \
		--ignore=modernc.org/mathutil \
		./...

# ---------------------------------------------------------------------------
# Targets added by later tasks/epics — intentionally NOT stubbed here so
# `make help` reflects only what currently works.
#
#   (none currently deferred)
# ---------------------------------------------------------------------------
