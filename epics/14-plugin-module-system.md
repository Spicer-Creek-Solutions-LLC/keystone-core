# Epic 14: Plugin / Module System (Starlark + Cosign + Filesystem Registry)

**Phase**: I • **Estimate**: 3 weeks • **Depends on**: 06, 08, 09, 10, 12 • **Blocks**: 15 (blueprint hooks may invoke modules)

## Goal

Salt-like extensibility on day 1. Sysadmins can author safe, sandboxed Starlark modules and ship them through a verified, reproducible distribution pipeline. **Strategic v0.1 scope decision**: Starlark-only + Cosign verification + filesystem registry — this delivers the *experience* of a real module system without the long tail of WASM/SumDB/cloud-backends.

## Scope (in)

### Manifest + lockfile

- `pkg/module/manifest/` — `Manifest{Name (namespaced vendor/pkg), Version (semver), Type (starlark in v1.0; wasm v1.1), Entrypoint, Description, Author, License, Capabilities, Limits, Dependencies}`.
- `CapabilityConfig` per capability: AllowedPaths, DeniedPaths, MaxFileSize, AllowedDomains, RateLimit, Timeout, AllowedCommands, AllowedSecretPaths, etc.
- Resource Limits: Timeout, Memory (string), CPU (float64).
- `LockFile{schemaVersion, modules map<name, LockedModule>}`; `LockedModule{version, hash}`.
- YAML serialization; round-trip stable.

### Capabilities (v1.0 — 9 core)

- `fs.read`, `fs.write` (path globs + denials + max_file_size)
- `http.get`, `http.post` (domain allowlists, request/response size limits, rate limits, timeouts)
- `exec` (command allowlists, working_dir, timeout)
- `secrets.read`, `secrets.write` (secret-path scoping)
- `kv` (in-process key-value)
- `log` (rate-limited)

### Verification

- `pkg/module/verify/`:
  - SHA-256 content addressing; CAS storage `~/.kscore/modules/<hash>/`.
  - Cosign signatures (RSA, ECDSA, Ed25519; KeyID-based key management).
  - Trust policy: TLS-trusted registry + Cosign signature in v1.0.

### Resolver

- `pkg/module/resolver/`:
  - Recursive dependency resolution against semver constraints (`>=1.0 <2.0`, `^1.5.0`, `~1.2.3`); prerelease-filter configurable.
  - DAG with cycle detection.
  - Conflict resolution: **Minimum Version Selection (MVS)** — Go modules pattern.
  - Module cache (content-addressed; CacheConfig: Dir, MaxSize, MaxAge, Readonly).
  - Lock file generation: topological sort + sorted by name.

### Registry (v1.0 — filesystem-backed)

- `pkg/module/registry/` + `cmd/kscore-registry/`.
- Go-mod-style HTTP endpoints:
  - `GET /<module>/@v/list`
  - `GET /<module>/@v/<ver>.info`
  - `GET /<module>/@v/<ver>.mod`
  - `GET /<module>/@v/<ver>.zip`
- Storage backend interface (`internal/registry/storage/`): `Get`, `Put`, `Delete`, `List`, `Exists`, `Stat`, `Health`. Filesystem backend in v1.0.

### Loader

- `pkg/module/loader/`:
  - `ModuleLoader` interface: `Load(path, options) → LoadResult`; `Execute(result, options) → ExecuteResult`; `LoadAndExecute`.
  - `LoadResult{Manifest, Runtime, VerificationResult, PolicyResult, CapabilityPolicyDecisions, RegisteredCapabilities, DeniedCapabilities, LoadDuration}`.
  - 7-step pipeline: parse → verify → policy check → capability policy → capability lock check (vs PreviousCapabilities) → runtime init → register granted capabilities only.
  - Module cache: load-time caching by path + content hash.
  - Load events: telemetry hooks at each pipeline phase.

### Runtime: Starlark

- `pkg/module/runtime/starlark/`:
  - `go.starlark.net`-backed.
  - Deterministic mode: `random()`, `time.now()` disabled by default (capability-gated).
  - Bytecode execution limits, recursion depth constraints, memory heap limits.

### Audit

- `pkg/module/audit/` — every capability invocation: `AuditEntry{Timestamp, Module, Version, Capability, Operation, Success, Duration, Details}`.
- `CapabilityInvoker` wraps the call → emit entry → integrate with §4.12 audit log.

### SDK

- `modules/sdk/starlark/` — host capability bindings as Starlark builtins; example modules.

### CLI

- `cmd/kscore-module`: `init`, `build`, `validate`, `resolve`, `verify`, `sign`, `test`, `publish`, `install`, `update`, `clean`, `tree`.
- Audit flags: `--audit-level`, `--audit-output`.

### Plugin discovery

- `pkg/plugin/`:
  - `Discovery.Discover()` scans `$PATH` for `kscore-*` binaries; caches.
  - `Executor.Execute()` runs them via `exec.Command`; stdin/stdout/stderr piping; context cancellation.
- This is what makes `kscorectl <foo>` dispatch to `kscore-foo` (Git-style plugin pattern).

## Scope (out / non-goals)

- **WASM runtime** (`tetratelabs/wazero`, WASI, instruction metering, memory bounds) — v1.1.
- Rust SDK, Go (TinyGo) SDK — v1.1.
- C++ SDK — v2.0.
- OCI registry backend — v1.1.
- S3/GCS/Azure storage backends — v1.1.
- `kscore-module mirror` air-gap — v1.1.
- SumDB transparency log — v1.2.
- Fine-grained capability model (per-syscall via seccomp/eBPF) — v1.2.
- Module vulnerability scanning + SBOM generation — v1.2.
- Federated module registries — v2.x.

## Design summary

See `PROJECT-DETAILS.md §4.18`.

## Tasks

1. **`Manifest` + `CapabilityConfig` + `LockFile`** types + YAML codec + tests.
   _(landed: new **`pkg/module/manifest/`** — **no new dep** (reuses the repo's direct `go.yaml.in/yaml/v3` + existing `pkg/semver`). `Manifest{Name,Version,Type,Entrypoint,Description,Author,License,Capabilities,Limits,Dependencies}`; single superset `CapabilityConfig` (`omitempty` per-capability fields — the epic's one-config design, not per-capability types); `Limits{Timeout,Memory,CPU}`; `LockFile{SchemaVersion,Modules}`/`LockedModule{Version,Hash}`. `MarshalManifest`/`UnmarshalManifest`/`MarshalLockFile`/`UnmarshalLockFile` (YAML); `ParseSize` (binary KB/MB/GB + KiB/MiB/GiB synonyms + bare bytes, fractional) + `ParseRate` (`<n>/<s|m|h>`) exported helpers. `Manifest.Validate()`: namespaced lowercase `vendor/pkg` name (regex — guards namespace squatting), `Version` via `semver.Parse`, `Type==starlark` (`wasm` explicitly rejected as v1.1-reserved), entrypoint required, capability keys ∈ the 9 core, per-capability size/rate/duration well-formed, `Limits` parse + `CPU>=0`, each dependency name namespaced + constraint shape via `semver.NewConstraint` (full resolution is task 6). `LockFile.Validate()` (schema==1, semver versions, `sha256:<64 hex>` hashes); lockfile marshals with deterministically sorted keys (yaml.v3) → byte-identical re-resolution = the reproducibility acceptance line. Verbatim §4.18 example manifest round-trips + validates; nil-safety; 97.2% cov (>80% `pkg/module/*` gate). `make lint`/`docs-lint` clean; default `go test ./... -race` unaffected (new isolated package; the pre-existing ROADMAP-logged `internal/statemgmt/stdlib/service` `-race` flake is not in this diff). Capability enforcement (T2/T3), verify/CAS (T4/T5), resolver/MVS (T6), registry (T8/T9), loader pipeline (T10), Starlark runtime (T11) NOT started.)_
2. **Capability registry + invoker + audit emission**.
   _(landed: new **`pkg/module/audit/`** + **`pkg/module/capability/`** — **no new dep** (stdlib + `internal/audit` + task-1 `pkg/module/manifest`). `pkg/module/audit`: module-domain `Entry{Timestamp,Module,Version,Capability,Operation,Success,Duration,Details}` (the §4.18 shape); `Auditor` interface (`Emit(ctx,Entry)`, fire-and-forget — the §4.11 "failure-to-log is a bug, not a request failure" contract) + `NoopAuditor`; **`StoreBridge`** adapts onto the §4.12 `internal/audit.Auditor` (maps `Entry`→`audit.NewAuditEntry`: `ResourceType=module`, `Action=module.<cap>`, `User=<module>@<ver>`, `Allowed=Success`, `Severity` Low / Medium-on-failure, `Metadata`={module,version,capability,operation,…Details} with reserved-key shadow guard); a construction error is logged + dropped, never propagated. `pkg/module/capability`: immutable **`Registry`** (granted-capability set, `NewRegistryFromManifest` projecting the validated manifest's capability keys — single-sourced via the newly-exported `manifest.KnownCapability`; unknown rejected defensively → `ErrUnknownCapability`; `Has`/sorted `List`; nil-safe = deny); **`Invoker.Invoke(ctx,cap,op,fn)`** — non-granted ⇒ fn NOT run, denied `Entry` emitted (`Operation=denied`, `Details.requested_operation`), `ErrCapabilityNotGranted` returned (the foundation for the "unauthorized exec/fs.write fails with audit entry" acceptance lines); granted ⇒ fn timed, outcome audited (`Success=err==nil`), error propagated unchanged. Clean one-way layering `capability → audit → internal/audit` (mirrors `pkg/api/cluster → internal/cluster`). audit 88.2% / capability 100% cov (the one uncovered audit branch is the unreachable-from-valid-input `NewAuditEntry` defensive error path); `make lint`/`docs-lint` clean; full `go test ./... -race` green (rc=0, no flake this run). The 9 capability *backends* + path/domain/command scoping (T3), verify/CAS (T4/T5), resolver/MVS (T6), registry server (T8/T9), loader pipeline that populates the Registry from the verified manifest (T10), Starlark runtime/builtins (T11/T12) NOT started.)_
3. **9 core capabilities** — pluggable backends (e.g., `SecretsStore`, `Logger`, `KVStore`, `HTTPClient`, `FSAccess`, `Executor`).
   _(landed: extends **`pkg/module/capability/`** — **no new dep** (`github.com/gobwas/glob` already a direct repo dep, used by policy/controlplane/targeting; rate-limiting is a tiny stdlib token bucket — `golang.org/x/time/rate` is deliberately not pulled, it is not in go.mod). Narrow injected host seams (`FSHost`/`HTTPHost`/`ExecHost`/`SecretsHost`/`Logger` + `Hosts` bundle) keep `pkg/module` dep-light; real `internal/secrets`/os-exec/net-http hosts wire at boot (deferred — the module boot-wiring ROADMAP item, like every Epic 14 host integration). The 9 scoped backends, each `New…(manifest.CapabilityConfig, host)` compiling scope once: **fs.read/fs.write** (`pathScope` = `glob.Compile(p,'/')` allow + deny globs, `path.Clean` first so `/a/../etc` can't escape `/a/**`; `MaxFileSize` via task-1 `ParseSize`), **http.get/http.post** (shared `httpCap`: domain-allowlist globs on `req.URL.Hostname()`, req/resp size caps with `io.LimitReader`, token-bucket `RateLimit`, per-call `Timeout` ctx), **exec** (command allowlist by exact + `filepath.Base`, `WorkingDir`, `Timeout` ctx), **secrets.read/secrets.write** (`SecretPaths` glob scope → `ErrSecretPathDenied`), **kv** (real in-process mutex map, optional key-count cap), **log** (token-bucket rate limit → `ErrRateLimited` on drop; nil host ⇒ slog). Typed sentinels `ErrPathDenied`/`ErrDomainDenied`/`ErrCommandDenied`/`ErrSecretPathDenied`/`ErrSizeExceeded`/`ErrRateLimited`/`ErrHostUnavailable` (host nil ⇒ fail-closed). `BuildCapabilities(m,Hosts)` assembles the granted+configured backend map (malformed scope fails the whole build — a module with a bad glob/size/rate must not load); the task-10 loader hands it to the runtime, task-12 exposes them as Starlark builtins. **Acceptance lines proven**: a dedicated test composes task-3 scoping with the task-2 `Invoker` — `fs.write` outside `Paths` → `ErrPathDenied` + audited failure entry; unauthorized `exec` (not granted) → fn-not-run + `ErrCapabilityNotGranted` + audited `denied` entry. `pkg/module/capability` 87.1% cov (>80% gate); `make lint`/`docs-lint` clean; full `go test ./... -race` green (rc=0). Fixed an in-scope FEATURES doc defect ("7 core capabilities" → "9", + source paths). Starlark builtins (T12), real host-backend boot wiring (deferred ROADMAP), verify/CAS (T4/T5), resolver (T6), registry (T8/T9), loader (T10), runtime (T11) NOT started.)_
4. **Cosign signature verifier** — accepts RSA/ECDSA/Ed25519; KeyID lookup against trust policy.
   _(landed: new **`pkg/module/verify/`** — **NO new dep** (decision: **Option C**, pure stdlib `crypto/*`, approved after a comparison table — `sigstore/cosign` pulls a Kubernetes-scale OCI/Rekor/Fulcio/TUF tree for capability that is entirely post-v1.0; `sigstore/sigstore`'s keyed path is a thin stdlib wrapper; the repo already does RSA/ECDSA/Ed25519 with stdlib in `internal/identity`). A cosign *keyed* detached-blob signature is exactly `verify(pubkey, sig, sha256(blob))` (ed25519 over the raw blob), so it is stdlib-only and cosign-`verify-blob`-compatible. `KeyID` = lowercase hex SHA-256 of the PKIX-DER public key (deterministic, rotation-friendly per §4.18). `TrustPolicy` (mutex map `KeyID→crypto.PublicKey`; `LoadTrustPolicy`/`AddKey`/`AddKeyPEM`/`KeyIDs`; v1.0 trust = a policy key **+** the TLS-trusted registry transport, the latter being the T8/T9 client's job). `Signature{KeyID,Algorithm,Value}` (`ecdsa-sha256`/`rsa-pkcs1v15-sha256`/`ed25519`). `Verifier.Verify(blob,sig)`: lookup→`ErrUnknownKeyID`; alg-tag vs key-type mismatch→`ErrUnsupportedAlgorithm`; ECDSA `VerifyASN1`/RSA `VerifyPKCS1v15` over `sha256(blob)`, Ed25519 over raw blob; fail→`ErrSignatureMismatch` (the "Cosign signature mismatch causes load to fail" acceptance line). Complementary `Sign(blob, crypto.Signer)` (key-type validated *before* KeyID so a bad signer is `ErrUnsupportedAlgorithm` not an opaque marshal error) produces cosign-verify-compatible detached sigs from a plain PKCS8/SEC1 PEM ("local.key") — feeds the T14 `kscore-module sign` CLI. Sentinels `ErrUnknownKeyID`/`ErrUnsupportedAlgorithm`/`ErrSignatureMismatch`/`ErrInvalidKey`. Tests: sign→verify round-trip all 3 algs, blob-tamper + sig-tamper → mismatch, unknown/wrong-key, alg mismatch, PEM load + garbage, KeyID stability + 2-key rotation, unsupported signer. `pkg/module/verify` 86.7% cov (>80% gate); `make lint`/`docs-lint`/trackerctl-mirror clean; full `go test ./... -race` green (rc=0). **Honest scope (ROADMAP-logged):** cosign keyless (Fulcio/Rekor), encrypted cosign keyfile interop, SumDB transparency → new `v1.x` ROADMAP entry "Module signing: cosign keyless / Rekor transparency / encrypted cosign keyfile interop" (+ release-order mirror). SHA-256 + CAS (T5), resolver (T6), registry/transport TLS (T8/T9), loader pipeline that calls `Verify` as step 2 (T10) NOT started.)_
5. **SHA-256 hasher + CAS storage**.
6. **Resolver** — semver constraints, DAG, cycle detection, MVS.
7. **Module cache** with CacheConfig.
8. **Filesystem registry** — Go-mod HTTP endpoints; backend storage interface.
9. **`cmd/kscore-registry`** server.
10. **`ModuleLoader` 7-step pipeline** — full implementation with tests for each phase.
11. **Starlark runtime** — sandboxed; deterministic mode.
12. **Starlark capability builtins** — Go shims wrapping Capability backends.
13. **`pkg/plugin`** Discovery + Executor; `kscorectl` integration.
14. **`cmd/kscore-module`** CLI with all listed subcommands.
15. **Module test framework** in `pkg/module/testing/` — Starlark unit-test runner.
16. **3 example modules** in `modules/examples/` to validate the full author UX (init → build → sign → publish → install → execute).
17. **Integration test**: end-to-end module flow with fake registry server.

## Acceptance criteria

- [ ] `kscore-module init my-module` scaffolds a Starlark module with manifest.
- [ ] `kscore-module build` packages module as ZIP.
- [ ] `kscore-module sign --key local.key` produces Cosign signature.
- [ ] `kscore-module publish --registry http://localhost:8181` uploads + indexes.
- [ ] `kscore-module install vendor/example@v1.0.0` resolves dependencies, downloads ZIPs, verifies signatures + hashes, populates lockfile.
- [ ] `kscore-module test` runs Starlark unit tests.
- [ ] Module attempting `fs.write` outside allowed paths fails with clear error + audit entry.
- [ ] Module attempting unauthorized `exec` fails with audit entry.
- [ ] Re-running install with same lockfile produces identical resolution (reproducible).
- [ ] Cosign signature mismatch causes load to fail.
- [ ] `kscorectl module` dispatches correctly to `kscore-module`.
- [ ] Coverage >80% on `pkg/module/*`.

## Risks

- **Starlark sandbox escape** — defense in depth: runtime limits + capability scoping + policy + audit. Regular security review.
- **Capability creep** — defaults are permissive for ergonomics; production policies tighten.
- **Signature key rotation** — multiple trusted keys + transparency log (v1.2) for tamper detection.
- **Determinism violations** — Starlark `random()` and `time.now()` disabled by default; capability gate.
- **Module version conflicts** — MVS guarantees consistent resolution; lock file pins exact versions.
- **Lock-file drift** — validate on every load; CI must check.
- **Registry availability (SPoF)** — local cache deduplication mitigates; v1.1 mirror command.
- **Namespace squatting** — registry enforces namespaced names; validation on publish.

## References

- PROJECT-DETAILS §4.18.
