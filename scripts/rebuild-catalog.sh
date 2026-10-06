#!/usr/bin/env bash
# scripts/rebuild-catalog.sh — rebuild <docs>/docs.json from <slug>/meta.json.
#
# docs.json is what marvindocs reads (slugs and their mtime), so it must be
# regenerated whenever a doc directory is added or rewritten. sync-docs.sh
# and run-parser.sh call this after they change anything.
#
# Usage: ./scripts/rebuild-catalog.sh [DOCS_DIR]   (default: data/public/docs)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA_DIR="${1:-$ROOT/data/public/docs}"
DOCS_JSON="$DATA_DIR/docs.json"
TMP_JSON="$DOCS_JSON.tmp"

count=0
sep=""
printf '[' >"$TMP_JSON"
for slug in $(ls -1 "$DATA_DIR" | LC_ALL=C sort); do
	meta="$DATA_DIR/$slug/meta.json"
	[ -f "$meta" ] || continue
	printf '%s' "$sep" >>"$TMP_JSON"
	cat "$meta" >>"$TMP_JSON"
	sep=","
	count=$((count + 1))
done
printf ']' >>"$TMP_JSON"
mv "$TMP_JSON" "$DOCS_JSON"
echo "  $count docs -> $DOCS_JSON"
