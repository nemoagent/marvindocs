# Vector search

Chunk embeddings are stored in the index and used by the MCP tool
`get_docs` for hybrid search (vectors + BM25). The feature is optional:
without `--embedder` nothing changes.

## What is stored

The index has a table `chunk_vecs(chunk_id INTEGER PRIMARY KEY, vec BLOB)`
with one row per chunk of `chunks`. `vec` is an int8-quantised bge-m3
vector: 4 bytes of float32 scale (little-endian) followed by one signed
byte per dimension (1024, about 1 KB per chunk). Vectors are computed
from the chunk's breadcrumb (`doc > page > heading`) plus its text,
cut to 4000 bytes (the ONNX embedder also cuts input to 512 tokens).
Rows are removed together with their doc on `index build` / sync.

## How search uses it

When `serve` runs with `--embedder` and a doc has vectors, `get_docs`
searches that doc in a hybrid way:

1. The topic is embedded into one query vector (about 30-50 ms with ONNX
   int8 on CPU).
2. Three ranked lists of chunks are fused with Reciprocal Rank Fusion
   (k=60): the score of a chunk is the sum of `weight / (60 + rank)` over
   the lists it appears in.

   | Signal | Weight | Chunks |
   |---|---|---|
   | vector | 1 | Top chunks by cosine similarity (a brute-force scan over the vectors of that one doc, cached in memory on first use). |
   | lexical | 0.5 | Top chunks by BM25 (the same lexical chunk search as `search_pages`). |
   | entries | 0.25 | Pages whose entries (`index.json` symbols and page titles) match every word of the query, ranked by BM25; each page counts as its first chunk. |

   The vector side leads because it handles paraphrases, the lexical side
   rescues exact names, and the entries side favours the page named after
   the topic.
3. Pages are ranked by their best fused chunk.

Docs without vectors, and the tools `search` and `search_pages`, keep the
lexical BM25 search unchanged.

## Bench

The bench is the Go test `TestBench` in `internal/mcpserver/bench_test.go`
over 96 cases in `internal/mcpserver/testdata/bench_cases.json`: 12 docs,
36 keyword (`lex`) queries and 60 natural-language (`nl`) queries, each
with an expected page. hit@1 / hit@3 is the number of queries where the
expected page is the first / among the first three pages returned by
`get_docs`.

Results on the fully embedded devdocs dump, hybrid search with the weights
above:

| Queries | hit@1 | hit@3 |
|---|---|---|
| all (96) | 63 | 81 |
| keyword (36) | 31 | 35 |
| natural language (60) | 32 | 46 |

Without the entries signal (`titleWeight` 0) the result is 61/96 and
80/96. `titleWeight` of 0.5 and higher starts to hurt natural-language
queries; lexical weight 1 is slightly worse than 0.5.

Run it:

```bash
BENCH_INDEX=marvindocs.db BENCH_DOCS=data/public/docs \
BENCH_OLLAMA=http://localhost:11434 \
go test -tags sqlite_fts5 -run 'TestBench$' -v ./internal/mcpserver/
```

- The test is skipped unless `BENCH_INDEX` and `BENCH_DOCS` are set.
- `BENCH_OLLAMA` (Ollama with bge-m3) switches on the hybrid search;
  without it the bench runs lexical-only.
- `BENCH_WEIGHTS="lex:title,lex:title,..."` (e.g. `0.5:0,0.5:0.25`) runs
  the bench once per weight pair.
- The index must have vectors for the 12 bench docs: ansible, docker,
  git, go, javascript, kubernetes, nginx, postgresql~17, python~3.13,
  react, rust, typescript.

## Building vectors

`marvindocs index embed` embeds the chunks that have no vector yet, so it
can be interrupted (Ctrl-C) and re-run; each batch is committed on its own.

```bash
marvindocs index build
marvindocs index embed --doc react --doc vue~3
marvindocs index embed                     # every doc in the index
```

| Flag | Default | Notes |
|---|---|---|
| `--doc` | all docs | Doc slug; may be repeated. |
| `--embedder` | `onnx` | `ollama`, `openai` or `onnx`. |
| `--ollama-url` | `http://localhost:11434` | Ollama base URL. |
| `--ollama-model` | `bge-m3` | Ollama model name. |
| `--openai-url` | `http://localhost:8000/v1` | OpenAI-compatible server base URL, with `/v1`. |
| `--openai-model` | `bge-m3` | Embedding model name on that server. |
| `--model-dir` | `data/onnx/model` | onnx: directory with `model_int8.onnx` and `tokenizer.json`. |
| `--onnxruntime` | `data/onnx/lib/libonnxruntime.so` | onnx: path to `libonnxruntime.so`. |
| `--batch` | 64 | Chunks per embedder call and per transaction. |
| `--docs-path`, `--index-path` | as in `index build` | |

The embedder defaults come from environment variables, which flags
override: `MARVINDOCS_EMBEDDER` (`none`, `ollama`, `openai`, `onnx`),
`MARVINDOCS_OLLAMA_URL`, `MARVINDOCS_OLLAMA_MODEL`, `MARVINDOCS_OPENAI_URL`,
`MARVINDOCS_OPENAI_MODEL`, `MARVINDOCS_MODEL_DIR`
and `MARVINDOCS_ONNXRUNTIME`. `scripts/update.sh` loads them from a
git-ignored `.env` (template: `.env.example`); see
[Configuration](configuration.md#environment-variables). `index embed`
defaults to `onnx` (local CPU, slow: hours for the whole dump), while
`serve` still defaults to `none`.

Measured throughput:

| Embedder | Speed | Notes |
|---|---|---|
| Ollama, bge-m3 on a GPU | ~110-130 chunks/s | The full devdocs dump (~5.2 M chunks) takes ~12 h and adds ~6.5 GB to the index (~1.3 KB per chunk); the full index ends up at ~9 GB. |
| ONNX int8 on CPU | far slower | One text at a time; fine for a few docs, not for the full dump. |

So Ollama on a GPU machine is the way to embed in bulk, and ONNX is the
recommended embedder for `serve`: it is local and needs no server.

### Ollama

```bash
ollama pull bge-m3
marvindocs index embed --doc react
```

### OpenAI-compatible server (vLLM)

`--embedder openai` calls `POST {url}/embeddings` of any OpenAI-compatible
server. With vLLM:

```bash
docker run --gpus all -p 8000:8000 vllm/vllm-openai:v0.32.0 \
    BAAI/bge-m3 --served-model-name bge-m3
marvindocs index embed --embedder openai \
    --openai-url http://localhost:8000/v1 --openai-model bge-m3
```

bge-m3 in vLLM and in Ollama gives the same vectors up to rounding, so an
index embedded through one can be served through the other.

## Serving with vectors

`serve` accepts the same embedder flags, with `--embedder none|ollama|openai|onnx`
(default `none`).

```bash
marvindocs serve --http :3333 --embedder onnx \
    --model-dir /path/to/model --onnxruntime /path/to/libonnxruntime.so
```

With Ollama:

```bash
marvindocs serve --http :3333 --embedder ollama \
    --ollama-url http://localhost:11434 --ollama-model bge-m3
```

The ONNX int8 model needs about 1.1 GB of RAM in the server process.

## Building with ONNX

The ONNX embedder needs a build with the `onnx` tag, which links the
tokenizer library statically through cgo. Without the tag `--embedder onnx`
fails with "built without onnx support (rebuild with -tags onnx)".

The Docker image (`Dockerfile`, alpine) is built without the `onnx` tag and
cannot use `--embedder onnx`, because the ONNX Runtime binaries are glibc.
In Docker use `--embedder ollama`, or run the host binary from
`make build-onnx`.

### Quick path

```bash
make build-onnx    # onnx-fetch + bin/marvindocs with -tags "sqlite_fts5 onnx"
make run-onnx      # build-onnx + serve --http :3333 --embedder onnx ...
```

`make onnx-fetch` runs `scripts/fetch-onnx.sh`, which downloads into
`data/onnx/` (git-ignored; the `ONNX_DIR` environment variable overrides
the path):

| File | Source |
|---|---|
| `lib/libtokenizers.a` | daulet/tokenizers v1.27.0 |
| `lib/libonnxruntime.so` | ONNX Runtime 1.30.0 |
| `model/model_int8.onnx` | Xenova/bge-m3 (568 MB) |
| `model/tokenizer.json` | Xenova/bge-m3 |

The script is idempotent: files that already exist are kept. It supports
Linux x86_64 only; on other platforms fetch the same four files by hand
into the same layout.

`make build-onnx` builds in Docker like `make build`, with
`CGO_LDFLAGS=-L/src/data/onnx/lib`. `make run-onnx` starts
`serve --http :3333 --embedder onnx --model-dir data/onnx/model
--onnxruntime data/onnx/lib/libonnxruntime.so`.

### By hand

1. Download `libtokenizers.a` for your platform from the releases of
   https://github.com/daulet/tokenizers and put it into a directory, e.g. `lib/`.
2. Download the ONNX Runtime release archive (v1.30 or compatible) and
   take `lib/libonnxruntime.so` from it.
3. Download `onnx/model_int8.onnx` and `tokenizer.json` from
   https://huggingface.co/Xenova/bge-m3 into one directory.
4. Build and run:

```bash
CGO_LDFLAGS="-L$PWD/lib" go build -tags "sqlite_fts5 onnx" -o marvindocs ./cmd/marvindocs
./marvindocs index embed --embedder onnx \
    --model-dir /path/to/model --onnxruntime /path/to/libonnxruntime.so
```

## Keeping vectors up to date

`marvindocs index build` (incremental sync by default) re-indexes only new
or changed docs and deletes their rows including vectors. Unchanged docs
keep their vectors.

A doc counts as changed by its content, not only by mtime: the index stores
a SHA-256 over the `db.zip` central directory (entry name, CRC-32,
uncompressed size) + `index.json` in `docs.content_hash`. If the mtime
changed but the content did not (typical after re-running a parser), only
the mtime is updated and the vectors are kept. If the content changed, the
doc is rebuilt and `index embed` embeds it again. An index from an older
version gets the column added automatically and the hashes backfilled on
the first `index build`. After converting an existing data directory with
`marvindocs pack --all`, run `index build --rehash` once to recompute the
hashes under the new definition without re-indexing or re-embedding.

`index embed` embeds only chunks without a vector, so after every
`index build` run `index embed` (or `make embed`) again: only the changed
docs are embedded.

`index build --full` drops everything, vectors included, so a full
re-embed is needed (~11 h for the full dump). To prevent accidents it
refuses to run when the index contains vectors and prints an error; add
`--drop-vectors` to confirm. Use plain `index build` instead.

`make update` (`scripts/update.sh`) does the whole cycle without ever
dropping vectors: sync, parsers, incremental `index build`, `index embed`.
See [Syncing the documentation data](sync-docs.md#one-command-update).

A doc with a mix of embedded and not-yet-embedded chunks is treated by
`get_docs` as having vectors. Run `index embed` to completion before
relying on hybrid results for that doc.

## Setting up on another machine

```bash
scripts/sync-docs.sh     # download the docs
make index               # build the index
make embed               # embed chunks; hours for the full dump
make run-onnx            # serve with the ONNX embedder
```

`make embed` runs `scripts/update.sh --embed-only`, i.e. `index embed` with
the embedder from `.env`. The default is local ONNX; to use an Ollama
server with bge-m3 (much faster on a GPU) put this into `.env`:

```bash
MARVINDOCS_EMBEDDER=ollama
MARVINDOCS_OLLAMA_URL=http://host:11434
```

Instead of re-embedding, you can copy the index file `marvindocs.db` with
the vectors to the other machine.
