# MCP-сервер

marvindocs реализует [Model Context Protocol](https://modelcontextprotocol.io/)
через официальный SDK [`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk).

## Транспорты

| Транспорт | Флаг | Endpoint |
|---|---|---|
| stdio | `--stdio` | stdin / stdout |
| Streamable HTTP | `--http ADDR` | `<addr>/mcp` |

Проба живости — `<addr>/healthz` (при включённом HTTP).

Адрес только с портом, например `--http :3333`, переписывается в
`127.0.0.1:3333` (только loopback). Чтобы слушать на всех интерфейсах,
укажи хост явно: `--http 0.0.0.0:3333`.

## Инструменты (tools)

### `list_docs`

Возвращает список всех проиндексированных наборов документации.

- **Вход:** `{}`
- **Выход:** `{ docs: [{ slug, name, type, version, release, mtime }] }`

Используй первым, чтобы узнать доступные slug'и; любой `slug` можно
передать в `search` или `get_page` как `doc_slug`.

### `search`

Полнотекстовый поиск по именам и путям записей, ранжирование по
SQLite FTS5 BM25; `name` имеет в 2 раза больший вес, чем `path`.
Когда doc-фильтр не активен (ни явный, ни определённый автоматически),
результаты из «популярных» документов (стандартный набор sync:
postgresql, python, react, go, ...) получают небольшой буст —
тайбрейкер для cross-doc запросов. Если FTS вернул меньше `limit/2`
результатов, выдача дополняется подстрочным (`LIKE`) поиском по именам
записей.

- **Вход:**
  - `query` *(string, обязательный)* — запрос пользователя. Токены
    матчатся с префиксом (`use` найдёт `useState`). Спецсимволы FTS
    экранируются, так что любой ввод безопасен.
  - `doc_slug` *(string, опциональный)* — ограничить одним документом.
    Должен быть точным slug'ом (см. `list_docs`). Если не задан, токены
    запроса без учёта регистра проверяются на совпадение с известными
    slug'ами (и их префиксами до `~`/`-`): первый совпавший токен
    удаляется из запроса, а поиск автоматически ограничивается этим
    доком или группой (например `python~*`). Алиасы сопоставляются с
    каноническими именами: `js` → javascript, `py` → python,
    `ts` → typescript, `pg` → postgresql, `mongo` → mongodb. Если токен
    дока — единственный в запросе, фильтр не применяется. Если поиск с
    фильтром ничего не нашёл (например `async function`, где `async` —
    тоже slug дока), исходный запрос ищется глобально.
    Пример: `{"query": "python requests"}` → поиск только по
    python-документации.
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

### `search_pages`

Полнотекстовый поиск по **тексту страниц** (а не только именам
записей). Используй, когда `search` возвращает ноль результатов из-за
того, что запрос не описывает имя сущности — `jsonb operators`,
`connection pool size`, `context manager with statement`.

Требует индекса, собранного с `--fulltext`; на обычном индексе
возвращает `{ hits: [] }`.

- **Вход:** такая же форма, как у `search`; авто-определение doc-фильтра
  по токенам запроса работает так же. Буст популярности здесь тоже
  применяется, и тоже только без активного doc-фильтра.
- **Выход:** `{ hits: [{ doc_slug, path, snippet, score }] }`.
  `snippet` — короткий фрагмент вокруг совпадения (~16 токенов), где
  совпавшие термы обёрнуты в `[ ]`.

### `get_page`

Возвращает одну страницу документации.

- **Вход:**
  - `doc_slug` *(string, обязательный)*
  - `path` *(string, обязательный)* — значение `search.path` или из
    `index.json`.
  - `format` *(string, опциональный)* — `markdown` *(по умолчанию;
    алиас `md`)* или `html`, без учёта регистра. Любое другое значение —
    ошибка `unknown format`.
- **Выход:**
  `{ doc_slug, path, format, content }`. При `format=markdown` контент
  конвертируется на лету из HTML.

## Настройка клиентов

Готовые к копированию конфиги для всех популярных агентов — в
**[clients.md](clients.md)**:

- Claude Desktop (stdio)
- Claude Code (stdio, через `claude mcp add`)
- Cursor (stdio)
- Windsurf (stdio)
- Zed (stdio)
- Continue — VS Code / JetBrains (stdio)
- VS Code / GitHub Copilot Chat (stdio и HTTP)
- Универсальный HTTP-клиент (пример на Go SDK)

Во всех клиентах после подключения становятся доступны четыре инструмента:
`list_docs`, `search`, `search_pages`, `get_page`.

## Безопасность

По умолчанию HTTP-сервер **не делает аутентификации**. Флаг
`--auth-token TOKEN` (или переменная `MARVINDOCS_AUTH_TOKEN`) включает
требование `Authorization: Bearer <token>` для каждого HTTP-запроса,
включая `/mcp` и веб-UI; исключение — только `/healthz`. На `--stdio`
токен не влияет.

Если выставляешь `--http` на публичный адрес — дополнительно ставь перед
ним reverse-proxy, который терминирует TLS (по обычному HTTP токен
передаётся открытым текстом) или занимается авторизацией (nginx + basic
auth, OAuth-шлюз и т.д.).

Клиенты могут только читать данные — инструменты никогда не пишут в
индекс и не меняют дамп devdocs.

## Как проверить, что сервер жив

В репозитории лежит smoke-тест, который прогоняет все инструменты по
HTTP:

```bash
marvindocs serve --http :3333 &
./scripts/mcp-smoke.sh                     # использует curl + jq
```

Ожидаемый вывод (сокращённый; число документов зависит от дампа):

```
==> initialize http://localhost:3333/mcp
==> tools/list
    - get_page: Fetch a documentation page ...
    - list_docs: List all indexed documentation sets ...
    - search: Search across all indexed documentation ...
    - search_pages: Full-text search across page bodies (not just symbol names). Use this ...
==> tools/call list_docs (first 3)
    791 docs, first 3:
      angular  Angular
      ...
==> tools/call search query=useState limit=3
    react  useState  reference/react/usestate
    ...
==> tools/call get_page react reference/react/usestate (markdown)
    length=24496 chars
OK
```

Указать другой адрес можно через `MCP_URL=http://host:3333/mcp`.

В `scripts/` есть ещё два stdio-скрипта (нужны собранный
`./bin/marvindocs` и индекс):

- `python3 scripts/mcp-bench.py` — бенч задержек: время round-trip
  каждого вызова по stdio, цель — `postgresql~17` из индекса.
- `python3 scripts/mcp-quality.py` — ручная оценка качества: прогоняет
  набор типичных запросов через `search` + `get_page` и печатает отчёт
  по каждому случаю (без assert'ов).

Быстрая ручная проверка одной командой:

```bash
# 1. initialize (идентификатор сессии приходит в заголовке ответа)
curl -sS -i -X POST http://localhost:3333/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize",
       "params":{"protocolVersion":"2025-06-18","capabilities":{},
                 "clientInfo":{"name":"c","version":"0"}}}'
```

Сервер отдаёт `Mcp-Session-Id: <id>` — его надо прокидывать тем же
заголовком в последующие `tools/list` / `tools/call`.

## Диагностика

| Симптом | Возможная причина |
|---|---|
| `index file not found` | Сначала запусти `marvindocs index build`. |
| `open docs.json: no such file` | Неправильный `--docs-path`; ожидается `<path>/docs.json`. |
| Tool возвращает пустой `hits` | Нет префиксных совпадений или этот док не в индексе. Сначала `list_docs`. |
| `get_page` возвращает «page not found» | `path` должен быть значением `search.path` или из `index.json`, а не URL'ом. |
