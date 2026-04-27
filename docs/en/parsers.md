# Parsers

Marvindocs ships with optional parser binaries that produce the
DevDocs on-disk format from arbitrary sources. Use them when you want
to index documentation that DevDocs upstream doesn't cover (e.g.
Laravel, internal handbooks, OpenAPI specs).

Parsers live in `parsers/` and have **no dependency on the marvindocs
main module**. They write three files in the same shape DevDocs uses,
so the rest of marvindocs picks them up unchanged.

## On-disk contract

For a slug `<slug>`, a parser produces in `data/public/docs/<slug>/`:

| File | Purpose |
|---|---|
| `meta.json` | Flat metadata: name, slug, type, version, release, mtime, db_size, attribution. |
| `index.json` | `{entries: [{name, path, type}], types: [{name, count, slug}]}`. |
| `db.json` | `{<path>: "<html fragment>", ...}` — body fragments only, no `<html>`/`<head>`. |
| `.last-modified` | Empty file whose mtime equals `meta.json.mtime`; used by `sync-docs.sh`. |

Marvindocs reads these files via `internal/docstore`, the same code
path that handles upstream DevDocs dumps.

## Available parsers

| Parser | Source | Output slug | README |
|---|---|---|---|
| `laravel` | github.com/laravel/docs | `laravel~<major>` | `parsers/laravel/README.md` |

## Workflow

```bash
# 1. Build the parser binary.
make parsers                                          # all parsers
# or:  cd parsers/laravel && go build -o ../../bin/laravel-parser .

# 2. Run it. Output goes wherever marvindocs reads from.
./bin/laravel-parser --version 12.x \
    --out data/public/docs/laravel~12 --force

# 3. Re-index. The new doc joins the rest.
./bin/marvindocs index build --full
```

After step 3 the doc is searchable through the web UI and MCP tools
(`list_docs`, `search`, `get_page`).

## Writing your own parser

1. `parsers/<name>/` with its own `go.mod`. Keep it isolated.
2. Add `replace github.com/piligrim/marvindocs/parsers/devdocsfmt => ../devdocsfmt`.
3. Define a `*devdocsfmt.Writer`, call `AddPage(path, html)` and
   `AddEntry(name, path, type)` for every page and symbol, then
   `Flush(out, force)`.
4. Use the standard `devdocsfmt.Flags` so users see the same
   `--source`, `--out`, `--version`, `--release`, `--force` options.

The Laravel parser (`parsers/laravel/`) is the simplest end-to-end
example.

## Why parsers are isolated

- Server binary stays small (no markdown/HTML scrubbers shipped in it).
- Parser dependencies (markdown engines, GitHub clients) don't leak
  into `internal/`.
- Anyone can write a parser in any language. Python, Rust, even shell
  — as long as it produces the four files above, marvindocs doesn't
  care.
