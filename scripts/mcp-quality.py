#!/usr/bin/env python3
"""Quality eval for marvindocs MCP server.

Sends a curated set of realistic developer queries through search +
get_page and rates each result on:
  - found: was a hit returned?
  - top_relevant: does the top hit's name/path look on-topic?
  - page_ok: did get_page on the top hit return non-trivial content?
  - keywords_present: do expected keywords appear in the markdown?

Prints a per-case report and a summary. No assertions — this is for
manual inspection of the corpus quality.
"""

import json, subprocess, time, threading, re, sys

PROC = subprocess.Popen(
    ["./bin/marvindocs", "serve", "--stdio"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    text=True,
    bufsize=1,
)

pending, results = {}, {}


def reader():
    for line in PROC.stdout:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except Exception:
            continue
        rid = msg.get("id")
        if rid in pending:
            label, t0 = pending.pop(rid)
            results[rid] = (label, (time.perf_counter() - t0) * 1000, msg)


threading.Thread(target=reader, daemon=True).start()

NEXT_ID = [10]


def call(method, params=None, label=""):
    rid = NEXT_ID[0]
    NEXT_ID[0] += 1
    req = {"jsonrpc": "2.0", "id": rid, "method": method}
    if params is not None:
        req["params"] = params
    pending[rid] = (label, time.perf_counter())
    PROC.stdin.write(json.dumps(req) + "\n")
    PROC.stdin.flush()
    while rid not in results:
        time.sleep(0.005)
    return results[rid][2]


def notify(method, params=None):
    req = {"jsonrpc": "2.0", "method": method}
    if params is not None:
        req["params"] = params
    PROC.stdin.write(json.dumps(req) + "\n")
    PROC.stdin.flush()


# Handshake
call(
    "initialize",
    {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {"name": "qual", "version": "0"},
    },
    "init",
)
notify("notifications/initialized")

# --- Test cases ----------------------------------------------------------
# Each case: (description, query, doc_slug or None, expected keywords in
#             returned page markdown — at least one of each group must hit)
CASES = [
    # postgresql: schema operations
    (
        "PostgreSQL: create database",
        "create database",
        "postgresql~17",
        [["CREATE DATABASE", "create"], ["OWNER", "TEMPLATE", "ENCODING"]],
    ),
    (
        "PostgreSQL: window functions",
        "window function",
        "postgresql~17",
        [["WINDOW", "OVER"], ["PARTITION", "ROWS", "RANGE"]],
    ),
    (
        "PostgreSQL: jsonb operators",
        "jsonb operators",
        "postgresql~17",
        [["jsonb", "json"], ["->", "->>", "@>", "?"]],
    ),
    # python: stdlib
    (
        "Python: list comprehensions",
        "list comprehension",
        "python~3.13",
        [["comprehension"], ["for", "in"]],
    ),
    (
        "Python: asyncio gather",
        "asyncio gather",
        "python~3.13",
        [["gather", "asyncio"], ["coroutine", "await", "Future"]],
    ),
    # react: hooks
    (
        "React: useEffect cleanup",
        "useEffect",
        "react",
        [["useEffect"], ["cleanup", "effect", "render"]],
    ),
    (
        "React: useState basics",
        "useState",
        "react",
        [["useState"], ["state", "setState", "initial"]],
    ),
    # go: stdlib
    (
        "Go: http server",
        "http handler",
        "go",
        [["Handler", "HandleFunc", "http"], ["ServeHTTP", "Request", "Response"]],
    ),
    (
        "Go: context cancellation",
        "context cancel",
        "go",
        [["Context", "context"], ["cancel", "Done", "deadline"]],
    ),
    # cross-doc (no slug filter)
    ("Cross-doc: 'select' query", "select", None, None),
    # tricky: vague query
    ("Vague: 'how to read file'", "read file", None, None),
]


def find_postgresql_jsonb_path():
    """Help: discover the actual jsonb page slug — devdocs paths vary."""
    pass


# --- Run -----------------------------------------------------------------
report = []
for desc, query, slug, kw_groups in CASES:
    args = {"query": query, "limit": 5}
    if slug:
        args["doc_slug"] = slug
    sr = call("tools/call", {"name": "search", "arguments": args}, f"search:{desc}")
    hits = sr.get("result", {}).get("structuredContent", {}).get("hits", []) or []

    entry = {
        "desc": desc,
        "query": query,
        "slug": slug,
        "n_hits": len(hits),
        "top": None,
        "page_chars": 0,
        "kw_hits": [],
        "kw_misses": [],
    }

    if not hits:
        report.append(entry)
        continue

    top = hits[0]
    entry["top"] = {
        "name": top["name"],
        "path": top["path"],
        "doc_slug": top["doc_slug"],
        "score": top["score"],
    }

    # Pull page for top hit
    if slug:  # only when scoped — cross-doc top hit might be from any doc
        # devdocs paths often include a "#anchor" — get_page wants the
        # page path only, so strip the fragment.
        page_path = top["path"].split("#", 1)[0]
        pr = call(
            "tools/call",
            {
                "name": "get_page",
                "arguments": {
                    "doc_slug": top["doc_slug"],
                    "path": page_path,
                    "format": "markdown",
                },
            },
            f"page:{desc}",
        )
        md = pr.get("result", {}).get("structuredContent", {}).get("content", "") or ""
        entry["page_chars"] = len(md)

        if kw_groups:
            md_lower = md.lower()
            for grp in kw_groups:
                hit_one = next((kw for kw in grp if kw.lower() in md_lower), None)
                if hit_one:
                    entry["kw_hits"].append(hit_one)
                else:
                    entry["kw_misses"].append("|".join(grp))

    report.append(entry)

# --- Print ---------------------------------------------------------------
print("\n" + "=" * 78)
print(" MARVINDOCS DATA QUALITY EVAL")
print("=" * 78)
for e in report:
    print(f"\n[{e['desc']}]")
    print(f"  query: {e['query']!r}  scope: {e['slug'] or '(all docs)'}")
    print(f"  hits:  {e['n_hits']}")
    if e["top"]:
        t = e["top"]
        print(
            f"  top:   {t['doc_slug']}/{t['path']}  ({t['name']!r}, bm25={t['score']:.2f})"
        )
    if e["page_chars"]:
        print(f"  page:  {e['page_chars']} chars markdown")
        if e["kw_hits"] or e["kw_misses"]:
            print(f"  found: {e['kw_hits']}")
            if e["kw_misses"]:
                print(f"  MISS:  {e['kw_misses']}")

# Summary
total = len(report)
found = sum(1 for e in report if e["n_hits"] > 0)
with_page = sum(1 for e in report if e["page_chars"] > 200)
all_kw = sum(
    1 for e in report if e["page_chars"] and not e["kw_misses"] and e["kw_hits"]
)
print("\n" + "=" * 78)
print(
    f" SUMMARY: {found}/{total} returned hits, "
    f"{with_page} pages fetched, {all_kw} pages had all expected keyword groups"
)
print("=" * 78)

PROC.stdin.close()
PROC.terminate()
PROC.wait(timeout=2)
