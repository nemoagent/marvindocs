# go-dev parser

Collects Go documentation from [go.dev](https://go.dev/) and emits a
DevDocs-shaped tree compatible with marvindocs.

go.dev publishes the **conceptual** Go documentation: install guide,
Effective Go, FAQ, Go modules reference, database tutorials, the
language specification, and per-version release notes. Core stdlib API
docs live elsewhere (`pkg.go.dev`); this parser does **not** cover
those — the upstream DevDocs `go` slug already does that well.

## Build

```bash
cd parsers/go-dev
go build -o ../../bin/go-dev-parser .
```

## Run

Two modes:

### `--mode overview` (default)

Pulls the version-agnostic portion of go.dev/doc/ — install, effective
Go, FAQ, modules, database, tutorials, plus `/ref/spec`. Single output
slug `go-dev`, no version suffix.

```bash
./bin/go-dev-parser --out data/public/docs/go-dev --force
```

Produces ~34 pages and ~550 entries grouped under five types
(`Overview`, `Reference`, `Modules`, `Database`, `Tutorial`).

### `--mode release --version 1.X`

Pulls the release notes for one specific Go release at
`/doc/go1.X`. Output slug `go-dev~1.X`.

```bash
./bin/go-dev-parser --mode release --version 1.22 \
                    --out data/public/docs/go-dev~1.22 --force
```

Verified versions: `1.1` through `1.26` (every Go release notes page
that go.dev currently serves). `1.0` returns 404.

## Flags

| Flag | Default | Notes |
|---|---|---|
| `--mode` | `overview` | `overview` or `release`. |
| `--source` | `https://go.dev` | Override only if mirroring go.dev. |
| `--version` | empty | Required in release mode (e.g. `1.22`). Ignored in overview. |
| `--release` | (empty -> version) | Free-form label written to `meta.json`. Defaults to `--version` in release mode only; overview mode always writes an empty release. |
| `--out` | (required) | Output directory. |
| `--force` | `false` | Overwrite existing files. |

## Slug naming

| Mode | Slug |
|---|---|
| `overview` | `go-dev` |
| `release` `--version 1.22` | `go-dev~1.22` |

The `go-dev` prefix avoids colliding with DevDocs upstream's `go` slug
(which indexes `pkg.go.dev` stdlib).

## Limitations

- The overview page list is a hand-curated whitelist in `overview.go`.
  If go.dev adds a new top-level page, append it there.
- Release-mode output is a single page (`release`) with all H2/H3
  ids exposed as anchors. We don't split release notes into per-section
  pages — the release-notes article is short enough.
- Internal links inside fetched HTML still point at absolute go.dev
  URLs. Marvindocs renders them as-is, which is correct for external
  references but not for cross-doc navigation.
