# Documentation site — `docs.keystone-core.io`

This directory holds the static-site source for `docs.keystone-core.io`, the
project's public-facing page.

It is a single hand-written page, not a generated site. The Hugo toolchain that
was going to render one belonged to Generation 1 and was removed with it in
reboot task R08; there is no `make docs-site`, no theme, and no content tree. A
documentation site returns when there is software to document — the repository
currently holds planning, governance and transition evidence, and the canonical
documentation is the Markdown in [`../../docs/`](../../docs/).

[`site/index.html`](site/index.html) is the served page. Reboot task R09
rewrote it into the reboot announcement: it states that there is nothing to
install, why the project restarted, where Generation 1 was preserved, and what
Generation 2 promises. It links only to files that exist on `main`.

## Deploying

The web host serves `deploy/docs/site/` at the root of the
`docs.keystone-core.io` virtual host, with unknown subpaths falling back to
`index.html` so that visitors who guess at deep URLs — or follow a link into
the old Generation 1 documentation tree — land on the announcement rather than
a 404.

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

Plus the standard HTTP-to-HTTPS redirect block, the same shape as the
vanity-site configuration in [`../vanity/README.md`](../vanity/README.md).

## Verifying

```bash
curl -fsSL https://docs.keystone-core.io/ | grep -E 'nothing to install'
curl -fsSL https://docs.keystone-core.io/some/deep/path | grep -E 'Keystone Core'
```

The second checks the fallback rule: a deep path that no longer exists must
still return the announcement.

## A caveat worth knowing

Nothing in CI checks this page. `make docs-links` globs `**/*.md` and runs
`lychee --offline`, so it parses no HTML and resolves no external URL. That is
how the previous placeholder came to announce a v0.5 documentation site and link
to three files that had been deleted. If you edit the links here, check them by
hand.

## Related

- [`../vanity/`](../vanity/) — the Go vanity-import site at
  `go.keystone-core.io`, same deployment pattern, different content
- [`../../docs/`](../../docs/) — the canonical Markdown documentation
