# MCP server

marvindocs implements the [Model Context Protocol](https://modelcontextprotocol.io/)
using the official [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk).

## Transports

| Transport | Flag | Endpoint |
|---|---|---|
| stdio | `--stdio` | stdin / stdout |
| Streamable HTTP | `--http ADDR` | `<addr>/mcp` |

A liveness probe is available at `<addr>/healthz` when HTTP is enabled.

## Tools

### `list_docs`

Returns every indexed documentation set.

- **Input:** `{}`
- **Output:** `{ docs: [{ slug, name, type, version, release, mtime }] }`

Use this first to discover available slugs; pass any `slug` value as
`doc_slug` to `search` or `get_page`.

### `search`

Full-text search over entry names and paths, ranked by SQLite FTS5 BM25.

- **Input:**
  - `query` *(string, required)* — user query. Tokens are prefix-matched
    (`use` matches `useState`). Special FTS characters are escaped, so
    arbitrary input is safe.
  - `doc_slug` *(string, optional)* — restrict results to a single doc.
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

### `get_page`

Fetches a single documentation page.

- **Input:**
  - `doc_slug` *(string, required)*
  - `path` *(string, required)* — path value from `search.path` or
    `index.json`.
  - `format` *(string, optional)* — `markdown` *(default)* or `html`.
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

All clients use the same three tools once connected: `list_docs`,
`search`, `get_page`.

## Security

The server performs **no authentication**. If you expose `--http` on a
public address, put it behind a reverse proxy that terminates TLS and
handles auth (e.g. nginx + basic auth, or an OAuth-aware gateway).

Clients can only read data — the tools never write to the index or the
devdocs dump.

## Verifying the server is alive

There is a bundled smoke test that drives a running HTTP server through
every tool:

```bash
marvindocs serve --http :3333 &
./scripts/mcp-smoke.sh                     # uses curl + jq
```

Expected output (abbreviated):

```
==> initialize http://localhost:3333/mcp
==> tools/list
    - get_page: Fetch a documentation page ...
    - list_docs: List all indexed documentation sets ...
    - search: Search across all indexed documentation ...
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
