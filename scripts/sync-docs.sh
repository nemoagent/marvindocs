#!/usr/bin/env bash
# scripts/sync-docs.sh — download devdocs documentation into data/public/docs/
#
# Usage:
#   ./scripts/sync-docs.sh                        # download --default (~30 popular docs)
#   ./scripts/sync-docs.sh --all                  # all ~790 docs (~17 GB, takes 1-2 hours)
#   ./scripts/sync-docs.sh react python~3.13      # specific docs only
#   ./scripts/sync-docs.sh --no-index             # skip marvindocs index build after sync
#
# Options:
#   --all          download every available doc (passed to thor docs:download --all)
#   --default      download only the default set (default behavior without explicit slugs)
#   --no-index     do not run marvindocs index build when finished
#   --devdocs-dir  path to the devdocs source tree (default: ../devdocs)
#
# Requirements:
#   - docker
#   - ./bin/marvindocs (run `make build` if missing)

set -euo pipefail

# ---- parse arguments -------------------------------------------------------
DEVDOCS_DIR="$(realpath "${DEVDOCS_DIR:-../devdocs}")"
DOWNLOAD_ARGS="--default"
RUN_INDEX=true
SLUGS=()

while [[ $# -gt 0 ]]; do
	case "$1" in
	--all)
		DOWNLOAD_ARGS="--all"
		shift
		;;
	--default)
		DOWNLOAD_ARGS="--default"
		shift
		;;
	--no-index)
		RUN_INDEX=false
		shift
		;;
	--devdocs-dir)
		DEVDOCS_DIR="$(realpath "$2")"
		shift 2
		;;
	-*)
		echo "Unknown option: $1" >&2
		echo "Usage: $0 [--all | --default | <slug> ...] [--no-index] [--devdocs-dir DIR]" >&2
		exit 2
		;;
	*)
		SLUGS+=("$1")
		shift
		;;
	esac
done

if [[ ${#SLUGS[@]} -gt 0 ]]; then
	DOWNLOAD_ARGS="${SLUGS[*]}"
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DATA_DIR="$REPO_DIR/data/public/docs"
IMAGE="marvindocs-devdocs-sync"
CONTAINER="marvindocs-devdocs-sync-$$"

echo "==> devdocs source: $DEVDOCS_DIR"
echo "==> output:         $DATA_DIR"
echo "==> docs to fetch:  $DOWNLOAD_ARGS"
echo ""

# ---- sanity checks ---------------------------------------------------------
if [[ ! -f "$DEVDOCS_DIR/Dockerfile" ]]; then
	echo "ERROR: $DEVDOCS_DIR/Dockerfile not found." >&2
	echo "       Set --devdocs-dir or clone https://github.com/freeCodeCamp/devdocs" >&2
	exit 1
fi

# ---- build a minimal image (skip the heavy RUN thor docs:download step) ----
# We use a temporary Dockerfile that copies the app WITHOUT running the full
# Dockerfile's download step (which would pull --all by default, ~17 GB).
echo "==> building docker image (deps only, no docs)..."

TMPDF="$(mktemp /tmp/Dockerfile-sync-XXXXXX)"
trap 'rm -f "$TMPDF"' EXIT

cat >"$TMPDF" <<'DOCKERFILE'
FROM ruby:3.4.8
ENV LANG=C.UTF-8
WORKDIR /devdocs

RUN apt-get update && \
    apt-get -y install git nodejs libcurl4 && \
    gem install bundler && \
    rm -rf /var/lib/apt/lists/*

COPY Gemfile Gemfile.lock Rakefile /devdocs/
RUN bundle config set path.system true && \
    bundle install && \
    rm -rf ~/.gem /root/.bundle/cache /usr/local/bundle/cache

COPY . /devdocs
DOCKERFILE

docker build -f "$TMPDF" -t "$IMAGE" "$DEVDOCS_DIR" 2>&1 |
	grep -E "^(Step|Successfully|=>|#[0-9]|ERROR)" || true
echo "    image: $IMAGE"

# ---- run download inside the container -------------------------------------
echo ""
echo "==> downloading docs inside container (thor docs:download $DOWNLOAD_ARGS)..."
echo "    this may take minutes to hours depending on --all vs specific slugs."
echo ""

# Run the container, execute thor, then keep it alive so we can copy.
docker run --name "$CONTAINER" \
	--user "$(id -u):$(id -g)" \
	-e HOME=/tmp \
	-e BUNDLE_PATH=/usr/local/bundle \
	"$IMAGE" \
	bash -c "cd /devdocs && thor docs:download $DOWNLOAD_ARGS"

echo ""
echo "==> copying docs from container to $DATA_DIR ..."

mkdir -p "$DATA_DIR"

# Copy the whole public/docs tree from the container.
docker cp "$CONTAINER:/devdocs/public/docs/." "$DATA_DIR/"

echo "    done. $(ls "$DATA_DIR" | wc -l) slugs now in $DATA_DIR"

# ---- cleanup ---------------------------------------------------------------
echo "==> removing container..."
docker rm "$CONTAINER"

# ---- rebuild index ---------------------------------------------------------
if [[ "$RUN_INDEX" == true ]]; then
	BINARY="$REPO_DIR/bin/marvindocs"
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
echo "  Docs: $DATA_DIR ($(ls "$DATA_DIR" | wc -l | tr -d ' ') slugs)"
if [[ "$RUN_INDEX" == true ]]; then
	echo "  Index: $REPO_DIR/marvindocs.db"
fi
echo ""
echo "Start the server:"
echo "  ./bin/marvindocs serve --http :3333"
