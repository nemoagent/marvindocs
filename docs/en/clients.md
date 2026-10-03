# MCP client configuration

This page shows how to wire up marvindocs into the popular AI agents.
All examples assume you already have:

```bash
marvindocs index build                   # created marvindocs.db
```

Two rules that apply to every stdio config below:

1. **Use absolute paths.** Most agents do not run in your repo and won't
   resolve `./bin/marvindocs`.
2. **Pass `--docs-path` and `--index-path` explicitly** if you don't want
   to run the binary from the repo root.

The minimal command you want to pass is:

```
/ABS/marvindocs serve --stdio \
  --docs-path  /ABS/data/public/docs \
  --index-path /ABS/marvindocs.db
```

Once connected, every client gets four tools: `list_docs`, `search`,
`search_pages`, `get_page`.

---

## Claude Desktop (stdio)

Config file location:

| OS | Path |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Linux | `~/.config/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

```json
{
  "mcpServers": {
    "marvindocs": {
      "command": "/ABS/marvindocs",
      "args": [
        "serve", "--stdio",
        "--docs-path",  "/ABS/data/public/docs",
        "--index-path", "/ABS/marvindocs.db"
      ]
    }
  }
}
```

Restart Claude Desktop. Open any chat; four tools should appear in the
tool picker.

---

## Claude Code (CLI)

Claude Code has a built-in `mcp add` command — no manual JSON editing.

```bash
claude mcp add marvindocs \
  /ABS/marvindocs serve --stdio \
  --docs-path  /ABS/data/public/docs \
  --index-path /ABS/marvindocs.db
```

Verify:

```bash
claude mcp list
# marvindocs   stdio   /ABS/marvindocs ...
```

Start a session; Claude Code will auto-list the tools on first use.

---

## Cursor (stdio)

Config file: `~/.cursor/mcp.json` (global) or `<project>/.cursor/mcp.json`
(per-project).

```json
{
  "mcpServers": {
    "marvindocs": {
      "command": "/ABS/marvindocs",
      "args": [
        "serve", "--stdio",
        "--docs-path",  "/ABS/data/public/docs",
        "--index-path", "/ABS/marvindocs.db"
      ]
    }
  }
}
```

Open **Settings → Cursor Settings → MCP** to confirm marvindocs is
listed and healthy.

---

## Windsurf (stdio)

Config file: `~/.codeium/windsurf/mcp_config.json`.

```json
{
  "mcpServers": {
    "marvindocs": {
      "command": "/ABS/marvindocs",
      "args": [
        "serve", "--stdio",
        "--docs-path",  "/ABS/data/public/docs",
        "--index-path", "/ABS/marvindocs.db"
      ]
    }
  }
}
```

Cascade reloads the file on save. Reference:
<https://docs.windsurf.com/windsurf/cascade/mcp>.

---

## Zed (stdio)

Zed supports MCP via "context servers". Open **Agent Panel → Settings →
Context Servers → Add Custom Server** and fill:

- Name: `marvindocs`
- Command: `/ABS/marvindocs`
- Arguments: `serve`, `--stdio`, `--docs-path`, `/ABS/data/public/docs`,
  `--index-path`, `/ABS/marvindocs.db`

Reference: <https://zed.dev/docs/ai/mcp>. The exact JSON shape in
`~/.config/zed/settings.json` has changed between Zed releases, so the
UI is the safest path.

---

## Continue — VS Code / JetBrains (stdio)

Continue stores its config in `~/.continue/config.yaml` (newer) or
`~/.continue/config.json` (older). YAML form:

```yaml
mcpServers:
  - name: marvindocs
    command: /ABS/marvindocs
    args:
      - serve
      - --stdio
      - --docs-path
      - /ABS/data/public/docs
      - --index-path
      - /ABS/marvindocs.db
```

Reload Continue (command palette → **Continue: Reload Window**). The
tools appear under the MCP section of the tool list.

---

## VS Code / GitHub Copilot Chat (stdio or HTTP)

VS Code (1.95+) with GitHub Copilot Chat reads `.vscode/mcp.json` in the
workspace, or the user-level `settings.json`.

### stdio

```json
{
  "servers": {
    "marvindocs": {
      "type": "stdio",
      "command": "/ABS/marvindocs",
      "args": [
        "serve", "--stdio",
        "--docs-path",  "/ABS/data/public/docs",
        "--index-path", "/ABS/marvindocs.db"
      ]
    }
  }
}
```

### HTTP (remote or shared instance)

Start the server (bare `:3333` binds loopback only; use
`0.0.0.0:3333` to accept remote connections):

```bash
marvindocs serve --http 0.0.0.0:3333
```

Then in `.vscode/mcp.json` (replace `localhost` with the server's host
for a remote instance):

```json
{
  "servers": {
    "marvindocs": {
      "type": "http",
      "url": "http://localhost:3333/mcp"
    }
  }
}
```

If the server runs with `--auth-token`, HTTP clients must send
`Authorization: Bearer <token>`; configure your client to send that
header.

Reference: <https://code.visualstudio.com/docs/copilot/chat/mcp-servers>.

---

## Generic HTTP client

Any MCP client that speaks the Streamable HTTP transport can connect to
`http://<host>:3333/mcp`. Minimal Go example using the official SDK:

```go
transport := &mcp.StreamableClientTransport{
    Endpoint: "http://localhost:3333/mcp",
}
session, _ := client.Connect(ctx, transport, nil)
```

To verify a running server without any client library, run
`scripts/mcp-smoke.sh` (curl + jq) — see [mcp.md](mcp.md#verifying-the-server-is-alive).

---

## One marvindocs instance for many clients

marvindocs is read-only and holds no per-client state. The most
practical layout is:

1. Run **one** HTTP server on the machine where the data lives:
   `marvindocs serve --http 0.0.0.0:3333` (bare `:3333` is
   loopback-only). Consider `--auth-token` when it is reachable by others.
2. Point every client at `http://host:3333/mcp`.

stdio configs spawn a private process per client and load the SQLite
index into each one — fine for one or two editors, wasteful beyond that.

## Troubleshooting a new client

- Run `marvindocs serve --stdio` by hand in a terminal; type **Ctrl-D**
  to close stdin. If the process errors out, fix that first — the
  client will hit the same error.
- Use absolute paths. Relative `./bin/marvindocs` breaks inside the
  client's own working directory.
- Check the client's MCP log. Claude Desktop: `~/Library/Logs/Claude/mcp*.log`
  (macOS). Cursor / VS Code: **Output** panel, channel "MCP".
- First tool call is slow on cold SQLite? That is the OS page cache
  warming up; subsequent calls are instant.
