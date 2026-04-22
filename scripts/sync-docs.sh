#!/usr/bin/env bash
# scripts/sync-docs.sh — download devdocs documentation into data/public/docs/
#
# No Docker required. Downloads .tar.gz archives directly from
# downloads.devdocs.io using curl and tar. Requires only:
#   curl, tar (standard on any Linux/macOS)
#   python3   (used to merge meta.json files into docs.json)
#
# Usage:
#   ./scripts/sync-docs.sh                        # popular docs from built-in list
#   ./scripts/sync-docs.sh react vue angular      # specific slugs
#   ./scripts/sync-docs.sh --all                  # all slugs from existing docs.json
#   ./scripts/sync-docs.sh --no-index             # skip marvindocs index build
#
# Options:
#   --all        re-download every slug already present in data/public/docs/docs.json
#   --no-index   do not run marvindocs index build when finished
#   --jobs N     parallel downloads (default 4)
#   --help       show this message

set -euo pipefail

# ---- defaults ---------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DATA_DIR="$REPO_DIR/data/public/docs"
BINARY="$REPO_DIR/bin/marvindocs"

RUN_INDEX=true
PARALLEL_JOBS=4
MODE="list" # list | all | slugs
SLUGS=()

# Popular default set — a curated subset that covers most common use-cases.
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
while [[ $# -gt 0 ]]; do
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
		sed -n '2,15p' "$0" | sed 's/^# \?//'
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
if [[ "$MODE" == "all" ]]; then
	if [[ ! -f "$DATA_DIR/docs.json" ]]; then
		echo "ERROR: $DATA_DIR/docs.json not found."
		echo "       Run without --all first to download the default set,"
		echo "       or pass specific slugs."
		exit 1
	fi
	mapfile -t SLUGS < <(python3 -c "
import json
docs = json.load(open('$DATA_DIR/docs.json'))
for d in docs: print(d['slug'])
")
elif [[ "$MODE" == "list" ]]; then
	SLUGS=("${DEFAULT_SLUGS[@]}")
fi

echo "==> target: $DATA_DIR"
echo "==> slugs:  ${#SLUGS[@]} (parallel: $PARALLEL_JOBS)"
echo ""

mkdir -p "$DATA_DIR"

# ---- download function ------------------------------------------------------
download_slug() {
	local slug="$1"
	local dir="$DATA_DIR/$slug"
	local url="https://downloads.devdocs.io/${slug}.tar.gz"

	mkdir -p "$dir"
	if curl -sS --fail -L "$url" | tar -xzf - -C "$dir" 2>/dev/null; then
		echo "  OK   $slug"
	else
		echo "  FAIL $slug (curl/tar error — slug may not exist)"
		rmdir "$dir" 2>/dev/null || true
	fi
}
export -f download_slug
export DATA_DIR

# ---- parallel download ------------------------------------------------------
printf '%s\n' "${SLUGS[@]}" |
	xargs -P "$PARALLEL_JOBS" -I{} bash -c 'download_slug "$@"' _ {}

echo ""
echo "==> rebuilding docs.json from meta.json files..."

python3 - "$DATA_DIR" <<'PYEOF'
import json, os, sys

root = sys.argv[1]
docs = []
for slug in sorted(os.listdir(root)):
    meta_path = os.path.join(root, slug, 'meta.json')
    if not os.path.isfile(meta_path):
        continue
    try:
        with open(meta_path) as fh:
            docs.append(json.load(fh))
    except Exception as e:
        print(f'  warn: could not read {meta_path}: {e}', file=sys.stderr)

out = os.path.join(root, 'docs.json')
with open(out, 'w') as fh:
    json.dump(docs, fh, ensure_ascii=False, separators=(',', ':'))
print(f'  {len(docs)} docs -> {out}')
PYEOF

# ---- rebuild index ----------------------------------------------------------
if [[ "$RUN_INDEX" == true ]]; then
	if [[ ! -x "$BINARY" ]]; then
		echo ""
		echo "==> binary not found at $BINARY, building..."
		make -C "$REPO_DIR" build
	fi
	echo ""
	echo "==> rebuilding marvindocs index..."
	"$BINARY" index build --docs-path "$DATA_DIR" --index-path "$REPO_DIR/marvindocs.db"
fi

echo ""
echo "All done!"
echo "  Docs:  $DATA_DIR ($(ls "$DATA_DIR" | grep -v '^docs\.json$' | wc -l | tr -d ' ') slugs)"
if [[ "$RUN_INDEX" == true ]]; then
	echo "  Index: $REPO_DIR/marvindocs.db"
fi
echo ""
echo "Start the server:"
echo "  ./bin/marvindocs serve --http :3333"
