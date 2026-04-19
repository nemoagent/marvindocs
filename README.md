# marvindocs

Single-binary MCP server and web UI for the DevDocs documentation archive
(~790 docs, ~17 GB on disk). Written in Go with SQLite FTS5 for search.

## What it does

- **Web UI** — browse and search all docs in a browser.
- **MCP server** — expose the same data to LLM agents over either
  stdio (for Claude Desktop / Cursor / Claude Code) or Streamable HTTP
  (for remote use).
- **On-the-fly HTML→Markdown** — MCP clients get clean Markdown instead
  of raw HTML fragments.

## Layout

```
docs/public/docs/     source data (devdocs dump, not in git)
cmd/marvindocs/       CLI entrypoint
internal/docstore/    loader for docs.json / index.json / db.json
internal/index/       SQLite FTS5 index builder + search
internal/mcpserver/   MCP tools
internal/web/         HTTP handlers and templates
documentation/ru/     Russian docs
documentation/en/     English docs
```

## Quick start

```bash
make build            # builds ./bin/marvindocs (via docker golang:1.23)
./bin/marvindocs index build
./bin/marvindocs serve --http :3333
# open http://localhost:3333
```

MCP over stdio (for Claude Desktop):

```bash
./bin/marvindocs serve --stdio
```

MCP over HTTP: `POST http://localhost:3333/mcp`

See `documentation/en/` for details, `documentation/ru/` for Russian.
