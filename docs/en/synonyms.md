# Synonym dictionary

Search queries are expanded with synonyms: `k8s` also finds
`kubernetes`, `regex` also finds `regular expression`. The dictionary is
a set of Markdown files, a built-in one is embedded in the binary and you
can add your own.

## File format

The format is the same as in Terraphim / Logseq knowledge graphs: one
Markdown file per concept.

- The concept is the first `# heading` line; if the file has no heading,
  the file name without `.md` is used.
- A line `synonyms:: a, b, c` lists other names of the concept. The key is
  case-insensitive; several such lines add up.
- Every name of a concept is a synonym of every other name.
- Other lines are ignored.
- Only `*.md` files in the directory are read (not recursive).

Example `regex.md`:

```
# regular expression

synonyms:: regex, regexp
```

Matching is case-insensitive and on whole words. A name may have several
words (`regular expression`); when several names match at one place, the
longest multi-word match wins.

## Built-in dictionary

The built-in dictionary lives in `internal/synonyms/builtin/*.md`
(16 files) and is embedded in the binary.

| Concept | Synonyms |
|---|---|
| `javascript` | `js`, `ecmascript` |
| `typescript` | `ts` |
| `python` | `py` |
| `postgresql` | `postgres`, `pg` |
| `mongodb` | `mongo` |
| `kubernetes` | `k8s`, `kube` |
| `go` | `golang` |
| `node` | `nodejs`, `node.js` |
| `regular expression` | `regex`, `regexp`, `регулярное выражение`, `регулярка` |
| `environment variable` | `env var`, `envvar`, `переменная окружения` |
| `asynchronous` | `async`, `асинхронный` |
| `command line` | `cli`, `командная строка` |
| `yaml` | `yml` |
| `closure` | `замыкание` |
| `authentication` | `authn`, `аутентификация` |
| `authorization` | `authz`, `авторизация` |

## Abbreviation dictionary (`synonyms/`)

The repository also ships an optional dictionary of 289 IT abbreviations in
`synonyms/`: `dns`, `csrf`, `jwt`, `cte`, `ssr`, `rbac` and others. It was
mined from the docs with `synonyms suggest`, then reviewed by an LLM:

- kept only exact equivalents;
- dropped abbreviations that are ordinary words or tokens (`pie`, `dos`,
  `cow`, `jar`, `pod`), ambiguous ones (`csr`, `san`, `sse`) and non-IT ones;
- added established Russian terms to 96 concepts, so `система доменных имён`
  or `межсайтовая подделка запроса` find English pages.

It is not embedded in the binary. Enable it with:

```bash
marvindocs serve --synonyms synonyms/
```

On the 96-case benchmark it changes nothing (no regressions): the benchmark
has almost no queries with such abbreviations.

## Configuration

| Flag | Environment variable | Meaning |
|---|---|---|
| `--synonyms DIR` | `MARVINDOCS_SYNONYMS` | Extra dictionary directory. The flag wins over the variable. |

The flag is available on `marvindocs serve` and `marvindocs search`.

| Value | Effect |
|---|---|
| empty (default) | Built-in dictionary only. |
| a directory | Its `*.md` files are **added** to the built-in dictionary. |
| `off` | Query expansion is disabled entirely, including synonym-based doc detection. |

A non-existent directory is an error at startup.

```bash
marvindocs serve --http :3333 --synonyms ./my-synonyms
MARVINDOCS_SYNONYMS=off marvindocs search "k8s deployment"
```

The dictionary is loaded at startup: restart `serve` to pick up changes.
No index rebuild is needed, because expansion happens at query time.

## How expansion works

Every query term that has synonyms becomes an OR group of the term and
its synonyms in the SQLite FTS5 query. For example `k8s deployment`
becomes:

```
("k8s" OR "kubernetes"* OR "kube") AND "deployment"*
```

Expansion applies to:

- the MCP tools `search` (entry names), `search_pages` and `get_docs`
  (the lexical part, including the lexical part of hybrid search);
- the web UI search;
- the CLI: `marvindocs search` and `marvindocs search --body`.

Snippets highlight synonym matches too.

### Doc auto-filter

The doc auto-filter (a doc name in the query restricts the search to that
doc) resolves synonyms as well: `pg jsonb` searches `postgresql`,
`golang context` searches `go`. This replaces the former hardcoded alias
table (`js`, `py`, `ts`, `pg`, `mongo`).

## Writing your own entries

Add only true equivalents: abbreviations, alternative spellings,
translations. An OR group ranks every synonym equally with the original
word, so a broad synonym drags loosely related pages up the ranking.

## Building a dictionary from the docs

Writing a dictionary by hand is slow. `marvindocs synonyms suggest` mines
abbreviation candidates from the docs themselves, and
`marvindocs synonyms import` turns a reviewed table into dictionary files.

### Mining candidates: `synonyms suggest`

```bash
marvindocs synonyms suggest [--docs-path DIR] [--doc SLUG]... [--min-docs N] [--min-count N] \
                            [--names-only] [--synonyms DIR] [--out FILE]
```

The command looks for pairs written as `Server-Side Rendering (SSR)` or
`SSR (Server-Side Rendering)`. The abbreviation (3-10 letters, at least 2
capitals, plural `s` dropped: `ASGs` -> `asg`) must be made of leading
letters of the words of the name, in order. Each word gives at least one
letter; stop words like `of`, `and`, `the`, `for`, `in`, `to` may give
none. The name must have at least two words; parts of a hyphenated word
count as separate words. Before a parenthesised abbreviation the shortest
run of preceding words that spells it is taken, so
`Migrating from Create React App (CRA)` gives `create react app`.

It scans entry names (`index.json`) and page text (`db.zip`) of **one
version per doc** (the most recently scraped one, because versions repeat
the same text), in parallel on all CPUs. Versions of a doc count as one
doc.

| Flag | Default | Notes |
|---|---|---|
| `--docs-path` | `data/public/docs` | Directory with `docs.json` and `<slug>/{index.json,db.zip}`. |
| `--doc` | all docs | Scan only this slug; repeatable. Exact slugs, including the version. |
| `--min-docs` | `2` | Keep pairs found in at least N docs. |
| `--min-count` | `3` | Keep pairs with at least N occurrences. |
| `--names-only` | `false` | Scan entry names only: seconds instead of minutes. |
| `--synonyms` | empty | Same flag and `MARVINDOCS_SYNONYMS` variable as in `serve`. Terms that already have synonyms in the built-in dictionary or in DIR are skipped, so re-running after an import suggests only new ones. |
| `--out` | stdout | Write the table to a file. |

On the full archive (830 docs, 234 doc names, 2.7 GB of `db.zip`):

| Mode | Time | Memory | Candidates |
|---|---|---|---|
| `--names-only` | about 2 s | | 5 |
| full text | about 10 min | about 500 MB | about 420 |

The top rows of a full run: `universally unique identifier` / `uuid`,
`transport layer security` / `tls`, `cross-origin resource sharing` /
`cors`, `content security policy` / `csp`, `abstract syntax tree` / `ast`,
`document object model` / `dom`, `server-side rendering` / `ssr`,
`json web token` / `jwt`, `common table expression` / `cte`.

### Filters

- An abbreviation with several meanings is dropped, unless one meaning
  makes at least 60% of its uses.
- Name spellings that differ by a final `s` are merged; the singular form
  is used as the concept when it was seen.
- Several abbreviations of one name share one row.
- If the name has hyphens, its spaced form is added as a synonym
  (`server-side rendering` -> `ssr, server side rendering`), because
  queries are split on spaces.

### Output format

A TSV table. The first line is a comment header:

```
# concept<TAB>synonyms<TAB>count<TAB>docs<TAB>examples
```

| Column | Content |
|---|---|
| `concept` | The full name. |
| `synonyms` | Comma-separated abbreviations and spellings. |
| `count` | Number of occurrences. |
| `docs` | Number of docs where the pair was found. |
| `examples` | Up to 5 example doc names. |

Rows are sorted by number of docs, then by count.

### Writing files: `synonyms import`

```bash
marvindocs synonyms import FILE --out DIR [--force]
```

`FILE` is the table; `-` reads stdin. It may come before or after the
flags. Only the first two columns are used (concept and comma-separated
synonyms); blank lines and lines starting with `#` are ignored; rows of
the same concept are merged.

| Flag | Default | Notes |
|---|---|---|
| `--out` | required | Dictionary directory; created if missing. |
| `--force` | `false` | Overwrite existing files. Without it existing files are skipped, so hand edits survive. |

One file per concept is written in the [dictionary format](#file-format):
`# concept`, a blank line, `synonyms:: a, b`. The file name is the concept
lower-cased, with every run of non letter/digit characters replaced by `-`,
plus `.md` (`server-side rendering` -> `server-side-rendering.md`;
Cyrillic letters are kept).

### Recommended workflow

1. Mine the candidates:
   ```bash
   marvindocs synonyms suggest --out synonyms.tsv
   ```
2. Review the table. Delete rows whose abbreviation is a common English
   word or is otherwise harmful for search: in a real run these were
   `dos`, `pie`, `pic`, `cow`, `mic`, `des`, `tos`, `dst` (a query word
   `pie` would also search `position independent executables`). Delete
   off-topic rows. Add Russian or other translations and spellings to the
   `synonyms` column. An LLM can do a first pass over the table (ask it to
   keep only exact equivalents and add established Russian terms), then a
   human skims the result: it is a few hundred rows.
3. Write the dictionary files:
   ```bash
   marvindocs synonyms import synonyms.tsv --out synonyms/
   ```
4. Start the server with the dictionary:
   ```bash
   marvindocs serve --synonyms synonyms/
   ```
   or set `MARVINDOCS_SYNONYMS=synonyms/`.
5. Check the benchmark against the default (see the note below).

No index rebuild is needed: expansion happens at query time.

## Benchmark note

The `get_docs` benchmark in `internal/mcpserver` has 96 cases. Result with
and without the built-in dictionary:

| | hit@1 | hit@3 |
|---|---|---|
| synonyms on | 47/96 | 72/96 |
| synonyms off | 47/96 | 72/96 |

One case improved (`async await`: from a miss to rank 4), no case got
worse. Broad concept synonyms (cleanup/teardown, hash map/dictionary) were
tried and removed from the built-in set because they hurt ranking.

`BENCH_SYNONYMS` accepts `off` (no synonyms at all) or a directory (added
to the built-in dictionary, like `--synonyms`). To compare a dictionary
built with `synonyms import` against the default:

```bash
BENCH_INDEX=marvindocs.db BENCH_DOCS=data/public/docs BENCH_SYNONYMS=synonyms \
  go test -tags sqlite_fts5 -run TestBench -v ./internal/mcpserver/
```
