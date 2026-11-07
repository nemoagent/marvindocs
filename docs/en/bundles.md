# Offline bundles

A bundle is a ready-made piece of the documentation set: doc pages plus
a search index with embeddings. Instead of downloading all of devdocs
(~17 GB), building the index and embedding chunks (hours without a GPU),
you install only the bundles you need and start the server right away.
Hybrid search in `get_docs` works immediately, no embedding needed.

Bundles are published as assets of the GitHub release
[`bundles`](https://github.com/nemoagent/marvindocs/releases/tag/bundles)
of `nemoagent/marvindocs`.

## Available bundles

| Bundle | Docs | Archive | Contents |
|---|---|---|---|
| `core` | 10 | 199 MB | bash, zsh, fish, git, http, man, markdown, jq, gnu_make, cmake |
| `go` | 3 | 13 MB | go, go-std, go-dev |
| `python` | 26 | 300 MB | python, django, flask, fastapi, numpy, pandas, pytorch, ... |
| `db` | 15 | 91 MB | postgresql, mariadb, sqlite, redis, mongodb, duckdb, ... |
| `infra` | 34 | 401 MB | nginx, caddy, docker, podman, kubernetes, helm, prometheus, grafana, terraform, ansible, ... |
| `iac` | 15 | 115 MB | terraform_aws, terraform_azurerm, terraform_google, terragrunt, tflint, pulumi, vault, consul, nomad, packer, ... |
| `smarthome` | 7 | 132 MB | home_assistant, home_assistant_dev, zigbee2mqtt, mosquitto, frigate, esphome, hacs |
| `js` | 80 | 287 MB | javascript, typescript, node, react, vue, angular, css, html, ... |
| `php` | 13 | 230 MB | php, composer, phpunit, laravel, symfony, wordpress, ... |
| `ruby` | 8 | 23 MB | ruby, rails, rack, minitest, nokogiri, ... |
| `jvm` | 16 | 697 MB | openjdk, kotlin, scala, groovy, clojure, spring_boot, ... |
| `native` | 18 | 384 MB | c, cpp, gcc, rust, ... |
| `other` | 73 | 308 MB | every doc not listed in another bundle |

All 13 bundles: 318 docs, ~3.2 GB of archives (`smarthome` measured on 2026-11-07). The exact lists are in
[`bundles.yaml`](../../bundles.yaml); sizes are from the 2026-10-29 build.

### Which versions are included

A bundle ships, for every doc it lists:

- the newest version (`python~3.14`, not `python~3.13`);
- supported LTS releases from the `lts` list of `bundles.yaml`
  (openjdk 8/11/17/21, node 22/24, django 5.2, haproxy 2.6-3.2,
  julia 1.10);
- non-version variants (`haxe~cpp`, `rethinkdb~java`), and per-variant
  newest versions (`gcc~14` and `gcc~14_cpp`).

Older versions are not in bundles; get them with
[`scripts/sync-docs.sh`](sync-docs.md) and `index build`.

## Install

Requirements: only the `marvindocs` binary and internet access. The
repository is public, so no GitHub CLI, login or token is needed.

```bash
marvindocs bundle install core go python
marvindocs serve --http :3333
```

What `install` does for each bundle:

1. downloads `SHA256SUMS` and `NAME.tar.gz` over HTTPS from
   `https://github.com/nemoagent/marvindocs/releases/download/bundles/` into a temporary directory and checks the archive checksum
   (a name missing from `SHA256SUMS` or a mismatch stops the install);
   a broken download is retried up to 5 times and resumes where it stopped
   (HTTP `Range`);
2. unpacks `docs/<slug>/` into `--docs-path` (`data/public/docs`),
   replacing existing directories of the same docs;
3. adds the docs to `docs.json` (entries with the same slug are replaced);
4. merges the bundle index into `--index-path` (`marvindocs.db`): docs
   with the same slug are replaced, others are kept; the full-text index
   is filled from the chunk texts, so no HTML conversion or embedding runs.

The index file is created if it does not exist. Installing the same
bundle again updates it in place. A running `serve` does not see new
docs: restart it.

Offline install from a directory with downloaded archives:

```bash
mkdir -p /tmp/b && cd /tmp/b
curl -LO https://github.com/nemoagent/marvindocs/releases/download/bundles/go.tar.gz -LO https://github.com/nemoagent/marvindocs/releases/download/bundles/SHA256SUMS
sha256sum -c --ignore-missing SHA256SUMS
marvindocs bundle install --from /tmp/b go
```

| Flag | Default | Notes |
|---|---|---|
| `--repo` | `nemoagent/marvindocs` | GitHub repository with the release; the download URL is `https://github.com/REPO/releases/download/TAG/`. |
| `--tag` | `bundles` | Release tag. |
| `--from` | (empty) | Directory with `NAME.tar.gz`; no download. |
| `--docs-path` | `data/public/docs` | Data directory. |
| `--index-path` | `marvindocs.db` | SQLite index file. |

Bundles mix with the other ways of getting docs: after an install,
`index build` (incremental) keeps the bundle docs and their vectors as
long as their files are unchanged, and `sync-docs.sh` + `index build` +
`index embed` can add docs that no bundle has.

For Docker, install into the mounted data directory and index file on
the host, then restart the container. The `marvindocs:full` image has
every bundle preinstalled, and in `marvindocs:empty` bundles are installed
inside the container, see
[Getting started](getting-started.md#images-with-bundles-full-and-empty).

## Build and publish (maintainers)

Bundles are built from a full local setup where every bundle doc is
downloaded, indexed and embedded.

```bash
make update                        # or sync-docs.sh + index build + index embed
marvindocs bundle build            # all bundles -> dist/bundles/NAME.tar.gz
marvindocs bundle build go js      # only these
```

`build` refuses a doc that is not indexed or changed since the last
`index build` (its chunks would not match the stored vectors): run
`index build` and `index embed` first. Building all bundles takes about
12 minutes. Docs without vectors are packed as is; their search in the
installed index is lexical only.

| Flag | Default | Notes |
|---|---|---|
| `--config` | `bundles.yaml` | Bundle definitions. |
| `--out` | `dist/bundles` | Output directory (`dist/` is git-ignored). |
| `--docs-path` | `data/public/docs` | Data directory. |
| `--index-path` | `marvindocs.db` | Full SQLite index. |

Publish (replaces existing assets):

```bash
cd dist/bundles
sha256sum *.tar.gz > SHA256SUMS
gh release upload bundles -R nemoagent/marvindocs SHA256SUMS *.tar.gz --clobber
```

Large uploads can stop half way; check the assets with
`gh release view bundles -R nemoagent/marvindocs` and re-upload missing
files one by one.

### Editing `bundles.yaml`

```yaml
lts:                 # releases kept besides the newest version
  - node~24_lts
bundles:
  go: [go, go-std, go-dev]   # doc names: slug part before "~"
```

A doc name may appear in one bundle only; unlisted docs go to `other`.
Review the `lts` list against <https://endoflife.date> when rebuilding.

## Archive format

`NAME.tar.gz` contains:

| Path | Contents |
|---|---|
| `bundle.db` | SQLite index of the bundle docs: `docs`, `entries`, `pages`, `chunks`, `chunk_vecs` and `chunk_text` (breadcrumb and body of every chunk, used to fill the contentless `chunks_fts` on merge). |
| `docs.json` | `docs.json` entries of the bundle docs. |
| `docs/<slug>/...` | Doc directories as described in [Data format](data-format.md). |

## Full index snapshot (maintainers)

Bundles hold only the newest versions and LTS. The full local index
(all versions, including bge-m3 vectors of old releases that are
expensive to recompute) is backed up as is to a separate release.
`bundle install` does not use it.

Current snapshot:
[`snapshot-2026-11-02`](https://github.com/nemoagent/marvindocs/releases/tag/snapshot-2026-11-02),
`marvindocs.db` (7.8 GB, 892 docs, 3.7M vectors) and `data/`; 11.5 GB
unpacked, 6.7 GB as `tar.zst` split into four parts plus `SHA256SUMS`.

Restore in the repository root:

```bash
gh release download snapshot-2026-11-02 -R nemoagent/marvindocs
sha256sum -c SHA256SUMS
cat marvindocs-snapshot.tar.zst.part* | zstd -dc | tar -xf -
```

Create a new one (do not run `index build` or `index embed` while
archiving). Parts are 1900 MiB because a release asset must be under
2 GiB:

```bash
TAG=snapshot-$(date +%F)
tar -cf - marvindocs.db data | zstd -T0 \
  | split -b 1900M -d - /backup/marvindocs-snapshot.tar.zst.part
cd /backup && sha256sum marvindocs-snapshot.tar.zst.part* > SHA256SUMS
gh release create "$TAG" -R nemoagent/marvindocs --latest=false \
  --title "Index snapshot $(date +%F)" --notes "Full index backup"
gh release upload "$TAG" -R nemoagent/marvindocs --clobber \
  SHA256SUMS marvindocs-snapshot.tar.zst.part*
```

Check the upload: GitHub reports a `digest` for each asset, compare it
with `SHA256SUMS`:

```bash
gh api repos/nemoagent/marvindocs/releases/tags/$TAG \
  -q '.assets[]|"\(.digest) \(.name)"'
```
