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
ansible, react):

| Method | hit@1 | hit@3 |
|---|---|---|
| BM25 only | 25/32 | 29/32 |
| Vectors only | 24/32 | 28/32 |
| Hybrid (RRF) | 30/32 | 31/32 |

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
| Ollama, bge-m3 on a GPU | ~110-130 chunks/s | The full devdocs dump (~5.2 M chunks) takes ~11 h and adds ~5 GB to the index (~1 KB per chunk). |
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
tokenizer library statically through cgo:

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

Without the `onnx` tag `--embedder onnx` fails with "built without onnx
support (rebuild with -tags onnx)".
