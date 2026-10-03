# MCP server

marvindocs implements the [Model Context Protocol](https://modelcontextprotocol.io/)
using the official [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk).

## Transports

| Transport | Flag | Endpoint |
|---|---|---|
| stdio | `--stdio` | stdin / stdout |
| Streamable HTTP | `--http ADDR` | `<addr>/mcp` |

A liveness probe is available at `<addr>/healthz` when HTTP is enabled.

A bare-port address such as `--http :3333` is rewritten to
`127.0.0.1:3333` (loopback only). To listen on all interfaces, pass an
explicit host: `--http 0.0.0.0:3333`.

## Tools

### `list_docs`

Returns every indexed documentation set.

- **Input:** `{}`
- **Output:** `{ docs: [{ slug, name, type, version, release, mtime }] }`

Use this first to discover available slugs; pass any `slug` value as
`doc_slug` to `search` or `get_page`.

### `search`

Full-text search over entry names and paths, ranked by SQLite FTS5 BM25
with `name` weighted 2x over `path`. When no doc filter is active
(neither explicit nor auto-detected), results from popular docs (the
default sync set: postgresql, python, react, go, ...) get a small score
boost as a tiebreaker for cross-doc queries. If FTS returns fewer than
`limit/2` hits, the result is topped up with a substring (`LIKE`) match
on entry names: first a prefix match (`q%`, served from an index in
milliseconds), and only if that finds nothing a `%q%` scan. The scan is
skipped for queries shorter than 3 characters unless a doc filter is
active.

- **Input:**
  - `query` *(string, required)* — user query. Tokens are prefix-matched
    (`use` matches `useState`). Special FTS characters are escaped, so
    arbitrary input is safe.
  - `doc_slug` *(string, optional)* — restrict results to a single doc.
    Must be an exact slug (see `list_docs`). If not set, query tokens
    are checked case-insensitively against known slugs (and their
    prefixes up to `~`/`-`): the first matching token is removed from the
    query and the search is automatically scoped to that doc or group
    (e.g. `python~*`). Aliases map to canonical names: `js` → javascript,
    `py` → python, `ts` → typescript, `pg` → postgresql, `mongo` →
    mongodb. If the doc token is the only token, no filter is applied.
    If the scoped search finds nothing (e.g. `async function`, where
    `async` is also a doc slug), the original query is searched globally.
    Example: `{"query": "python requests"}` → scoped to python docs only.
  - `limit` *(int, optional)* — default `20`, max `100`.
- **Output:**
  `{ hits: [{ doc_slug, name, path, type, score }] }`. Lower `score`
  means a better match.

Example (pseudo-JSON):

```json
{"name": "search", "arguments": {"query": "useState", "limit": 3}}
```

```json
{
  "hits": [
    {"doc_slug": "react", "name": "useState", "path": "reference/react/useState", "type": "Hooks", "score": -7.51},
    ...
  ]
}
```

### `search_pages`

Full-text search across **page bodies** (not just entry names). Use
this when `search` returns no hits because your query mentions concepts
that aren't entry names — `jsonb operators`, `connection pool size`,
`context manager with statement`.

- **Input:** same shape as `search`; the doc-filter auto-detection from
  query tokens works the same way. The popularity boost is applied here
  too, again only when no doc filter is active.
- **Output:** `{ hits: [{ doc_slug, path, snippet, score }] }`.
  `snippet` is a short excerpt (~24 words around the first match, rebuilt
  from `db.json` at query time) with the matched terms wrapped in `[ ]`.

### `get_page`

Fetches a single documentation page.

- **Input:**
  - `doc_slug` *(string, required)*
  - `path` *(string, required)* — path value from `search.path` or
    `index.json`.
  - `format` *(string, optional)* — `markdown` *(default; alias `md`)*
    or `html`, case-insensitive. Any other value returns an
    `unknown format` error.
- **Output:**
  `{ doc_slug, path, format, content }`. When `format=markdown`
  the content is produced on-the-fly by the HTML→Markdown converter.

## Client configuration

See **[clients.md](clients.md)** for ready-to-paste configuration for:

- Claude Desktop (stdio)
- Claude Code (stdio, via `claude mcp add`)
- Cursor (stdio)
- Windsurf (stdio)
- Zed (stdio)
- Continue — VS Code / JetBrains (stdio)
- VS Code / GitHub Copilot Chat (stdio and HTTP)
- Generic HTTP client (Go SDK example)

All clients use the same four tools once connected: `list_docs`,
`search`, `search_pages`, `get_page`.

## Security

By default the HTTP server performs **no authentication**. Pass
`--auth-token TOKEN` (or set `MARVINDOCS_AUTH_TOKEN`) to require
`Authorization: Bearer <token>` on every HTTP request, including `/mcp`
and the web UI; only `/healthz` is exempt. The token does not apply to
`--stdio`.

If you expose `--http` on a public address, also put it behind a reverse
proxy that terminates TLS (the token is sent in clear text over plain
HTTP) or handles auth (e.g. nginx + basic auth, or an OAuth-aware
gateway).

The `/mcp` endpoint has DNS-rebinding protection built into the MCP SDK:
a request that arrives on a loopback address (127.0.0.1, ::1) but carries
a non-loopback `Host` header is rejected with 403. Use `localhost` or
`127.0.0.1` in client URLs for a local server; LAN access via the
server's IP or hostname is not affected.

Clients can only read data — the tools never write to the index or the
devdocs dump.

## Verifying the server is alive

There is a bundled smoke test that drives a running HTTP server through
every tool:

```bash
marvindocs serve --http :3333 &
./scripts/mcp-smoke.sh                     # uses curl + jq
```

Expected output (abbreviated; the doc count depends on your dump):

```
==> initialize http://localhost:3333/mcp
==> tools/list
    - get_page: Fetch a documentation page ...
    - list_docs: List all indexed documentation sets ...
    - search: Search across all indexed documentation ...
    - search_pages: Full-text search across page bodies (not just symbol names). Use this ...
==> tools/call list_docs (first 3)
    791 docs, first 3:
      angular  Angular
      ...
==> tools/call search query=useState limit=3
    react  useState  reference/react/usestate
    ...
==> tools/call get_page react reference/react/usestate (markdown)
    length=24496 chars
OK
```

Point the script at another host with `MCP_URL=http://host:3333/mcp`.

Two more stdio helpers live in `scripts/` (both need a built
`./bin/marvindocs` and an index):

- `python3 scripts/mcp-bench.py` — latency bench: per-call round-trip
  time over stdio, using `postgresql~17` from the index as the target.
- `python3 scripts/mcp-quality.py` — manual quality eval: runs curated
  developer queries through `search` + `get_page` and prints a per-case
  report (no assertions).

For a quick manual check you can also just do:

```bash
# 1. initialize (grab the session id from the response headers)
curl -sS -i -X POST http://localhost:3333/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize",
       "params":{"protocolVersion":"2025-06-18","capabilities":{},
                 "clientInfo":{"name":"c","version":"0"}}}'
```

The server responds with `Mcp-Session-Id: <id>`; resend it as a header
on subsequent `tools/list` / `tools/call` requests.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `index file not found` | Run `marvindocs index build` first. |
| `open docs.json: no such file` | `--docs-path` is wrong; expected `<path>/docs.json`. |
| Tool call returns an empty `hits` list | Query has no prefix matches, or the doc isn't indexed. Try `list_docs` first. |
| `get_page` returns "page not found" | `path` must be a value from `search.path` or `index.json`, not a URL. |
