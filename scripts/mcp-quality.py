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

# search_pages cases — query page bodies (pages_fts), always indexed.
# Query the page-body FTS for things that aren't entry names.
SEARCH_PAGES_CASES = [
    ("PG full-text: jsonb operators", "jsonb operators", "postgresql~17"),
    ("PG full-text: connection pool", "connection pool", "postgresql~17"),
    ("Python full-text: with statement", "context manager with", "python~3.13"),
]

# get_docs cases — token-budgeted doc fragments.
# Each case: (description, topic, doc_slug or None (auto-detect), token budget,
#             expected keyword groups — at least one of each group must appear
#             in the concatenated fragment contents)
GET_DOCS_CASES = [
    (
        "get_docs PG: jsonb containment",
        "jsonb containment operators",
        "postgresql~17",
        1500,
        [["@>"], ["jsonb"]],
    ),
    (
        "get_docs PG: create index concurrently",
        "create index concurrently",
        "postgresql~17",
        2000,
        [["CONCURRENTLY"], ["lock", "index"]],
    ),
    (
        "get_docs Python: asyncio gather",
        "asyncio gather exceptions",
        "python~3.13",
        1500,
        [["gather"], ["return_exceptions", "exception"]],
    ),
    (
        "get_docs Python: dataclass defaults",
        "dataclass field default_factory",
        "python~3.13",
        1500,
        [["default_factory"], ["field"]],
    ),
    (
        "get_docs React: useEffect cleanup",
        "useEffect cleanup function",
        "react",
        1500,
        [["cleanup"], ["useEffect", "effect"]],
    ),
    (
        "get_docs Go: context timeout",
        "context WithTimeout cancel",
        "go",
        1500,
        [["WithTimeout"], ["cancel"]],
    ),
    (
        "get_docs JS: array reduce",
        "array reduce initial value",
        "javascript",
        1500,
        [["reduce"], ["initialValue", "initial"]],
    ),
    (
        "get_docs CSS: grid template areas",
        "grid template areas",
        "css",
        1500,
        [["grid-template-areas"], ["grid"]],
    ),
    (
        "get_docs auto-slug: postgresql vacuum",
        "postgresql vacuum analyze autovacuum",
        None,
        2000,
        [["VACUUM"], ["autovacuum", "ANALYZE"]],
    ),
    (
        "get_docs auto-slug: python logging",
        "python logging handlers formatter",
        None,
        2000,
        [["Formatter", "formatter"], ["Handler", "handler"]],
    ),
    (
        "get_docs tiny budget",
        "jsonb operators",
        "postgresql~17",
        200,
        [["jsonb"]],
    ),
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

# search_pages probes
print("\n" + "=" * 78)
print(" SEARCH_PAGES")
print("=" * 78)
sp_total = len(SEARCH_PAGES_CASES)
sp_found = 0
for desc, query, slug in SEARCH_PAGES_CASES:
    args = {"query": query, "limit": 3}
    if slug:
        args["doc_slug"] = slug
    sr = call(
        "tools/call",
        {"name": "search_pages", "arguments": args},
        f"sp:{desc}",
    )
    hits = sr.get("result", {}).get("structuredContent", {}).get("hits", []) or []
    print(f"\n[{desc}]")
    print(f"  query: {query!r}  scope: {slug or '(all)'}  hits: {len(hits)}")
    if hits:
        sp_found += 1
        for h in hits:
            snip = (h.get("snippet", "") or "")[:120].replace("\n", " ")
            print(f"  bm25={h['score']:.2f}  {h['doc_slug']}/{h['path']}")
            print(f"    {snip}")
print(
    f"\n SUMMARY: {sp_found}/{sp_total} full-text queries returned hits "
    f"(0 means pages_fts is empty - rebuild the index)"
)
print("=" * 78)

# get_docs probes
print("\n" + "=" * 78)
print(" GET_DOCS")
print("=" * 78)
gd_rows = []
for desc, topic, slug, budget, kw_groups in GET_DOCS_CASES:
    args = {"topic": topic, "tokens": budget}
    if slug:
        args["doc_slug"] = slug
    t0 = time.perf_counter()
    gr = call(
        "tools/call",
        {"name": "get_docs", "arguments": args},
        f"gd:{desc}",
    )
    latency_ms = (time.perf_counter() - t0) * 1000
    sc = gr.get("result", {}).get("structuredContent", {}) or {}
    frags = sc.get("fragments", []) or []
    total_tokens = sc.get("tokens", 0) or 0
    within = total_tokens <= budget * 1.05
    sources = sorted({f"{f['doc_slug']}/{f['path']}" for f in frags})
    headings = [f.get("heading", "") for f in frags]
    first = frags[0] if frags else None
    first_snip = (
        (first.get("content", "") or "")[:120].replace("\n", " ") if first else ""
    )

    text_lower = "\n".join(f.get("content", "") or "" for f in frags).lower()
    kw_hits, kw_misses = [], []
    for grp in kw_groups:
        hit_one = next((kw for kw in grp if kw.lower() in text_lower), None)
        if hit_one:
            kw_hits.append(hit_one)
        else:
            kw_misses.append("|".join(grp))

    auto_ok = None
    if slug is None and first:
        prefix = topic.split()[0].lower()
        auto_ok = first["doc_slug"].lower().startswith(prefix)

    print(f"\n[{desc}]")
    print(f"  topic: {topic!r}  scope: {slug or '(auto)'}  budget: {budget}")
    print(
        f"  fragments: {len(frags)}  tokens: {total_tokens}/{budget}  "
        f"{'OK' if within else 'OVER BUDGET'}  latency: {latency_ms:.0f} ms"
    )
    if frags:
        print(f"  sources:  {sources}")
        print(f"  headings: {headings}")
        print(f"  first:    {first_snip}")
        if auto_ok is not None:
            print(
                f"  auto-doc: {first['doc_slug']}  "
                f"{'OK' if auto_ok else 'MISMATCH (expected ' + topic.split()[0].lower() + '*)'}"
            )
        print(f"  found: {kw_hits}")
        if kw_misses:
            print(f"  MISS:  {kw_misses}")
    else:
        print("  (no fragments)")

    gd_rows.append(
        {
            "found": bool(frags),
            "within": within,
            "all_kw": bool(frags) and not kw_misses,
            "latency": latency_ms,
            "n_frags": len(frags),
        }
    )

gd_total = len(gd_rows)
gd_found = sum(1 for r in gd_rows if r["found"])
gd_within = sum(1 for r in gd_rows if r["found"] and r["within"])
gd_kw = sum(1 for r in gd_rows if r["all_kw"])
gd_lat = sum(r["latency"] for r in gd_rows) / gd_total if gd_total else 0
gd_nf = sum(r["n_frags"] for r in gd_rows) / gd_total if gd_total else 0
print(
    f"\n SUMMARY: {gd_total} cases, {gd_found} found, {gd_within} within budget, "
    f"{gd_kw} with all keyword groups, avg latency {gd_lat:.0f} ms, "
    f"avg fragments {gd_nf:.1f}"
)
print("=" * 78)

PROC.stdin.close()
PROC.terminate()
PROC.wait(timeout=2)
