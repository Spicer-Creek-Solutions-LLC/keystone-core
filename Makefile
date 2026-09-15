# Generation 2 baseline.
#
# This repository holds planning, governance and transition evidence. There is
# no product to build here: the Generation 1 implementation was archived at
# `archive/2026-09-pre-v0.6-reboot` and removed from the tip by reboot task R08.
# Generation 2 code begins at P11, which reintroduces a Go module, a build and
# the gates that go with it.
#
# One developer tool survives, in its own module so it does not depend on a root
# module that no longer exists:
#
#   tools/capcheck    validates the archive capability catalog (permanent)
#
# `tools/transition` retired the Generation 1 tracker and published the
# Generation 2 planning state; R09 removed it. Its artifacts are in
# docs/transition/.

GO_PACKAGES := ./...
VERSION_PKG := go.keystone-core.io/keystone-core/internal/version
COMMIT      := $(shell git rev-parse HEAD 2>/dev/null)
LDFLAGS     := -X $(VERSION_PKG).commit=$(COMMIT)
BINARIES    := keystone keystone-server keystone-agent

.PHONY: help docs-lint docs-lint-fix docs-links capability-catalog-check stray-binary-check \
	whitespace-check build fmt fmt-check vet test test-race vuln check

help: ## Show available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}'

docs-lint: ## Lint Markdown via markdownlint-cli2 (.markdownlint-cli2.yaml)
	@command -v npx >/dev/null || { echo "ERROR: docs-lint needs npm/npx"; exit 1; }
	npx --yes markdownlint-cli2

docs-lint-fix: ## Auto-fix Markdown lint issues where possible
	@command -v npx >/dev/null || { echo "ERROR: docs-lint-fix needs npm/npx"; exit 1; }
	npx --yes markdownlint-cli2 --fix

docs-links: ## Check internal relative links via lychee (offline)
	# The file list comes from git, not from a glob. lychee's `**/*.md` does not
	# descend into dot-directories, so `.forgejo/` was never link-checked - and
	# markdownlint's identical-looking glob does descend, so the two gates
	# silently disagreed about what "every tracked file" meant. Enumerating from
	# git removes the assumption rather than patching this instance of it, and a
	# dot-directory added later is covered without anyone remembering to.
	@command -v lychee >/dev/null || { echo "ERROR: docs-links needs lychee on PATH"; exit 1; }
	@command -v git >/dev/null || { echo "ERROR: docs-links needs git on PATH"; exit 1; }
	@files="$$(git ls-files '*.md' ':!:docs/transition/r10/raw')"; \
	if [ -z "$$files" ]; then echo "ERROR: docs-links matched no files"; exit 1; fi; \
	echo "lychee: $$(echo "$$files" | wc -l) tracked markdown files"; \
	echo "$$files" | xargs lychee --offline --config .lychee.toml --root-dir "$(CURDIR)"

stray-binary-check: ## Fail if a `go build` left a binary inside a tool module
	# `go build` inside a nested module drops a binary named after its directory,
	# and `git add -A` will happily commit it. This caught a 10MB binary once.
	@for d in tools/*/; do \
		b="$${d}$$(basename $$d)"; \
		[ -f "$$b" ] && { echo "stray binary $$b — remove it"; exit 1; } || true; \
	done

capability-catalog-check: stray-binary-check ## Verify the archive capability catalog still covers the archived sources
	# Reads the Generation 1 sources at the commit recorded in
	# docs/transition/manifest.json, so it keeps working now that those files
	# are gone from the tip. Needs full history: CI checks out fetch-depth 0.
	cd tools/capcheck && go run . ../..

whitespace-check: ## Fail on trailing whitespace in any tracked text file
	# markdownlint's MD009 flags trailing spaces and does NOT look inside fenced
	# code blocks -- which is where every generated artifact in this repository
	# lives: the demonstration records and embedded checkers in the evidence
	# files. A trailing space there reached review on #323 with `make check`
	# green.
	#
	# The file list comes from git rather than from a diff range. `git diff
	# --check` compares the worktree to the index, which after a clean CI
	# checkout is empty, so it would pass unconditionally -- a gate that cannot
	# fail. Enumerating removes the assumption instead of patching it, the same
	# reason docs-links does.
	#
	# `git ls-files` lists TRACKED files, so stage new work before trusting a
	# green run. That blind spot is real and is why P11's dossier names it.
	@command -v git >/dev/null || { echo "ERROR: whitespace-check needs git on PATH"; exit 1; }
	@files="$$(git ls-files --eol | awk '$$2 ~ /^w\// { sub(/^[^\t]*\t/, ""); print }')"; \
	if [ -z "$$files" ]; then echo "ERROR: whitespace-check matched no files"; exit 1; fi; \
	echo "whitespace: $$(echo "$$files" | wc -l) tracked text files"; \
	bad="$$(echo "$$files" | xargs -d '\n' grep -nP '[ \t]+$$' 2>/dev/null || true)"; \
	if [ -n "$$bad" ]; then \
		echo "$$bad" | sed 's/^/trailing whitespace: /'; \
		echo "whitespace-check: failed"; exit 1; \
	fi; \
	echo "whitespace-check: ok"

fmt: ## Format Go source
	gofmt -w $$(git ls-files '*.go' ':!:tools/*')

fmt-check: ## Fail if Go source is unformatted
	@out="$$(gofmt -l $$(git ls-files '*.go' ':!:tools/*'))"; \
	if [ -n "$$out" ]; then echo "unformatted:"; echo "$$out"; exit 1; fi; \
	echo "fmt-check: ok"

vet: ## go vet
	go vet $(GO_PACKAGES)

test: ## Unit tests
	go test $(GO_PACKAGES)

test-race: ## Unit tests under the race detector
	go test -race $(GO_PACKAGES)

vuln: ## Scan dependencies for known vulnerabilities
	# The scan runs against the module as built. It is a release-candidate gate
	# in TESTING.md and is run per-merge here because the dependency set is
	# currently tiny and the cost is seconds; C13 owns the release ceremony.
	@command -v govulncheck >/dev/null 2>&1 || { \
		echo "govulncheck not on PATH; install with:"; \
		echo "  go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1; }
	govulncheck $(GO_PACKAGES)

build: ## Build every binary with a derived version stamp
	# The commit is injected rather than written into the source. VERSIONING.md
	# fixes the form as 0.0.0-dev+g<commit>, and a literal would report a build
	# that does not exist the moment anything is committed.
	@mkdir -p bin
	@for b in $(BINARIES); do \
		echo "build: $$b"; \
		go build -ldflags "$(LDFLAGS)" -o "bin/$$b" "./cmd/$$b" || exit 1; \
	done

check: fmt-check vet test-race whitespace-check docs-lint docs-links capability-catalog-check ## Run every gate CI runs
	@echo "check: ok"
