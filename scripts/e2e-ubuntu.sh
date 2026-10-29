#!/usr/bin/env bash
# End-to-end check of a release on a clean Ubuntu 24.04 in docker:
# install the .deb, install a bundle, search from the CLI, start the server
# and call the web UI and MCP over HTTP. Fails on the first broken step.
#
#   scripts/e2e-ubuntu.sh [VERSION]       released .deb, default: newest git tag
#   scripts/e2e-ubuntu.sh --deb FILE      local .deb (scripts/release-package.sh)
#
# Environment:
#   BUNDLE        bundle to install (default db)
#   EMBEDDER_URL  OpenAI-compatible embeddings URL with bge-m3 for hybrid
#                 search, e.g. http://host:8001/v1; BM25 only when unset
#   IMAGE         docker image (default ubuntu:24.04)
set -euo pipefail

if [[ ${1:-} != --inside ]]; then
	deb="" version=""
	if [[ ${1:-} == --deb ]]; then
		deb=$(realpath "${2:?--deb needs a file}")
	else
		version=${1:-$(git describe --tags --abbrev=0)}
	fi
	args=(--rm -e "RELEASE=$version" -e "BUNDLE=${BUNDLE:-db}" -e "EMBEDDER_URL=${EMBEDDER_URL:-}"
		-v "$(realpath "$0"):/e2e.sh:ro")
	[[ -n $deb ]] && args+=(-v "$deb:/pkg/$(basename "$deb"):ro")
	exec docker run "${args[@]}" "${IMAGE:-ubuntu:24.04}" bash /e2e.sh --inside
fi

step() { printf '\n=== %s\n' "$*"; }
fail() {
	echo "FAIL: $*" >&2
	exit 1
}
# nonempty CMD... runs CMD, fails when it prints nothing to stdout and
# shows the first lines.
nonempty() {
	local out
	out=$("$@")
	[[ -n $out ]] || fail "no output: $*"
	head -10 <<<"$out"
}

step "prerequisites"
apt-get update -qq >/dev/null
DEBIAN_FRONTEND=noninteractive apt-get install -yqq curl ca-certificates jq >/dev/null
. /etc/os-release
echo "$PRETTY_NAME"

step "package"
if [[ -n $RELEASE ]]; then
	url=https://github.com/nemoagent/marvindocs/releases/download/$RELEASE
	deb=marvindocs_${RELEASE#v}_$(dpkg --print-architecture).deb
	cd /tmp
	curl -fsSLO "$url/$deb" -fsSLO "$url/SHA256SUMS"
	sha256sum -c --ignore-missing SHA256SUMS
	deb=/tmp/$deb
else
	deb=$(ls /pkg/*.deb)
fi
DEBIAN_FRONTEND=noninteractive apt-get install -yqq "$deb" >/dev/null
marvindocs version
if [[ -n $RELEASE ]]; then
	[[ $(marvindocs version) == *"$RELEASE"* ]] || fail "version is not $RELEASE"
fi
# The docker image skips /usr/share/doc on install: check the package list.
dpkg -L marvindocs | grep -q /usr/share/doc/marvindocs/docs/en/cli.md || fail "no docs in the package"

step "bundle install $BUNDLE"
mkdir -p /srv/md && cd /srv/md
time marvindocs bundle install "$BUNDLE"
du -sh data marvindocs.db
nonempty marvindocs list

if [[ $BUNDLE == db ]]; then
	step "CLI search"
	nonempty marvindocs search "CREATE INDEX" --limit 3
	nonempty marvindocs search "CREATE INDEX" --doc postgresql --limit 3
	nonempty marvindocs search "ZADD sorted set" --limit 3
	nonempty marvindocs search "upsert on conflict" --doc sqlite --body --limit 3

	step "get page"
	p=$(marvindocs search "CREATE INDEX" --doc sqlite --limit 1 --json | jq -r '.[0].path')
	nonempty marvindocs get sqlite "$p" --format markdown
fi

step "serve: web and MCP"
emb=()
if [[ -n $EMBEDDER_URL ]]; then
	emb=(--embedder openai --openai-url "$EMBEDDER_URL")
	echo "hybrid search via $EMBEDDER_URL"
else
	echo "BM25 only (EMBEDDER_URL is not set)"
fi
marvindocs serve --http 127.0.0.1:3333 --insecure "${emb[@]}" >/tmp/serve.log 2>&1 &
for _ in $(seq 30); do
	curl -fs -o /dev/null http://127.0.0.1:3333/ && break
	sleep 1
done
code=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3333/)
[[ $code == 200 ]] || fail "web UI returned $code: $(tail -5 /tmp/serve.log)"
echo "web: $code"

H=(-H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream')
sid=$(curl -s -D - -o /dev/null "${H[@]}" http://127.0.0.1:3333/mcp -d \
	'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}' |
	awk -F': ' 'tolower($1)=="mcp-session-id"{print $2}' | tr -d '\r')
[[ -n $sid ]] || fail "MCP initialize returned no session"
S=(-H "Mcp-Session-Id: $sid")
curl -s -o /dev/null "${H[@]}" "${S[@]}" http://127.0.0.1:3333/mcp -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'
mcp() { curl -s "${H[@]}" "${S[@]}" http://127.0.0.1:3333/mcp -d "$1" | sed -n 's/^data: //p' | tail -1; }

tools=$(mcp '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | jq -r '[.result.tools[].name]|join(", ")')
echo "tools: $tools"
[[ $tools == *get_docs* ]] || fail "MCP has no get_docs tool"
topic="create an index"
[[ $BUNDLE == db ]] && topic="how to make a redis key expire after some seconds"
echo "--- get_docs: $topic"
mcp "$(jq -nc --arg q "$topic" '{jsonrpc:"2.0",id:3,method:"tools/call",params:{name:"get_docs",arguments:{topic:$q,tokens:600}}}')" |
	jq -r '.result.content[0].text' | grep -E '^(#|Source:)' | head -6 | grep . || fail "get_docs returned nothing"

step "OK"
