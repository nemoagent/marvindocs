# Project overview

**marvindocs** is a local documentation service for AI assistants and
developers, an alternative to Context7.

## What it does

It takes the official documentation of popular languages, frameworks and
libraries (Go, React, PostgreSQL, Git, Python and hundreds more), stores it in a
single database on your machine and provides fast search over it.

The main consumer is an AI agent (Claude Code, Cursor and the like):
instead of recalling an API from memory and getting it wrong, the agent
asks marvindocs and receives an up-to-date fragment of the real
documentation.

## Relation to DevDocs

The project is inspired by and built on [DevDocs](https://devdocs.io/), an
open documentation browser for people. Most documentation (837 of 892 sets)
comes from DevDocs packs, and all docs are stored in its format. The rest
is produced by our own parsers: the Go standard library, go.dev, every
Laravel version, the MongoDB Manual, Metasploit.

What marvindocs adds on top of DevDocs:

- an MCP server, so AI agents can use the documentation directly;
- search over page text, not only over titles, by words and by meaning;
- answers as Markdown fragments of a given size instead of whole HTML pages;
- one program for the whole thing (search, MCP, web UI) and ready-made
  bundles with a built index.

marvindocs shares no code with DevDocs and is not affiliated with it.

## How it works

1. **Loading.** You pick the documentation sets you need (about 27 of the
   most common ones by default, or the whole archive). The service
   downloads them and builds a search index.
2. **Search.** Documentation is split into small meaningful pieces, each
   with an "address" (doc → page → section). A query is searched in two
   ways at once: by words (exact terms, function names) and by meaning
   (when the question is phrased in plain language, e.g. "wait for a group
   of goroutines to finish"). The results are merged and the best page is
   chosen.
3. **Answer.** The agent gets a fragment of the documentation of the
   requested size — exactly the part that answers the question, with a
   reference to where it came from.

## How to use it

- As an agent tool (MCP): the agent calls `get_docs` with a topic and a
  documentation name.
- Through the web UI — read and search the documentation in a browser.
- Through HTTP requests from your own scripts.

## Why

- Everything runs offline and locally: no external services, rate limits
  or code leaks.
- A single executable, no GPU required; an ordinary laptop is enough.
- Answers come from official sources rather than the model's memory, so
  fewer invented APIs and outdated examples.
- On an internal test of 96 questions the right page comes first in two
  cases out of three and within the top three in 85 %.

Details: [getting started](getting-started.md), [MCP](mcp.md),
[vector search](vectors.md).
