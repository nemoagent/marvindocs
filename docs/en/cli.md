# CLI reference

```
marvindocs <command> [flags]
```

Run `marvindocs help` or `marvindocs --help` to see the built-in summary.

## `marvindocs version`

Prints the build version (defaults to `dev`; overridable with
`-ldflags "-X main.version=..."`).

```bash
marvindocs version
# marvindocs dev
```

## `marvindocs index build`

Rebuilds the SQLite FTS5 index from the devdocs dump.

```bash
marvindocs index build [--docs-path DIR] [--index-path FILE] [--fulltext]
```

| Flag | Default | Notes |
|---|---|---|
| `--docs-path` | `data/public/docs` | Must contain `docs.json` and `<slug>/{index,db}.json`. |
| `--index-path` | `marvindocs.db` | Output SQLite file. Overwritten in place. |
| `--fulltext` | `false` | Reserved. Currently accepted but ignored. |

Exit codes: `0` on success, `1` on any error (data not found, SQLite
write failure, etc.), `2` on usage error.

## `marvindocs index stats`

Prints counts from an existing index.

```bash
marvindocs index stats [--index-path FILE]
```

Example output:

```
index:   marvindocs.db
size:    476532736 bytes
docs:    791
entries: 2677981
```

## `marvindocs serve`

Runs the MCP server.

```bash
marvindocs serve [--stdio] [--http ADDR] \
                 [--docs-path DIR] [--index-path FILE]
```

| Flag | Default | Notes |
|---|---|---|
| `--stdio` | `false` | Serve MCP over stdin/stdout. |
| `--http` | empty | Serve the web UI at `<addr>/` and MCP Streamable HTTP at `<addr>/mcp`. |
| `--docs-path` | `data/public/docs` | Needed by `get_page` to read `db.json` on demand. |
| `--index-path` | `marvindocs.db` | Must already exist (run `index build` first). |

You must pass at least one of `--stdio` / `--http`. Both can be set at
once — the same server instance handles both transports.

The process listens for `SIGINT` and `SIGTERM` and performs a clean
shutdown on either.

## Exit codes (all commands)

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Runtime error (I/O, SQLite, etc.) |
| `2` | Usage error (unknown flag, missing argument) |
