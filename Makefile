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
	whitespace-check build fmt fmt-check vet test test-race vuln doclint approved-documents-check \
	contract pending-contract contract-immutability-check archlint container-suite dco-exempt-check gates-agree check

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
	# `go build` inside a nested module drops a binary and `git add -A` will
	# happily commit it. This has caught three now, the third only because the
	# check was wrong about the name.
	#
	# It predicted `tools/<dir>/<dir>` -- the binary named after its DIRECTORY.
	# Go names it after the module path's last element, which is the same thing
	# only when they happen to match. `tools/pendingcontract/` declares module
	# `go.keystone-core.io/pending-contract`, so `go build` drops
	# `pending-contract`, the check looked for `pendingcontract`, and a 4.7MB
	# binary was committed at G39 with the gate green.
	#
	# So it ENUMERATES rather than predicts: any executable regular file under
	# tools/ is a stray, because none belongs there. A check that has to guess a
	# name is a check that is wrong whenever the name is.
	@found="$$(find tools -type f -perm -u+x 2>/dev/null)"; \
	if [ -n "$$found" ]; then \
		echo "stray binaries under tools/ — remove them:"; \
		echo "$$found" | sed 's/^/  /'; \
		exit 1; \
	fi; \
	echo "stray-binary-check: no built binaries under tools/"

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
# G33 added `container-suite`, and with it a working Docker daemon to the things
# `check` requires of a developer machine. That cost was weighed against letting
# the suite skip where Docker is absent, and the skip lost: a gate that reports
# green on a machine that could not have run it is the defect this repository
# keeps finding, and § 11's contract is not met by a gate CI runs and a local
# run silently omits.
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
	capability-catalog-check doclint approved-documents-check contract pending-contract contract-immutability-check archlint container-suite dco-exempt-check \
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

approved-documents-check: ## Compare merged documents with explicit approval snapshots
	@cd tools/doclint && go run . -root ../.. -approved-documents tools/doclint/approved-documents.json -approval-only

pending-contract: ## Prove every registered C01-A case still fails for its reason
	@cd tools/pendingcontract && go run . -root ../..

contract: ## Run every contract package, skipping only registered pending cases
	# Enumerated, not listed. G39 did this for pending-contract and left its two
	# siblings in this file single-package; a list beside the thing it lists is
	# a second copy, and the copy goes stale.
	@pkgs="$$(find test/contract -mindepth 1 -maxdepth 1 -type d | sort)"; \
	[ -n "$$pkgs" ] || { echo "contract: no contract package found under test/contract"; exit 1; }; \
	for p in $$pkgs; do echo "contract: $$p"; go test -tags contract "./$$p" || exit 1; done

contract-immutability-check: ## Reject an undeclared change to any accepted Cxx-A surface
	# Each Cxx-A evidence file declares the package it governs and the commit
	# that froze it. The directory name and the task id are unrelated, so the
	# link has to be stated somewhere; stating it beside the commit keeps the
	# two facts in one place.
	#
	# Matched BOTH WAYS, as archlint checks the register: a surface with no
	# declaration fails rather than being skipped, and a declaration naming a
	# package that does not exist fails too. Skipping an undeclared surface
	# would be inference from absence, which is the defect G39 had to undo.
	#
	# THE FREEZE COMMIT NEVER MOVES. Until G42 this target ran
	# `git diff --quiet $$sha -- $$frozen`, so a pull request that changed a
	# surface and moved `Contract commit:` to match passed: the help text said
	# it rejected changes to an accepted surface, and what it did was
	# re-baseline them. A frozen surface that any commit may silently rewrite
	# is not frozen, and G42 was itself the first amendment, so it should not
	# have been the change that walked through that hole.
	#
	# An amendment is legitimate because it is RECORDED, not because a gate
	# permits it. Every commit touching a frozen file after the freeze is
	# enumerated from git and must be declared under `Contract amendments:`
	# with the task that approved it, and that task must exist in the epic.
	# Matched both ways again: a declaration naming a commit that touched
	# nothing is as wrong as a commit no declaration names.
	@ok=1; declared=""; \
	for ev in docs/dossiers/*-A-acceptance-evidence.md; do \
		[ -f "$$ev" ] || continue; \
		pkg="$$(sed -n 's/^Contract package: `\([^`]*\)`.*/\1/p' $$ev)"; \
		sha="$$(sed -n 's/^Contract commit: `\([0-9a-f]\{40\}\)`.*/\1/p' $$ev)"; \
		[ -n "$$pkg" ] || continue; \
		if [ -z "$$sha" ]; then \
			echo "contract-immutability-check: $$ev declares $$pkg and records no commit"; ok=0; continue; fi; \
		if [ ! -f "$$pkg/contract_surface.go" ]; then \
			echo "contract-immutability-check: $$ev declares $$pkg, which has no contract_surface.go"; ok=0; continue; fi; \
		git cat-file -e "$$sha^{commit}" 2>/dev/null || { \
			echo "contract-immutability-check: $$ev records $$sha, which is unavailable"; ok=0; continue; }; \
		declared="$$declared $$pkg"; \
		frozen="$$( { git ls-tree -r --name-only "$$sha" -- "$$pkg"; git ls-files -- "$$pkg"; } \
			| grep -E 'contract_surface\.go$$|\.json$$' | grep -v 'pending-requirements\.json$$' | sort -u)"; \
		[ -n "$$frozen" ] || frozen="$$pkg/contract_surface.go"; \
		amends="$$(sed -n 's/^- `\([0-9a-f]\{40\}\)` — `\([PCRG][0-9][0-9][ab]\{0,1\}\)`.*/\1 \2/p' $$ev)"; \
		bad=0; \
		echo "$$amends" | while read -r a t; do [ -n "$$a" ] || continue; \
			git cat-file -e "$$a^{commit}" 2>/dev/null || { echo "contract-immutability-check: $$ev declares amendment $$a, which is unavailable"; exit 1; }; \
			grep -qE "^ *- \[[ x]\] \*{0,2}$$t\*{0,2}([^0-9a-zA-Z]|$$)" epics/20-generation-2-reboot.md || { \
				echo "contract-immutability-check: $$ev declares amendment $$a as $$t, which is not a task in the epic"; exit 1; }; \
			git log --format=%H "$$sha..HEAD" -- $$frozen | grep -qx "$$a" || { \
				echo "contract-immutability-check: $$ev declares amendment $$a, which touched no frozen file in $$pkg"; exit 1; }; \
		done || bad=1; \
		for c in $$(git log --format=%H "$$sha..HEAD" -- $$frozen); do \
			echo "$$amends" | awk '{print $$1}' | grep -qx "$$c" || { \
				echo "contract-immutability-check: $$pkg changed in $$c, which no amendment in $$ev declares"; bad=1; }; \
		done; \
		last="$$(echo "$$amends" | awk 'NF{print $$1}' | tail -1)"; [ -n "$$last" ] || last="$$sha"; \
		if ! git diff --quiet "$$last" -- $$frozen; then \
			echo "contract-immutability-check: $$pkg differs from $$last in the working tree"; bad=1; fi; \
		[ $$bad -eq 0 ] || { ok=0; continue; }; \
		n="$$(echo "$$amends" | grep -c . || true)"; \
		echo "contract-immutability-check: $$pkg matches $$last ($$n declared amendment(s) since $$sha)"; \
	done; \
	for s in test/contract/*/contract_surface.go; do \
		[ -f "$$s" ] || continue; \
		p="$$(dirname $$s)"; \
		case " $$declared " in *" $$p "*) ;; *) \
			echo "contract-immutability-check: $$p has an accepted surface that no Cxx-A evidence file declares"; ok=0;; \
		esac; \
	done; \
	[ -n "$$declared" ] || { echo "contract-immutability-check: no package was checked"; ok=0; }; \
	[ $$ok -eq 1 ] || exit 1

archlint: ## The requirements register, both directions, with liveness from the epic
	cd tools/archlint && go run . -root ../..

container-suite: ## The Docker topology and its probes
	# In `check` and in CI, landed together at G33 so `gates-agree` never saw a
	# set it could not reconcile. R6 held once the job had a client: the host's
	# socket was already mounted, and node:22-bookworm carries no docker binary,
	# so the client is the repository's to supply (CI-RUNNER.md, R6).
	#
	# THIS GATE REQUIRES DOCKER AND DOES NOT SKIP WITHOUT IT. Until G33 the
	# suite called t.Skip when the client or the daemon was absent, so a CI job
	# whose client install had silently failed would have reported a green gate
	# -- DL-1, a check that cannot fail mistaken for evidence. The environment
	# variable below is what turns that skip into a failure, and it is set HERE
	# rather than in the workflow so there is one setter: `check` and CI both
	# reach it by running this target. A developer invoking `go test ./test/...`
	# directly still gets the skip, which is the only path that should.
	#
	# Containers this suite creates are SIBLINGS on the host's daemon and
	# outlive the job that created them -- CI-RUNNER.md "How a job gets Docker"
	# states that as the cost of mounting the host socket, and this is where it
	# is paid. Teardown is by label and runs on entry as well as exit, because
	# t.Cleanup does not run when the test binary is killed by a timeout or the
	# run is cancelled, and this forge cancels in-progress runs on push.
	KEYSTONE_REQUIRE_DOCKER=1 go test -count=1 ./test/...

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
