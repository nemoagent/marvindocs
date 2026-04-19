# MCP-сервер

marvindocs реализует [Model Context Protocol](https://modelcontextprotocol.io/)
через официальный SDK [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk).

## Транспорты

| Транспорт | Флаг | Endpoint |
|---|---|---|
| stdio | `--stdio` | stdin / stdout |
| Streamable HTTP | `--http ADDR` | `<addr>/mcp` |

Проба живости — `<addr>/healthz` (при включённом HTTP).

## Инструменты (tools)

### `list_docs`

Возвращает список всех проиндексированных наборов документации.

- **Вход:** `{}`
- **Выход:** `{ docs: [{ slug, name, type, version, release, mtime }] }`

Используй первым, чтобы узнать доступные slug'и; любой `slug` можно
передать в `search` или `get_page` как `doc_slug`.

### `search`

Полнотекстовый поиск по именам и путям записей, ранжирование по
SQLite FTS5 BM25.

- **Вход:**
  - `query` *(string, обязательный)* — запрос пользователя. Токены
    матчатся с префиксом (`use` найдёт `useState`). Спецсимволы FTS
    экранируются, так что любой ввод безопасен.
  - `doc_slug` *(string, опциональный)* — ограничить одним документом.
  - `limit` *(int, опциональный)* — по умолчанию `20`, максимум `100`.
- **Выход:**
  `{ hits: [{ doc_slug, name, path, type, score }] }`. Чем меньше
  `score`, тем лучше совпадение.

Пример (псевдо-JSON):

```json
{"name": "search", "arguments": {"query": "useState", "limit": 3}}
```

```json
{
  "hits": [
    {"doc_slug": "react", "name": "useState", "path": "reference/react/useState", "type": "Hooks", "score": -7.51}
  ]
}
```

### `get_page`

Возвращает одну страницу документации.

- **Вход:**
  - `doc_slug` *(string, обязательный)*
  - `path` *(string, обязательный)* — значение `search.path` или из
    `index.json`.
  - `format` *(string, опциональный)* — `markdown` *(по умолчанию)* или
    `html`.
- **Выход:**
  `{ doc_slug, path, format, content }`. При `format=markdown` контент
  конвертируется на лету из HTML.

## Настройка клиентов

### Claude Desktop / Claude Code (stdio)

Добавить в `~/.config/claude/claude_desktop_config.json` (или
аналогичный файл на твоей ОС):

```json
{
  "mcpServers": {
    "marvindocs": {
      "command": "/absolute/path/to/marvindocs",
      "args": ["serve", "--stdio",
               "--docs-path", "/absolute/path/to/data/public/docs",
               "--index-path", "/absolute/path/to/marvindocs.db"]
    }
  }
}
```

Перезапустить клиент; должны появиться три инструмента: `list_docs`,
`search`, `get_page`.

### Cursor (stdio)

Cursor читает такой же формат из `mcp.json`:

```json
{
  "mcpServers": {
    "marvindocs": {
      "command": "marvindocs",
      "args": ["serve", "--stdio"]
    }
  }
}
```

### Удалённые агенты (HTTP)

Поднять HTTP-сервер в доступном месте:

```bash
marvindocs serve --http :3333
```

Указать клиенту адрес `http://<host>:3333/mcp`. Пример подключения
официальным Go-клиентом:

```go
transport := &mcp.StreamableClientTransport{
    Endpoint: "http://localhost:3333/mcp",
}
session, _ := client.Connect(ctx, transport, nil)
```

## Безопасность

Сервер **не делает аутентификации**. Если выставляешь `--http` на
публичный адрес — ставь перед ним reverse-proxy, который терминирует
TLS и занимается авторизацией (nginx + basic auth, OAuth-шлюз и т.д.).

Клиенты могут только читать данные — инструменты никогда не пишут в
индекс и не меняют дамп devdocs.

## Диагностика

| Симптом | Возможная причина |
|---|---|
| `index file not found` | Сначала запусти `marvindocs index build`. |
| `open docs.json: no such file` | Неправильный `--docs-path`; ожидается `<path>/docs.json`. |
| Tool возвращает пустой `hits` | Нет префиксных совпадений или этот док не в индексе. Сначала `list_docs`. |
| `get_page` возвращает «page not found» | `path` должен быть значением `search.path` или из `index.json`, а не URL'ом. |
