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
build. Chunk embeddings of unchanged docs are kept (see
[Vector search](vectors.md)).

```bash
marvindocs index build [--docs-path DIR] [--index-path FILE] [--full [--drop-vectors]] [--rehash]
```

| Flag | Default | Notes |
|---|---|---|
| `--docs-path` | `data/public/docs` | Must contain `docs.json` and `<slug>/{index.json,db.zip}`. |
| `--index-path` | `marvindocs.db` | Output SQLite file. Updated in place. |
| `--full` | `false` | Force a full rebuild (drop everything and reinsert). Destructive: refuses to run when the index contains chunk embeddings unless `--drop-vectors` is also given. |
| `--drop-vectors` | `false` | With `--full`: confirm discarding all chunk embeddings (`chunk_vecs`). Re-embedding the whole dump takes hours. |
| `--rehash` | `false` | Recompute `docs.content_hash` for docs whose `mtime` is unchanged, without re-indexing or re-embedding them. |

`--rehash` is needed once after [`marvindocs pack --all`](#marvindocs-pack)
on an existing index, because the content hash definition changed: it is
now a SHA-256 over the `db.zip` central directory (entry name, CRC-32 and
uncompressed size per entry) followed by the bytes of `index.json`; pages
are never inflated. Without `--rehash` nothing breaks (an unchanged
`mtime` means the doc is skipped), but the next `mtime` bump with
identical content would re-index the doc instead of just touching it.

After a successful build the index is compacted: FTS5 segments are
merged and, if more than 10% of the file is free pages, `VACUUM` is run
and the WAL is checkpointed. Deleted docs and `--full` rebuilds otherwise
leave the file at its peak size. A
compaction is logged as `index compacted reclaimed_mb=N`; when there is
nothing to reclaim it is skipped. Expect roughly 30 s per 4 GB.

Plain `index build` is the recommended path: it is incremental and keeps
vectors. `--full` on an index that has vectors fails with an error like
`--full would discard N chunk embeddings ...`; add `--drop-vectors` only
when you really want to throw them away.

Exit codes: `0` on success, `1` on any error (data not found, SQLite
write failure, etc.), `2` on usage error.

> **Build requirement.** The SQLite driver is `github.com/mattn/go-sqlite3`,
> which is CGo-based. The provided `Makefile` builds inside the
> `golang:1.25` container with `CGO_ENABLED=1` and `-tags sqlite_fts5`.
> If you build by hand, both are required — without `sqlite_fts5` the
> FTS5 extension is not compiled in and the index will fail to open.

## `marvindocs index embed`

Computes chunk embeddings into the index (local ONNX by default, or
Ollama). Only
chunks without a vector are embedded, so it is resumable. The vectors are
used by the hybrid search in `get_docs` when `serve` runs with
`--embedder`. After `index build` only the changed docs need re-embedding;
`index build --full --drop-vectors` drops all vectors. See [Vector search](vectors.md) for
all flags.

```bash
marvindocs index embed [--doc SLUG]... [--embedder ollama|openai|onnx] [--batch N]
```

`--embedder` defaults to `onnx` (local CPU, slow: hours for the whole
dump); the embedder flags default to the `MARVINDOCS_*` environment
variables, see [Configuration](configuration.md#environment-variables).

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
marvindocs search QUERY [--doc SLUG] [--limit N] [--json] [--body] [--index-path FILE] [--docs-path DIR] [--synonyms DIR]
```

| Flag | Default | Notes |
|---|---|---|
| `--doc` | empty | Restrict results to a single doc slug (e.g. `react`, `python~3.13`). If omitted, a query token matching a known slug or its prefix (up to `~`/`-`) is removed and applied as an automatic filter; if that finds nothing, the full query is searched globally. Synonyms of a doc name work too (`pg` -> `postgresql`, `golang` -> `go`), see [Synonyms](synonyms.md). |
| `--limit` | `20` | Maximum number of results (1–100). |
| `--json` | `false` | Emit the raw hit list as JSON on stdout. |
| `--body` | `false` | Search page bodies (Markdown chunks, a page is ranked by its best chunk) instead of entry names. |
| `--index-path` | `marvindocs.db` | SQLite index file (must exist). |
| `--docs-path` | `data/public/docs` | Used with `--body` to build snippets from `db.zip`; without it hits are listed without excerpts. |
| `--synonyms` | empty | Directory with extra synonym `*.md` files, added to the builtin dictionary; `off` disables query expansion. Falls back to `MARVINDOCS_SYNONYMS`; see [Synonyms](synonyms.md). |

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
`<slug>/db.zip` directly.

```bash
marvindocs get SLUG PATH [--format markdown|html] [--docs-path DIR]
```

| Flag | Default | Notes |
|---|---|---|
| `--format` | `markdown` | Either `markdown` (alias `md`) or `html`. |
| `--docs-path` | `data/public/docs` | Directory containing `<slug>/db.zip`. |

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

## `marvindocs pack`

Converts docs from `<slug>/db.json` (a JSON map path -> html, the format
of devdocs.io archives) to `<slug>/db.zip` (one deflate-compressed entry
per page, see [Data format](data-format.md#5-slugdbzip--page-bodies)) and
removes `db.json`. Does not need the SQLite index.

```bash
marvindocs pack (--all | SLUG...) [--docs-path DIR]
```

| Flag | Default | Notes |
|---|---|---|
| `--all` | `false` | Convert every doc listed in `docs.json`. Mutually exclusive with `SLUG` arguments. |
| `--docs-path` | `data/public/docs` | Data directory containing `<slug>/db.json`. |

Docs without `db.json` are skipped, so the command is safe to re-run.
`scripts/sync-docs.sh` calls `marvindocs pack <slug>` right after
extracting each downloaded doc. `--all` migrates an existing data
directory in place (about 12 minutes for 830 docs); afterwards run
[`marvindocs index build --rehash`](#marvindocs-index-build) once. The
result is ~4x smaller on disk (full devdocs set: ~8 GB -> ~2.4 GB).

## `marvindocs synonyms suggest`

Mines abbreviation candidates (`Server-Side Rendering (SSR)`) from the
docs and prints them as a TSV table. Reads `index.json` and `db.zip`, does
not need the SQLite index. Details, filters and the review workflow are in
[Synonyms](synonyms.md#building-a-dictionary-from-the-docs).

```bash
marvindocs synonyms suggest [--docs-path DIR] [--doc SLUG]... [--min-docs N] [--min-count N] [--names-only] [--synonyms DIR] [--out FILE]
```

| Flag | Default | Notes |
|---|---|---|
| `--docs-path` | `data/public/docs` | Directory with `docs.json` and `<slug>/{index.json,db.zip}`. |
| `--doc` | all docs | Scan only this slug (exact, including the version); repeatable. |
| `--min-docs` | `2` | Keep pairs found in at least N docs. |
| `--min-count` | `3` | Keep pairs with at least N occurrences. |
| `--names-only` | `false` | Scan entry names only: seconds instead of minutes. |
| `--synonyms` | empty | Skip terms that already have synonyms in the builtin dictionary or in DIR. Falls back to `MARVINDOCS_SYNONYMS`. |
| `--out` | stdout | Write the table to a file. |

The full-text scan of the whole archive takes about 10 minutes.

## `marvindocs synonyms import`

Turns a reviewed table from `synonyms suggest` into dictionary files, one
`*.md` per concept. `FILE` may be `-` (stdin) and may come before or after
the flags. See [Synonyms](synonyms.md#writing-files-synonyms-import).

```bash
marvindocs synonyms import FILE --out DIR [--force]
```

| Flag | Default | Notes |
|---|---|---|
| `--out` | required | Dictionary directory; created if missing. |
| `--force` | `false` | Overwrite existing files. Without it existing files are skipped, so hand edits survive. |

## `marvindocs serve`

Runs the MCP server.

```bash
marvindocs serve [--stdio] [--http ADDR] [--auth-token TOKEN] [--insecure] \
                 [--docs-path DIR] [--index-path FILE] \
                 [--embedder none|ollama|openai|onnx] [--ollama-url URL] \
                 [--ollama-model NAME] [--openai-url URL] [--openai-model NAME] \
                 [--model-dir DIR] [--onnxruntime PATH] \
                 [--synonyms DIR]
```

| Flag | Default | Notes |
|---|---|---|
| `--stdio` | `false` | Serve MCP over stdin/stdout. |
| `--http` | empty | Serve the web UI at `<addr>/` and MCP Streamable HTTP at `<addr>/mcp`. Bare `:3333` binds `127.0.0.1` only; use `0.0.0.0:3333` for public exposure. |
| `--auth-token` | empty | When set, every HTTP request (including `/mcp` and the web UI) must carry `Authorization: Bearer TOKEN`; only `/healthz` is exempt. Falls back to the `MARVINDOCS_AUTH_TOKEN` env var. |
| `--insecure` | `false` | Allow a non-loopback `--http` bind without an auth token. Without it (or `MARVINDOCS_INSECURE=1`) `serve` refuses to start on a public address when no token is set. |
| `--docs-path` | `data/public/docs` | Needed by `get_page` to read `db.zip` on demand. |
| `--index-path` | `marvindocs.db` | Must already exist (run `index build` first). |
| `--embedder` | `none` | `none`, `ollama`, `openai` or `onnx`. With an embedder, `get_docs` uses hybrid vector + BM25 search for docs that have vectors. |
| `--ollama-url`, `--ollama-model` | `http://localhost:11434`, `bge-m3` | Ollama settings. |
| `--openai-url`, `--openai-model` | `http://localhost:8000/v1`, `bge-m3` | OpenAI-compatible server settings (vLLM). |
| `--model-dir`, `--onnxruntime` | | onnx: model directory and path to `libonnxruntime.so`. |
| `--synonyms` | empty | Directory with extra synonym `*.md` files, added to the builtin dictionary; `off` disables query expansion. Falls back to `MARVINDOCS_SYNONYMS`. A non-existent directory is a startup error. See [Synonyms](synonyms.md). |

The embedder flags are the same as in `index embed`; see
[Vector search](vectors.md).

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
