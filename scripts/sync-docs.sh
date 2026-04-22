#!/usr/bin/env bash
# scripts/sync-docs.sh — download devdocs documentation into data/public/docs/
#
# Requires only: curl, tar  (standard on any Linux/macOS/WSL)
#
# Usage:
#   ./scripts/sync-docs.sh                        # built-in popular set
#   ./scripts/sync-docs.sh react vue angular      # specific slugs
#   ./scripts/sync-docs.sh --all                  # all slugs from existing docs.json
#   ./scripts/sync-docs.sh --no-index             # skip marvindocs index build
#
# Options:
#   --all        re-download every slug listed in data/public/docs/docs.json
#   --no-index   do not run marvindocs index build when finished
#   --jobs N     parallel downloads (default 4)
#   --help       show this message

set -euo pipefail

# ---- paths ------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DATA_DIR="$REPO_DIR/data/public/docs"
BINARY="$REPO_DIR/bin/marvindocs"

# ---- defaults ---------------------------------------------------------------
RUN_INDEX=true
PARALLEL_JOBS=4
MODE="list" # list | all | slugs
SLUGS=()

# Popular default set. Edit to taste.
DEFAULT_SLUGS=(
	angular
	bash
	c
	cpp
	css
	django
	docker
	go
	html
	http
	javascript
	jinja
	jquery
	kubernetes
	linux
	markdown
	nginx
	node
	npm
	postgresql
	python~3.13
	react
	redis
	ruby
	rust
	sqlite
	typescript
	vue~3
)

# ---- parse arguments --------------------------------------------------------
while [ $# -gt 0 ]; do
	case "$1" in
	--all)
		MODE="all"
		shift
		;;
	--no-index)
		RUN_INDEX=false
		shift
		;;
	--jobs)
		PARALLEL_JOBS="$2"
		shift 2
		;;
	--help | -h)
		sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	-*)
		echo "Unknown option: $1" >&2
		echo "Run $0 --help for usage." >&2
		exit 2
		;;
	*)
		MODE="slugs"
		SLUGS+=("$1")
		shift
		;;
	esac
done

# ---- resolve slug list ------------------------------------------------------
case "$MODE" in
all)
	if [ ! -f "$DATA_DIR/docs.json" ]; then
		echo "ERROR: $DATA_DIR/docs.json not found." >&2
		echo "       Run without --all first, or pass specific slugs." >&2
		exit 1
	fi
	# Extract "slug" values with grep + cut — no python, no jq needed.
	# docs.json format: [{"slug":"react",...}, ...]
	while IFS= read -r slug; do
		SLUGS+=("$slug")
	done < <(grep -o '"slug":"[^"]*"' "$DATA_DIR/docs.json" | cut -d'"' -f4)
	;;
list)
	SLUGS=("${DEFAULT_SLUGS[@]}")
	;;
esac

echo "==> target: $DATA_DIR"
echo "==> slugs:  ${#SLUGS[@]} (parallel downloads: $PARALLEL_JOBS)"
echo ""

mkdir -p "$DATA_DIR"

# ---- download one slug (called via xargs in parallel) ----------------------
download_slug() {
	local slug="$1"
	local dir="$DATA_DIR/$slug"
	local url="https://downloads.devdocs.io/${slug}.tar.gz"
	mkdir -p "$dir"
	if curl -sS --fail -L "$url" | tar -xzf - -C "$dir" 2>/dev/null; then
		echo "  OK   $slug"
	else
		echo "  FAIL $slug"
		rmdir "$dir" 2>/dev/null || true
	fi
}
export -f download_slug
export DATA_DIR

# ---- parallel download ------------------------------------------------------
printf '%s\n' "${SLUGS[@]}" |
	xargs -P "$PARALLEL_JOBS" -I{} bash -c 'download_slug "$@"' _ {}

# ---- rebuild docs.json from meta.json files (pure bash) --------------------
echo ""
echo "==> rebuilding docs.json from meta.json files..."

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

# ---- rebuild index ----------------------------------------------------------
if [ "$RUN_INDEX" = true ]; then
	if [ ! -x "$BINARY" ]; then
		echo ""
		echo "==> binary not found at $BINARY, building..."
		make -C "$REPO_DIR" build
	fi
	echo ""
	echo "==> rebuilding marvindocs index..."
	"$BINARY" index build \
		--docs-path "$DATA_DIR" \
		--index-path "$REPO_DIR/marvindocs.db"
fi

echo ""
echo "All done!"
echo "  Docs:  $DATA_DIR ($count slugs)"
if [ "$RUN_INDEX" = true ]; then
	echo "  Index: $REPO_DIR/marvindocs.db"
fi
echo ""
echo "Start the server:"
echo "  ./bin/marvindocs serve --http :3333"
