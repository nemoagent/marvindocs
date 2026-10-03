# Getting started

This page takes you from a fresh checkout to a running MCP server.

## 1. Prerequisites

- **Docker** — the default `Makefile` runs Go inside `golang:1.25` so you
  don't need Go or a C toolchain installed locally.
- **~20 GB disk** — 17 GB for the full devdocs dump; the index size depends
  on the doc set and grows a lot with `--fulltext`.
- *(Optional)* **Go 1.25+ and a C toolchain** — if you prefer native
  builds, the SQLite driver is CGo-based, so build with
  `CGO_ENABLED=1 go build -tags sqlite_fts5 ./cmd/marvindocs`. Without
  the `sqlite_fts5` build tag the FTS5 extension is missing and the
  index will not open.

## 2. Get the devdocs data

marvindocs does not ship the documentation itself — it reads the raw
devdocs archive. There are two ways to obtain it.

### Option A: run the devdocs Rails app locally and download packs

See https://github.com/freeCodeCamp/devdocs for the canonical instructions.
Once the app has downloaded the packs, copy its `public/docs/` directory
into this repo as `data/public/docs/`.

### Option B: use an existing dump

If you already have a snapshot somewhere, put it under `data/public/docs/`
so the final layout looks like:

```
data/public/docs/
├── docs.json               # list of all documents
├── angular/
│   ├── meta.json
│   ├── index.json
│   └── db.json
├── python~3.12/
│   └── ...
└── ...
```

`docs.json` is mandatory. Each doc slug directory must contain at least
`index.json` and `db.json`.

## 3. Build the binary

```bash
make build
```

This runs the build inside Docker and drops a single binary at
`./bin/marvindocs` (about 19 MB; dynamically linked against the system
libc because the SQLite driver uses CGo).

## 4. Build the search index

```bash
./bin/marvindocs index build
```

Expected output (illustrative numbers):

```
level=INFO msg="loaded docstore" docs=27 path=data/public/docs
level=INFO msg="index synced" docs=27 entries=120000 pages=0 duration=15s path=marvindocs.db
```

The command creates `marvindocs.db`; its size depends on the doc set.
Add `--fulltext` to also index page bodies (needed for `search_pages` and
`search --body`) — this is much slower and the index grows by gigabytes
on a large dump. It is safe to rerun: by default the build is
incremental, so only changed docs are re-indexed. Pass `--full` to
rebuild everything (the last line then reads `index indexed`).

Check the result:

```bash
./bin/marvindocs index stats
```

## 5. Start the MCP server

### Over stdio (for Claude Desktop, Cursor, Claude Code)

```bash
./bin/marvindocs serve --stdio
```

The process reads JSON-RPC from stdin and writes to stdout. See
[MCP client configuration](mcp.md#client-configuration).

### Over HTTP (MCP + web UI)

```bash
./bin/marvindocs serve --http :3333
```

Opens two things at once:

- Web UI for browsing docs in a browser: <http://localhost:3333/>
- MCP Streamable HTTP endpoint: <http://localhost:3333/mcp>

A liveness probe is also exposed at `/healthz`.

A bare `:3333` binds `127.0.0.1` only. To serve the LAN, bind an explicit
address and set a token (the server refuses to start on a non-loopback
address without one):

```bash
./bin/marvindocs serve --http 0.0.0.0:3333 --auth-token "$(openssl rand -hex 32)"
```

Clients then send `Authorization: Bearer <token>`; see
[Configuration](configuration.md#auth-is-required-off-loopback).

You can also run both transports at once:

```bash
./bin/marvindocs serve --stdio --http :3333
```

## 6. Verify

With `curl`:

```bash
curl -s http://localhost:3333/healthz
# ok
```

For a proper MCP round-trip, use any MCP-aware client and point it at
`http://localhost:3333/mcp` (HTTP) or the `marvindocs serve --stdio`
command (stdio). See [MCP server](mcp.md) for examples.

## 7. Run with Docker Compose

The compose file builds the image, mounts `./data/public/docs` and
`./data/index`, and publishes port 3333 on every host interface. Build
the index into `data/index/` first, then start it with a token:

```bash
./bin/marvindocs index build --index-path data/index/marvindocs.db
MARVINDOCS_AUTH_TOKEN=$(openssl rand -hex 32) docker compose up -d
```

Or keep the token in a `.env` file next to `docker-compose.yml`:

```
MARVINDOCS_AUTH_TOKEN=change-me
```

Without a token the container exits with
`error: --http 0.0.0.0:3333 is reachable from other hosts but no auth token is set`.
On a machine nobody else can reach, `MARVINDOCS_INSECURE=1 docker compose up -d`
runs it unauthenticated.
