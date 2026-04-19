# Настройка клиентов MCP

На этой странице — как подключить marvindocs к популярным AI-агентам.
Во всех примерах предполагается, что уже сделано:

```bash
marvindocs index build                   # создан marvindocs.db
```

Два правила, применимых ко всем stdio-конфигам ниже:

1. **Используй абсолютные пути.** Большинство агентов стартует не из
   твоего репозитория и не найдёт `./bin/marvindocs`.
2. **Явно указывай `--docs-path` и `--index-path`**, если не
   запускаешь бинарь из корня репозитория.

Минимальная команда, которую надо прокинуть:

```
/ABS/marvindocs serve --stdio \
  --docs-path  /ABS/data/public/docs \
  --index-path /ABS/marvindocs.db
```

После подключения клиент получит три инструмента: `list_docs`,
`search`, `get_page`.

---

## Claude Desktop (stdio)

Путь к файлу конфига:

| ОС | Путь |
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

Перезапусти Claude Desktop. Открой любой чат — три инструмента должны
появиться в списке.

---

## Claude Code (CLI)

У Claude Code есть встроенная команда `mcp add` — JSON руками править
не надо.

```bash
claude mcp add marvindocs \
  /ABS/marvindocs serve --stdio \
  --docs-path  /ABS/data/public/docs \
  --index-path /ABS/marvindocs.db
```

Проверить:

```bash
claude mcp list
# marvindocs   stdio   /ABS/marvindocs ...
```

Запусти сессию — Claude Code сам подгрузит список tool'ов при первом
обращении.

---

## Cursor (stdio)

Файл конфига: `~/.cursor/mcp.json` (глобальный) либо
`<project>/.cursor/mcp.json` (на уровне проекта).

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

Открой **Settings → Cursor Settings → MCP** — marvindocs должен быть в
списке и светиться «здоровым».

---

## Windsurf (stdio)

Файл конфига: `~/.codeium/windsurf/mcp_config.json`.

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

Cascade перечитывает файл при сохранении. Справка:
<https://docs.windsurf.com/windsurf/cascade/mcp>.

---

## Zed (stdio)

Zed поддерживает MCP через «context servers». Открой **Agent Panel →
Settings → Context Servers → Add Custom Server** и заполни:

- Name: `marvindocs`
- Command: `/ABS/marvindocs`
- Arguments: `serve`, `--stdio`, `--docs-path`, `/ABS/data/public/docs`,
  `--index-path`, `/ABS/marvindocs.db`

Справка: <https://zed.dev/docs/ai/mcp>. Точный JSON-формат в
`~/.config/zed/settings.json` менялся между релизами Zed, поэтому
через UI — надёжнее.

---

## Continue — VS Code / JetBrains (stdio)

Continue хранит конфиг в `~/.continue/config.yaml` (свежие версии) либо
`~/.continue/config.json` (старые). В формате YAML:

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

Перезапусти Continue (command palette → **Continue: Reload Window**).
Инструменты появятся в разделе MCP.

---

## VS Code / GitHub Copilot Chat (stdio и HTTP)

VS Code (1.95+) с GitHub Copilot Chat читает `.vscode/mcp.json` в
рабочей директории или пользовательский `settings.json`.

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

### HTTP (удалённый или общий инстанс)

Подними сервер:

```bash
marvindocs serve --http :3333
```

В `.vscode/mcp.json`:

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

Справка: <https://code.visualstudio.com/docs/copilot/chat/mcp-servers>.

---

## Произвольный HTTP-клиент

Любой MCP-клиент, умеющий Streamable HTTP, может подключиться к
`http://<host>:3333/mcp`. Минимальный пример на Go с официальным SDK:

```go
transport := &mcp.StreamableClientTransport{
    Endpoint: "http://localhost:3333/mcp",
}
session, _ := client.Connect(ctx, transport, nil)
```

Чтобы проверить сервер без клиента — `scripts/mcp-smoke.sh` (curl + jq),
см. [mcp.md](mcp.md#как-проверить-что-сервер-жив).

---

## Один инстанс marvindocs на много клиентов

marvindocs работает только на чтение и не хранит клиентского состояния.
Практичная схема:

1. На машине с данными подними **один** HTTP-сервер:
   `marvindocs serve --http :3333`.
2. Все клиенты указывают на `http://host:3333/mcp`.

stdio-конфиги порождают отдельный процесс на каждого клиента и каждый
тянет SQLite-индекс в память — нормально для одного-двух редакторов,
расточительно дальше.

## Диагностика подключения нового клиента

- Запусти `marvindocs serve --stdio` руками в терминале; нажми **Ctrl-D**,
  чтобы закрыть stdin. Если процесс падает — сначала чини это, клиент
  упрётся в ту же ошибку.
- Используй абсолютные пути. Относительный `./bin/marvindocs` ломается
  в рабочем каталоге клиента.
- Смотри MCP-лог клиента. Claude Desktop: `~/Library/Logs/Claude/mcp*.log`
  (macOS). Cursor / VS Code: панель **Output**, канал «MCP».
- Первый вызов инструмента тормозит? Это прогрев страничного кэша
  SQLite; следующие — мгновенные.
