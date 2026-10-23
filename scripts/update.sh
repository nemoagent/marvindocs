#!/usr/bin/env bash
# scripts/update.sh — refresh everything in one safe step.
#
#   1. scripts/sync-docs.sh --all      download changed devdocs.io docs
#   2. scripts/run-parser.sh --all     rebuild parser-built docs (go-dev,
#                                      go-std, laravel, mongodb, ...)
#   3. marvindocs index build          incremental: only changed docs are
#                                      re-indexed, embeddings of unchanged
#                                      docs are kept
#   4. marvindocs index embed          vectors for chunks that have none
#
# Nothing here drops the index or its vectors; `index build --full` is
# never used. Embedder settings come from .env (see .env.example):
# MARVINDOCS_EMBEDDER=onnx (default, local CPU, slow), ollama or openai.
#
# Usage:
#   ./scripts/update.sh                 # full refresh
#   ./scripts/update.sh --no-parsers    # skip step 2 (no Docker/GitHub needed)
#   ./scripts/update.sh --no-embed      # skip step 4
#   ./scripts/update.sh --embed-only    # only step 4
#   ./scripts/update.sh --index-only    # only steps 3-4

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR"

RUN_SYNC=true
RUN_PARSERS=true
RUN_INDEX=true
RUN_EMBED=true
PARSER_FAILED=false

for arg in "$@"; do
	case "$arg" in
	--no-parsers) RUN_PARSERS=false ;;
	--no-embed) RUN_EMBED=false ;;
	--embed-only)
		RUN_SYNC=false
		RUN_PARSERS=false
		RUN_INDEX=false
		;;
	--index-only)
		RUN_SYNC=false
		RUN_PARSERS=false
		;;
	--help | -h)
		sed -n '2,21p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "error: unknown option: $arg" >&2
		exit 2
		;;
	esac
done

# .env is optional; its values become defaults for the marvindocs flags.
if [ -f .env ]; then
	set -a
	# shellcheck disable=SC1091
	. ./.env
	set +a
fi
EMBEDDER="${MARVINDOCS_EMBEDDER:-onnx}"

BINARY=bin/marvindocs
if [ ! -x "$BINARY" ]; then
	echo "==> $BINARY not found, building..."
	if [ "$EMBEDDER" = onnx ]; then
		make build-onnx
	else
		make build
	fi
fi

if [ "$RUN_SYNC" = true ]; then
	echo "==> [1/4] devdocs.io sync"
	./scripts/sync-docs.sh --all --no-index
fi

if [ "$RUN_PARSERS" = true ]; then
	echo ""
	echo "==> [2/4] parser-built docs"
	if ls bin/*-parser >/dev/null 2>&1; then
		# A failed parser must not stop the index/embed steps; the catalog is
		# rebuilt for the ones that succeeded and the failures are listed.
		./scripts/run-parser.sh --all || PARSER_FAILED=true
	else
		echo "  no bin/*-parser binaries, run 'make parsers' first; skipping"
	fi
fi

if [ "$RUN_INDEX" = true ]; then
	echo ""
	echo "==> [3/4] index build (incremental)"
	"$BINARY" index build
fi

if [ "$RUN_EMBED" = true ]; then
	echo ""
	echo "==> [4/4] embeddings (chunks without vectors)"
	case "$EMBEDDER" in
	onnx)
		echo "  embedder: onnx (local bge-m3 on CPU). This is the default and it is"
		echo "  slow: a large doc takes minutes, the whole dump many hours. To use a"
		echo "  GPU Ollama server put MARVINDOCS_EMBEDDER=ollama and"
		echo "  MARVINDOCS_OLLAMA_URL=... into .env (see .env.example)."
		;;
	ollama)
		echo "  embedder: ollama at ${MARVINDOCS_OLLAMA_URL:-http://localhost:11434}" \
			"(model ${MARVINDOCS_OLLAMA_MODEL:-bge-m3})"
		;;
	openai)
		echo "  embedder: openai at ${MARVINDOCS_OPENAI_URL:-http://localhost:8000/v1}" \
			"(model ${MARVINDOCS_OPENAI_MODEL:-bge-m3})"
		;;
	none)
		echo "  MARVINDOCS_EMBEDDER=none: skipping, search stays lexical"
		;;
	esac
	if [ "$EMBEDDER" != none ]; then
		"$BINARY" index embed --embedder "$EMBEDDER"
	fi
fi

echo ""
if [ "$PARSER_FAILED" = true ]; then
	echo "Update done, but some parsers failed (see FAILED above)."
	exit 1
fi
echo "Update done."
