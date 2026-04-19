# Быстрый старт

Эта страница проведёт от свежего клона до работающего MCP-сервера.

## 1. Требования

- **Docker** — по умолчанию `Makefile` запускает Go внутри `golang:1.25`,
  локальный Go ставить не нужно.
- **~20 ГБ диска** — 17 ГБ под дамп devdocs, ~500 МБ под индекс.
- *(Опционально)* **Go 1.25+** — для нативной сборки:
  `CGO_ENABLED=0 go build ./cmd/marvindocs`.

## 2. Получить данные devdocs

marvindocs не содержит саму документацию — он читает архив devdocs.
Получить его можно двумя способами.

### Вариант A: поднять Rails-приложение devdocs и скачать паки

Инструкции — https://github.com/freeCodeCamp/devdocs. После того как
приложение скачает паки, скопируй его каталог `public/docs/` в этот
репозиторий по пути `data/public/docs/`.

### Вариант B: использовать готовый дамп

Если у тебя уже есть снимок, положи его так, чтобы структура была:

```
data/public/docs/
├── docs.json               # список всех документов
├── angular/
│   ├── meta.json
│   ├── index.json
│   └── db.json
├── python~3.12/
│   └── ...
└── ...
```

`docs.json` обязателен. В каталоге каждого slug должны быть
`index.json` и `db.json`.

## 3. Собрать бинарник

```bash
make build
```

Сборка идёт в Docker, на выходе — один статический бинарь
`./bin/marvindocs` (~9 МБ).

## 4. Построить поисковый индекс

```bash
./bin/marvindocs index build
```

Ожидаемый вывод:

```
loaded docs.json: 791 docs from data/public/docs
indexed 791 docs, 2677981 entries in 2m31s -> marvindocs.db
```

Команда создаёт `marvindocs.db` (~455 МБ для 791 дока). Повторный
запуск безопасен — прежнее содержимое перезаписывается атомарно.

Проверить результат:

```bash
./bin/marvindocs index stats
```

## 5. Запустить MCP-сервер

### По stdio (для Claude Desktop / Cursor / Claude Code)

```bash
./bin/marvindocs serve --stdio
```

Процесс читает JSON-RPC со stdin и пишет на stdout. Примеры конфигов —
в [MCP server](mcp.md#настройка-клиентов).

### По HTTP (для удалённых агентов)

```bash
./bin/marvindocs serve --http :3333
```

Endpoint MCP Streamable HTTP — `http://localhost:3333/mcp`. Проба
живости — `/healthz`.

Можно включить оба транспорта сразу:

```bash
./bin/marvindocs serve --stdio --http :3333
```

## 6. Проверить

Через `curl`:

```bash
curl -s http://localhost:3333/healthz
# ok
```

Для полноценной проверки MCP подключись любым MCP-клиентом к
`http://localhost:3333/mcp` (HTTP) либо используй команду
`marvindocs serve --stdio` как stdio-сервер. Подробнее — см.
[MCP-сервер](mcp.md).
