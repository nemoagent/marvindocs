# Project overview

**marvindocs** is a local documentation service for AI assistants and
developers, an alternative to Context7.

## What it does

It takes the official documentation of popular languages, frameworks and
libraries (Go, React, PostgreSQL, Git, Python and hundreds more, sourced
from [DevDocs](https://devdocs.io/)), stores it in a single database on
your machine and provides fast search over it.

The main consumer is an AI agent (Claude Code, Cursor and the like):
instead of recalling an API from memory and getting it wrong, the agent
asks marvindocs and receives an up-to-date fragment of the real
documentation.

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
