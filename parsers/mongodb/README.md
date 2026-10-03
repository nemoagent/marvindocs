# MongoDB parser

Fetches the official MongoDB Manual from the
[`mongodb/docs`](https://github.com/mongodb/docs) repository (raw
reStructuredText sources on GitHub) and emits a DevDocs-shaped tree
that drops into `data/public/docs/mongodb~<short>/`.

DevDocs upstream does not ship the MongoDB Manual, so this parser fills
that gap.

## Build

```bash
cd parsers/mongodb
go build -o ../../bin/mongodb-parser .
```

## Run

```bash
./bin/mongodb-parser --version v8.0 --out data/public/docs/mongodb~8.0 --force
./bin/mongodb-parser --version v7.0 --out data/public/docs/mongodb~7.0 --force
./bin/mongodb-parser --version v6.0 --out data/public/docs/mongodb~6.0 --force
```

| Flag | Default | Notes |
|---|---|---|
| `--source` | `https://raw.githubusercontent.com/mongodb/docs/main/content/manual` | Override only if mirroring elsewhere. |
| `--version` | `v8.0` | Folder under `content/manual/`. Available: `v4.4`, `v5.0`, `v6.0`, `v7.0`, `v8.0`, `upcoming`, `manual`. |
| `--release` | (defaults to `--version`) | Free-form label written to `meta.json`. |
| `--out` | (required) | Output directory. Convention: `data/public/docs/mongodb~<short>`. |
| `--force` | `false` | Overwrite existing files. |

### Slug naming

`<short>` is the version with the leading `v` stripped:

| `--version` | output slug |
|---|---|
| `v8.0` | `mongodb~8.0` |
| `v7.0` | `mongodb~7.0` |
| `manual` | `mongodb~manual` |

After the parser finishes, refresh the index:

```bash
./bin/marvindocs index build --full
```

The new doc shows up in `marvindocs list`, `marvindocs search`, the web
UI and MCP `list_docs`. The parser also auto-updates
`data/public/docs/docs.json` via `devdocsfmt.upsertCatalog`, so adding
or replacing a version is a single command.

## What it produces

- One page per `.txt` file under `source/` (excluding `includes/`,
  which are reST transclusion fragments).
- One top-level entry per page plus one entry per heading
  (H1..H6) with anchor ids derived from heading text or any
  preceding `.. _target:` reference.
- The first path segment becomes the `type` of each entry
  (`Reference`, `Core`, `Tutorial`, ...), so marvindocs groups them
  in search results.
- An `index` page rendered from the page list, grouped by section.

## How it parses reST

The parser is a small, purpose-built reST renderer (no docutils, no
external dependencies) that handles the slice of reST that
`mongodb/docs` actually uses:

- Headings with under/overlines (`=`, `-`, `~`, `^`, `"`).
- Paragraphs, bullet/numeric lists.
- `.. code-block:: lang` (and `.. code::`, `.. sourcecode::`) plus
  literal indented blocks.
- `.. _target:` references hoisted onto the next heading as its
  anchor id, so cross-doc `:ref:` targets land on the right
  fragment id.
- Inline: ``` ``literal`` ```, `` `text <url>`_ ``, `:role:` (kept as
  visible text, target stripped), `**strong**`, `*emphasis*`.
- Substitutions: `.. |name| replace:: text` definitions are inlined.
- Drops directives that have no useful HTML rendering
  (`toctree`, `facet`, `meta`, `contents`, `dismissible-skills-card`,
  `cta-banner`, `include`, `default-domain`, ...) along with their
  indented body.

## Limitations

- Cross-document `:ref:`/`:doc:` links lose their target. The visible
  text is preserved but the link is not resolved — that would require
  a second pass over Snooty's target index.
- Nested directive bodies (e.g. `tabs`/`tab-set`) are dropped wholesale
  because their structure depends on Snooty extensions.
- Definition lists, field lists inside content, and grid layouts are
  rendered as paragraphs.
- `includes/` fragments are skipped entirely, which means inlined
  shared content (admonitions, fact tables) doesn't appear in pages.
  Adding include resolution is a follow-up.
