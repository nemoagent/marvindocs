# Consistency checks

Two helper scripts validate the integrity of the documentation and the
devdocs dump. Run them after `scripts/sync-docs.sh` to catch issues
early.

## check-docs-parity.sh

Verifies that `docs/en` and `docs/ru` are in sync:

- both directories contain the same set of `.md` files (by relative path);
  files present only in `docs/ru` are reported as well
- every shared file has the same ordered sequence of heading levels
  (`#`…`######`) — a quick structural guard against a translation that
  was started but not finished. Reordering headings fails the check
  too; heading text is not compared

No options or arguments.

### Usage

```bash
./scripts/check-docs-parity.sh
```

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Both language trees are consistent. |
| `1` | At least one file is missing from either tree or a heading structure differs. |
| `2` | IO error — `docs/en` or `docs/ru` not found. |

### Example output

```
OK: docs/en and docs/ru are consistent
```

Or on mismatch:

```
Files present in docs/en but missing in docs/ru:
  - new-feature.md
Heading structure differs: configuration.md (en=12, ru=11)
```

## check-devdocs-dump.sh

Validates the integrity of the devdocs dump on disk:

- `docs.json` is present at the dump root and contains valid JSON
- for every slug listed in `docs.json`: `index.json`, `db.json`, and
  `meta.json` exist, are non-empty (empty files are reported as
  `EMPTY` and counted under "Missing/empty"), and contain valid JSON
- orphan slug directories present on disk but absent from `docs.json`
  are reported

`jq` is mandatory (there is no fallback). An invalid `docs.json` aborts
the run with exit code `2`.

### Usage

```bash
./scripts/check-devdocs-dump.sh [--docs-path DIR]
```

### Options

| Flag | Default | Notes |
|---|---|---|
| `--docs-path DIR` | `data/public/docs` | Dump root to inspect. |

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Dump integrity check passed. |
| `1` | At least one issue found (missing file, invalid JSON, orphan dir). |
| `2` | Usage error, dump root / `docs.json` not found, or `docs.json` is not valid JSON. |

### Examples

```bash
# Check the default dump location
./scripts/check-devdocs-dump.sh

# Check a custom path
./scripts/check-devdocs-dump.sh --docs-path /custom/path
```

### Typical workflow

Run after `scripts/sync-docs.sh` to detect partially downloaded or
corrupted slugs. Any slug flagged as `MISSING`, `EMPTY` or `INVALID` can be
re-fetched with:

```bash
./scripts/sync-docs.sh --force <slug>
```

### Example output

```
MISSING  rust/db.json
ORPHAN   some-old-slug

Summary:
  OK slugs:       28
  Missing/empty:  1
  Invalid JSON:   0
  Orphan dirs:    1
```
