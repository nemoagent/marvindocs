# DevDocs data format

This page is the **authoritative reference** for the on-disk format
marvindocs reads. Anyone writing their own scraper/parser can target
this exact layout and the result will just work with `marvindocs index
build` and `marvindocs serve`.

All information below has been verified against the devdocs dump used
by this project.

## 1. Directory layout

```
<data-root>/
├── docs.json                 # catalogue of every indexed doc
├── <slug>/
│   ├── meta.json             # per-doc metadata
│   ├── index.json            # entries + types
│   ├── db.json               # page contents, keyed by path
│   ├── .last-modified        # stamp file: mtime = doc mtime (used by sync-docs.sh)
│   ├── index.html            # optional: same as db.json["index"]
│   └── <path>.html           # optional: one file per db.json entry
├── <other-slug>/
│   └── ...
```

- `<data-root>` is what you pass via `--docs-path`
  (default: `data/public/docs`).
- `<slug>` is the doc's identifier (lowercase, ASCII, may contain
  `~` for versioned variants, e.g. `python~3.12`).
- `.last-modified` is an empty stamp file whose filesystem mtime equals
  the doc's `mtime`. `scripts/sync-docs.sh` compares it with the remote
  catalog to decide whether to re-download; marvindocs itself does not
  read it. Parsers from `parsers/` write it too.
- The `.html` files are a convenience for direct web serving — marvindocs
  **does not read them**; it reads only the `.json` files. You can
  safely omit them to save disk.

## 2. `docs.json` — global catalogue

A JSON **array** of objects, one per indexed doc.

```json
[
  {
    "name":        "Angular",
    "slug":        "angular",
    "type":        "angular",
    "links":       { "home": "https://angular.dev/",
                     "code": "https://github.com/angular/angular" },
    "version":     "",
    "release":     "21.2.0",
    "mtime":       1772652661,
    "db_size":     10014653,
    "attribution": "Super-powered by Google ...",
    "alias":       "ng"
  }
]
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Human-readable title ("Angular", "Python", "React"). |
| `slug` | string | yes | Must match the directory name under `<data-root>`. Lowercase ASCII; `~` allowed for versioned variants. |
| `type` | string | yes | Logical family (`"angular"`, `"python"` …). Used to group related slugs. |
| `version` | string | yes | May be empty. Human label shown next to `name`. |
| `release` | string | yes | May be empty. Specific release tag, e.g. `"21.2.0"`. |
| `mtime` | integer | yes | Unix time (seconds) of last index rebuild. |
| `db_size` | integer | yes | Uncompressed size of `db.json` in bytes. marvindocs uses it only as a display hint. |
| `links` | object | no | String→URL map. Typical keys: `home`, `code`. Shown as-is. |
| `attribution` | string | no | HTML-safe legal/credits snippet. |
| `alias` | string | no | Short shorthand (`"ng"` for angular). Not used by marvindocs but preserved. |

**Invariants**

- Every `slug` in `docs.json` must have a directory `<data-root>/<slug>/`
  containing at least `index.json` and `db.json`.
- `slug` values must be unique across the array.
- The array may be in any order.

## 3. `<slug>/meta.json` — per-doc metadata

A single JSON object with the **same fields** as the corresponding
entry in `docs.json` (except `attribution` and `alias`, which are
optional here too). Example:

```json
{
  "name":   "Angular",
  "slug":   "angular",
  "type":   "angular",
  "links":  { "home": "https://angular.dev/",
              "code": "https://github.com/angular/angular" },
  "version": "",
  "release": "21.2.0",
  "mtime":   1772652661,
  "db_size": 10014653
}
```

`meta.json` is useful for stand-alone tools that want to read one doc
without loading the global `docs.json`. marvindocs itself reads only
`docs.json` and ignores `meta.json`, but producers are expected to keep
the two in sync.

## 4. `<slug>/index.json` — searchable catalogue

A single JSON object:

```json
{
  "entries": [
    { "name": "$localize",            "path": "api/localize/init/$localize",   "type": "localize" },
    { "name": "$locationShim",        "path": "api/common/upgrade/$locationshim", "type": "common" }
  ],
  "types": [
    { "name": "animations", "count": 36, "slug": "animations" },
    { "name": "aria",       "count": 31, "slug": "aria" }
  ]
}
```

### 4.1 `entries[]`

One entry per navigable symbol / page fragment.

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | What the user sees in search results (e.g. `"useState"`). |
| `path` | string | yes | Location of the entry. See §4.3 below. |
| `type` | string | yes | One of the `types[].name` values from the same file. |

An entry is **unique by `(name, path)`** — the same name may appear at
several paths (e.g. `useState` in `react` and `react~18`).

### 4.2 `types[]`

One entry per category. Used only for grouping; marvindocs doesn't
require `types[]` but keeps it if present.

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Human-readable label. Matches `entries[].type`. |
| `count` | integer | yes | Number of entries with this type. Informational only; marvindocs does not use it. |
| `slug` | string | yes | Short slug for the category (used in URLs in devdocs.io's own UI; marvindocs ignores it). |

### 4.3 Path syntax

A `path` is either:

- **`"foo/bar"`** — refers to the page stored as `db.json["foo/bar"]`.
- **`"foo/bar#fragment"`** — refers to a sub-section of that same page;
  look up `db.json["foo/bar"]` and scroll to the anchor `#fragment`.

Only the **portion before `#`** is used as a `db.json` key. Everything
after `#` is forwarded verbatim to the browser / consumer.

In the shipped dump ~80% of entries have a `#fragment` (anchor into a
larger page) and ~20% point to a page of their own.

## 5. `<slug>/db.json` — page bodies

A JSON **object** (string → string):

```json
{
  "index":        "<h1>Angular</h1> ...",
  "guide/ssr":    "<h2>Server-side rendering</h2> ...",
  "api":          "<section>...</section>"
}
```

- **Keys** are page paths. Every `index.json` entry whose `path` (minus
  `#fragment`) is not a key here is unreachable — marvindocs silently
  skips such entries at serve time.
- **Values** are **HTML fragments**, not full documents. They must not
  contain `<html>`, `<head>`, `<body>`, or `<link rel="stylesheet">`.
  Assume they will be injected into an already-styled host page.

A single page may be very large (tens of KB of HTML) — the whole file
for angular is ~10 MB and for ansible ~270 MB. marvindocs never holds a
decoded `db.json` in memory: on first access it scans the file once to
record the byte range of every value (a few hundred KB per doc, cached
per slug) and then reads individual pages with a positioned read.

### 5.1 Allowed HTML

- Standard block and inline elements (`<h1>`–`<h6>`, `<p>`, `<ul>`,
  `<ol>`, `<li>`, `<table>`, `<pre>`, `<code>`, `<a>`, `<img>`, `<em>`,
  `<strong>`, `<div>`, `<span>`, …).
- Relative links of the form `<a href="foo/bar">` are resolved by the
  web UI against the current page URL. Anchor-only links
  (`<a href="#section">`) work as usual.
- External `<img src="https://…">` are loaded from their original host;
  marvindocs does not proxy or rewrite them.
- Inline CSS (`style="..."`) and `<svg>` are allowed.

### 5.2 Forbidden HTML

- No `<script>`, `<iframe>`, `<form>` — these would run inside the
  host page and break isolation. marvindocs doesn't explicitly strip
  them today, but future sanitization may.
- No `<!DOCTYPE>`, no `<html>`, no `<head>`, no `<body>`.
- No document-level `<title>` tag — use `<h1>` for the page title.

## 6. Optional `.html` files

Some dumps include sibling `.html` files next to each `db.json` entry:

```
angular/
├── db.json
├── index.html         # == db.json["index"]
└── guide/
    └── ssr.html       # == db.json["guide/ssr"]
```

They are identical byte-for-byte to the corresponding `db.json` value
and exist purely so that devdocs' own Rails app can serve them as
static files. marvindocs does **not** use them — you can omit them in
your own dumps.

## 7. Encoding and whitespace

- All JSON files are **UTF-8** without BOM.
- No trailing commas; standard JSON only.
- HTML fragments may contain any valid UTF-8, including non-BMP code
  points and HTML entities.

## 8. Minimal producing algorithm

For anyone writing a new scraper, the minimal set of steps is:

1. For each doc you want to publish, generate HTML fragments for every
   reachable page. Store them in a flat `path -> html` map.
2. Generate `index.json` by listing every navigable symbol with its
   `name`, `path` (optionally `#fragment`), and `type`.
3. Aggregate types into `types[]`; precompute `count`.
4. Write `db.json` (the map from step 1).
5. Write `meta.json` with the doc's identity.
6. After processing all docs, write the global `docs.json` array —
   one entry per `meta.json` you produced.

`index build` will then pick everything up in one pass.

## 9. Versioned variants

When a single product ships multiple documented versions, use `~` in
the slug:

```
python~3.11
python~3.12
python~3.13
```

Each is fully independent (own `docs.json` entry, own directory, own
`db.json`). marvindocs has no special handling — they are just
separate docs whose `type` is usually the same (`"python"`).

## 10. Size expectations

Measured on a full dump (830 docs):

- `docs.json`: ~175 KB (one JSON object per doc).
- `meta.json`: ~200 bytes typical.
- `index.json`: 1 KB – 8 MB per doc, median ~100 KB.
- `db.json`: 30 KB – 280 MB per doc, median ~3 MB.

Plan disk accordingly: the total depends on how many docs you keep. The
size of marvindocs' own index also depends on the doc set; it tokenizes
Markdown chunks of the page bodies (with a few bytes of offsets per
chunk) but does not store the text, so it stays well below the combined
`db.json` size. For the full devdocs dump (~5.2 M chunks) the index is
about 2.4 GB, or about 9 GB with chunk vectors (see
[vectors.md](vectors.md)).
