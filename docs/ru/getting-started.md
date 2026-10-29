# Быстрый старт

Эта страница проведёт от свежего клона до работающего MCP-сервера.

## 1. Требования

- **Docker** — по умолчанию `Makefile` запускает Go внутри `golang:1.25`,
  локальный Go и C-тулчейн ставить не нужно.
- **~20 ГБ диска** — 17 ГБ под полный дамп devdocs; размер индекса зависит
  от набора документов (текст страниц индексируется как Markdown-чанки, но
  не хранится; для полного дампа около 2,8 ГБ).
- *(Опционально)* **Go 1.25+ и C-тулчейн** — для нативной сборки:
  драйвер SQLite использует CGo, поэтому собирать нужно как
  `CGO_ENABLED=1 go build -tags sqlite_fts5 ./cmd/marvindocs`. Без
  build-тега `sqlite_fts5` расширение FTS5 не включается и индекс
  не откроется.

## 2. Получить данные devdocs

> Короткий путь: `marvindocs bundle install NAME...` (после шага 3)
> ставит готовые доки вместе с индексом и эмбеддингами, и шаги 2 и 4
> можно пропустить. См. [Оффлайн-наборы](bundles.md).

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
│   └── db.zip
├── python~3.12/
│   └── ...
└── ...
```

`docs.json` обязателен. В каталоге каждого slug должны быть
`index.json` и `db.zip`.

## 3. Собрать бинарник

```bash
make build
```

Сборка идёт в Docker, на выходе — один бинарь
`./bin/marvindocs` (~19 МБ; динамически слинкован с системным libc,
поскольку драйвер SQLite использует CGo).

## 4. Построить поисковый индекс

```bash
./bin/marvindocs index build
```

Ожидаемый вывод (числа условные):

```
level=INFO msg="loaded docstore" docs=27 path=data/public/docs
level=INFO msg="index synced" docs=27 entries=120000 pages=0 duration=15s path=marvindocs.db
```

Команда создаёт `marvindocs.db`; его размер зависит от набора документов.
Тела страниц тоже индексируются (как Markdown-чанки), что включает `search_pages` и
`search --body`. Повторный запуск безопасен: по
умолчанию сборка инкрементальная, переиндексируются только изменившиеся
документы, а существующие эмбеддинги сохраняются. `--full` пересобирает
всё (последняя строка тогда начинается с `index indexed`), но отказывается
работать на индексе с векторами без `--drop-vectors`.

Чтобы позже обновить всё разом (синхронизация devdocs, парсеры,
инкрементальный индекс, эмбеддинги), запустите `make update`; см.
[Синхронизацию данных](sync-docs.md#обновление-одной-командой).

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

### По HTTP (MCP + веб-интерфейс)

```bash
./bin/marvindocs serve --http :3333
```

Одной командой поднимает сразу два:

- Веб-интерфейс для чтения документации в браузере: <http://localhost:3333/>
- MCP Streamable HTTP-endpoint: <http://localhost:3333/mcp>

Проба живости — `/healthz`.

Голый `:3333` привязывается только к `127.0.0.1`. Чтобы раздавать по LAN,
укажите явный адрес и задайте токен (на не-loopback адресе без токена
сервер не стартует):

```bash
./bin/marvindocs serve --http 0.0.0.0:3333 --auth-token "$(openssl rand -hex 32)"
```

Клиенты передают `Authorization: Bearer <token>`; см.
[Конфигурацию](configuration.md#вне-loopback-токен-обязателен).

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

## 7. Запуск через Docker Compose

Compose-файл собирает образ, монтирует `./data/public/docs` и
`./data/index` и публикует порт 3333 на всех интерфейсах хоста. Сначала
постройте индекс в `data/index/`, затем запустите с токеном:

```bash
./bin/marvindocs index build --index-path data/index/marvindocs.db
MARVINDOCS_AUTH_TOKEN=$(openssl rand -hex 32) docker compose up -d
```

Или храните токен в `.env` рядом с `docker-compose.yml`:

```
MARVINDOCS_AUTH_TOKEN=change-me
```

Без токена контейнер завершится с ошибкой
`error: --http 0.0.0.0:3333 is reachable from other hosts but no auth token is set`.
На машине, недоступной никому другому, можно запустить без аутентификации:
`MARVINDOCS_INSECURE=1 docker compose up -d`.

По умолчанию контейнер отвечает только лексическим поиском. Для гибридного
поиска `get_docs` нужен эмбеддер; проще всего — Ollama на docker-хосте,
который контейнер видит как `host.docker.internal`:

```
MARVINDOCS_AUTH_TOKEN=change-me
MARVINDOCS_EMBEDDER=ollama
MARVINDOCS_OLLAMA_URL=http://host.docker.internal:11434
MARVINDOCS_OLLAMA_MODEL=bge-m3
```

В индексе уже должны быть векторы (`index embed`), см.
[Векторный поиск](vectors.md). Первый запрос после старта занимает несколько
секунд, пока Ollama загружает модель; прогретые запросы — десятки миллисекунд.
