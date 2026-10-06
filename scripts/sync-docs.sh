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
#   ./scripts/sync-docs.sh --list-remote          # list all docs available on devdocs.io
#   ./scripts/sync-docs.sh --search-remote QUERY  # grep remote catalog by slug/name
#
# Options:
#   --all            re-download every slug listed in data/public/docs/docs.json
#   --check          dry-run: show up-to-date / outdated / missing status only
#   --force          ignore catalog mtime, always re-download
#   --no-index       do not run marvindocs index build when finished
#   --list-remote    fetch devdocs.io catalog and print slug/name/version table
#   --search-remote Q  filter the remote catalog by substring (case-insensitive)
#   --jobs N         parallel downloads (default 8)
#   --help           show this message
#
# Update detection
#
# devdocs.io publishes a single catalog at https://devdocs.io/docs.json
# that lists every documentation set together with its build `mtime`
# (unix timestamp). The script fetches that catalog once per run and
# compares each slug's catalog mtime to a local stamp file
# (data/public/docs/<slug>/.last-modified). Equal mtimes -> SKIP
# without any network roundtrip. One GET replaces N HEAD requests,
# which used to dominate runtime for `--check` and large slug lists.
#
# Parser-built docs (parsers/registry.json, written by run-parser.sh) carry
# a data/public/docs/<slug>/.parser marker. Such slugs are reported as LOCAL
# and never overwritten, even with --force and even if devdocs.io has the
# same slug (e.g. laravel~11). Slugs without a marker that are neither in
# the catalog nor on downloads.devdocs.io but have a local stamp are also
# reported as LOCAL.

set -euo pipefail

# ---- paths ------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DATA_DIR="$REPO_DIR/data/public/docs"
BINARY="$REPO_DIR/bin/marvindocs"

# ---- defaults ---------------------------------------------------------------
RUN_INDEX=true
PARALLEL_JOBS=8
MODE="list"      # list | all | slugs
CHECK_ONLY=false # --check: print status table, do not download
FORCE=false      # --force: ignore If-Modified-Since
REMOTE_ACTION="" # --list-remote / --search-remote
REMOTE_QUERY=""
SLUGS=()

# Popular default set. Edit to taste.
DEFAULT_SLUGS=(
	angular
	bash
	c
	cpp
	css
	django~5.2
	docker
	go
	html
	http
	javascript
	jinja~3.1
	jquery
	kubernetes
	markdown
	nginx
	node
	npm
	postgresql~17
	python~3.13
	react
	redis
	ruby~3.4
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
	--list-remote)
		REMOTE_ACTION="list"
		shift
		;;
	--search-remote)
		REMOTE_ACTION="search"
		REMOTE_QUERY="${2:-}"
		if [ -z "$REMOTE_QUERY" ]; then
			echo "--search-remote needs a query argument" >&2
			exit 2
		fi
		shift 2
		;;
	--help | -h)
		sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'
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

# ---- remote catalog (no download, no index) --------------------------------
if [ -n "$REMOTE_ACTION" ]; then
	CATALOG_URL="https://devdocs.io/docs.json"
	echo "==> fetching remote catalog: $CATALOG_URL" >&2
	catalog=$(curl -sSLf "$CATALOG_URL") || {
		echo "ERROR: failed to fetch $CATALOG_URL" >&2
		exit 1
	}

	# Parse JSON array of flat doc objects. Splits top-level objects using
	# brace-depth tracking so that nested "links": {...} doesn't confuse us.
	rows=$(printf '%s' "$catalog" | awk '
		BEGIN { depth=0; buf="" }
		{
			line=$0
			n=length(line)
			for (i=1; i<=n; i++) {
				c=substr(line,i,1)
				if (c=="{") { if (depth==0) buf=""; depth++; buf=buf c; continue }
				if (c=="}") { depth--; buf=buf c; if (depth==0) print buf; continue }
				if (depth>0) buf=buf c
			}
		}' | awk '
		{
			s=$0
			slug=""; name=""; version=""; release=""
			if (match(s, /"slug":[[:space:]]*"[^"]*"/))    { t=substr(s,RSTART,RLENGTH); sub(/"slug":[[:space:]]*"/,"",t);    sub(/"$/,"",t); slug=t }
			if (match(s, /"name":[[:space:]]*"[^"]*"/))    { t=substr(s,RSTART,RLENGTH); sub(/"name":[[:space:]]*"/,"",t);    sub(/"$/,"",t); name=t }
			if (match(s, /"version":[[:space:]]*"[^"]*"/)) { t=substr(s,RSTART,RLENGTH); sub(/"version":[[:space:]]*"/,"",t); sub(/"$/,"",t); version=t }
			if (match(s, /"release":[[:space:]]*"[^"]*"/)) { t=substr(s,RSTART,RLENGTH); sub(/"release":[[:space:]]*"/,"",t); sub(/"$/,"",t); release=t }
			if (slug == "") next
			v = (version != "") ? version : release
			printf "%s\t%s\t%s\n", slug, name, v
		}')

	if [ "$REMOTE_ACTION" = "search" ]; then
		q=$(printf '%s' "$REMOTE_QUERY" | tr '[:upper:]' '[:lower:]')
		rows=$(printf '%s\n' "$rows" | awk -F'\t' -v q="$q" '
			{ l = tolower($1 "\t" $2); if (index(l, q)) print $0 }')
	fi

	count=$(printf '%s\n' "$rows" | grep -c . || true)
	if [ "$count" -eq 0 ]; then
		echo "no matches" >&2
		exit 0
	fi
	printf '%-32s  %-40s  %s\n' "SLUG" "NAME" "VERSION"
	printf '%s\n' "$rows" | awk -F'\t' '{ printf "%-32s  %-40s  %s\n", $1, $2, $3 }'
	echo >&2
	echo "$count doc(s)" >&2
	exit 0
fi

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
	done < <(grep -oE '"slug":[[:space:]]*"[^"]*"' "$DATA_DIR/docs.json" |
		sed -E 's/"slug":[[:space:]]*"([^"]*)"/\1/')
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

# local_mtime <slug>  -> echoes unix timestamp, empty if none.
local_mtime() {
	local stamp="$DATA_DIR/$1/.last-modified"
	[ -f "$stamp" ] || return 0
	stat -c %Y "$stamp" 2>/dev/null || stat -f %m "$stamp" 2>/dev/null
}

# ---- fetch remote catalog (one GET for all slugs) --------------------------
#
# Produces a TSV file CATALOG_TSV with rows: <slug>\t<mtime>.
# Used by both --check and the parallel download path so the
# whole run makes exactly one HTTPS request to devdocs.io for
# metadata, regardless of how many slugs are processed.
CATALOG_TSV="$(mktemp)"
trap 'rm -f "$CATALOG_TSV" "${DOWNLOAD_LOG:-}"' EXIT

echo "==> fetching catalog from https://devdocs.io/docs.json"
if ! curl -sSLf "https://devdocs.io/docs.json" |
	awk '
		BEGIN { depth=0; buf="" }
		{
			n=length($0)
			for (i=1; i<=n; i++) {
				c=substr($0,i,1)
				if (c=="{") { if (depth==0) buf=""; depth++; buf=buf c; continue }
				if (c=="}") { depth--; buf=buf c; if (depth==0) print buf; continue }
				if (depth>0) buf=buf c
			}
		}' | awk '
		{
			s=$0; slug=""; mt=""
			if (match(s, /"slug":[[:space:]]*"[^"]*"/)) { t=substr(s,RSTART,RLENGTH); sub(/"slug":[[:space:]]*"/,"",t); sub(/"$/,"",t); slug=t }
			if (match(s, /"mtime":[[:space:]]*[0-9]+/))  { t=substr(s,RSTART,RLENGTH); sub(/"mtime":[[:space:]]*/,"",t); mt=t }
			if (slug != "" && mt != "") printf "%s\t%s\n", slug, mt
		}' >"$CATALOG_TSV"; then
	echo "ERROR: failed to fetch devdocs catalog" >&2
	exit 1
fi
catalog_count=$(wc -l <"$CATALOG_TSV" | tr -d ' ')
echo "    $catalog_count docs in catalog"
echo ""

# remote_mtime <slug>  -> unix timestamp from cached catalog, empty if absent.
# Falls back to a single HEAD against downloads.devdocs.io for slugs that
# are served from S3 but not listed in the public catalog (e.g. unversioned
# django, ruby, jinja). The result is appended to the catalog so subsequent
# lookups in the same run hit the cache.
remote_mtime() {
	local slug="$1" lm ts
	ts=$(awk -F'\t' -v s="$slug" '$1==s { print $2; exit }' "$CATALOG_TSV")
	if [ -n "$ts" ]; then
		printf '%s' "$ts"
		return
	fi
	lm=$(curl -sI --fail --max-time 10 "https://downloads.devdocs.io/${slug}.tar.gz" 2>/dev/null |
		grep -i '^last-modified:' | sed 's/^[^:]*: *//; s/\r$//')
	[ -n "$lm" ] || return 0
	ts=$(date -u -d "$lm" +%s 2>/dev/null || date -u -jf "%a, %d %b %Y %H:%M:%S %Z" "$lm" +%s 2>/dev/null)
	[ -n "$ts" ] || return 0
	printf '%s\t%s\n' "$slug" "$ts" >>"$CATALOG_TSV"
	printf '%s' "$ts"
}

# ---- dry-run: --check ------------------------------------------------------
if [ "$CHECK_ONLY" = true ]; then
	printf '%-22s %-14s %-14s  %s\n' "SLUG" "LOCAL MTIME" "REMOTE MTIME" "STATUS"
	n_up=0
	n_out=0
	n_miss=0
	n_local=0
	n_err=0
	for slug in "${SLUGS[@]}"; do
		lm=$(local_mtime "$slug")
		if [ -f "$DATA_DIR/$slug/.parser" ]; then
			rm=""
			status="LOCAL (parser-built)"
			n_local=$((n_local + 1))
			printf '%-22s %-14s %-14s  %s\n' "$slug" "${lm:--}" "-" "$status"
			continue
		fi
		rm=$(remote_mtime "$slug")
		if [ -z "$rm" ] && [ -n "$lm" ]; then
			status="LOCAL (parser-built)"
			n_local=$((n_local + 1))
		elif [ -z "$rm" ]; then
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
	echo "summary: $n_up up-to-date, $n_out outdated, $n_miss missing, $n_local local, $n_err errors"
	exit 0
fi

# ---- download one slug (called via xargs in parallel) ----------------------
#
# Skips network entirely when the local stamp matches catalog mtime.
# Otherwise unconditionally downloads (HTTP 200) — no `curl -z` because
# we already know from the catalog that the archive changed.
download_slug() {
	local slug="$1"
	local dir="$DATA_DIR/$slug"
	local url="https://downloads.devdocs.io/${slug}.tar.gz"
	local stamp="$dir/.last-modified"

	if [ -f "$dir/.parser" ]; then
		echo "  LOCAL   $slug (parser-built, skipped)"
		return
	fi

	local remote_ts local_ts lm
	remote_ts=$(awk -F'\t' -v s="$slug" '$1==s { print $2; exit }' "$CATALOG_TSV")
	if [ -z "$remote_ts" ]; then
		# Slug not in catalog (e.g. unversioned django/ruby/jinja).
		# Probe S3 with a single HEAD so we still get conditional skip.
		lm=$(curl -sI --fail --max-time 10 "$url" 2>/dev/null |
			grep -i '^last-modified:' | sed 's/^[^:]*: *//; s/\r$//')
		if [ -n "$lm" ]; then
			remote_ts=$(date -u -d "$lm" +%s 2>/dev/null || date -u -jf "%a, %d %b %Y %H:%M:%S %Z" "$lm" +%s 2>/dev/null)
		fi
	fi
	if [ -z "$remote_ts" ]; then
		# Not on devdocs.io at all. A local stamp means the doc was
		# produced by one of the parsers (parsers/registry.json): keep it.
		if [ -f "$stamp" ]; then
			echo "  LOCAL   $slug (parser-built, skipped)"
		else
			echo "  FAIL    $slug (not found)"
		fi
		return
	fi

	if [ "$FORCE" != true ] && [ -f "$stamp" ]; then
		local_ts=$(stat -c %Y "$stamp" 2>/dev/null || stat -f %m "$stamp" 2>/dev/null)
		if [ -n "$local_ts" ] && [ "$local_ts" -ge "$remote_ts" ]; then
			echo "  SKIP    $slug (up-to-date)"
			return
		fi
	fi

	# Stream the archive straight into tar so extraction overlaps with
	# download — saves both wall time and a tmpfile. pipefail makes
	# curl errors (4xx/5xx, network) fail the whole pipeline, so we
	# don't end up with a half-extracted tree being stamped.
	#
	# The archive ships every page twice: inside db.json and as a loose
	# *.html file. marvindocs reads pages only from db.zip (db.json
	# packed one entry per page, ~4x smaller), so the html copies are
	# skipped and db.json is converted right after extraction.
	mkdir -p "$dir"
	if (
		set -o pipefail
		curl -sSfL --max-time 300 "$url" | tar -xz -C "$dir" --exclude='*.html'
	) 2>/dev/null; then
		if ! "$BINARY" pack --docs-path "$DATA_DIR" "$slug" >/dev/null 2>&1; then
			echo "  FAIL    $slug (pack db.json)"
			return
		fi
		touch -d "@$remote_ts" "$stamp" 2>/dev/null || touch -t "$(date -r "$remote_ts" +%Y%m%d%H%M.%S)" "$stamp"
		echo "  UPDATED $slug"
	else
		echo "  FAIL    $slug (download or extract error)"
	fi
}
export -f download_slug
export DATA_DIR FORCE CATALOG_TSV BINARY

# pack runs inside download_slug, so the binary is needed before the
# first download, not only for the index rebuild.
if [ ! -x "$BINARY" ]; then
	echo "==> binary not found at $BINARY, building..."
	make -C "$REPO_DIR" build
fi

# ---- parallel download ------------------------------------------------------
# Tee xargs output so we can both display progress and count UPDATED lines.
DOWNLOAD_LOG="$(mktemp)"

printf '%s\n' "${SLUGS[@]}" |
	xargs -P "$PARALLEL_JOBS" -I{} bash -c 'download_slug "$@"' _ {} |
	tee "$DOWNLOAD_LOG"

updated_count=$(grep -c '^  UPDATED ' "$DOWNLOAD_LOG" || true)

# ---- rebuild docs.json from meta.json files (pure bash) --------------------
echo ""
if [ "$updated_count" -eq 0 ] && [ -f "$DATA_DIR/docs.json" ]; then
	echo "==> nothing changed, keeping existing docs.json"
	count=$(grep -oE '"slug":[[:space:]]*"[^"]*"' "$DATA_DIR/docs.json" | wc -l | tr -d ' ')
else
	echo "==> rebuilding docs.json from meta.json files..."
	"$SCRIPT_DIR/rebuild-catalog.sh" "$DATA_DIR"
	count=$(grep -oE '"slug":[[:space:]]*"[^"]*"' "$DATA_DIR/docs.json" | wc -l | tr -d ' ')
fi

# ---- rebuild index ----------------------------------------------------------
if [ "$RUN_INDEX" = true ]; then
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
