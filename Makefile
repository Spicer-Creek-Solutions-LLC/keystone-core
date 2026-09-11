# Generation 2 baseline.
#
# This repository holds planning, governance and transition evidence. There is
# no product to build here: the Generation 1 implementation was archived at
# `archive/2026-09-pre-v0.6-reboot` and removed from the tip by reboot task R08.
# Generation 2 code begins at P11, which reintroduces a Go module, a build and
# the gates that go with it.
#
# Two developer tools survive, each in its own module so neither depends on a
# root module that no longer exists:
#
#   tools/capcheck    validates the archive capability catalog (permanent)
#   tools/transition  retires Generation 1 tracker state (removed or adopted at R09)

.PHONY: help docs-lint docs-lint-fix docs-links capability-catalog-check transition-check stray-binary-check check

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
	@command -v lychee >/dev/null || { echo "ERROR: docs-links needs lychee on PATH"; exit 1; }
	lychee --offline --config .lychee.toml --root-dir "$(CURDIR)" "**/*.md"

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

transition-check: stray-binary-check ## Vet, lint and test the tracker-retirement tool
	cd tools/transition && gofmt -l . | tee /dev/stderr | (! read)
	cd tools/transition && go vet ./...
	cd tools/transition && CGO_ENABLED=1 go test -race -cover ./...

check: docs-lint docs-links capability-catalog-check transition-check ## Run every gate
	@echo "check: ok"
