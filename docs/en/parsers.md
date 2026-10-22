# Parsers

Marvindocs ships with optional parser binaries that produce the
DevDocs on-disk format from arbitrary sources. Use them when you want
to index documentation that DevDocs upstream doesn't cover (e.g.
Laravel, internal handbooks, OpenAPI specs).

Parsers live in `parsers/` and have **no dependency on the marvindocs
main module**. They write the same files DevDocs uses (`index.json`,
`db.zip`, `meta.json` and a `.last-modified` stamp) and upsert the
parent `docs.json`, so the rest of marvindocs picks them up unchanged.

## On-disk contract

For a slug `<slug>`, a parser produces in `data/public/docs/<slug>/`:

| File | Purpose |
|---|---|
| `meta.json` | Flat metadata: name, slug, type, version, release, mtime, db_size, attribution. |
| `index.json` | `{entries: [{name, path, type}], types: [{name, count, slug}]}`. |
| `db.zip` | Zip archive, one deflate-compressed entry per page: entry name = page path, entry body = HTML fragment (body fragments only, no `<html>`/`<head>`). |
| `.last-modified` | Empty file whose mtime equals `meta.json.mtime`; used by `sync-docs.sh`. |

`scripts/run-parser.sh` additionally writes a `.parser` marker (the parser
name) into the output directory after every successful run. `sync-docs.sh`
reports such directories as `LOCAL` and never overwrites them, even with
`--force`.

Marvindocs reads these files via `internal/docstore`, the same code
path that handles upstream DevDocs dumps.

## Available parsers

| Parser | Source | Output slug | README |
|---|---|---|---|
| `laravel` | github.com/laravel/docs | `laravel~<short>` (e.g. `laravel~12`, `laravel~5.8`) | `parsers/laravel/README.md` |
| `go-dev` | go.dev/doc/ | `go-dev` (overview) and `go-dev~1.X` (release notes) | `parsers/go-dev/README.md` |
| `go-std` | `go doc` inside a Go Docker image | `go-std~<version>` (e.g. `go-std~1.25`) | `parsers/go-std/README.md` |
| `mongodb` | github.com/mongodb/docs (reST Manual sources) | `mongodb~<version>` (e.g. `mongodb~8.0`) | `parsers/mongodb/README.md` |
| `metasploit` | docs.metasploit.com (HTML) | `metasploit` | [Metasploit](metasploit.md) |

## Workflow

```bash
# 1. Build the parser binary.
make parsers                                          # all parsers
# or:  cd parsers/laravel && go build -o ../../bin/laravel-parser .

# 2. Run it. Output goes wherever marvindocs reads from.
./bin/laravel-parser --version 12.x \
    --out data/public/docs/laravel~12 --force

# 3. Re-index. The new doc joins the rest.
./bin/marvindocs index build
```

`index build` is incremental and keeps existing embeddings. Do not use
`index build --full` here: it refuses to run on an index with vectors,
and `--full --drop-vectors` discards all of them (re-embedding takes
hours).

After step 3 the doc is searchable through the web UI and MCP tools
(`list_docs`, `search`, `get_page`).

`make parsers` builds every parser into `bin/<name>-parser`;
`make parsers-test` runs `go test ./...` for every `parsers/*/` module
(including `devdocsfmt`). Both targets run inside a Docker `golang`
image, so no local Go toolchain is needed.

## Running registered parsers

`parsers/registry.json` lists the docs that can be generated with a
parser. The top-level key is `parsers`; each entry has a parser name, a
slug, fixed arguments and a description.
`scripts/run-parser.sh` reads it and runs the matching
`bin/<parser>-parser` binary with `--out <docs-path>/<slug>` and
`--force`.

An entry with `"versions"` is a template: its slug contains `{v}`
(`laravel~{v}`, `go-dev~{v}`, `go-std~{v}`, `mongodb~{v}`) and `{v}` in
`args` is replaced by the version. Fixed entries such as `go-dev` have no
`versions`. Any version matching a template is accepted, even if it is not
listed yet:

```bash
./scripts/run-parser.sh go-std~1.26         # works although 1.26 is not in the registry
```

After a successful run of a version that is not listed, it is appended to
`versions` in `parsers/registry.json`.

```bash
./scripts/run-parser.sh --list              # slug | parser | description
./scripts/run-parser.sh laravel~12          # run one slug
./scripts/run-parser.sh --all               # run every slug from the registry (continues past failures)
./scripts/run-parser.sh --check laravel~12  # print the resolved command, don't run
```

| Option | Meaning |
|---|---|
| `--no-force` | Don't pass `--force` to the parser. |
| `--docs-path DIR` | Output root (default `data/public/docs`). |

`--all` does not stop at the first failure: it runs every slug, prints
`summary: N ok, M failed` and the list of failed slugs, and exits with `1`
if any failed.

Requires `jq`. Exit codes: `0` OK, `1` parser failed, `2` usage error,
unknown slug, missing `jq`/registry or missing parser binary (run
`make parsers` first).

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
