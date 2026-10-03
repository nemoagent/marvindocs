# CLI reference

```
marvindocs <command> [flags]
```

Run `marvindocs help` (also `--help` / `-h`) to see the built-in summary.

## `marvindocs version`

Prints the build version (defaults to `dev`; overridable with
`-ldflags "-X main.version=..."`). Also accepted as `--version` / `-v`.

```bash
marvindocs version
# marvindocs dev
```

## `marvindocs index build`

Updates the SQLite FTS5 index from the devdocs dump. By default the
build is **incremental**: docs whose `mtime` (from `meta.json`) hasn't
changed are skipped, and docs that disappeared from the dump are
removed. The first run on an empty index file is equivalent to a full
build.

```bash
marvindocs index build [--docs-path DIR] [--index-path FILE] [--fulltext] [--full]
```

| Flag | Default | Notes |
|---|---|---|
| `--docs-path` | `data/public/docs` | Must contain `docs.json` and `<slug>/{index,db}.json`. |
| `--index-path` | `marvindocs.db` | Output SQLite file. Updated in place. |
| `--fulltext` | `false` | Also index page body text into `pages_fts`. Much slower; the index file grows by roughly the combined size of every `db.json`. |
| `--full` | `false` | Force a full rebuild (drop everything and reinsert). Useful after schema changes. |

After a successful build the index is compacted: FTS5 segments are
merged and, if more than 10% of the file is free pages, `VACUUM` is run
and the WAL is checkpointed. Deleted docs, `--full` rebuilds and dropped
`--fulltext` bodies otherwise leave the file at its peak size. A
compaction is logged as `index compacted reclaimed_mb=N`; when there is
nothing to reclaim it is skipped. Expect roughly 30 s per 4 GB.

Exit codes: `0` on success, `1` on any error (data not found, SQLite
write failure, etc.), `2` on usage error.

> **Build requirement.** The SQLite driver is `github.com/mattn/go-sqlite3`,
> which is CGo-based. The provided `Makefile` builds inside the
> `golang:1.25` container with `CGO_ENABLED=1` and `-tags sqlite_fts5`.
> If you build by hand, both are required — without `sqlite_fts5` the
> FTS5 extension is not compiled in and the index will fail to open.

## `marvindocs index stats`

Prints counts from an existing index. Pass `--by-doc` for a per-doc
breakdown sorted by entry count (descending), useful for spotting
outliers.

```bash
marvindocs index stats [--index-path FILE] [--by-doc] [--top N]
```

| Flag | Default | Notes |
|---|---|---|
| `--index-path` | `marvindocs.db` | SQLite index file. |
| `--by-doc` | off | Print per-doc table after the summary. |
| `--top` | 0 (all) | When combined with `--by-doc`, limit to top N rows. |

Example output (sample; the numbers depend on your dump):

```
index:   marvindocs.db
size:    476532736 bytes
docs:    791
entries: 2677981
pages:   0

SLUG                               ENTRIES       PAGES
openjdk~25                           49730           0
scala~3.1                            48073           0
qt~6.9                               45235           0
```

## `marvindocs search`

Runs a full-text search against the index and prints hits. Useful for
shell pipelines and quick lookups without starting a server.

```bash
marvindocs search QUERY [--doc SLUG] [--limit N] [--json] [--body] [--index-path FILE]
```

| Flag | Default | Notes |
|---|---|---|
| `--doc` | empty | Restrict results to a single doc slug (e.g. `react`, `python~3.13`). If omitted, a query token matching a known slug or its prefix (up to `~`/`-`) is removed and applied as an automatic filter; if that finds nothing, the full query is searched globally. Aliases: `js`, `py`, `ts`, `pg`, `mongo`. |
| `--limit` | `20` | Maximum number of results (1–100). |
| `--json` | `false` | Emit the raw hit list as JSON on stdout. |
| `--body` | `false` | Search page bodies instead of entry names. Requires an index built with `index build --fulltext`; otherwise there are no results. |
| `--index-path` | `marvindocs.db` | SQLite index file (must exist). |

Positional arguments after the flags are joined with spaces, so
`marvindocs search http server` is equivalent to `marvindocs search "http server"`.

Plain text output columns: `DOC_SLUG  NAME  PATH` (with `--body`:
`DOC_SLUG  PATH  SNIPPET`). The trailing
`N result(s)` summary is written to stderr so it is easy to pipe the
data portion into other tools.

Exit code `0` even when there are no hits (a message is printed to
stderr); `1` on index errors; `2` on usage errors.

## `marvindocs get`

Fetches a single page from the devdocs dump and prints it as Markdown
or raw HTML. Does **not** require the SQLite index — it reads
`<slug>/db.json` directly.

```bash
marvindocs get SLUG PATH [--format markdown|html] [--docs-path DIR]
```

| Flag | Default | Notes |
|---|---|---|
| `--format` | `markdown` | Either `markdown` (alias `md`) or `html`. |
| `--docs-path` | `data/public/docs` | Directory containing `<slug>/db.json`. |

`PATH` is the page path as stored in devdocs (for example
`hooks/use-state` for the React doc). Use `marvindocs search` to find
valid paths.

## `marvindocs list`

Prints the docs currently present in the index, one per line:

```bash
marvindocs list [--json] [--index-path FILE]
```

| Flag | Default | Notes |
|---|---|---|
| `--json` | `false` | Emit the doc list as JSON. |
| `--index-path` | `marvindocs.db` | SQLite index file (must exist). |

Plain text columns: `SLUG  NAME  VERSION` (falls back to `release`
when `version` is empty). The summary line goes to stderr.

## `marvindocs serve`

Runs the MCP server.

```bash
marvindocs serve [--stdio] [--http ADDR] [--auth-token TOKEN] \
                 [--docs-path DIR] [--index-path FILE]
```

| Flag | Default | Notes |
|---|---|---|
| `--stdio` | `false` | Serve MCP over stdin/stdout. |
| `--http` | empty | Serve the web UI at `<addr>/` and MCP Streamable HTTP at `<addr>/mcp`. Bare `:3333` binds `127.0.0.1` only; use `0.0.0.0:3333` for public exposure. |
| `--auth-token` | empty | When set, every HTTP request (including `/mcp` and the web UI) must carry `Authorization: Bearer TOKEN`; only `/healthz` is exempt. Falls back to the `MARVINDOCS_AUTH_TOKEN` env var. |
| `--docs-path` | `data/public/docs` | Needed by `get_page` to read `db.json` on demand. |
| `--index-path` | `marvindocs.db` | Must already exist (run `index build` first). |

You must pass at least one of `--stdio` / `--http`. Both can be set at
once — the same server instance handles both transports.

The process listens for `SIGINT` and `SIGTERM` and performs a clean
shutdown on either, with up to 10s grace for in-flight HTTP requests.

The HTTP listener closes connections that take longer than 10s to send
request headers and idle keep-alive connections after 120s. There is
no write timeout: `/mcp` keeps long-lived SSE streams open.

Opening an index built by an older version adds any missing schema
objects. The first start after upgrading to the `idx_entries_name`
index builds it in place; on a full devdocs index (2.6M entries) that
takes about 30-40s once.

### Logging

All commands honor two environment variables:

| Variable | Values | Default |
|---|---|---|
| `MARVINDOCS_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `MARVINDOCS_LOG_FORMAT` | `text`, `json` | `text` |

Logs are written to stderr. The HTTP server emits one line per request
(method, path, status, dur_ms, remote).

## Exit codes (all commands)

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Runtime error (I/O, SQLite, etc.) |
| `2` | Usage error (unknown flag, missing argument) |
