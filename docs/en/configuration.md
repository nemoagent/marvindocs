# Configuration

marvindocs is configured through command-line flags and a few
environment variables (see [Environment variables](#environment-variables)).
There is no configuration file — on purpose, to keep the tool trivial to
deploy in containers and to reason about in scripts.

## Directory layout

```
data/public/docs/     source data (devdocs dump, gitignored)
marvindocs.db         SQLite FTS5 index (gitignored)
bin/marvindocs        compiled binary (gitignored)
```

Both `data/public/` and `marvindocs.db*` are in `.gitignore`.

## Flags shared by serve and index build

| Flag | Default | Description |
|---|---|---|
| `--docs-path DIR` | `data/public/docs` | Root of the devdocs dump. Must contain `docs.json`. |
| `--index-path FILE` | `marvindocs.db` | SQLite index file. Created if missing by `index build`, required by `serve`. |

## `serve`-only flags

| Flag | Default | Description |
|---|---|---|
| `--stdio` | `false` | Run MCP server over stdin/stdout (newline-delimited JSON-RPC). |
| `--http ADDR` | empty | Listen for HTTP clients on `ADDR`. MCP Streamable transport is at `/mcp`. A bare `:3333` binds `127.0.0.1` only; use `0.0.0.0:3333` for public access. |
| `--auth-token TOKEN` | empty | Require `Authorization: Bearer TOKEN` on every HTTP request (web UI and `/mcp` included) except `/healthz`. Falls back to `MARVINDOCS_AUTH_TOKEN`. |
| `--insecure` | `false` | Allow `--http` on a non-loopback address without a token. Also enabled by `MARVINDOCS_INSECURE=1`. |

You must specify at least one of `--stdio` or `--http`.

### Auth is required off loopback

`serve` refuses to start when `--http` binds an address other than
`127.0.0.1`/`localhost`/`::1` (for example `0.0.0.0:3333` or a LAN IP)
and no token is set:

```
error: --http 0.0.0.0:3333 is reachable from other hosts but no auth token is set; pass --auth-token (or MARVINDOCS_AUTH_TOKEN), or --insecure to serve without auth
```

Set `--auth-token` / `MARVINDOCS_AUTH_TOKEN`, or pass `--insecure`
(`MARVINDOCS_INSECURE=1`) if the host is genuinely trusted. Loopback binds
never require a token.

## Environment variables

| Variable | Values | Default | Description |
|---|---|---|---|
| `MARVINDOCS_AUTH_TOKEN` | any string | empty | Bearer token for HTTP; used when `--auth-token` is not given. |
| `MARVINDOCS_INSECURE` | `1` | unset | Same as `--insecure`: allow a non-loopback `--http` bind without a token. |
| `MARVINDOCS_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` | Log level. |
| `MARVINDOCS_LOG_FORMAT` | `text`, `json` | `text` | Log format. Logs go to stderr. |

## `index build`-only flags

| Flag | Default | Description |
|---|---|---|
| `--full` | `false` | Force a full rebuild instead of the default incremental sync (only changed docs are re-indexed). |

## Build-time configuration

The Docker image and Go flags live in the `Makefile`:

```make
GO_IMAGE = golang:1.25
# docker run ... -e CGO_ENABLED=1 $(GO_IMAGE)
#   go build -tags sqlite_fts5 -buildvcs=false -o bin/marvindocs ./cmd/marvindocs
```

SQLite is provided by the CGo driver `github.com/mattn/go-sqlite3`, so
the build needs `CGO_ENABLED=1` and the `sqlite_fts5` build tag
(without it FTS5 is missing and the index will not open). The resulting
binary is dynamically linked against the system libc.

## Makefile targets

All Go targets run inside the `golang:1.25` container.

| Target | What it does |
|---|---|
| `build` | Build `./bin/marvindocs` (CGO, `-tags sqlite_fts5`). |
| `tidy` | `go mod tidy`. |
| `test` | `go test -short -tags sqlite_fts5 ./...`: fixture-based tests only, finishes in seconds. |
| `test-full` | `go test -timeout 30m -tags sqlite_fts5 ./...`: also builds a full index from `data/public/docs` (10-20 minutes; skipped when the data is absent). |
| `vet` | `go vet -tags sqlite_fts5 ./...`. |
| `lint` | `golangci-lint run ./...` (v2.13.2, config in `.golangci.yml`). |
| `vuln` | `govulncheck ./...` (v1.1.4) against the project and the toolchain in the image. |
| `check` | `vet`, `lint`, `vuln`, `test` in sequence; the gate before a commit or PR. |
| `fmt` | `gofmt -w cmd internal parsers`. |
| `index` | `build`, then `./bin/marvindocs index build`. |
| `run-http` | `build`, then `serve --http :3333`. |
| `run-stdio` | `build`, then `serve --stdio`. |
| `parsers` | Build each `parsers/*` module (except `devdocsfmt`) into `bin/<name>-parser` without CGO. |
| `parsers-test` | Run `go test ./...` in each parser module. |
| `clean` | Remove `bin`, `.gocache`, `.gomodcache` and `marvindocs.db*`. |

## Docker

The `Dockerfile` builds the binary in `golang:1.25-alpine` (CGO,
`sqlite_fts5`) and ships it in a small `alpine` image. `docker compose up -d`
builds the image and serves HTTP on port 3333, with bind mounts
`./data/public/docs` to `/data` (read-only) and `./data/index` to `/index`.
Build the index beforehand so that `./data/index/marvindocs.db` exists, e.g.
`./bin/marvindocs index build --index-path data/index/marvindocs.db`.
The compose file sets `MARVINDOCS_LOG_FORMAT=json` and
`MARVINDOCS_LOG_LEVEL=info`, and passes `MARVINDOCS_AUTH_TOKEN` and
`MARVINDOCS_INSECURE` through from the host environment (or a `.env`
file next to `docker-compose.yml`).

The image `CMD` is `serve --http 0.0.0.0:3333 --docs-path /data
--index-path /index/marvindocs.db`. Because this is a non-loopback bind
and compose publishes port 3333 on all host interfaces, the container
exits immediately unless `MARVINDOCS_AUTH_TOKEN` is set (or
`MARVINDOCS_INSECURE=1` for a trusted host). See
[Getting started](getting-started.md#7-run-with-docker-compose).

## Where things are written

| Path | Writer | Purpose |
|---|---|---|
| `./marvindocs.db` | `index build` | SQLite main file |
| `./marvindocs.db-wal`, `-shm` | SQLite runtime | WAL journal, shared memory |
| `./bin/marvindocs` | `make build` | Compiled CLI |
| `./.gocache/`, `./.gomodcache/` | Docker build | Go build + module caches |

None of these directories need to exist ahead of time; `make` creates
them.
