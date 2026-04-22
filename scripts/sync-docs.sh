#!/usr/bin/env bash
# scripts/sync-docs.sh — download / update devdocs documentation.
#
# Requires only: curl, tar  (standard on any Linux/macOS/WSL)
#
# Usage:
#   ./scripts/sync-docs.sh                        # built-in popular set
#   ./scripts/sync-docs.sh react vue angular      # specific slugs
#   ./scripts/sync-docs.sh --all                  # all slugs from existing docs.json
#   ./scripts/sync-docs.sh --check                # print what would change, no download
#   ./scripts/sync-docs.sh --force                # re-download everything, ignore mtime
#   ./scripts/sync-docs.sh --no-index             # skip marvindocs index build
#
# Options:
#   --all        re-download every slug listed in data/public/docs/docs.json
#   --check      dry-run: show up-to-date / outdated / missing status only
#   --force      ignore If-Modified-Since, always re-download
#   --no-index   do not run marvindocs index build when finished
#   --jobs N     parallel downloads (default 4)
#   --help       show this message
#
# Update detection
#
# Each devdocs archive ships a meta.json with an `mtime` field (unix timestamp
# of the server-side build). The script reads that timestamp from the local
# data/public/docs/<slug>/meta.json, converts it to HTTP date and passes it
# to `curl -z` — the server then returns 304 Not Modified if the archive
# hasn't been rebuilt, and the slug is reported as UP-TO-DATE without
# downloading a single byte.

set -euo pipefail

# ---- paths ------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DATA_DIR="$REPO_DIR/data/public/docs"
BINARY="$REPO_DIR/bin/marvindocs"

# ---- defaults ---------------------------------------------------------------
RUN_INDEX=true
PARALLEL_JOBS=4
MODE="list"      # list | all | slugs
CHECK_ONLY=false # --check: print status table, do not download
FORCE=false      # --force: ignore If-Modified-Since
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
	--check)
		CHECK_ONLY=true
		RUN_INDEX=false
		shift
		;;
	--force)
		FORCE=true
		shift
		;;
	--jobs)
		PARALLEL_JOBS="$2"
		shift 2
		;;
	--help | -h)
		sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
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
	while IFS= read -r slug; do
		SLUGS+=("$slug")
	done < <(grep -o '"slug":"[^"]*"' "$DATA_DIR/docs.json" | cut -d'"' -f4)
	;;
list)
	SLUGS=("${DEFAULT_SLUGS[@]}")
	;;
esac

echo "==> target: $DATA_DIR"
if [ "$CHECK_ONLY" = true ]; then
	echo "==> check only, no downloads"
elif [ "$FORCE" = true ]; then
	echo "==> force: ignoring local mtime"
fi
echo "==> slugs:  ${#SLUGS[@]} (parallel: $PARALLEL_JOBS)"
echo ""

mkdir -p "$DATA_DIR"

# ---- helpers ---------------------------------------------------------------

# local_mtime <slug>  -> echoes unix timestamp from meta.json, empty if none
local_mtime() {
	local meta="$DATA_DIR/$1/meta.json"
	[ -f "$meta" ] || return 0
	grep -o '"mtime":[0-9]*' "$meta" | head -n1 | cut -d: -f2
}

# unix_to_http <timestamp>  -> HTTP-date format (GNU date or BSD date)
unix_to_http() {
	local ts="$1"
	date -u -d "@$ts" "+%a, %d %b %Y %H:%M:%S GMT" 2>/dev/null ||
		date -u -r "$ts" "+%a, %d %b %Y %H:%M:%S GMT"
}

# remote_mtime <slug>  -> unix timestamp of the archive's Last-Modified,
# empty on error. Uses HEAD only; no bytes transferred.
remote_mtime() {
	local slug="$1"
	local lm
	lm=$(curl -sI --fail "https://downloads.devdocs.io/${slug}.tar.gz" 2>/dev/null |
		grep -i '^last-modified:' | sed 's/^[^:]*: *//; s/\r$//')
	[ -n "$lm" ] || return 0
	date -u -d "$lm" +%s 2>/dev/null ||
		date -u -jf "%a, %d %b %Y %H:%M:%S GMT" "$lm" +%s 2>/dev/null
}

# ---- dry-run: --check ------------------------------------------------------
if [ "$CHECK_ONLY" = true ]; then
	printf '%-22s %-14s %-14s  %s\n' "SLUG" "LOCAL MTIME" "REMOTE MTIME" "STATUS"
	n_up=0
	n_out=0
	n_miss=0
	n_err=0
	for slug in "${SLUGS[@]}"; do
		lm=$(local_mtime "$slug")
		rm=$(remote_mtime "$slug")
		if [ -z "$rm" ]; then
			status="ERROR (no remote)"
			n_err=$((n_err + 1))
		elif [ -z "$lm" ]; then
			status="MISSING"
			n_miss=$((n_miss + 1))
		elif [ "$rm" -gt "$lm" ]; then
			status="OUTDATED"
			n_out=$((n_out + 1))
		else
			status="up-to-date"
			n_up=$((n_up + 1))
		fi
		printf '%-22s %-14s %-14s  %s\n' "$slug" "${lm:--}" "${rm:--}" "$status"
	done
	echo ""
	echo "summary: $n_up up-to-date, $n_out outdated, $n_miss missing, $n_err errors"
	exit 0
fi

# ---- download one slug (called via xargs in parallel) ----------------------
download_slug() {
	local slug="$1"
	local dir="$DATA_DIR/$slug"
	local url="https://downloads.devdocs.io/${slug}.tar.gz"

	# Conditional GET if we have a local mtime and --force wasn't set.
	local zflag=()
	if [ "$FORCE" != true ]; then
		local lm http
		lm=$(local_mtime "$slug")
		if [ -n "$lm" ]; then
			http=$(unix_to_http "$lm")
			zflag=(--time-cond "$http")
		fi
	fi

	# HTTP status is written to a temp file so we can distinguish 200/304/404.
	local tmp
	tmp="$(mktemp)"
	local code
	code=$(curl -sS --fail-with-body -o "$tmp" -w '%{http_code}' -L \
		"${zflag[@]}" "$url" 2>/dev/null || true)

	case "$code" in
	200)
		mkdir -p "$dir"
		if tar -xzf "$tmp" -C "$dir" 2>/dev/null; then
			echo "  UPDATED $slug"
		else
			echo "  FAIL    $slug (tar error)"
		fi
		;;
	304)
		echo "  SKIP    $slug (up-to-date)"
		;;
	"" | 000)
		echo "  FAIL    $slug (network error)"
		;;
	*)
		echo "  FAIL    $slug (HTTP $code)"
		;;
	esac
	rm -f "$tmp"
}
export -f download_slug local_mtime unix_to_http
export DATA_DIR FORCE

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
