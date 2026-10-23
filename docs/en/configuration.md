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
| `--embedder NAME` | `none` | `none`, `ollama`, `openai` or `onnx`. Enables hybrid vector + BM25 search in `get_docs`; see [Vector search](vectors.md). |
| `--ollama-url URL` | `http://localhost:11434` | Ollama base URL (`--embedder ollama`). |
| `--ollama-model NAME` | `bge-m3` | Ollama model name (`--embedder ollama`). |
| `--openai-url URL` | `http://localhost:8000/v1` | Base URL of an OpenAI-compatible server such as vLLM, including `/v1` (`--embedder openai`). |
| `--openai-model NAME` | `bge-m3` | Embedding model name on that server (`--embedder openai`). |
| `--model-dir DIR` | empty | `--embedder onnx`: directory with `model_int8.onnx` and `tokenizer.json`. |
| `--onnxruntime PATH` | empty | `--embedder onnx`: path to `libonnxruntime.so`. |
| `--synonyms DIR` | empty | Directory with extra synonym `*.md` files, added to the builtin dictionary. `off` disables query expansion. Falls back to `MARVINDOCS_SYNONYMS`; see [Synonyms](synonyms.md). |

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
| `MARVINDOCS_EMBEDDER` | `none`, `ollama`, `openai`, `onnx` | `onnx` for `index embed`, `none` for `serve` | Default of `--embedder`. |
| `MARVINDOCS_OLLAMA_URL` | URL | `http://localhost:11434` | Default of `--ollama-url`. |
| `MARVINDOCS_OLLAMA_MODEL` | model name | `bge-m3` | Default of `--ollama-model`. |
| `MARVINDOCS_OPENAI_URL` | URL | `http://localhost:8000/v1` | Default of `--openai-url`. |
| `MARVINDOCS_OPENAI_MODEL` | model name | `bge-m3` | Default of `--openai-model`. |
| `MARVINDOCS_MODEL_DIR` | path | `data/onnx/model` | Default of `--model-dir`. |
| `MARVINDOCS_ONNXRUNTIME` | path | `data/onnx/lib/libonnxruntime.so` | Default of `--onnxruntime`. |
| `MARVINDOCS_SYNONYMS` | directory, `off` | empty | Default of `--synonyms`. |

The embedder variables are read by the binary as flag defaults; an
explicit flag always wins.

### `.env` file

The binary does not read `.env` itself. `scripts/update.sh` (and so
`make update` / `make embed`) sources `.env` from the repository root and
exports it, so the `MARVINDOCS_*` values reach the binary. `.env` is
git-ignored; copy the template `.env.example` and uncomment what you need:

```bash
cp .env.example .env
# MARVINDOCS_EMBEDDER=ollama
# MARVINDOCS_OLLAMA_URL=http://gpu-host:11434
```

`docker compose` reads `.env` as well (see [Docker](#docker)).

## `index build`-only flags

| Flag | Default | Description |
|---|---|---|
| `--full` | `false` | Force a full rebuild instead of the default incremental sync (only changed docs are re-indexed). Refuses to run when the index has chunk embeddings unless `--drop-vectors` is given. |
| `--drop-vectors` | `false` | With `--full`: confirm discarding all chunk embeddings. |

## `index embed`-only flags

`index embed` takes `--doc`, `--embedder` (default `onnx`, or
`MARVINDOCS_EMBEDDER`), `--batch` and the same `--ollama-*`, `--openai-*`, `--model-dir` and `--onnxruntime` flags as
`serve`. See [Vector search](vectors.md).

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
| `onnx-fetch` | Run `scripts/fetch-onnx.sh`: download the ONNX libs and the bge-m3 model into `data/onnx/`. |
| `build-onnx` | `onnx-fetch`, then build `./bin/marvindocs` with `-tags "sqlite_fts5 onnx"` and `CGO_LDFLAGS=-L/src/data/onnx/lib`. |
| `run-onnx` | `build-onnx`, then `serve --http :3333 --embedder onnx --model-dir data/onnx/model --onnxruntime data/onnx/lib/libonnxruntime.so`. |
| `embed` | `scripts/update.sh --embed-only`: run `index embed` with the embedder from `.env` (`MARVINDOCS_EMBEDDER`, default `onnx`). |
| `update` | `scripts/update.sh`: devdocs sync, parsers, incremental `index build`, `index embed`. See [Syncing the documentation data](sync-docs.md#one-command-update). |
| `clean` | Remove `bin`, `.gocache`, `.gomodcache`. The index `marvindocs.db*` is kept. |
| `clean-index` | Remove `marvindocs.db*`, including all chunk embeddings. |

Variable used by the ONNX targets: `ONNX_DIR = data/onnx`. The embedder
for `make embed` / `make update` is configured in `.env`. See
[Vector search](vectors.md).

## Docker

The `Dockerfile` builds the binary in `golang:1.25-alpine` (CGO,
`sqlite_fts5`) and ships it in a small `alpine` image. `docker compose up -d`
builds the image and serves HTTP on port 3333, with bind mounts
`./data/public/docs` to `/data` (read-only) and `./data/index` to `/index`.
Build the index beforehand so that `./data/index/marvindocs.db` exists, e.g.
`./bin/marvindocs index build --index-path data/index/marvindocs.db`.
The compose file sets `MARVINDOCS_LOG_FORMAT=json` and
`MARVINDOCS_LOG_LEVEL=info`, and passes `MARVINDOCS_AUTH_TOKEN`,
`MARVINDOCS_INSECURE`, `MARVINDOCS_EMBEDDER` (default `none`),
`MARVINDOCS_OLLAMA_URL` (default `http://host.docker.internal:11434`, the
docker host via `extra_hosts`), `MARVINDOCS_OLLAMA_MODEL`,
`MARVINDOCS_OPENAI_URL` (default `http://host.docker.internal:8000/v1`) and
`MARVINDOCS_OPENAI_MODEL` through from
the host environment (or a `.env` file next to `docker-compose.yml`).

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
