# Configuration

marvindocs is configured entirely through command-line flags. There are
no environment variables and no configuration file — on purpose, to keep
the tool trivial to deploy in containers and to reason about in scripts.

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
| `--http ADDR` | empty | Listen for HTTP clients on `ADDR`. MCP Streamable transport is at `/mcp`. |

You must specify at least one of `--stdio` or `--http`.

## `index build`-only flags

| Flag | Default | Description |
|---|---|---|
| `--fulltext` | `false` | *(Reserved)* also index page body text. Not implemented yet — the flag is accepted but ignored with a warning. |

## Build-time configuration

The Docker image and Go flags live in the `Makefile`:

```make
GO_IMAGE   = golang:1.25
CGO_ENABLED = 0            # pure-Go build, no glibc dependency
```

SQLite is provided by the pure-Go driver `modernc.org/sqlite`; there is
no CGO, no external SQLite library, no dynamic linking. The resulting
binary runs on any Linux box matching the target arch.

## Where things are written

| Path | Writer | Purpose |
|---|---|---|
| `./marvindocs.db` | `index build` | SQLite main file |
| `./marvindocs.db-wal`, `-shm` | SQLite runtime | WAL journal, shared memory |
| `./bin/marvindocs` | `make build` | Compiled CLI |
| `./.gocache/`, `./.gomodcache/` | Docker build | Go build + module caches |

None of these directories need to exist ahead of time; `make` creates
them.
