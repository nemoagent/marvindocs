# Конфигурация

marvindocs настраивается только через флаги командной строки. Никаких
переменных окружения и файлов конфигурации — намеренно, чтобы было
просто разворачивать в контейнерах и разбирать в скриптах.

## Структура каталогов

```
data/public/docs/     исходные данные (дамп devdocs, в .gitignore)
marvindocs.db         SQLite FTS5-индекс (в .gitignore)
bin/marvindocs        скомпилированный бинарь (в .gitignore)
```

Пути `data/public/` и `marvindocs.db*` добавлены в `.gitignore`.

## Общие флаги для serve и index build

| Флаг | Значение по умолчанию | Описание |
|---|---|---|
| `--docs-path DIR` | `data/public/docs` | Корень дампа devdocs. Должен содержать `docs.json`. |
| `--index-path FILE` | `marvindocs.db` | Файл SQLite-индекса. `index build` создаёт его, `serve` требует, чтобы файл уже существовал. |

## Флаги только для `serve`

| Флаг | Значение по умолчанию | Описание |
|---|---|---|
| `--stdio` | `false` | Запустить MCP-сервер через stdin/stdout (JSON-RPC с разделителем `\n`). |
| `--http ADDR` | пусто | Слушать HTTP-клиентов на `ADDR`. MCP Streamable transport — на `/mcp`. |

Надо указать хотя бы один из `--stdio` / `--http`.

## Флаги только для `index build`

| Флаг | Значение по умолчанию | Описание |
|---|---|---|
| `--fulltext` | `false` | Также индексирует тело страниц в `pages_fts`. Существенно медленнее, итоговый файл индекса вырастает примерно до суммарного объёма всех `db.json`. |

## Конфигурация времени сборки

Образ Docker и флаги Go лежат в `Makefile`:

```make
GO_IMAGE   = golang:1.25
CGO_ENABLED = 0            # чистый Go, без зависимости от glibc
```

SQLite приходит из pure-Go-драйвера `modernc.org/sqlite`; никакого
CGO, никакой внешней libsqlite, никакого динамического линка. Бинарь
запускается на любой Linux-машине нужной архитектуры.

## Куда что пишется

| Путь | Пишет | Назначение |
|---|---|---|
| `./marvindocs.db` | `index build` | основной файл SQLite |
| `./marvindocs.db-wal`, `-shm` | runtime SQLite | WAL-журнал, shared memory |
| `./bin/marvindocs` | `make build` | скомпилированный CLI |
| `./.gocache/`, `./.gomodcache/` | Docker-сборка | кэши Go build + модулей |

Ни один из этих каталогов заранее создавать не нужно — `make` делает
всё сам.
