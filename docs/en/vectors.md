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
2. The top chunks by cosine similarity (a brute-force scan over the
   vectors of that one doc, cached in memory on first use) and the top
   chunks by BM25 (the same lexical chunk search as `search_pages`) are
   fused with Reciprocal Rank Fusion: k=60, lexical weight 0.5, vector
   weight 1.
3. Pages are ranked by their best fused chunk.

Docs without vectors, and the tools `search` and `search_pages`, keep the
lexical BM25 search unchanged.

Bench: 32 developer questions over 5 docs (postgresql, python, go,
ansible, react), run against the fully embedded devdocs dump:

| Method | hit@1 | hit@3 |
|---|---|---|
| BM25 only | 25/32 | 28/32 |
| Hybrid (RRF) | 28/32 | 30/32 |

On a small index with only those 5 docs the same bench gave 25/29 for
BM25, 24/28 for vectors alone and 30/31 for the hybrid.

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
| `--embedder` | `ollama` | `ollama` or `onnx`. |
| `--ollama-url` | `http://localhost:11434` | Ollama base URL. |
| `--ollama-model` | `bge-m3` | Ollama model name. |
| `--model-dir` | | onnx: directory with `model_int8.onnx` and `tokenizer.json`. |
| `--onnxruntime` | | onnx: path to `libonnxruntime.so`. |
| `--batch` | 64 | Chunks per embedder call and per transaction. |
| `--docs-path`, `--index-path` | as in `index build` | |

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

## Serving with vectors

`serve` accepts the same embedder flags, with `--embedder none|ollama|onnx`
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
or changed docs, detected by the mtime of the doc files, and deletes their
rows including vectors. Unchanged docs keep their vectors.

`index embed` embeds only chunks without a vector, so after every
`index build` run `index embed` (or `make embed`) again: only the changed
docs are embedded.

`index build --full` drops everything, vectors included, so a full
re-embed is needed (~11 h for the full dump).

A doc with a mix of embedded and not-yet-embedded chunks is treated by
`get_docs` as having vectors. Run `index embed` to completion before
relying on hybrid results for that doc.

## Setting up on another machine

```bash
scripts/sync-docs.sh     # download the docs
make index               # build the index
make embed               # needs Ollama with bge-m3; hours for the full dump
make run-onnx            # serve with the ONNX embedder
```

`make embed` runs `index embed --embedder ollama --ollama-url $(OLLAMA_URL)`;
the default is `http://localhost:11434`, override it with
`make embed OLLAMA_URL=http://host:11434`.

Instead of re-embedding, you can copy the index file `marvindocs.db` with
the vectors to the other machine.
