# marvindocs parsers

Standalone tools that produce the DevDocs on-disk format from arbitrary
sources. Each parser is its own Go module under `parsers/<name>/` and
shares the format definition with `parsers/devdocsfmt/`.

## Why a separate workspace

- The main `marvindocs` module stays focused on indexing and serving.
- Parsers can pull in heavy parsing dependencies (markdown engines, HTML
  scrubbers, GitHub clients) without bloating the server binary.
- The contract between a parser and marvindocs is the **on-disk format**,
  not a Go API. Anyone can write a parser in any language as long as they
  write the same three files; `devdocsfmt` is just a convenient Go helper.

## On-disk contract

A parser produces, in `<out>/`:

```
meta.json        flat metadata (name, slug, version, release, mtime, db_size, ...)
index.json       {entries: [{name, path, type}], types: [...]}
db.json          {<path>: "<html fragment>", ...}
.last-modified   stamp file (mtime = doc mtime) for sync-docs.sh integration
```

After a parser finishes, run `marvindocs index build` and the new doc
joins the rest. No marvindocs code change required.

## Available parsers

| Parser | Source | Output slug | README |
|---|---|---|---|
| `laravel` | github.com/laravel/docs (markdown) | `laravel~<major>` | [parsers/laravel/README.md](laravel/README.md) |

## Build

Each parser is built independently:

```bash
cd parsers/laravel && go build -o ../../bin/laravel-parser .
```

Or build all of them at once from the repo root:

```bash
make parsers
```

## Writing a new parser

1. Create `parsers/<name>/` with its own `go.mod` and `replace` directive
   pointing at `../devdocsfmt`.
2. Use `devdocsfmt.Flags` for `--source`, `--out`, `--version`, etc.
3. Build a `devdocsfmt.Writer`, call `AddPage` and `AddEntry`, then
   `Flush(out, force)`.
4. Add a row to the table above and write a per-parser README.

The Laravel parser is the simplest end-to-end example; copy and adapt.
