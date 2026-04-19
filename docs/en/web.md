# Web UI

`marvindocs serve --http :3333` starts an HTTP server that provides both
the MCP endpoint (`/mcp`) and a minimal browser UI for reading the
indexed documentation.

Open `http://localhost:3333/` in any browser.

## Routes

| Route | Purpose |
|---|---|
| `GET /` | List of all indexed documents (791 out of the box). |
| `GET /search?q=...&doc=...` | Full-text search over entry names and paths. `doc` is optional and restricts results to one slug. |
| `GET /docs/{slug}/` | Doc's `index` page plus a sidebar with all its entries. |
| `GET /docs/{slug}/{path}` | Single page. `path` comes from `search.path` or `index.json`. |
| `GET /static/app.css` | Stylesheet (embedded in the binary). |
| `GET /mcp` | MCP Streamable HTTP endpoint (used by agents, not humans). |
| `GET /healthz` | Liveness probe, returns `ok`. |

## Rendering

- The page body is the **raw HTML fragment** from devdocs `db.json`,
  wrapped in a thin shell (topbar, sidebar, CSS reset). No transformation
  is applied, so code blocks, tables and headings look as they did on
  devdocs.io.
- Relative links inside a page (e.g. `foo`, `../bar`) are resolved by
  the browser against the current URL
  (`/docs/{slug}/{current-path}`), which is exactly where other pages
  of the same doc live — so intra-doc navigation just works.
- External assets (images, scripts) referenced by the HTML are loaded
  from their original URLs. marvindocs does not proxy or cache them.

## Limitations

- **No authentication.** If you expose this on a public address, put it
  behind a reverse proxy with auth (nginx + basic auth, OAuth gateway).
- **No JavaScript.** Search and navigation are pure server-side HTML;
  there is no autocomplete or client-side routing.
- **No devdocs CSS.** The fragments ship without their original
  styling; headings, links and code blocks get a minimal look from
  `internal/web/static/app.css`.
- **No dark theme toggle** yet.

## Files

```
internal/web/
├── web.go                  handlers + embed.FS
├── templates/
│   ├── layout.html         shared head/foot
│   ├── index.html          document list
│   ├── search.html         search results
│   └── page.html           doc page + sidebar
└── static/
    └── app.css             minimal stylesheet
```

All templates and CSS are compiled into the binary via `//go:embed`.
