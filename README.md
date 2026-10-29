# marvindocs

Single-binary MCP server and web UI that lets AI agents and browsers
search and read ~890 documentation sets (React, Python, Go, Rust, …) from a
local SQLite index.

The project is inspired by and built on [DevDocs](https://devdocs.io/): most
documentation comes from DevDocs packs, and every doc, including those from
our own parsers, is stored in the DevDocs on-disk format. The search engine,
MCP server and web UI are written from scratch. See
[Relation to DevDocs](#relation-to-devdocs).

## What it does

| Feature | How |
|---|---|
| **Web UI** | Browse and search all docs in a browser at `http://localhost:3333/` |
| **MCP server** | Expose five tools to AI agents over stdio or HTTP |
| **HTML→Markdown** | `get_page` converts devdocs HTML to clean Markdown on the fly |
| **Full-text search** | SQLite FTS5, BM25 ranking, prefix matching |
| **Synonyms** | Search queries expand with a synonym dictionary (`k8s` finds `kubernetes`), see [docs/en/synonyms.md](docs/en/synonyms.md); `synonyms suggest` mines candidates from the docs |
| **Vector search** | Optional hybrid bge-m3 + BM25 search for `get_docs`, see [docs/en/vectors.md](docs/en/vectors.md) |

## Relation to DevDocs

[DevDocs](https://github.com/freeCodeCamp/devdocs) is a documentation browser
for people: Ruby scrapers turn official docs into packs, and a JavaScript web
app shows them and can keep them offline in the browser. marvindocs reuses
the packs and their format, and builds a service for AI agents on top.

| | DevDocs | marvindocs |
|---|---|---|
| Main user | a person in a browser | an AI agent over MCP; the web UI is secondary |
| Doc sources | its own scrapers | DevDocs packs (837 of 892 in the full set) plus own parsers: Go stdlib via `go doc`, go.dev, Laravel 4.0-13, MongoDB Manual, Metasploit (55 docs), see [docs/en/parsers.md](docs/en/parsers.md) |
| Storage | `index.json` + `db.json` per doc | the same `index.json`; pages packed into `db.zip` (`marvindocs pack`) |
| Search | entry names in the browser | SQLite FTS5 over names and page bodies, BM25, synonyms, hybrid bge-m3 vector search over chunks |
| Output | HTML pages | Markdown, and `get_docs` returns the best fragments within a token budget |
| Delivery | web app, offline cache in the browser | one Go binary (CLI, stdio and HTTP MCP, web UI), Docker, prebuilt bundles with a ready index |

marvindocs does not share code with DevDocs and is not affiliated with it.
Documentation content is covered by the licenses of its original sources.

## Requirements

- **Docker** — for `make build` (Go 1.25 inside container). Not needed to run the binary.
- **curl + tar** — for `scripts/sync-docs.sh` to download the docs.
- **Optional, for vector search** — Ollama with bge-m3 (embedding) and ~600 MB for the ONNX model and libs (`make onnx-fetch`).
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
`--no-index`). Page bodies are indexed too, so `search_pages` works out of
the box.

To refresh everything later (devdocs sync, parsers, incremental index,
embeddings) run `make update`; it never drops existing embeddings. See
[docs/en/sync-docs.md](docs/en/sync-docs.md#one-command-update).

### Offline bundles with a ready index

Instead of syncing and embedding, install prebuilt bundles (docs + index +
embeddings) from the public `bundles` GitHub release; no `gh` or token
needed, archives are checked against the release `SHA256SUMS`:

```bash
./bin/marvindocs bundle install core go python
```

Bundles: `core`, `go`, `python`, `db`, `infra`, `js`, `php`, `ruby`, `jvm`,
`native`, `other` (~2.9 GB in total). See
[docs/en/bundles.md](docs/en/bundles.md).

### Download specific docs or everything

```bash
# Specific slugs
./scripts/sync-docs.sh react typescript python~3.13 go

# All DevDocs docs (~840, ~17 GB, may take 30-90 min)
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

Once connected, the agent has five tools:

| Tool | What it does |
|---|---|
| `list_docs` | List all indexed docs with slug, name, version |
| `search` | FTS search over entry names/paths; returns ranked hits with `doc_slug` and `path` |
| `search_pages` | Full-text search over page bodies; returns hits with a snippet. |
| `get_page` | Fetch a page as Markdown (default) or HTML by `doc_slug` + `path` |
| `get_docs` | One-call answer: the most relevant Markdown fragments for a topic within a token budget; uses hybrid vector + BM25 search for docs that have vectors |

Verify the server with the bundled smoke test (needs `curl` + `jq`):

```bash
./bin/marvindocs serve --http :3333 &
./scripts/mcp-smoke.sh
```

## CLI reference

```
marvindocs index build   [--docs-path DIR] [--index-path FILE] [--full [--drop-vectors]] [--rehash]
                                                                # incremental sync by default (keeps vectors);
                                                                # --full rebuilds and needs --drop-vectors if vectors exist;
                                                                # --rehash recomputes content hashes only (once after pack --all)
marvindocs pack          (--all | SLUG...) [--docs-path DIR]    # convert db.json -> db.zip, remove db.json
marvindocs bundle build  [--out DIR] [NAME...]                  # pack bundles.yaml bundles with a ready index
marvindocs bundle install [--from DIR] NAME...                  # download bundles (gh) and merge into the index
marvindocs index stats   [--index-path FILE] [--by-doc] [--top N]  # show counts
marvindocs index embed   [--doc SLUG]... [--embedder ollama|onnx] [--batch N]
                                                                # chunk embeddings, see docs/en/vectors.md
marvindocs serve         [--stdio] [--http ADDR] [--auth-token TOKEN] [--insecure]
                         [--docs-path DIR] [--index-path FILE]
                         [--embedder none|ollama|onnx] [--ollama-url URL] [--ollama-model NAME]
                         [--model-dir DIR] [--onnxruntime PATH] [--synonyms DIR]
                                                                # start server; --embedder enables hybrid get_docs
marvindocs search QUERY  [--doc SLUG] [--limit N] [--json] [--body] [--index-path FILE] [--docs-path DIR] [--synonyms DIR]
                                                                # --body searches page bodies
marvindocs get SLUG PATH [--format markdown|html] [--docs-path DIR]  # fetch one page
marvindocs list          [--json] [--index-path FILE]           # list indexed docs
marvindocs synonyms suggest [--docs-path DIR] [--doc SLUG]... [--min-docs N] [--min-count N]
                         [--names-only] [--synonyms DIR] [--out FILE]
                                                                # mine abbreviation candidates (TSV), see docs/en/synonyms.md
marvindocs synonyms import FILE --out DIR [--force]             # reviewed TSV -> dictionary files for --synonyms
marvindocs version
```

Page bodies are indexed as well (needed for `search_pages` and
`search --body`). The text itself is not stored; snippets are rebuilt from
`db.zip` on demand.

### Environment variables

| Variable | Values | Default |
|---|---|---|
| `MARVINDOCS_AUTH_TOKEN` | any string | empty (no auth). Used when `--auth-token` is not given; with a token set, every HTTP request except `/healthz` needs `Authorization: Bearer <token>` |
| `MARVINDOCS_INSECURE` | `1` | allow a public `--http` bind without a token (same as `--insecure`); otherwise `serve` refuses to start |
| `MARVINDOCS_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `MARVINDOCS_LOG_FORMAT` | `text`, `json` | `text` |
| `MARVINDOCS_EMBEDDER` | `none`, `ollama`, `onnx` | default of `--embedder` (`onnx` for `index embed`, `none` for `serve`) |
| `MARVINDOCS_OLLAMA_URL`, `MARVINDOCS_OLLAMA_MODEL` | URL, model name | `http://localhost:11434`, `bge-m3` |
| `MARVINDOCS_MODEL_DIR`, `MARVINDOCS_ONNXRUNTIME` | paths | `data/onnx/model`, `data/onnx/lib/libonnxruntime.so` |
| `MARVINDOCS_SYNONYMS` | directory, `off` | empty (builtin dictionary only). Default of `--synonyms`; a directory adds its `*.md` files to the builtin dictionary, `off` disables query expansion |

The embedder variables are flag defaults (flags win). `scripts/update.sh` loads
them from a git-ignored `.env`; copy the template `.env.example`.

### Docker

`docker compose up -d` builds the image and serves HTTP on port 3333.
The compose file bind-mounts `./data/public/docs` to `/data` (read-only)
and `./data/index` to `/index`. The index file `./data/index/marvindocs.db`
must be built beforehand, for example:

```bash
./bin/marvindocs index build --index-path data/index/marvindocs.db
# or install bundles straight into the mounted paths
./bin/marvindocs bundle install --docs-path data/public/docs --index-path data/index/marvindocs.db core go
```

> The image listens on `0.0.0.0:3333` inside the container and compose
> publishes the port on all host interfaces. Set `MARVINDOCS_AUTH_TOKEN`
> in the compose `environment` if the host is reachable by others.

## Layout

```
scripts/sync-docs.sh     download docs (curl + tar, no Docker)
scripts/mcp-smoke.sh     verify the MCP server end-to-end (no auth header: use a server without a token)
scripts/fetch-onnx.sh    download the ONNX runtime, tokenizer and bge-m3 model for --embedder onnx
data/public/docs/        DevDocs dump and parser output (not in git, ~17 GB)
bundles.yaml             offline bundle definitions (docs/en/bundles.md)
dist/bundles/            archives from `bundle build` (not in git)
marvindocs.db            SQLite FTS5 index (size depends on the doc set; page bodies indexed, text not stored)
cmd/marvindocs/          CLI entrypoint
internal/chunk/          Markdown section splitting and scoring for get_docs
internal/docstore/       read-only data accessor + HTML→Markdown
internal/embed/          embedding backends (Ollama, ONNX) and int8 vector quantisation
internal/mdcache/        in-memory LRU cache of HTML→Markdown conversions
internal/logging/        slog setup (level and text/json format)
internal/index/          indexer and FTS search
internal/synonyms/       synonym dictionary for query expansion (builtin/*.md embedded in binary)
internal/mcpserver/      MCP tools (list_docs, search, search_pages, get_page, get_docs)
internal/web/            browser UI (templates + CSS, embedded in binary)
synonyms/                optional IT abbreviation dictionary for --synonyms (289 concepts)
docs/en/                 English documentation
docs/ru/                 Russian documentation
```

## Documentation

| Topic | EN | RU |
|---|---|---|
| Getting started | [docs/en/getting-started.md](docs/en/getting-started.md) | [docs/ru/getting-started.md](docs/ru/getting-started.md) |
| Syncing docs | [docs/en/sync-docs.md](docs/en/sync-docs.md) | [docs/ru/sync-docs.md](docs/ru/sync-docs.md) |
| Offline bundles | [docs/en/bundles.md](docs/en/bundles.md) | [docs/ru/bundles.md](docs/ru/bundles.md) |
| Configuration | [docs/en/configuration.md](docs/en/configuration.md) | [docs/ru/configuration.md](docs/ru/configuration.md) |
| CLI reference | [docs/en/cli.md](docs/en/cli.md) | [docs/ru/cli.md](docs/ru/cli.md) |
| MCP server | [docs/en/mcp.md](docs/en/mcp.md) | [docs/ru/mcp.md](docs/ru/mcp.md) |
| Client setup | [docs/en/clients.md](docs/en/clients.md) | [docs/ru/clients.md](docs/ru/clients.md) |
| Web UI | [docs/en/web.md](docs/en/web.md) | [docs/ru/web.md](docs/ru/web.md) |
| Data format | [docs/en/data-format.md](docs/en/data-format.md) | [docs/ru/data-format.md](docs/ru/data-format.md) |
| Synonyms | [docs/en/synonyms.md](docs/en/synonyms.md) | [docs/ru/synonyms.md](docs/ru/synonyms.md) |
| Vector search | [docs/en/vectors.md](docs/en/vectors.md) | [docs/ru/vectors.md](docs/ru/vectors.md) |
| Metasploit docs | [docs/en/metasploit.md](docs/en/metasploit.md) | [docs/ru/metasploit.md](docs/ru/metasploit.md) |
| Parsers | [docs/en/parsers.md](docs/en/parsers.md) | [docs/ru/parsers.md](docs/ru/parsers.md) |
| Go docs via Docker | [docs/en/go-doc.md](docs/en/go-doc.md) | [docs/ru/go-doc.md](docs/ru/go-doc.md) |
| Consistency checks | [docs/en/consistency-checks.md](docs/en/consistency-checks.md) | [docs/ru/consistency-checks.md](docs/ru/consistency-checks.md) |

## License

MIT (pending).
