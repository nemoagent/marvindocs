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

To run the benchmark without synonyms, set `BENCH_SYNONYMS=off`.
