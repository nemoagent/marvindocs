# marvindocs

Single-binary MCP server and web UI for the [DevDocs](https://devdocs.io/)
documentation archive. Lets AI agents and browsers search and read ~790
documentation sets (React, Python, Go, Rust, …) from a local SQLite index.

## What it does

| Feature | How |
|---|---|
| **Web UI** | Browse and search all docs in a browser at `http://localhost:3333/` |
| **MCP server** | Expose four tools to AI agents over stdio or HTTP |
| **HTML→Markdown** | `get_page` converts devdocs HTML to clean Markdown on the fly |
| **Full-text search** | SQLite FTS5, BM25 ranking, prefix matching |

## Requirements

- **Docker** — for `make build` (Go 1.25 inside container). Not needed to run the binary.
- **curl + tar** — for `scripts/sync-docs.sh` to download the docs.
- **~2 GB disk** for the default doc set; ~17 GB for everything.

## Getting started

```bash
# 1. Build the binary
make build

# 2. Download documentation (27 popular docs, ~0.6 GB on disk)
./scripts/sync-docs.sh

# 3. Start
./bin/marvindocs serve --http :3333
```

Open <http://localhost:3333/> to browse docs.
The MCP endpoint is at <http://localhost:3333/mcp>.

> `--http :3333` binds `127.0.0.1` only. Use `--http 0.0.0.0:3333` to
> accept connections from other hosts, and set `--auth-token` (or
> `MARVINDOCS_AUTH_TOKEN`) when you do.

By default `sync-docs.sh` also runs `index build` when it finishes (skip with
`--no-index`). For page-body search run
`./bin/marvindocs index build --fulltext` yourself.

### Download specific docs or everything

```bash
# Specific slugs
./scripts/sync-docs.sh react typescript python~3.13 go

# All ~790 docs (~17 GB, may take 30-90 min)
./scripts/sync-docs.sh --all
```

Slugs match names on <https://devdocs.io>. Versioned variants use a tilde:
`python~3.12`, `angular~18`, `react~18`.

### MCP over stdio (Claude Desktop, Cursor, Claude Code…)

The server also runs over stdin/stdout for local MCP clients:

```bash
./bin/marvindocs serve --stdio
```

Run both transports at once:

```bash
./bin/marvindocs serve --stdio --http :3333
```

## Connecting to AI agents

### Claude Code

```bash
claude mcp add marvindocs /absolute/path/to/marvindocs serve --stdio \
  --docs-path  /absolute/path/to/data/public/docs \
  --index-path /absolute/path/to/marvindocs.db
```

### Claude Desktop

Add to `claude_desktop_config.json`
(`~/Library/Application Support/Claude/` on macOS,
`~/.config/Claude/` on Linux):

```json
{
  "mcpServers": {
    "marvindocs": {
      "command": "/absolute/path/to/marvindocs",
      "args": ["serve", "--stdio",
               "--docs-path",  "/absolute/path/to/data/public/docs",
               "--index-path", "/absolute/path/to/marvindocs.db"]
    }
  }
}
```

### Cursor / Windsurf

Same JSON shape, different config file:

- Cursor: `~/.cursor/mcp.json`
- Windsurf: `~/.codeium/windsurf/mcp_config.json`

For all agents including Zed, Continue, VS Code Copilot Chat — see
[docs/en/clients.md](docs/en/clients.md).

## MCP tools

Once connected, the agent has four tools:

| Tool | What it does |
|---|---|
| `list_docs` | List all indexed docs with slug, name, version |
| `search` | FTS search over entry names/paths; returns ranked hits with `doc_slug` and `path` |
| `search_pages` | Full-text search over page bodies; returns hits with a snippet. Requires an index built with `index build --fulltext`, otherwise returns no hits |
| `get_page` | Fetch a page as Markdown (default) or HTML by `doc_slug` + `path` |

Verify the server with the bundled smoke test (needs `curl` + `jq`):

```bash
./bin/marvindocs serve --http :3333 &
./scripts/mcp-smoke.sh
```

## CLI reference

```
marvindocs index build   [--docs-path DIR] [--index-path FILE] [--fulltext] [--full]
                                                                # incremental sync by default; --full rebuilds
marvindocs index stats   [--index-path FILE] [--by-doc] [--top N]  # show counts
marvindocs serve         [--stdio] [--http ADDR] [--auth-token TOKEN]
                         [--docs-path DIR] [--index-path FILE]  # start server
marvindocs search QUERY  [--doc SLUG] [--limit N] [--json] [--body] [--index-path FILE]
                                                                # --body searches page bodies
marvindocs get SLUG PATH [--format markdown|html] [--docs-path DIR]  # fetch one page
marvindocs list          [--json] [--index-path FILE]           # list indexed docs
marvindocs version
```

`--fulltext` also indexes page bodies (needed for `search_pages` and
`search --body`); it is much slower and makes the index many times larger.

### Environment variables

| Variable | Values | Default |
|---|---|---|
| `MARVINDOCS_AUTH_TOKEN` | any string | empty (no auth). Used when `--auth-token` is not given; with a token set, every HTTP request except `/healthz` needs `Authorization: Bearer <token>` |
| `MARVINDOCS_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `MARVINDOCS_LOG_FORMAT` | `text`, `json` | `text` |

### Docker

`docker compose up -d` builds the image and serves HTTP on port 3333.
The compose file bind-mounts `./data/public/docs` to `/data` (read-only)
and `./data/index` to `/index`. The index file `./data/index/marvindocs.db`
must be built beforehand, for example:

```bash
./bin/marvindocs index build --index-path data/index/marvindocs.db
```

> The image listens on `0.0.0.0:3333` inside the container and compose
> publishes the port on all host interfaces. Set `MARVINDOCS_AUTH_TOKEN`
> in the compose `environment` if the host is reachable by others.

## Layout

```
scripts/sync-docs.sh     download docs (curl + tar, no Docker)
scripts/mcp-smoke.sh     verify the MCP server end-to-end
data/public/docs/        devdocs dump (not in git, ~17 GB)
marvindocs.db            SQLite FTS5 index (size depends on the doc set; much larger with --fulltext)
cmd/marvindocs/          CLI entrypoint
internal/docstore/       read-only data accessor + HTML→Markdown
internal/index/          indexer and FTS search
internal/mcpserver/      MCP tools (list_docs, search, search_pages, get_page)
internal/web/            browser UI (templates + CSS, embedded in binary)
docs/en/                 English documentation
docs/ru/                 Russian documentation
```

## Documentation

| Topic | EN | RU |
|---|---|---|
| Getting started | [docs/en/getting-started.md](docs/en/getting-started.md) | [docs/ru/getting-started.md](docs/ru/getting-started.md) |
| Syncing docs | [docs/en/sync-docs.md](docs/en/sync-docs.md) | [docs/ru/sync-docs.md](docs/ru/sync-docs.md) |
| Configuration | [docs/en/configuration.md](docs/en/configuration.md) | [docs/ru/configuration.md](docs/ru/configuration.md) |
| CLI reference | [docs/en/cli.md](docs/en/cli.md) | [docs/ru/cli.md](docs/ru/cli.md) |
| MCP server | [docs/en/mcp.md](docs/en/mcp.md) | [docs/ru/mcp.md](docs/ru/mcp.md) |
| Client setup | [docs/en/clients.md](docs/en/clients.md) | [docs/ru/clients.md](docs/ru/clients.md) |
| Web UI | [docs/en/web.md](docs/en/web.md) | [docs/ru/web.md](docs/ru/web.md) |
| Data format | [docs/en/data-format.md](docs/en/data-format.md) | [docs/ru/data-format.md](docs/ru/data-format.md) |

## License

MIT (pending).
