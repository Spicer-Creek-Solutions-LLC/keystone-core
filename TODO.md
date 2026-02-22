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

### Migrate from raw protoc to buf v2

**Resolution:** both

Migrate protobuf code generation from raw `protoc` invocations to `buf` using the v2 config format. The repo already has `buf.yaml` and `buf.gen.yaml` (v1 format, unused) — these need to be updated to v2 and wired into the build.

**Tasks:**

- [ ] Update `buf.yaml` to v2 format (modules, deps, lint, breaking sections)
- [ ] Update `buf.gen.yaml` to v2 format with remote plugins (`buf.build/protocolbuffers/go`, `buf.build/grpc/go`), output to `pkg/api/v1`
- [ ] Replace `make proto` target to call `buf generate` instead of raw `protoc`
- [ ] Update CI workflow (`.github/workflows/ci.yml`) to use `bufbuild/buf-setup-action`, add `buf lint` and `buf breaking` checks, remove manual protoc/plugin installs
- [ ] Verify generated output is identical (no import changes needed)
- [ ] Update developer docs to reference `buf` instead of `protoc`
