#!/usr/bin/env python3
"""MCP stdio bench: send JSON-RPC requests over stdio, measure per-call
round-trip time. Uses postgresql~17 from the indexed dump as the test
target; needs only python3 and a built ./bin/marvindocs.

Run:  python3 scripts/mcp-bench.py
"""

import json, subprocess, time, sys, threading

proc = subprocess.Popen(
    ["./bin/marvindocs", "serve", "--stdio"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    text=True,
    bufsize=1,
)

pending = {}  # id -> (label, send_time)
results = {}  # id -> (label, elapsed_ms)
done_event = threading.Event()
EXPECTED = 0  # set later


def reader():
    for line in proc.stdout:
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
        if EXPECTED and len(results) >= EXPECTED:
            done_event.set()


t = threading.Thread(target=reader, daemon=True)
t.start()


def send(req, label=None):
    line = json.dumps(req)
    proc.stdin.write(line + "\n")
    proc.stdin.flush()
    if "id" in req:
        pending[req["id"]] = (label or req.get("method"), time.perf_counter())


# warm-up handshake
send(
    {
        "jsonrpc": "2.0",
        "id": 0,
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "bench", "version": "0"},
        },
    },
    "initialize",
)
send({"jsonrpc": "2.0", "method": "notifications/initialized"})
time.sleep(0.2)

# Benchmark suite. Mixing cold+warm:
suite = [
    (
        1,
        "search 'create database'",
        {"name": "search", "arguments": {"query": "create database", "limit": 10}},
    ),
    (
        2,
        "search scoped pg17 select",
        {
            "name": "search",
            "arguments": {"query": "select", "doc_slug": "postgresql~17", "limit": 20},
        },
    ),
    (3, "list_docs", {"name": "list_docs", "arguments": {}}),
    (
        4,
        "get_page md (cold)",
        {
            "name": "get_page",
            "arguments": {
                "doc_slug": "postgresql~17",
                "path": "sql-createdatabase",
                "format": "markdown",
            },
        },
    ),
    (
        5,
        "get_page md (warm)",
        {
            "name": "get_page",
            "arguments": {
                "doc_slug": "postgresql~17",
                "path": "sql-createdatabase",
                "format": "markdown",
            },
        },
    ),
    (
        6,
        "get_page md (warm)",
        {
            "name": "get_page",
            "arguments": {
                "doc_slug": "postgresql~17",
                "path": "sql-createdatabase",
                "format": "markdown",
            },
        },
    ),
    (
        7,
        "get_page html",
        {
            "name": "get_page",
            "arguments": {
                "doc_slug": "postgresql~17",
                "path": "sql-createdatabase",
                "format": "html",
            },
        },
    ),
    (
        8,
        "get_page md (other page cold)",
        {
            "name": "get_page",
            "arguments": {
                "doc_slug": "postgresql~17",
                "path": "sql-createtable",
                "format": "markdown",
            },
        },
    ),
    (
        9,
        "get_page md (other page warm)",
        {
            "name": "get_page",
            "arguments": {
                "doc_slug": "postgresql~17",
                "path": "sql-createtable",
                "format": "markdown",
            },
        },
    ),
]
EXPECTED = len(suite) + 1  # + initialize

for rid, label, params in suite:
    send({"jsonrpc": "2.0", "id": rid, "method": "tools/call", "params": params}, label)
    # Wait for this id before sending the next one so cache hits actually
    # show up (the server handles concurrent tools/call in parallel).
    while rid not in results:
        time.sleep(0.005)

done_event.wait(timeout=30)
proc.stdin.close()
proc.terminate()
proc.wait(timeout=2)

print(f"{'id':>3}  {'ms':>8}   label")
for rid in sorted(results):
    label, ms, _ = results[rid]
    print(f"{rid:>3}  {ms:>8.2f}   {label}")
