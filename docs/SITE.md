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

## Building

```sh
make install-hugo   # one-time: pinned Hugo Extended into $(GOPATH)/bin
make docs-site      # renders to docs/public/ (gitignored)
```

Hugo Extended is required (Hextra compiles SCSS). The theme is pulled as
a Hugo module per `go.mod` / `go.sum`. CI builds the site on every PR as
a gate (it must render cleanly).

## Layout

| Path                     | Purpose                                        |
|--------------------------|------------------------------------------------|
| `hugo.toml`              | Site config + theme import + content mounts.   |
| `go.mod` / `go.sum`      | Hugo-module pin for the Hextra theme.          |
| `content/`              | Hand-authored pages: the landing page + the section index pages that organise the mounted trees. |
| `layouts/shortcodes/`    | Local shortcode shims (e.g. an `alert` compat shim for the Docsy-style callout used in a few docs). |
| `public/`                | Build output (gitignored).                     |

## Not yet wired (follow-ups)

Per-page front-matter/title curation and sidebar ordering, full-text
search tuning, a link-check pass over the rendered `public/` output, and
the `docs.keystone-core.io` publish path (`deploy/docs/`) land in
follow-up PRs.
