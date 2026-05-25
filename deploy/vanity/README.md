# Go vanity import path — `go.keystone-core.io`

This directory holds the static-site artifacts that make
`go.keystone-core.io/keystone-core` resolve as a Go vanity import
path. The actual source code lives on Codeberg (and is mirrored to
GitHub); the vanity domain is a stable indirection so the public
import path can stay constant even if the underlying git host
changes.

## What's in here

- [`vangen.json`](vangen.json) — config consumed by
  [vangen](https://github.com/leighmcculloch/vangen) (module path
  `4d63.com/vangen`) to generate the static HTML. One repo entry,
  pointing at the canonical Codeberg URL.
- [`site/`](site/) — the generated HTML, committed so deploys
  don't need the `vangen` tool installed. Regenerated via
  `make vanity-regen` (see below).

The `site/` tree currently contains a single file,
`site/keystone-core/index.html`. The web host serves it at the path
`/keystone-core` (or any path under `/keystone-core/...` — Go's
remote-import resolver walks up the path looking for the meta tag).

## How Go's vanity-import resolution works

When `go get go.keystone-core.io/keystone-core/pkg/foo` runs, the
Go toolchain issues an HTTP GET to:

```text
https://go.keystone-core.io/keystone-core/pkg/foo?go-get=1
```

It walks up the path until it finds an HTML response containing a
`<meta name="go-import">` tag. The tag's content tells Go where the
actual git repository lives:

```html
<meta name="go-import" content="go.keystone-core.io/keystone-core git https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core">
```

The generated `index.html` also includes a `go-source` meta tag so
that `pkg.go.dev` (and other doc indexes) can deep-link individual
files and directories back to the Codeberg source view.

## Regenerating the dist tree

```bash
make install-tools   # installs vangen (and the other dev tools) if not present
make vanity-regen
```

Or directly:

```bash
vangen -config deploy/vanity/vangen.json -out deploy/vanity/site/
```

Commit the regenerated `site/` alongside any `vangen.json` change.
Reviewers should diff both the config and the generated HTML to
confirm the meta tags reflect the intended change.

## Deploying

The web host needs to:

1. Serve every path under `https://go.keystone-core.io/keystone-core` from
   `site/keystone-core/index.html` — Go's resolver appends arbitrary
   subpaths (`/pkg/api/v1`, `/internal/secrets`, …) and the same
   meta tag is the correct response for all of them.
2. Serve HTTPS (Go's resolver requires HTTPS by default; HTTP-only
   triggers an `insecure` failure).
3. Honour the `?go-get=1` query string (no special handling needed;
   the meta tag is valid HTML for human visitors too).

Concrete host-agnostic config: any static-site host that supports
"serve `<file>` for any path under `<prefix>`" works. Examples:

- **Caddy**: `try_files {path} /keystone-core/index.html` under a
  site block for `go.keystone-core.io`.
- **nginx**: `try_files $uri $uri/ /keystone-core/index.html;` inside
  a `location /keystone-core` block.
- **Cloudflare Pages / Workers**: serve the `site/` tree as the site
  root with a catch-all rewrite to `/keystone-core/index.html` for
  paths starting with `/keystone-core/`.
- **Codeberg / GitHub Pages**: works without rewrite rules because
  Go's resolver walks up the path; ensure the published site root
  matches the `site/` tree so `/keystone-core/index.html` is reachable.

## Verifying

After deploying, confirm the meta tag is being served:

```bash
curl -fsSL -H 'User-Agent: Go-http-client/1.1' \
  'https://go.keystone-core.io/keystone-core?go-get=1' \
  | grep -E 'go-import|go-source'
```

Should print both meta tags with the Codeberg URL filled in. Then,
from a clean module (outside this repo), confirm Go resolves the
path end-to-end:

```bash
cd /tmp && mkdir vanity-smoke && cd vanity-smoke
go mod init smoke
GOPROXY=direct go list -m -versions go.keystone-core.io/keystone-core
# Should list the project's tags (e.g. v0.1.0, v0.1.0-rc1, ...).
```

`GOPROXY=direct` bypasses `proxy.golang.org` so the test exercises
the vanity-tag → Codeberg-git path end-to-end. Once the meta tag is
live, the module proxy will also start mirroring tags after the
first resolution.

## Updating when things change

- **Repo URL change** (e.g. Codeberg → another forge): edit the
  `url`, `source.home`, `source.dir`, `source.file`, and
  `website.url` fields in `vangen.json`, regenerate, deploy.
- **New module at the same vanity domain** (e.g. an
  `sdk-go` companion module): add a second entry to the
  `repositories` array in `vangen.json` with its own `prefix` +
  `url` + `source`, regenerate. `site/` will gain a new subdirectory
  per entry; deploy serves each from its respective path.
- **Vangen version bump**: regeneration may produce diff noise in
  the HTML body (styles, layout) without changing the meta tags.
  That's fine; the meta-tag content is what matters for Go.

## Why vangen instead of hand-rolling

A single HTML file with two meta tags is genuinely trivial to write
by hand. The reasons to use vangen anyway:

- **Standard tool**. Used by other Go projects with vanity paths;
  failure modes are well-known.
- **Future-proof for multi-module domains**. The next time we add
  an SDK or sister module at `go.keystone-core.io/<name>`, it's a
  JSON entry, not a hand-written HTML file.
- **Self-documenting config**. `vangen.json` reads as "what does
  the vanity domain serve?"; raw HTML reads as "here is some
  text that includes some Go-import-protocol jargon."
- **No deploy-time dependency**. Vangen runs at regen time, not
  deploy time — the dist tree ships as static files.

The cost (one Go binary installable via `make install-tools`) is
small compared to the maintenance benefits.

## Related

- The vanity domain itself: see
  [`../../OWNERSHIP.md`](../../OWNERSHIP.md) (SCS LLC owns
  `keystone-core.io`).
- The provisioning context: see
  [`../../docs/project/PUBLIC-LAUNCH-CHECKLIST.md`](../../docs/project/PUBLIC-LAUNCH-CHECKLIST.md)
  Phase B6 + F-phase for the domain-provisioning story.
- The lychee exclusion that lets CI link-checking pass before the
  domain is live: see [`../../.lychee.toml`](../../.lychee.toml)
  (`@keystone-core.io` exclusion + pre-launch-placeholder comment).
