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

# GIT_DIR and its relatives override repository discovery, so every `git` below
# would read whatever repository the environment names rather than this one.
# **Git hooks set GIT_DIR**, which makes `make check` from a pre-commit hook
# enough to reach it, and the failure is silent: the gates pass against the wrong
# tree. `unexport` keeps them out of every recipe; `$(shell ...)` below runs
# before that applies, so it strips them itself.
unexport GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE
unexport GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES
unexport GIT_CEILING_DIRECTORIES GIT_DISCOVERY_ACROSS_FILESYSTEM
unexport GIT_NAMESPACE GIT_PREFIX

GO_PACKAGES := ./...
VERSION_PKG := go.keystone-core.io/keystone-core/internal/version
COMMIT      := $(shell env -u GIT_DIR -u GIT_WORK_TREE -u GIT_COMMON_DIR \
                        -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
                        git rev-parse HEAD 2>/dev/null)
LDFLAGS     := -X $(VERSION_PKG).commit=$(COMMIT)
BINARIES    := keystone keystone-server keystone-agent

.PHONY: help docs-lint docs-lint-fix docs-links capability-catalog-check stray-binary-check \
	whitespace-check build fmt fmt-check vet test test-race vuln doclint archlint container-suite dco-exempt-check deferred-gates-check gates-agree check

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

whitespace-check: ## Fail on trailing whitespace or control characters in any tracked text file
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
	: "Control characters. tools/doclint/main.go carried two literal backspace"; \
	: "bytes inside a regex literal since P11b -- (?i)\\x08it is not\\x08|... --"; \
	: "so that alternative could never match. gofmt, vet and every editor show"; \
	: "the line as correct, and the tool reported a clean classification for"; \
	: "hits it should have caught. Tab and newline are the only ones allowed."; \
	ctrl="$$(echo "$$files" | xargs -d '\n' grep -nP '[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]' 2>/dev/null || true)"; \
	if [ -n "$$ctrl" ]; then \
		echo "$$ctrl" | sed 's/^/control character: /' | cat -v; \
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

# ADR-0010 § 11: one local command runs what CI runs. That is a contract, not a
# description, so `check` carries every gate CI runs that can run at all locally
# — build and the vulnerability scan included.
#
# Exactly one CI gate is not here, and it is named rather than left to be
# noticed: the DCO sign-off check. It verifies the commits a branch adds over
# main, and locally those are still in flux — a developer amends and rebases
# until they push, which is the moment the check exists to guard. It also needs
# a fetched `origin/main`, so it is not a gate that runs offline.
#
# G25 changed its stated reason and the reason is now this one. Before G25 it
# read the base and head of a pull request, so there was no local base at all;
# after G25 it derives the range from the merge base, which a local run could
# compute. Whether it should therefore become a local gate is open, and is
# recorded in that task rather than decided here.
#
# `dco-exempt-check` asserts it still exists, so the exemption cannot become a
# missing gate nobody spots.
check: fmt-check vet test-race tools-test whitespace-check build vuln docs-lint docs-links \
	capability-catalog-check doclint archlint dco-exempt-check deferred-gates-check \
	gates-agree ## Run every CI gate that can run locally
	@echo "check: ok"

doclint: ## Standing document sweeps, with lifetimes
	# Sweeps only. An acceptance case proves a document was correct when it was
	# accepted and then passes forever; a sweep asserts a conclusion has not been
	# contradicted since. Every cross-task regression in Stage P was caught by a
	# sweep, and no per-ADR structural case ever caught a later one.
	#
	# --tasks-complete makes a rule past its retirement a FAILURE. A sweep guards
	# a correction and every task adds one; without forced retirement the set
	# grows until its failures stop being read.
	@cd tools/doclint && go run . -root ../.. \
		-tasks-complete "$$(sed -n 's/^ *- \[x\] \([PCRG][0-9][0-9][ab]\?\) .*/\1/p' ../../epics/20-generation-2-reboot.md | paste -sd,)"

archlint: ## The requirements register, both directions, with liveness from the epic
	cd tools/archlint && go run . -root ../..

container-suite: ## The Docker topology and its probes
	# DELIBERATELY NOT IN `check`, and `deferred-gates-check` asserts that rather
	# than leaving it as an absence. The runner that would run this in CI cannot
	# yet: jobs there have no Docker client and no socket (CI-RUNNER.md, R6).
	#
	# Putting it in `check` today would make `gates-agree` fail, because CI has
	# no step for it -- correctly, since there is nowhere to run it. It lands in
	# `check` and in CI together, when R6 goes green.
	go test -count=1 ./test/...

deferred-gates-check: ## Assert a deferred gate is deferred on purpose, and says what lands it
	# A gate in neither `check` nor CI is invisible to `gates-agree`: it is not
	# missing from either set, so nothing reports it. That is a gap tolerated
	# rather than asserted, which is the shape this repository keeps finding.
	@ok=1; \
	grep -q '^container-suite:' $(MAKEFILE_LIST) || { \
		echo "deferred-gates-check: container-suite is named as deferred and does not exist"; ok=0; }; \
	: "the prerequisite list spans continuations; reading only the first line"; \
	: "is the bug gates-agree already had, and it hides anything on line two"; \
	if sed -n '/^check:/,/[^\\]$$/p' $(MAKEFILE_LIST) | sed 's/##.*//' | grep -qw container-suite; then \
		echo "deferred-gates-check: container-suite is in check; remove it from the deferred list"; ok=0; fi; \
	: "scoped to the container-suite recipe. Searching the whole file finds the"; \
	: "phrase in THIS grep's own pattern, so the check would read itself and"; \
	: "pass however the comment changed -- which the demonstration caught"; \
	sed -n '/^container-suite:/,/^$$/p' $(MAKEFILE_LIST) | grep -q 'R6 goes green' || { \
		echo "deferred-gates-check: the deferral does not say what would end it"; ok=0; }; \
	[ $$ok -eq 1 ] || exit 1; \
	echo "deferred-gates-check: container-suite deferred until CI-RUNNER.md's R6 holds"

gates-agree: ## Assert make check and CI run the same gate set, both directions
	# ADR-0010 § 11 requires one local command to run what CI runs. This is the
	# assertion, and it lives here rather than inline in the workflow so that CI
	# and a local run execute the same code -- an assertion written twice is two
	# things to drift.
	#
	# Review of #331 found the reverse direction open: CI ran build and the
	# vulnerability scan, `check` did not, and its help text claimed it ran every
	# gate. Documenting that gap did not make the contract hold.
	@targets="$$(sed -n '/^check:/,/[^\\]$$/p' $(MAKEFILE_LIST) \
		| sed 's/^check://; s/##.*//; s/\\$$//' | tr ' \t' '\n\n' | grep -v '^$$' | sort -u)"; \
	steps="$$(grep -oE '^ *run: make [a-z-]+|make [a-z-]+$$' .forgejo/workflows/reboot-baseline.yml \
		| grep -oE 'make [a-z-]+' | awk '{print $$2}' | sort -u)"; \
	if [ -z "$$targets" ] || [ -z "$$steps" ]; then \
		echo "ERROR: gates-agree parsed an empty set; it is checking nothing"; exit 1; fi; \
	: "process substitution is bash-only and make runs /bin/sh; grep -vxF takes"; \
	: "a newline-separated pattern string, which is portable and needs no shell"; \
	missing="$$(echo "$$targets" | grep -vxF "$$steps" || true)"; \
	extra="$$(echo "$$steps" | grep -vxF "$$targets" || true)"; \
	rc=0; \
	if [ -n "$$missing" ]; then echo "make check runs these and CI does not:"; echo "$$missing"; rc=1; fi; \
	if [ -n "$$extra" ]; then echo "CI runs these and make check does not:"; echo "$$extra"; rc=1; fi; \
	[ $$rc -eq 0 ] || exit 1; \
	echo "gates-agree: both directions agree on $$(echo "$$targets" | tr '\n' ' ')"

tools-test: ## Run the tests in each tools module, which `go test ./...` does not reach
	# Each tool is its own module, so the root `./...` never sees it. capcheck
	# has had a main_test.go since R08 and NOTHING HAS EVER RUN IT -- found while
	# adding doclint's, which review of #339 asked for. A test no gate executes
	# reports nothing, which is the shape `DL-1` describes.
	@found=0; \
	for m in tools/*/; do \
		[ -f "$$m/go.mod" ] || continue; \
		found=$$((found+1)); \
		echo "tools-test: $$m"; \
		( cd "$$m" && go test ./... ) || exit 1; \
	done; \
	if [ "$$found" -eq 0 ]; then \
		echo "ERROR: tools-test found no modules; it is checking nothing"; exit 1; \
	fi; \
	echo "tools-test: $$found module(s) ok"

dco-exempt-check: ## Assert the one CI-only gate still exists, and can still run
	@grep -q 'DCO sign-off on every commit this branch adds' .forgejo/workflows/reboot-baseline.yml || { \
		echo "ERROR: the DCO gate is named as check's only exemption and is no longer in CI"; \
		exit 1; }
	# Presence is not enough. G25 removed the `pull_request` trigger while a step
	# was conditioned on `github.event_name == 'pull_request'` — the step would
	# have stayed present, this grep would have stayed green, and the gate would
	# never have run again. So: every event a step conditions on must be an event
	# this workflow actually triggers on.
	@triggers="$$(sed -n '/^on:/,/^[a-z]/p' .forgejo/workflows/reboot-baseline.yml \
		| sed -n 's/^  \([a-z_]*\):.*/\1/p' | sort -u)"; \
	: "Comment lines are dropped first. The workflow explains this very check in"; \
	: "prose, and a sweep that reads its own explanation reports itself."; \
	guarded="$$(grep -v '^[[:space:]]*#' .forgejo/workflows/reboot-baseline.yml \
		| grep -oE "github\.event_name *== *'[a-z_]+'" \
		| sed "s/.*'\(.*\)'/\1/" | sort -u)"; \
	if [ -z "$$triggers" ]; then \
		echo "ERROR: dco-exempt-check parsed no triggers; it is checking nothing"; exit 1; fi; \
	dead="$$(echo "$$guarded" | grep -v '^$$' | grep -vxF "$$triggers" || true)"; \
	if [ -n "$$dead" ]; then \
		echo "ERROR: a step is conditioned on an event this workflow does not trigger on:"; \
		echo "$$dead"; \
		echo "Such a step is present but can never run."; \
		exit 1; fi
	@echo "dco-exempt-check: the one CI-only gate is present, and no step waits on an absent event"
