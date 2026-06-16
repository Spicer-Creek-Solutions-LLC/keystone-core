<!-- SPDX-License-Identifier: Apache-2.0 -->

# Documentation site (Hugo + Hextra)

This directory is the [Hugo](https://gohugo.io) root for the rendered
documentation site (the [Hextra](https://imfing.github.io/hextra/)
theme). It graduates the in-repo Markdown into a searchable, navigable
site — the gate-v0.5 "Hugo docs site" milestone.

## Single source of truth

The site does **not** copy or move any docs. The canonical Markdown
trees stay exactly where they are and are *mounted* into the Hugo
content tree in place (see `hugo.toml` `[[module.mounts]]`):

| Source (canonical)   | Rendered under     |
|----------------------|--------------------|
| `docs/project/`      | `/docs/reference/` |
| `docs/runbooks/`     | `/docs/operations/`|
| `docs/adr/`          | `/docs/adr/`       |

The auto-generated CLI / configuration / API references keep being
written to `docs/project/` by `make docs-sync` — edit the generators,
not the rendered pages.

The same mount-don't-copy rule covers branding: the repo-root `assets/`
directory (the canonical logo) is mounted to `static/keystone`, so the
navbar logo (`[params.navbar.logo]` in `hugo.toml`) is served verbatim
from the single source of truth — replace `assets/logo.svg` to update it
everywhere.

## Building

```sh
make install-hugo     # one-time: pinned Hugo Extended into $(GOPATH)/bin
make docs-site        # renders to docs/public/ (gitignored)
make docs-links-site  # build + link-check the rendered site (lychee)
```

Hugo Extended is required (Hextra compiles SCSS). The theme is pulled as
a Hugo module per `go.mod` / `go.sum`. CI builds the site **and**
link-checks the rendered output on every PR (both must pass).

## Layout

| Path                     | Purpose                                        |
|--------------------------|------------------------------------------------|
| `hugo.toml`              | Site config + theme import + content mounts.   |
| `go.mod` / `go.sum`      | Hugo-module pin for the Hextra theme.          |
| `content/`              | Hand-authored pages: the landing page + the section index pages that organise the mounted trees. |
| `layouts/shortcodes/`    | Local shortcode shims (e.g. an `alert` compat shim for the Docsy-style callout used in a few docs). |
| `layouts/_default/_markup/render-link.html` | Link render hook (see below). |
| `public/`                | Build output (gitignored).                     |

## Link rewriting

The mounted docs use relative Markdown links written for the repo
(`ROADMAP.md`, `../../RELEASE-PLAYBOOK.md`, `../../internal/foo.go`).
A link render hook (`layouts/_default/_markup/render-link.html`) resolves
each one against the page's real repo source directory and rewrites it to
either the **rendered in-site URL** (when the target is another mounted
doc) or an **absolute Codeberg source URL** (for repo-root files and
source code). So the rendered site is self-consistent and
`make docs-links-site` passes — without editing any canonical file.

## Deployment

The published artifact is the `make docs-site` output (`docs/public/`).
See [`../deploy/docs/README.md`](../deploy/docs/README.md) for the
publish path — serving `docs/public/` at `docs.keystone-core.io`. No
hosting infrastructure for that domain exists yet (it is still
aspirational), so a branded placeholder is served there in the meantime.
