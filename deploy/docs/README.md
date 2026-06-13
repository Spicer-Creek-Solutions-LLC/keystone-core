# Documentation site — `docs.keystone-core.io`

This directory holds the static-site source for `docs.keystone-core.io`,
the public-facing documentation surface.

The full Hugo site has now landed (the gate-v0.5
["Hugo docs site"](../../docs/project/ROADMAP.md) milestone): the Hugo
source lives under [`../../docs/`](../../docs/) and renders with
`make docs-site` (see [`docs/SITE.md`](../../docs/SITE.md)). The
**branded placeholder** here remains the served page until the rendered
site is actually deployed at `docs.keystone-core.io` — there is no
hosting infrastructure for that domain yet (it is still aspirational; it
is excluded from the CI link gate in [`.lychee.toml`](../../.lychee.toml)
for that reason).

## Publishing the real site

The published artifact is the **`make docs-site` output** (`docs/public/`,
a build artifact — gitignored, not committed). The deploy step is:

1. `make install-hugo` (one-time: pinned Hugo Extended).
2. `make docs-site` → renders `docs/public/`.
3. Serve `docs/public/` at the root of the `docs.keystone-core.io`
   virtual host (the same serving shape as the placeholder below).

`make docs-links-site` link-checks the rendered output (CI gates both
the build and the links on every PR). When the site goes live, swap the
web host's document root from `deploy/docs/site/` (placeholder) to the
generated `docs/public/`.

## What's in here

- [`site/index.html`](site/index.html) — the **placeholder** page
  currently served at `docs.keystone-core.io`. Links visitors to the
  canonical docs in the source repo. Retired once `docs/public/` is
  deployed.

## How the placeholder is meant to be deployed

Same pattern as the Go vanity-import site under [`../vanity/`](../vanity/):

1. The web host serves `deploy/docs/site/` at the root of the
   `docs.keystone-core.io` virtual host.
2. The site root resolves to `index.html` (the placeholder).
3. Unknown subpaths fall back to `index.html` too — so visitors who
   guess at deep URLs (e.g., `/cli`, `/getting-started`) still get
   the placeholder pointing them at the real docs in the repo.

The fallback rule (deep paths → index.html) intentionally mirrors how
Hugo-generated sites will behave at v0.5: SPA-style routing keeps
deep links working.

### Caddy

```caddyfile
docs.keystone-core.io {
    root * /var/www/docs.keystone-core.io
    try_files {path} /index.html
    file_server
}
```

Where `/var/www/docs.keystone-core.io` is `deploy/docs/site/` on disk.

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name docs.keystone-core.io;

    ssl_certificate     /etc/letsencrypt/live/docs.keystone-core.io/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/docs.keystone-core.io/privkey.pem;

    root /var/www/docs.keystone-core.io;

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

Plus the standard HTTP→HTTPS redirect block (same shape as the
vanity-site nginx config in [`../vanity/README.md`](../vanity/README.md)).

## Verifying

```bash
curl -fsSL https://docs.keystone-core.io/ | grep -E 'Keystone Core|v0.5'
# Should print the placeholder's heading + the v0.5 note.

curl -fsSL https://docs.keystone-core.io/some/deep/path | grep -E 'Keystone Core'
# Should also return the placeholder (fallback rule working).
```

## When this gets replaced

The v0.5 ROADMAP entry "Hugo docs site" describes the eventual
acceptance criteria:

- `docs/` builds a Hugo site under `docs/public/` via `make docs-site`
- Auto-generated CLI / config / API references regenerate via
  `make docs-sync` into the Hugo content tree
- Site mirrors `docs/project/` structure with per-page navigation
- Hosted at `docs.keystone-core.io` (this URL)
- `lychee` link-check covers the rendered site

At that point this README gets rewritten to describe the Hugo
toolchain + content tree, and `site/index.html` is replaced by
Hugo's generated output.

## Related

- [`../vanity/`](../vanity/) — the parallel setup for the Go
  vanity-import static site at `go.keystone-core.io`. Same deployment
  pattern, different content.
- [`../../docs/`](../../docs/) — the source Markdown docs that this
  site (eventually) renders.
- [`../../docs/project/ROADMAP.md`](../../docs/project/ROADMAP.md)
  &mdash; the "Hugo docs site" gate-v0.5 entry that this placeholder
  is interim coverage for.
