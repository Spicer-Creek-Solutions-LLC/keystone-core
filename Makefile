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
        build build-all-platforms clean deps install-tools \
        test test-verbose test-coverage coverage-gate race-policy goleak-policy test-integration slo test-cross-distro check \
        fmt lint lint-fix smoke \
        proto proto-lint proto-breaking \
        openapi-lint \
        docs-lint docs-lint-fix docs-lint-container \
        dev dev-server dev-agent \
        e2e-build e2e-up e2e-down e2e-logs e2e-test \
        release-snapshot release-dry-run \
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

clean: ## Remove build artifacts
	rm -rf build/ dist/
	rm -f coverage.out
	rm -f trackerctl

deps: ## Download and verify Go module dependencies
	go mod download
	go mod verify

install-tools: ## Install Go-installable dev tools (golangci-lint, gosec, govulncheck, buf, protoc-gen-go*, goreleaser, gitleaks, go-licenses)
	@command -v golangci-lint >/dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@command -v gosec >/dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	@command -v govulncheck >/dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	@command -v buf >/dev/null || go install github.com/bufbuild/buf/cmd/buf@latest
	@command -v protoc-gen-go >/dev/null || go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@command -v protoc-gen-go-grpc >/dev/null || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@command -v goreleaser >/dev/null || go install github.com/goreleaser/goreleaser/v2@latest
	@command -v gitleaks >/dev/null || go install github.com/zricethezav/gitleaks/v8@latest
	@command -v go-licenses >/dev/null || go install github.com/google/go-licenses@latest

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

test-cross-distro: ## Run state stdlib smoke across the v0.5 distro matrix (docker-compose; gated)
	# Layer C of Epic 08 task 13 — exercises the modules that touch
	# live system state (package / service / user / hostname / …)
	# against the v0.5 distro matrix (Debian 12, Ubuntu 22.04/24.04,
	# Rocky 9, Alpine 3.19). The harness is scaffolded under
	# test/e2e/state/ but is gated to v0.5 — see
	# docs/project/ROADMAP.md `Cross-distro state stdlib docker
	# matrix harness` for the gate criteria.
	@bash test/e2e/state/run.sh

check: lint docs-lint test ## Run lint + docs-lint + tests

smoke: ## Run quick smoke checks (compile + SQLite pragmas)
	scripts/smoke-test.sh quick

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

# Epic 19 task 1 — single-topology E2E harness. 1× kscore-server +
# 2× kscore-agent + Postgres + NATS via docker-compose. Task 1 ships
# the harness; task 2 wires the 9 feature scenarios on top.

E2E_COMPOSE := docker compose -f test/e2e/single/docker-compose.yml

e2e-build: ## Build kscore-server + kscore-agent images for the single-topology E2E
	$(E2E_COMPOSE) build

e2e-up: ## Bring the single-topology E2E up + wait for healthy
	$(E2E_COMPOSE) up -d --wait

e2e-down: ## Tear down the single-topology E2E (removes volumes)
	$(E2E_COMPOSE) down -v

e2e-logs: ## Follow logs from the single-topology E2E
	$(E2E_COMPOSE) logs -f

e2e-test: ## Full cycle: build, up, run e2e tests, down (cleanup on failure)
	$(E2E_COMPOSE) up -d --wait --build
	# Epic 19 task 5 — -race on the Go-side test code (the docker
	# container processes are out of scope; race detector doesn't
	# cross process boundaries). Race instrumentation adds modest
	# wall-clock to the in-process scenario helpers; still well
	# inside the 300s timeout.
	KSCORE_E2E_NO_COMPOSE=1 CGO_ENABLED=1 go test -race -tags=e2e -count=1 -timeout=300s ./test/e2e/single/... ; \
	rc=$$? ; \
	$(E2E_COMPOSE) down -v ; \
	exit $$rc

# ---- Release --------------------------------------------------------------

release-snapshot: ## Build multi-arch snapshot tarballs to dist/
	goreleaser release --snapshot --clean

release-dry-run: ## Validate the goreleaser config without building
	goreleaser check

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
