#!/usr/bin/env bash
# scripts/mcp-smoke.sh - end-to-end check of the MCP server over HTTP.
#
# Usage:
#   ./scripts/mcp-smoke.sh                       # default http://localhost:3333/mcp
#   MCP_URL=http://host:3333/mcp ./scripts/mcp-smoke.sh
#
# Requires curl and jq.

set -euo pipefail

MCP_URL="${MCP_URL:-http://localhost:3333/mcp}"
HDR_CT='Content-Type: application/json'
HDR_ACC='Accept: application/json, text/event-stream'

command -v jq >/dev/null || { echo "jq not installed" >&2; exit 1; }

# --- 1. initialize + capture Mcp-Session-Id from response headers ----------
echo "==> initialize $MCP_URL"
SID=$(curl -sS -i -X POST "$MCP_URL" -H "$HDR_CT" -H "$HDR_ACC" -d '{
  "jsonrpc":"2.0","id":1,"method":"initialize",
  "params":{"protocolVersion":"2025-06-18","capabilities":{},
            "clientInfo":{"name":"mcp-smoke","version":"0"}}}' \
  | awk -F': ' 'tolower($1)=="mcp-session-id"{print $2}' | tr -d '\r\n')

if [[ -z "$SID" ]]; then
  echo "no Mcp-Session-Id returned; is the server running?" >&2
  exit 1
fi
echo "    session=$SID"

# call METHOD [PARAMS_JSON] -> JSON result on stdout
call() {
  local method="$1" params="${2:-}"
  local id=$((RANDOM))
  local payload
  if [[ -n "$params" ]]; then
    payload=$(printf '{"jsonrpc":"2.0","id":%d,"method":"%s","params":%s}' "$id" "$method" "$params")
  else
    payload=$(printf '{"jsonrpc":"2.0","id":%d,"method":"%s"}' "$id" "$method")
  fi
  curl -sS -X POST "$MCP_URL" -H "$HDR_CT" -H "$HDR_ACC" -H "Mcp-Session-Id: $SID" -d "$payload" \
    | sed -n 's/^data: //p'
}

notify() {
  curl -sS -o /dev/null -X POST "$MCP_URL" -H "$HDR_CT" -H "$HDR_ACC" -H "Mcp-Session-Id: $SID" -d "$1"
}

notify '{"jsonrpc":"2.0","method":"notifications/initialized"}'

# --- 2. tools/list --------------------------------------------------------
echo "==> tools/list"
call tools/list | jq -r '.result.tools[] | "    - \(.name): \(.description[0:70])"'

# --- 3. list_docs ---------------------------------------------------------
echo "==> tools/call list_docs (first 3)"
call tools/call '{"name":"list_docs","arguments":{}}' \
  | jq -r '.result.structuredContent.docs | length as $n
           | "    \($n) docs, first 3:",
             (.[0:3][] | "      \(.slug)\t\(.name)")'

# --- 4. search ------------------------------------------------------------
echo "==> tools/call search query=useState limit=3"
call tools/call '{"name":"search","arguments":{"query":"useState","limit":3}}' \
  | jq -r '.result.structuredContent.hits[]
           | "    \(.doc_slug)\t\(.name)\t\(.path)"'

# --- 5. get_page (markdown) -----------------------------------------------
echo "==> tools/call get_page react reference/react/usestate (markdown)"
call tools/call '{"name":"get_page","arguments":{"doc_slug":"react","path":"reference/react/usestate","format":"markdown"}}' \
  | jq -r '.result.structuredContent.content
           | "    length=\(length) chars",
             "    head: \(.[0:160] | gsub("\n";" "))"'

echo "OK"
