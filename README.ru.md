# marvindocs

[English](README.md) | **Русский**

MCP-сервер и веб-интерфейс в одном бинарнике: ИИ-агенты и браузеры ищут и
читают ~890 наборов документации (React, Python, Go, Rust, …) из локального
индекса SQLite.

Проект вдохновлён [DevDocs](https://devdocs.io/) и основан на нём: большая
часть документации берётся из пакетов DevDocs, и все доки, включая собранные
нашими парсерами, хранятся в дисковом формате DevDocs. Поисковый движок,
MCP-сервер и веб-интерфейс написаны с нуля. См.
[Связь с DevDocs](#связь-с-devdocs).

## Что делает

| Возможность | Как |
|---|---|
| **Веб-интерфейс** | Просмотр и поиск по всем докам в браузере на `http://localhost:3333/` |
| **MCP-сервер** | Пять инструментов для ИИ-агентов через stdio или HTTP |
| **HTML→Markdown** | `get_page` на лету превращает HTML из devdocs в чистый Markdown |
| **Полнотекстовый поиск** | SQLite FTS5, ранжирование BM25, поиск по префиксу |
| **Синонимы** | Запросы расширяются по словарю синонимов (`k8s` находит `kubernetes`), см. [docs/ru/synonyms.md](docs/ru/synonyms.md); `synonyms suggest` собирает кандидатов из доков |
| **Векторный поиск** | Необязательный гибридный поиск bge-m3 + BM25 для `get_docs`, см. [docs/ru/vectors.md](docs/ru/vectors.md) |

## Связь с DevDocs

[DevDocs](https://github.com/freeCodeCamp/devdocs) — браузер документации
для людей: скраперы на Ruby превращают официальные доки в пакеты, а
веб-приложение на JavaScript показывает их и умеет хранить офлайн в
браузере. marvindocs использует эти пакеты и их формат и строит поверх них
сервис для ИИ-агентов.

| | DevDocs | marvindocs |
|---|---|---|
| Основной пользователь | человек в браузере | ИИ-агент через MCP; веб-интерфейс вторичен |
| Источники доков | собственные скраперы | пакеты DevDocs (837 из 892 в полном наборе) плюс свои парсеры: стандартная библиотека Go через `go doc`, go.dev, Laravel 4.0-13, MongoDB Manual, Metasploit (55 доков), см. [docs/ru/parsers.md](docs/ru/parsers.md) |
| Хранение | `index.json` + `db.json` на каждый док | тот же `index.json`; страницы упакованы в `db.zip` (`marvindocs pack`) |
| Поиск | по названиям записей в браузере | SQLite FTS5 по названиям и тексту страниц, BM25, синонимы, гибридный векторный поиск bge-m3 по фрагментам |
| Результат | HTML-страницы | Markdown, а `get_docs` возвращает лучшие фрагменты в пределах бюджета токенов |
| Поставка | веб-приложение, офлайн-кэш в браузере | один бинарник на Go (CLI, MCP через stdio и HTTP, веб-интерфейс), Docker, готовые наборы с собранным индексом |

marvindocs не использует код DevDocs и не связан с ним.
Содержимое документации распространяется по лицензиям исходных источников.

## Требования

- **Docker** — для `make build` (Go 1.25 внутри контейнера). Для запуска бинарника не нужен.
- **curl + tar** — чтобы `scripts/sync-docs.sh` скачал доки.
- **Необязательно, для векторного поиска** — Ollama с bge-m3 (эмбеддинги) и ~600 МБ под ONNX-модель и библиотеки (`make onnx-fetch`).
- **~2 ГБ диска** для набора доков по умолчанию; ~17 ГБ для всех.

## Начало работы

```bash
# 1. Собрать бинарник
make build

# 2. Скачать документацию (27 популярных доков, ~0.6 ГБ на диске)
./scripts/sync-docs.sh

# 3. Запустить
./bin/marvindocs serve --http :3333
```

Открой <http://localhost:3333/>, чтобы смотреть доки.
MCP-эндпоинт доступен на <http://localhost:3333/mcp>.

> `--http :3333` слушает только `127.0.0.1`. Чтобы принимать подключения
> с других хостов, используй `--http 0.0.0.0:3333` и в этом случае задай
> `--auth-token` (или `MARVINDOCS_AUTH_TOKEN`).

По умолчанию `sync-docs.sh` в конце также запускает `index build` (отключается
через `--no-index`). Текст страниц тоже индексируется, поэтому `search_pages`
работает сразу.

Чтобы потом обновить всё (синхронизация devdocs, парсеры, инкрементальный
индекс, эмбеддинги), запусти `make update`; существующие эмбеддинги он
никогда не удаляет. См.
[docs/ru/sync-docs.md](docs/ru/sync-docs.md#обновление-одной-командой).

### Офлайн-наборы с готовым индексом

Вместо синхронизации и расчёта эмбеддингов можно установить готовые наборы
(доки + индекс + эмбеддинги) из публичного GitHub-релиза `bundles`; `gh` и
токен не нужны, архивы проверяются по `SHA256SUMS` релиза:

```bash
./bin/marvindocs bundle install core go python
```

Наборы: `core`, `go`, `python`, `db`, `infra`, `js`, `php`, `ruby`, `jvm`,
`native`, `other` (~2.9 ГБ в сумме). См.
[docs/ru/bundles.md](docs/ru/bundles.md).

### Скачать отдельные доки или всё

```bash
# Отдельные слаги
./scripts/sync-docs.sh react typescript python~3.13 go

# Все доки DevDocs (~840, ~17 ГБ, может занять 30-90 мин)
./scripts/sync-docs.sh --all
```

Слаги совпадают с названиями на <https://devdocs.io>. Версии указываются
через тильду: `python~3.12`, `angular~18`, `react~18`.

### MCP через stdio (Claude Desktop, Cursor, Claude Code…)

Для локальных MCP-клиентов сервер также работает через stdin/stdout:

```bash
./bin/marvindocs serve --stdio
```

Оба транспорта одновременно:

```bash
./bin/marvindocs serve --stdio --http :3333
```

## Подключение к ИИ-агентам

### Claude Code

```bash
claude mcp add marvindocs /absolute/path/to/marvindocs serve --stdio \
  --docs-path  /absolute/path/to/data/public/docs \
  --index-path /absolute/path/to/marvindocs.db
```

### Claude Desktop

Добавь в `claude_desktop_config.json`
(`~/Library/Application Support/Claude/` на macOS,
`~/.config/Claude/` на Linux):

```json
{
  "mcpServers": {
    "marvindocs": {
      "command": "/absolute/path/to/marvindocs",
      "args": ["serve", "--stdio",
               "--docs-path",  "/absolute/path/to/data/public/docs",
               "--index-path", "/absolute/path/to/marvindocs.db"]
    }
  }
}
```

### Cursor / Windsurf

Тот же JSON, другой файл конфигурации:

- Cursor: `~/.cursor/mcp.json`
- Windsurf: `~/.codeium/windsurf/mcp_config.json`

Для всех агентов, включая Zed, Continue, VS Code Copilot Chat, — см.
[docs/ru/clients.md](docs/ru/clients.md).

## Инструменты MCP

После подключения у агента есть пять инструментов:

| Инструмент | Что делает |
|---|---|
| `list_docs` | Список всех проиндексированных доков со слагом, названием и версией |
| `search` | FTS-поиск по названиям/путям записей; возвращает ранжированные результаты с `doc_slug` и `path` |
| `search_pages` | Полнотекстовый поиск по тексту страниц; возвращает результаты со сниппетом. |
| `get_page` | Получить страницу в Markdown (по умолчанию) или HTML по `doc_slug` + `path` |
| `get_docs` | Ответ за один вызов: самые релевантные Markdown-фрагменты по теме в пределах бюджета токенов; для доков с векторами использует гибридный поиск векторы + BM25 |

Проверить сервер можно встроенным smoke-тестом (нужны `curl` + `jq`):

```bash
./bin/marvindocs serve --http :3333 &
./scripts/mcp-smoke.sh
```

## Справочник CLI

```
marvindocs index build   [--docs-path DIR] [--index-path FILE] [--full [--drop-vectors]] [--rehash]
                                                                # по умолчанию инкрементальная синхронизация (векторы сохраняются);
                                                                # --full пересобирает индекс, при наличии векторов нужен --drop-vectors;
                                                                # --rehash только пересчитывает хэши содержимого (один раз после pack --all)
marvindocs pack          (--all | SLUG...) [--docs-path DIR]    # преобразовать db.json -> db.zip, удалить db.json
marvindocs bundle build  [--out DIR] [NAME...]                  # упаковать наборы из bundles.yaml с готовым индексом
marvindocs bundle install [--from DIR] NAME...                  # скачать наборы (gh) и влить в индекс
marvindocs index stats   [--index-path FILE] [--by-doc] [--top N]  # показать счётчики
marvindocs index embed   [--doc SLUG]... [--embedder ollama|onnx] [--batch N]
                                                                # эмбеддинги фрагментов, см. docs/ru/vectors.md
marvindocs serve         [--stdio] [--http ADDR] [--auth-token TOKEN] [--insecure]
                         [--docs-path DIR] [--index-path FILE]
                         [--embedder none|ollama|onnx] [--ollama-url URL] [--ollama-model NAME]
                         [--model-dir DIR] [--onnxruntime PATH] [--synonyms DIR]
                                                                # запустить сервер; --embedder включает гибридный get_docs
marvindocs search QUERY  [--doc SLUG] [--limit N] [--json] [--body] [--index-path FILE] [--docs-path DIR] [--synonyms DIR]
                                                                # --body ищет по тексту страниц
marvindocs get SLUG PATH [--format markdown|html] [--docs-path DIR]  # получить одну страницу
marvindocs list          [--json] [--index-path FILE]           # список проиндексированных доков
marvindocs synonyms suggest [--docs-path DIR] [--doc SLUG]... [--min-docs N] [--min-count N]
                         [--names-only] [--synonyms DIR] [--out FILE]
                                                                # найти кандидатов-сокращения (TSV), см. docs/ru/synonyms.md
marvindocs synonyms import FILE --out DIR [--force]             # проверенный TSV -> файлы словаря для --synonyms
marvindocs version
```

Текст страниц тоже индексируется (нужно для `search_pages` и
`search --body`). Сам текст не хранится; сниппеты собираются заново из
`db.zip` по запросу.

### Переменные окружения

| Переменная | Значения | По умолчанию |
|---|---|---|
| `MARVINDOCS_AUTH_TOKEN` | любая строка | пусто (без авторизации). Используется, если не задан `--auth-token`; с токеном каждый HTTP-запрос, кроме `/healthz`, требует `Authorization: Bearer <token>` |
| `MARVINDOCS_INSECURE` | `1` | разрешить публичный `--http` без токена (как `--insecure`); иначе `serve` отказывается стартовать |
| `MARVINDOCS_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `MARVINDOCS_LOG_FORMAT` | `text`, `json` | `text` |
| `MARVINDOCS_EMBEDDER` | `none`, `ollama`, `onnx` | значение `--embedder` по умолчанию (`onnx` для `index embed`, `none` для `serve`) |
| `MARVINDOCS_OLLAMA_URL`, `MARVINDOCS_OLLAMA_MODEL` | URL, имя модели | `http://localhost:11434`, `bge-m3` |
| `MARVINDOCS_MODEL_DIR`, `MARVINDOCS_ONNXRUNTIME` | пути | `data/onnx/model`, `data/onnx/lib/libonnxruntime.so` |
| `MARVINDOCS_SYNONYMS` | каталог, `off` | пусто (только встроенный словарь). Значение `--synonyms` по умолчанию; каталог добавляет свои файлы `*.md` к встроенному словарю, `off` отключает расширение запросов |

Переменные эмбеддера задают значения флагов по умолчанию (флаги важнее).
`scripts/update.sh` загружает их из игнорируемого git файла `.env`; скопируй
шаблон `.env.example`.

### Docker

`docker compose up -d` собирает образ и отдаёт HTTP на порту 3333.
Compose-файл монтирует `./data/public/docs` в `/data` (только чтение)
и `./data/index` в `/index`. Файл индекса `./data/index/marvindocs.db`
нужно собрать заранее, например:

```bash
./bin/marvindocs index build --index-path data/index/marvindocs.db
# или установить наборы прямо в смонтированные пути
./bin/marvindocs bundle install --docs-path data/public/docs --index-path data/index/marvindocs.db core go
```

> Внутри контейнера образ слушает `0.0.0.0:3333`, а compose публикует
> порт на всех интерфейсах хоста. Если хост доступен другим, задай
> `MARVINDOCS_AUTH_TOKEN` в `environment` compose-файла.

## Структура

```
scripts/sync-docs.sh     скачать доки (curl + tar, без Docker)
scripts/mcp-smoke.sh     сквозная проверка MCP-сервера (без заголовка авторизации: нужен сервер без токена)
scripts/fetch-onnx.sh    скачать ONNX runtime, токенизатор и модель bge-m3 для --embedder onnx
data/public/docs/        дамп DevDocs и результат парсеров (не в git, ~17 ГБ)
bundles.yaml             описания офлайн-наборов (docs/ru/bundles.md)
dist/bundles/            архивы из `bundle build` (не в git)
marvindocs.db            индекс SQLite FTS5 (размер зависит от набора доков; текст страниц индексируется, но не хранится)
cmd/marvindocs/          точка входа CLI
internal/chunk/          разбиение Markdown на разделы и их оценка для get_docs
internal/docstore/       доступ к данным только на чтение + HTML→Markdown
internal/embed/          бэкенды эмбеддингов (Ollama, ONNX) и int8-квантование векторов
internal/mdcache/        LRU-кэш в памяти для преобразований HTML→Markdown
internal/logging/        настройка slog (уровень и формат text/json)
internal/index/          индексатор и FTS-поиск
internal/synonyms/       словарь синонимов для расширения запросов (builtin/*.md встроены в бинарник)
internal/mcpserver/      инструменты MCP (list_docs, search, search_pages, get_page, get_docs)
internal/web/            браузерный интерфейс (шаблоны + CSS, встроены в бинарник)
synonyms/                необязательный словарь IT-сокращений для --synonyms (289 понятий)
docs/en/                 документация на английском
docs/ru/                 документация на русском
```

## Документация

| Тема | EN | RU |
|---|---|---|
| Начало работы | [docs/en/getting-started.md](docs/en/getting-started.md) | [docs/ru/getting-started.md](docs/ru/getting-started.md) |
| Синхронизация доков | [docs/en/sync-docs.md](docs/en/sync-docs.md) | [docs/ru/sync-docs.md](docs/ru/sync-docs.md) |
| Офлайн-наборы | [docs/en/bundles.md](docs/en/bundles.md) | [docs/ru/bundles.md](docs/ru/bundles.md) |
| Конфигурация | [docs/en/configuration.md](docs/en/configuration.md) | [docs/ru/configuration.md](docs/ru/configuration.md) |
| Справочник CLI | [docs/en/cli.md](docs/en/cli.md) | [docs/ru/cli.md](docs/ru/cli.md) |
| MCP-сервер | [docs/en/mcp.md](docs/en/mcp.md) | [docs/ru/mcp.md](docs/ru/mcp.md) |
| Настройка клиентов | [docs/en/clients.md](docs/en/clients.md) | [docs/ru/clients.md](docs/ru/clients.md) |
| Веб-интерфейс | [docs/en/web.md](docs/en/web.md) | [docs/ru/web.md](docs/ru/web.md) |
| Формат данных | [docs/en/data-format.md](docs/en/data-format.md) | [docs/ru/data-format.md](docs/ru/data-format.md) |
| Синонимы | [docs/en/synonyms.md](docs/en/synonyms.md) | [docs/ru/synonyms.md](docs/ru/synonyms.md) |
| Векторный поиск | [docs/en/vectors.md](docs/en/vectors.md) | [docs/ru/vectors.md](docs/ru/vectors.md) |
| Документация Metasploit | [docs/en/metasploit.md](docs/en/metasploit.md) | [docs/ru/metasploit.md](docs/ru/metasploit.md) |
| Парсеры | [docs/en/parsers.md](docs/en/parsers.md) | [docs/ru/parsers.md](docs/ru/parsers.md) |
| Документация Go через Docker | [docs/en/go-doc.md](docs/en/go-doc.md) | [docs/ru/go-doc.md](docs/ru/go-doc.md) |
| Проверки согласованности | [docs/en/consistency-checks.md](docs/en/consistency-checks.md) | [docs/ru/consistency-checks.md](docs/ru/consistency-checks.md) |

## Лицензия

MIT (в процессе).
