# marvindocs documentation (English)

- [Metasploit Framework](metasploit.md) — import and update official documentation
- [Project overview](overview.md) — what it is and how it works, no technical details
- [Getting started](getting-started.md) — install, build, first index
- [Syncing docs](sync-docs.md) — download the devdocs data with the helper script
- [Offline bundles](bundles.md) — install prebuilt docs with a ready
  index and embeddings, build and publish bundles
- [Configuration](configuration.md) — paths, flags, directory layout
- [CLI reference](cli.md) — every subcommand and flag
- [MCP server](mcp.md) — stdio + HTTP transports, tool reference,
  verifying it's alive
- [MCP clients](clients.md) — ready-to-paste configs for Claude Desktop,
  Claude Code, Cursor, Windsurf, Zed, Continue, VS Code Copilot Chat
- [Vector search](vectors.md) — chunk embeddings, hybrid search in
  `get_docs`, `index embed`
- [Synonym dictionary](synonyms.md) — query expansion with synonyms,
  builtin dictionary, `--synonyms`
- [Web UI](web.md) — routes, navigation, styling
- [Data format](data-format.md) — authoritative spec of the on-disk
  layout, for writing your own scraper / parser
- [Parsers](parsers.md) — bundled parser binaries that produce the
  data format from custom sources (Laravel docs, internal handbooks)
- [Consistency checks](consistency-checks.md) — scripts that validate
  docs/en <-> docs/ru parity and devdocs dump integrity
- [Go documentation via Docker](go-doc.md) — run `go doc` in a
  container with `scripts/go-doc.sh`
