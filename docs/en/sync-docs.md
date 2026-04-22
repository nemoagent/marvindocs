# Syncing the documentation data

marvindocs does not ship the devdocs documentation itself. The helper
script `scripts/sync-docs.sh` downloads it directly from
`downloads.devdocs.io` using only `curl` and `tar` — no Docker, no Ruby,
no devdocs checkout needed.

## Prerequisites

- `curl`
- `tar`
- `python3` (used to assemble `docs.json` from individual `meta.json` files)
- `./bin/marvindocs` — only if you want the index rebuilt automatically
  (`make build`).

All of these ship on any Linux or macOS out of the box.

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
| `--all` | Re-download every slug already listed in `data/public/docs/docs.json`. |
| `--no-index` | Skip rebuilding `marvindocs.db` after download. |
| `--jobs N` | Parallel downloads (default `4`). |

### Examples

```bash
# A few specific docs
./scripts/sync-docs.sh react vue~3 typescript python~3.13

# All slugs from the current docs.json (update everything)
./scripts/sync-docs.sh --all

# Download without rebuilding the index
./scripts/sync-docs.sh --all --no-index
./bin/marvindocs index build      # build index manually later
```

## What the script does

1. Resolves the list of slugs (built-in list, `--all` from existing
   `docs.json`, or the slugs you supplied).
2. Downloads `https://downloads.devdocs.io/<slug>.tar.gz` in parallel
   with `curl` and extracts each archive into `data/public/docs/<slug>/`.
   Failed slugs (404/403) are reported and skipped without aborting.
3. Rebuilds `data/public/docs/docs.json` from the `meta.json` file
   inside each downloaded slug directory using Python.
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
python3 -c "import json; [print(d['slug']) for d in json.load(open('data/public/docs/docs.json'))]"
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
