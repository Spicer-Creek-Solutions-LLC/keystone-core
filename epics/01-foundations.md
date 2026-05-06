# Epic 01: Foundations

**Phase**: A • **Estimate**: 2 weeks • **Depends on**: nothing • **Blocks**: everything

## Goal

Establish the build, config, logging, version, error, time/wait, and DB-utility primitives that every other domain depends on. Deliver a "hello world" that builds across linux/darwin/windows × amd64/arm64, parses YAML config, emits structured JSON logs, and reports its version.

## Scope (in)

- Repo scaffold matching `PROJECT-DETAILS.md §3.6` layout (`cmd/`, `internal/`, `pkg/`, `api/`, `Makefile`, `.golangci.yml`, `buf.{yaml,gen.yaml}`).
- `pkg/version` — Version, GitCommit, BuildDate populated via `-ldflags -X`.
- `pkg/semver` — full SemVer 2.0.0 (Parse, MustParse, comparisons, Constraint interface, Diff with breaking/feature/bugfix predicates, Sort).
- `pkg/wait` — `ForCondition(ctx, interval, fn)` cancellable poller. No bare `time.Sleep` allowed elsewhere in the codebase.
- `pkg/dbutil` — `OpenSQLite(path, opts...)` with WAL, busy timeout, FK on, single-writer.
- `pkg/api/apierror` — `Response{Error, Message, Details map}`, `StatusCode()` HTTP↔gRPC mapping.
- `internal/config` — koanf-based loader; YAML + env (`KSCORE_` prefix); `Validate()` post-unmarshal; `ProductionWarnings()` (SQLite / TLS off in production; embedded NATS warning lands with Epic 05).
- `internal/logging` — `log/slog`-backed; JSON / logfmt / text formatters; correlation ID context helpers; stdout-only in v1.0.
- `Makefile` targets per `PROJECT-DETAILS.md §3.4`: `proto`, `build`, `build-all-platforms`, `clean`, `deps`, `install-tools`, `test`, `test-coverage`, `test-integration`, `check`, `fmt`, `lint`, `lint-fix`, `proto-lint`, `proto-breaking`, `dev`, `release-snapshot`.
- `.golangci.yml` v1.0 baseline lint set (errcheck, govet, ineffassign, staticcheck, unused, bodyclose, gosec). `.pb.go` files exempt.
- `buf.yaml` STANDARD lint with documented exclusions; `buf.gen.yaml` configured for Go + gRPC plugins outputting to `pkg/api/v1/`.
- `.pre-commit-config.yaml` — gofmt, golangci-lint, smoke-test.
- `scripts/smoke-test.sh quick` — SQLite + embedded-NATS-ready smoke.
- "Hello world" `kscore-server` and `kscore-agent` and `kscorectl` that:
  - Parse a config file (`--config` flag).
  - Print version on `--version`.
  - Log a startup line in JSON.
  - Exit cleanly on SIGTERM.

## Scope (out / non-goals)

- Real gRPC services, NATS, storage repositories, or business logic. Those land in later epics.
- Hugo docs site — README + reference docs in v1.0; Hugo is v1.1.
- Multi-party signing ceremony — v1.2.
- Hot-reload dev server (`air`) — v1.0.x dot release.

## Design summary

See `PROJECT-DETAILS.md §3` (Tech Stack & Build), §4.1 (Foundations).

## Tasks

1. **Repo scaffold**: directory layout, `go.mod` (Go 1.25+), top-level files (`README.md`, `LICENSE` Apache 2.0, `NOTICE`, `CODE_OF_CONDUCT.md`, `CONTRIBUTING.md`, `SECURITY.md`, `CHANGELOG.md`, `AGENTS.md`).
2. **`pkg/version`** + tests; build-time injection via Makefile LDFLAGS.
3. **`pkg/semver`** + tests — facade over `github.com/Masterminds/semver/v3`. Project-facing API: `Parse`, `MustParse`, comparisons, accessors, `NextMajor/Minor/Patch`, `Sort`, `Constraint` interface (caret/tilde/wildcard/compound/OR), and project-specific `Diff{Kind, Direction}` with `IsBreaking`/`IsFeature`/`IsBugFix` predicates. Decided in task 3 review to build on Masterminds rather than roll our own — only `Diff` is project-specific.
4. **`pkg/wait`** + tests.
5. **`pkg/dbutil.OpenSQLite`** + tests (WAL pragma verified, busy timeout, FK on, single writer).
6. **`pkg/api/apierror`** + tests (status code mapping for both directions).
7. **`internal/config`** — koanf-backed loader (YAML + `KSCORE_`-prefixed env), strict unmarshal, post-unmarshal `Validate()`, and `ProductionWarnings()`. Foundations ships 3 sub-configs (`Server`, `Logging`, `Storage`) plus a top-level `Mode`; remaining domain sub-configs are added by their owning epics. Decided in task 7 review to use koanf over Viper for cleaner unmarshal semantics and lighter deps; Cobra remains for CLI parsing in task 13 with a small flag→koanf bridge.
8. **`internal/logging`** — `log/slog` factory; JSON/logfmt/text formatters; correlation-ID context helpers (auto-injected by a wrapping `slog.Handler`); tests for each formatter. Decided in task 8 review to use stdlib `log/slog` over `go.uber.org/zap`: zero new dep, idiomatic for new Go projects, native context awareness fits correlation IDs cleanly.
9. **`Makefile`** with the v1.0 targets that the foundation supports today: build/test/lint/security/help/clean/deps/install-tools/fmt/check + cross-compile matrix. Decisions recorded in task 9 review: BINARIES auto-detected from `cmd/` (eliminates the gotcha in PROJECT-DETAILS §4.1); `CGO_ENABLED=0` exported for builds, overridden to 1 for `go test -race`; deferred targets (`proto*`, `dev*`, `release*`, `e2e*`) omitted rather than stubbed — added by their owning tasks (11, 13, 15, Epic 19).
10. **`.golangci.yml`** baseline.
11. **`buf.yaml` + `buf.gen.yaml`** with empty proto file to verify codegen wiring.
12. **`.pre-commit-config.yaml`** + `scripts/smoke-test.sh` skeleton.
13. **`cmd/kscore-server/main.go`, `cmd/kscore-agent/main.go`, `cmd/kscorectl/main.go`** — minimal Cobra commands with `--version`, `--config`, structured-log startup line, graceful SIGTERM exit.
14. **CI (GitHub Actions or chosen)**: lint + test + smoke + cross-build matrix.
15. **`.goreleaser.yaml`** — snapshot config for the three hello-world binaries.

## Acceptance criteria

- [ ] `make build` produces three binaries in `build/bin/$GOOS/$GOARCH/`.
- [ ] `make build-all-platforms` produces linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 binaries with no CGO.
- [ ] `kscore-server --version` prints version + commit + build date.
- [ ] `kscore-server --config testdata/dev.yaml` parses and emits a JSON startup log line including correlation ID.
- [ ] `make test` runs all unit tests; coverage >70% on `pkg/*`.
- [ ] `make lint` passes the baseline rule set.
- [ ] `make proto` round-trips an empty proto file successfully.
- [ ] Pre-commit hook passes locally.
- [ ] `make release-snapshot` produces multi-arch tarballs in `dist/`.

## Risks

- **koanf env-var convention**: single-word keys avoid env-mapping ambiguity (`grpcport` not `grpc_port`); document expected env keys.
- **Cross-compilation surprises**: any accidentally CGO-dependent dep will break Windows + Alpine; CI matrix must catch immediately.
- **Pre-commit overhead**: keep smoke fast (<10s) or contributors disable it.

## References

- PROJECT-DETAILS §3 (Tech Stack), §4.1 (Foundations), §5.4 (Build & Release).
