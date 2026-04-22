# Syncing the documentation data

marvindocs does not ship the devdocs documentation itself. The helper
script `scripts/sync-docs.sh` downloads it directly from
`downloads.devdocs.io` using only `curl` and `tar` — no Docker, no Ruby,
no devdocs checkout needed.

## Prerequisites

- `curl`
- `tar`

That's it. No Python, no Docker, no Ruby — both ship on any Linux, macOS
and WSL out of the box.

## Quick start

```bash
make build                    # build the marvindocs binary (once)
./scripts/sync-docs.sh        # download ~29 popular docs, rebuild index
./bin/marvindocs serve --http :3333
```

## Usage

```
./scripts/sync-docs.sh [DOCS...] [OPTIONS]
```

| Argument / option | Meaning |
|---|---|
| *(nothing)* | Download the built-in popular set (~29 docs). |
| `react`, `python~3.13`, … | Download only these specific slugs. |
| `--all` | Process every slug already listed in `data/public/docs/docs.json`. |
| `--check` | Dry-run: print up-to-date / outdated / missing status and exit without downloading. |
| `--force` | Ignore local mtime — re-download every requested slug unconditionally. |
| `--no-index` | Skip rebuilding `marvindocs.db` after download. |
| `--jobs N` | Parallel downloads (default `4`). |

### Examples

```bash
# A few specific docs
./scripts/sync-docs.sh react vue~3 typescript python~3.13

# Just check what is outdated, don't download anything
./scripts/sync-docs.sh --check --all

# Update every doc already on disk (unchanged ones are skipped with HTTP 304)
./scripts/sync-docs.sh --all

# Re-download everything from scratch even if nothing changed
./scripts/sync-docs.sh --all --force

# Download without rebuilding the index
./scripts/sync-docs.sh --all --no-index
./bin/marvindocs index build      # build index manually later
```

## Update detection

Each devdocs archive is served with a `Last-Modified` HTTP header.
After a successful download the script creates a marker file
`data/public/docs/<slug>/.last-modified` whose filesystem mtime equals
the server-side `Last-Modified`. On the next run:

- `curl --time-cond <marker>` sends the marker's mtime as
  `If-Modified-Since`.
- If the archive hasn't been re-uploaded, the server returns **304 Not
  Modified** and the script prints `SKIP <slug> (up-to-date)` — zero
  bytes transferred beyond the headers.
- If the archive is newer, it's downloaded and extracted, and the
  script prints `UPDATED <slug>` and refreshes the marker.
- `--force` bypasses the check and always re-downloads.
- `--check` only prints the status table and exits without touching
  any files.

Why a marker file instead of the `mtime` field inside `meta.json`?
The `meta.json mtime` is the devdocs *build* time, which can be
several seconds older than the S3 upload time. Comparing against it
leaves every doc in a permanent OUTDATED state. The marker uses the
same clock the server sends, so the comparison is exact.

This means re-running the script daily/weekly is cheap: one HEAD-like
request per slug and nothing else when upstream hasn't changed.

## What the script does

1. Resolves the list of slugs (built-in list, `--all` from existing
   `docs.json`, or the slugs you supplied).
2. Downloads `https://downloads.devdocs.io/<slug>.tar.gz` in parallel
   with `curl` and extracts each archive into `data/public/docs/<slug>/`.
   Failed slugs (404/403) are reported and skipped without aborting.
3. Rebuilds `data/public/docs/docs.json` from the `meta.json` file
   inside each downloaded slug directory (pure bash, no Python).
4. *(Unless `--no-index`)* Runs `./bin/marvindocs index build`.

## Built-in popular set

When called without arguments the script downloads:

```
angular  bash      c         cpp       css        django
docker   go        html      http      javascript  jinja
jquery   kubernetes linux    markdown  nginx       node
npm      postgresql python~3.13 react  redis      ruby
rust     sqlite    typescript  vue~3
```

Edit `DEFAULT_SLUGS` at the top of `scripts/sync-docs.sh` to change this.

## Finding slug names

Every name on <https://devdocs.io> is a downloadable slug.
Versioned docs use a tilde: `python~3.12`, `angular~18`, `react~18`.
Check what is already in your `docs.json`:

```bash
grep -o '"slug":"[^"]*"' data/public/docs/docs.json | cut -d'"' -f4
```

## Re-syncing / updating

The script is idempotent — re-running it overwrites existing files.
To update only changed docs:

```bash
./scripts/sync-docs.sh react vue~3
```

## Disk usage

| Set | Approx size | Download time (100 Mbit) |
|---|---|---|
| Built-in (~29 docs) | ~1–2 GB | ~2–5 min |
| All (~790 docs) | ~17 GB | ~30–90 min |
