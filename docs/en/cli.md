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
| `--full` | `false` | Force a full rebuild (drop everything and reinsert). Useful after schema changes or to defragment. |

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

Example output:

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
marvindocs search QUERY [--doc SLUG] [--limit N] [--json] [--index-path FILE]
```

| Flag | Default | Notes |
|---|---|---|
| `--doc` | empty | Restrict results to a single doc slug (e.g. `react`, `python~3.13`). |
| `--limit` | `20` | Maximum number of results (1–100). |
| `--json` | `false` | Emit the raw hit list as JSON on stdout. |
| `--index-path` | `marvindocs.db` | SQLite index file (must exist). |

Positional arguments after the flags are joined with spaces, so
`marvindocs search http server` is equivalent to `marvindocs search "http server"`.

Plain text output columns: `DOC_SLUG  NAME  PATH`. The trailing
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
