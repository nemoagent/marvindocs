# Syncing the documentation data

marvindocs does not ship the devdocs documentation itself. The helper
script `scripts/sync-docs.sh` downloads it directly from
`downloads.devdocs.io` using standard command-line tools — no Docker, no
Ruby, no devdocs checkout needed.

## Prerequisites

- `curl`, `tar`, `awk`, `xargs`, `stat`, `date`, `touch`

No Python, no Docker, no Ruby and no `jq` — these tools ship on any
Linux, macOS and WSL out of the box.

If `bin/marvindocs` is missing and the index is being rebuilt (no
`--no-index`/`--check`), the script runs `make build` first, which
compiles inside Docker (see the `Makefile`).

## Quick start

```bash
make build                    # build the marvindocs binary (once)
./scripts/sync-docs.sh        # download the 27 built-in docs, rebuild index
./bin/marvindocs serve --http :3333
```

## Usage

```
./scripts/sync-docs.sh [DOCS...] [OPTIONS]
```

| Argument / option | Meaning |
|---|---|
| *(nothing)* | Download the built-in popular set (27 docs). |
| `react`, `python~3.13`, … | Download only these specific slugs. |
| `--all` | Process every slug already listed in `data/public/docs/docs.json`. |
| `--check` | Dry-run: print up-to-date / outdated / missing status and exit without downloading. |
| `--force` | Ignore local mtime — re-download every requested slug unconditionally. |
| `--no-index` | Skip rebuilding `marvindocs.db` after download. |
| `--jobs N` | Parallel downloads (default `8`). |
| `--list-remote` | Fetch the live devdocs.io catalog and print every available slug with its name and version. No download, no index touched. |
| `--search-remote QUERY` | Same as `--list-remote` but filter rows whose `slug` or `name` contain `QUERY` (case-insensitive). |
| `--help`, `-h` | Print usage and exit. |

### Examples

```bash
# A few specific docs
./scripts/sync-docs.sh react vue~3 typescript python~3.13

# Just check what is outdated, don't download anything
./scripts/sync-docs.sh --check --all

# Update every doc already on disk (unchanged ones are skipped)
./scripts/sync-docs.sh --all

# Re-download everything from scratch even if nothing changed
./scripts/sync-docs.sh --all --force

# Download without rebuilding the index
./scripts/sync-docs.sh --all --no-index
./bin/marvindocs index build      # build index manually later

# Browse the remote catalog before downloading anything
./scripts/sync-docs.sh --list-remote | head
./scripts/sync-docs.sh --search-remote kubernetes
```

## Update detection

devdocs.io publishes one catalog, `https://devdocs.io/docs.json`, that
lists every doc together with its build `mtime` (unix timestamp). On
each run the script:

- fetches the catalog **once** and reads the `mtime` of every slug;
- compares it with the mtime of the local stamp file
  `data/public/docs/<slug>/.last-modified`;
- prints `SKIP <slug> (up-to-date)` when the local stamp is not older
  than the catalog `mtime` — no network request for that slug;
- otherwise downloads and extracts the archive, prints `UPDATED <slug>`
  and sets the stamp's mtime to the catalog value
  (`touch -d @<remote_ts>`);
- for a slug missing from the catalog it sends a single `HEAD` request
  to `downloads.devdocs.io` and uses the `Last-Modified` header; if
  that fails too but a local stamp exists, the doc was produced by a
  parser (`parsers/registry.json`): it is reported as `LOCAL ...
  (parser-built, skipped)` and left untouched; with no stamp the slug is
  reported as `FAIL ... (not found)`;
- `--force` bypasses the comparison and always re-downloads;
- `--check` only prints the status table (`up-to-date`, `OUTDATED`,
  `MISSING`, `LOCAL (parser-built)`, `ERROR (no remote)`) and exits
  without downloading.

Parsers from `parsers/` write the same `.last-modified` stamp, so
locally generated docs fit into the same layout.

Why a stamp file instead of the `mtime` field inside `meta.json`? The
stamp's mtime is set directly from the catalog value, so the comparison
with the catalog is exact and does not depend on how `meta.json` was
produced.

Re-running the script daily/weekly is cheap: one catalog request plus
a `HEAD` only for slugs absent from the catalog.

## What the script does

1. Resolves the list of slugs (built-in list, `--all` from existing
   `docs.json`, or the slugs you supplied).
2. Downloads `https://downloads.devdocs.io/<slug>.tar.gz` in parallel
   with `curl` and extracts each archive into `data/public/docs/<slug>/`.
   Failed slugs (404/403) are reported and skipped without aborting.
3. Rebuilds `data/public/docs/docs.json` from the `meta.json` file
   inside every slug directory (pure bash, no Python) — only if at
   least one slug was updated or `docs.json` is missing; otherwise the
   existing file is kept.
4. *(Unless `--no-index`)* Runs `./bin/marvindocs index build`.

## Built-in popular set

When called without arguments the script downloads:

```
angular     bash        c           cpp         css         django~5.2
docker      go          html        http        javascript  jinja~3.1
jquery      kubernetes  markdown    nginx       node        npm
postgresql~17  python~3.13  react   redis       ruby~3.4    rust
sqlite      typescript  vue~3
```

Edit `DEFAULT_SLUGS` at the top of `scripts/sync-docs.sh` to change this.

## Finding slug names

Every name on <https://devdocs.io> is a downloadable slug.
Versioned docs use a tilde: `python~3.12`, `angular~18`, `react~18`.

Browse the live catalog directly from the script — no browser needed:

```bash
# Every doc in the live catalog
./scripts/sync-docs.sh --list-remote

# Grep by slug or name (case-insensitive)
./scripts/sync-docs.sh --search-remote python
./scripts/sync-docs.sh --search-remote "vue"
./scripts/sync-docs.sh --search-remote rust

# Pipe into shell tools
./scripts/sync-docs.sh --list-remote | awk 'NR>1 {print $1}' | sort > all-slugs.txt
```

The catalog comes from `https://devdocs.io/docs.json` at request time,
so it's always current.

To see what you already have locally:

```bash
grep -o '"slug":"[^"]*"' data/public/docs/docs.json | cut -d'"' -f4
```

## Re-syncing / updating

The script is idempotent — re-running it re-downloads only slugs whose
catalog `mtime` is newer than the local stamp (or all of them with
`--force`). To update only specific docs:

```bash
./scripts/sync-docs.sh react vue~3
```

## Disk usage

| Set | Approx size on disk |
|---|---|
| Built-in (27 docs) | ~0.6 GB |
| Every slug in the catalog | ~17 GB |

Sizes were measured on a local dump and grow as devdocs publishes more
docs. Download time depends on your connection.

## Exit codes

- `0` — success, even if individual slugs report `FAIL`.
- `1` — the catalog fetch failed, or `--all` found no `docs.json`.
- `2` — an unknown option, or `--search-remote` without a query.
