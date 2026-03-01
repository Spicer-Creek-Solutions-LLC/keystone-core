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

### Vanity import path and hosting migration

Moved to **Epic 62**. See `epics/62-vanity-import-and-hosting-migration.md`.
