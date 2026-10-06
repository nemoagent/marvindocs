# go-std parser

Builds Go standard library documentation from `go doc` executed inside a
Docker image and emits a DevDocs-shaped tree compatible with marvindocs.

Unlike web scrapers, this parser does not parse external websites. The
source of truth is the Go toolchain inside the selected Docker image.

## Build

```bash
cd parsers/go-std
go build -o ../../bin/go-std-parser .
```

## Run

```bash
# default version (1.25), default output data/public/docs/go-std~1.25
./bin/go-std-parser --force

# specific Go version
./bin/go-std-parser --version 1.24 --out data/public/docs/go-std~1.24 --force

# custom image tag
./bin/go-std-parser --version 1.23 --image golang:1.23 --out data/public/docs/go-std~1.23 --force
```

## Flags

| Flag | Default | Notes |
|---|---|---|
| `--version` | `1.25` | Go version label for slug/meta. |
| `--image` | `golang:<version>` | Docker image used to run `go doc`. |
| `--source` | `docker://<image>` | Written to metadata; no external network parsing. |
| `--release` | (empty -> version) | Release label in `meta.json`. |
| `--out` | `data/public/docs/go-std~<version>` | Output directory. |
| `--force` | `false` | Overwrite existing files. |

## Slug naming

| Version | Output slug |
|---|---|
| `1.25` | `go-std~1.25` |
| `1.24` | `go-std~1.24` |

## What it produces

- An `index` page with a "Standard library" entry of type `Index`.
- One page per stdlib package (path `/<package>`).
- One top-level entry per package.
- Symbol entries for exported `func/type/var/const` declarations from
  package overview (`go doc <package>`).
- Page body from `go doc -all <package>` rendered as HTML `<pre><code>`.

After generation:

```bash
./bin/marvindocs index build
```

The build is incremental and keeps existing embeddings; avoid `index build --full`
(it needs `--drop-vectors` on an index with vectors and discards them all).
