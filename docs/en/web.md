# Web UI

`marvindocs serve --http :3333` (loopback only; use `0.0.0.0:3333` for
remote access) starts an HTTP server that provides both
the MCP endpoint (`/mcp`) and a minimal browser UI for reading the
indexed documentation.

Open `http://localhost:3333/` in any browser.

## Routes

| Route | Purpose |
|---|---|
| `GET /` | List of all indexed documents (the count depends on the synced dump). |
| `GET /search?q=...&doc=...` | Search over entry names and paths (not page bodies), up to 50 results. `doc` is optional and restricts results to one slug. |
| `GET /docs/{slug}/` | Doc's `index` page plus a sidebar with all its entries. |
| `GET /docs/{slug}/{path}` | Single page. `path` comes from `search.path` or `index.json`. |
| `GET /static/...` | Embedded assets (`app.css`, `theme.js`, `app.js`), served with `Cache-Control: public, max-age=86400`. |
| `/mcp` | MCP Streamable HTTP endpoint (used by agents, not humans). Registered for all methods; clients mostly use `POST`. |
| `GET /healthz` | Liveness probe, returns `ok`. |

`/search` uses the same index search as the MCP `search` tool: the doc
filter is auto-detected from the query when `doc` is empty, popular docs
get a small boost, and a substring fallback tops up sparse results. An
empty `q` shows a prompt instead of results.

## Rendering

- The page body is the **raw HTML fragment** from devdocs `db.json`,
  wrapped in a thin shell (topbar, sidebar, CSS reset). No transformation
  is applied, so code blocks, tables and headings look as they did on
  devdocs.io.
- Relative links inside a page (e.g. `foo`, `../bar`) are resolved by
  the browser against the current URL
  (`/docs/{slug}/{current-path}`), which is exactly where other pages
  of the same doc live — so intra-doc navigation just works.
- Doc pages are sent with a strong `ETag` and
  `Cache-Control: public, max-age=300`; a matching `If-None-Match`
  gets `304 Not Modified`. An unknown slug or page returns `404`.
- External images referenced by the HTML are loaded from their original
  URLs (`https:` only). marvindocs does not proxy or cache them.
- Every response carries hardening headers: a
  `Content-Security-Policy` that permits scripts only from the same
  origin (no inline), inline styles, same-origin and `https:`/`data:`
  images, and no framing (`frame-ancestors 'none'`); plus
  `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY` and
  `Referrer-Policy: no-referrer`. A `<script>` or `on*=` handler that
  slipped into a devdocs dump is therefore not executed; external
  scripts, iframes and fonts inside doc pages are blocked too.

## Limitations

- **Authentication is Bearer-token only.** Loopback binds run without
  auth; any other `--http` address requires `--auth-token` (or
  `MARVINDOCS_AUTH_TOKEN`) unless `--insecure` is given, and then every
  request except `/healthz` must carry `Authorization: Bearer <token>`.
  If you expose this on a public address, also put it behind a reverse
  proxy with TLS and/or auth (nginx + basic auth, OAuth gateway).
- **Minimal JavaScript.** Search and navigation are server-side HTML;
  the only scripts are `/static/theme.js` (applies the theme before
  first paint) and `/static/app.js` (the toggle). There is no autocomplete or
  client-side routing.
- **No devdocs CSS.** The fragments ship without their original
  styling; headings, links and code blocks get a minimal look from
  `internal/web/static/app.css`.
- **Theme.** A light/dark toggle in the topbar stores the choice in
  `localStorage`; without a stored choice the browser's
  `prefers-color-scheme` is used.

## Files

```
internal/web/
├── web.go                  handlers + embed.FS
├── gzip.go                 gzip response compression
├── log.go                  access log middleware
├── templates/
│   ├── layout.html         shared head/foot
│   ├── index.html          document list
│   ├── search.html         search results
│   └── page.html           doc page + sidebar
└── static/
    ├── app.css             minimal stylesheet
    ├── theme.js            applies saved theme before first paint
    └── app.js              theme toggle
```

All templates and CSS are compiled into the binary via `//go:embed`.
