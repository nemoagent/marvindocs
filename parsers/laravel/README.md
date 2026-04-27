# Laravel parser

Fetches the official Laravel documentation from the
[`laravel/docs`](https://github.com/laravel/docs) repository (raw markdown
on GitHub) and emits a DevDocs-shaped tree that drops straight into
`data/public/docs/laravel~<major>/`.

DevDocs upstream does not currently ship Laravel docs, so this parser
fills that gap.

## Build

```bash
cd parsers/laravel
go build -o ../../bin/laravel-parser .
```

## Run

```bash
./bin/laravel-parser --version 13.x --out data/public/docs/laravel~13 --force
./bin/laravel-parser --version 12.x --out data/public/docs/laravel~12 --force
./bin/laravel-parser --version 5.1  --out data/public/docs/laravel~5.1 --force
```

| Flag | Default | Notes |
|---|---|---|
| `--source` | `https://raw.githubusercontent.com/laravel/docs` | Override only if mirroring the repo elsewhere. |
| `--version` | `12.x` | Branch in `laravel/docs`. Verified across all 20 release branches: `4.0`, `4.1`, `4.2`, `5.0`-`5.8`, `6.x`-`13.x`, plus `master`. |
| `--release` | (empty -> version) | Free-form label written to `meta.json`. |
| `--out` | (required) | Output directory. Convention: `data/public/docs/laravel~<short>`. |
| `--force` | `false` | Overwrite existing files. |

### Slug naming

`<short>` is the version with the trailing `.x` stripped. Dotted
versions stay intact so they don't collide:

| `--version` | output slug |
|---|---|
| `12.x` | `laravel~12` |
| `6.x` | `laravel~6` |
| `5.8` | `laravel~5.8` |
| `4.0` | `laravel~4.0` |
| `master` | `laravel~master` |

After the parser finishes, integrate with marvindocs the usual way:

```bash
./bin/marvindocs index build --full
```

The new doc shows up in `marvindocs list`, `marvindocs search`, the web
UI and MCP `list_docs`.

## What it produces

- One page per markdown file referenced from `documentation.md`.
- One top-level entry per page, plus one entry per `##`/`###` heading
  with auto-generated anchor ids (matching gomarkdown's rendered ids).
- Sections from the TOC (`## Prologue`, `## The Basics`, ...) become the
  `type` of each entry, so marvindocs groups them in search results.

## Limitations

- TOC parser uses regexes for both the modern (`- ## Section` with
  `{{version}}` placeholder, 5.x+) and legacy (`- Section` with literal
  version, 4.x) formats. If Laravel introduces a third shape, adjust
  `parseTOC` in `parse.go`.
- Internal links inside pages still point to `/docs/{{version}}/...`;
  marvindocs renders them as relative paths, so they break in the web UI
  navigation. A follow-up pass should rewrite `{{version}}` to the real
  version.
