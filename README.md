# marvindocs

Single-binary MCP server and web UI for the [DevDocs](https://devdocs.io/)
documentation archive (~790 docs, ~17 GB on disk). Written in Go with
SQLite FTS5 for search.

## What it does

- **Web UI** — browse and search all docs in a browser.
- **MCP server** — exposes three tools (`list_docs`, `search`, `get_page`)
  over stdio (Claude Desktop / Cursor / Claude Code) and Streamable HTTP
  (remote agents).
- **On-the-fly HTML→Markdown** — MCP clients get clean Markdown instead
  of raw HTML fragments.

## Quick start

```bash
make build                            # builds ./bin/marvindocs (docker golang:1.25)
./bin/marvindocs index build          # ~2.5 min, produces ~455 MB marvindocs.db
./bin/marvindocs serve --http :3333   # web UI at / and MCP at /mcp
./bin/marvindocs serve --stdio        # MCP over stdio
```

For the devdocs dump layout, put the data under `data/public/docs/`
(same layout the official `devdocs` Rails app produces under `public/docs/`).

## Documentation

- English: [docs/en/](docs/en/README.md)
- Русский: [docs/ru/](docs/ru/README.md)

## Layout

```
data/public/docs/     source data (devdocs dump, not in git)
cmd/marvindocs/       CLI entrypoint
internal/docstore/    read-only accessor + HTML->Markdown/plain-text
internal/index/       SQLite FTS5 indexer and search
internal/mcpserver/   MCP tools
internal/web/         HTTP browser UI (templates + CSS)
docs/en/              English docs
docs/ru/              Russian docs
```

## License

MIT (pending).
