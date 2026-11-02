# Caddy parser

Converts the [Caddy 2](https://caddyserver.com/docs/) documentation into
a DevDocs-shaped tree for `data/public/docs/caddy/`. DevDocs upstream has
nginx and HAProxy but no Caddy, so this parser fills that gap.

Source: the Markdown pages in `src/docs/markdown/` of
[`caddyserver/website`](https://github.com/caddyserver/website); the
sidebar in `src/docs/index.html` gives the entry types (Get Caddy,
Tutorials, Reference, Articles, Developers). Directive pages are grouped
as `Caddyfile directives`.

## Build and run

```bash
cd parsers/caddy && go build -o ../../bin/caddy-parser .
./bin/caddy-parser --version 2 --out data/public/docs/caddy --force
# or via the registry:
./scripts/run-parser.sh caddy
```

| Flag | Default | Notes |
|---|---|---|
| `--source` | tarball of `master` from codeload.github.com | A `.tar.gz` URL or a local checkout of `caddyserver/website`. |
| `--version` | (empty) | Version label for `meta.json` (`2` in the registry). |
| `--out` | (required) | Output directory. |
| `--force` | `false` | Overwrite existing files. |

## What it does

- strips front matter, `<script>`/`<style>` blocks and `{{include ...}}`
  template calls; evaluates the `{{printf "..."}}` and `` {{`...`}} ``
  escapes the site uses to print literal `{{ }}`;
- renders Markdown with gomarkdown, one page per file;
- rewrites `/docs/<page>` links to relative ones, other site links to
  absolute `https://caddyserver.com/...`;
- adds an entry per page plus `<page>: <heading>` entries for H2/H3;
- skips `json` and `modules`: they are filled by JavaScript from the
  live module registry and have no static content.

Docs are (c) Matthew Holt and The Caddy Authors, Apache License 2.0.
