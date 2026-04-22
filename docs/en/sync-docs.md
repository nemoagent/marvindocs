# Syncing the documentation data

marvindocs does not ship the devdocs documentation itself. This page
explains how to use the bundled helper script to download the data and
build the search index in one step.

## Prerequisites

- **Docker** — the script builds a devdocs image to run the download
  inside the same Ruby environment that devdocs uses.
- **devdocs source tree** — the script expects `../devdocs` by default
  (one level up from this repo). Clone it if you don't have it:

  ```bash
  git clone https://github.com/freeCodeCamp/devdocs ../devdocs
  ```

- **`./bin/marvindocs`** — built from this repo (`make build`). Only
  needed if you want the index rebuilt automatically after the download.

## Quick start

```bash
# 1. Build the marvindocs binary (if not done yet)
make build

# 2. Download the default set (~30 popular docs, ~1-2 GB, a few minutes)
./scripts/sync-docs.sh

# 3. Start the server
./bin/marvindocs serve --http :3333
```

That's it. After step 2 you'll have docs in `data/public/docs/` and a
fresh `marvindocs.db`.

## Usage

```
./scripts/sync-docs.sh [DOCS...] [OPTIONS]
```

| Argument / option | Meaning |
|---|---|
| *(nothing)* | Download the default set (same as `--default`). |
| `--default` | Download ~30 popular docs chosen by devdocs maintainers. |
| `--all` | Download every available doc (~790, ~17 GB, 1–2 hours). |
| `react`, `python~3.13`, … | Download only specific slugs. Pass as many as you like. |
| `--no-index` | Skip rebuilding `marvindocs.db` after the download. |
| `--devdocs-dir DIR` | Path to the devdocs source tree (default: `../devdocs`). |

### Examples

```bash
# Just react and a specific Python version
./scripts/sync-docs.sh react python~3.13

# Everything, no index rebuild (you'll run marvindocs index build manually)
./scripts/sync-docs.sh --all --no-index

# devdocs is somewhere else
./scripts/sync-docs.sh --devdocs-dir ~/src/devdocs
```

## What the script does

1. Writes a minimal `Dockerfile` (no `thor docs:download --all`) and
   builds a Docker image named `marvindocs-devdocs-sync`. Only the
   Ruby gem dependencies are installed at build time — downloading docs
   at build time would bake all 17 GB into the image layer.
2. Runs a container; inside it calls
   `thor docs:download <your args>`, which fetches `.tar.gz` archives
   from `downloads.devdocs.io` and extracts them to
   `/devdocs/public/docs/`.
3. Copies `/devdocs/public/docs/` out of the container into
   `data/public/docs/` in this repo.
4. Removes the container.
5. *(Unless `--no-index`)* Runs `./bin/marvindocs index build` to
   populate `marvindocs.db`.

## Available doc slugs

To see the full list of slugs devdocs knows about:

```bash
docker run --rm marvindocs-devdocs-sync thor docs:list
```

Or browse https://devdocs.io — every documentation name there
corresponds to a downloadable slug.

## Re-syncing / updating

The script is safe to re-run. `thor docs:download` overwrites existing
archives, and `marvindocs index build` rebuilds the index from scratch.
To update only a few docs:

```bash
./scripts/sync-docs.sh react vue angular
```

## Disk usage

| Set | Approx size | Time (good connection) |
|---|---|---|
| `--default` (~30 docs) | 1–3 GB | 5–15 min |
| `--all` (~790 docs) | ~17 GB | 1–2 hours |

The Docker image itself is ~700 MB (Ruby + gems). You can remove it
after the download:

```bash
docker rmi marvindocs-devdocs-sync
```
