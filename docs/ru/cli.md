# Справочник CLI

```
marvindocs <command> [flags]
```

Краткая справка — `marvindocs help` или `marvindocs --help`.

## `marvindocs version`

Выводит версию сборки (по умолчанию `dev`, перезаписывается через
`-ldflags "-X main.version=..."`).

```bash
marvindocs version
# marvindocs dev
```

## `marvindocs index build`

Пересобирает SQLite FTS5-индекс из дампа devdocs.

```bash
marvindocs index build [--docs-path DIR] [--index-path FILE] [--fulltext]
```

| Флаг | По умолчанию | Примечание |
|---|---|---|
| `--docs-path` | `data/public/docs` | Должен содержать `docs.json` и `<slug>/{index,db}.json`. |
| `--index-path` | `marvindocs.db` | Выходной файл SQLite. Перезаписывается. |
| `--fulltext` | `false` | Зарезервировано. Пока принимается, но игнорируется. |

Коды выхода: `0` — успех, `1` — ошибка (нет данных, ошибка записи
SQLite и т.д.), `2` — ошибка использования.

## `marvindocs index stats`

Выводит счётчики из готового индекса.

```bash
marvindocs index stats [--index-path FILE]
```

Пример вывода:

```
index:   marvindocs.db
size:    476532736 bytes
docs:    791
entries: 2677981
```

## `marvindocs search`

Выполняет полнотекстовый поиск по индексу и печатает результаты.
Удобно для shell-пайплайнов и быстрых проверок без запуска сервера.

```bash
marvindocs search QUERY [--doc SLUG] [--limit N] [--json] [--index-path FILE]
```

| Флаг | По умолчанию | Примечание |
|---|---|---|
| `--doc` | пусто | Ограничить поиск одним slug (например, `react`, `python~3.13`). |
| `--limit` | `20` | Максимум результатов (1–100). |
| `--json` | `false` | Выводить сырой список результатов как JSON в stdout. |
| `--index-path` | `marvindocs.db` | Файл SQLite-индекса (должен существовать). |

Позиционные аргументы после флагов склеиваются через пробел, то есть
`marvindocs search http server` эквивалентно `marvindocs search "http server"`.

Текстовый вывод — колонки `DOC_SLUG  NAME  PATH`. Итоговая строка
`N result(s)` пишется в stderr, чтобы было удобно передавать данные
по пайпу.

Код выхода `0` даже при отсутствии результатов (в stderr выводится
сообщение); `1` — ошибка индекса; `2` — ошибка использования.

## `marvindocs get`

Возвращает одну страницу из дампа devdocs в виде Markdown или
исходного HTML. SQLite-индекс не нужен — читает `<slug>/db.json`
напрямую.

```bash
marvindocs get SLUG PATH [--format markdown|html] [--docs-path DIR]
```

| Флаг | По умолчанию | Примечание |
|---|---|---|
| `--format` | `markdown` | `markdown` (алиас `md`) или `html`. |
| `--docs-path` | `data/public/docs` | Каталог с `<slug>/db.json`. |

`PATH` — путь страницы, как он хранится в devdocs (например
`hooks/use-state` для React). Узнать корректный путь можно через
`marvindocs search`.

## `marvindocs list`

Печатает документации, присутствующие в индексе, по одной в строке:

```bash
marvindocs list [--json] [--index-path FILE]
```

| Флаг | По умолчанию | Примечание |
|---|---|---|
| `--json` | `false` | Выводить список как JSON. |
| `--index-path` | `marvindocs.db` | Файл SQLite-индекса (должен существовать). |

Текстовый вывод — колонки `SLUG  NAME  VERSION` (если `version`
пусто, используется `release`). Итоговая строка идёт в stderr.

## `marvindocs serve`

Запускает MCP-сервер.

```bash
marvindocs serve [--stdio] [--http ADDR] \
                 [--docs-path DIR] [--index-path FILE]
```

| Флаг | По умолчанию | Примечание |
|---|---|---|
| `--stdio` | `false` | MCP через stdin/stdout. |
| `--http` | пусто | Web UI на `<addr>/` и MCP Streamable HTTP на `<addr>/mcp`. |
| `--docs-path` | `data/public/docs` | Нужен `get_page`, чтобы читать `db.json` по требованию. |
| `--index-path` | `marvindocs.db` | Файл должен уже существовать (сначала `index build`). |

Надо передать как минимум один из `--stdio` / `--http`. Оба можно
указать одновременно — один и тот же инстанс сервера обслуживает
оба транспорта.

Процесс слушает `SIGINT` и `SIGTERM` и при любом из них корректно
завершает работу.

## Коды выхода (для всех команд)

| Код | Значение |
|---|---|
| `0` | Успех |
| `1` | Ошибка времени выполнения (I/O, SQLite и т.д.) |
| `2` | Ошибка использования (неизвестный флаг, нет аргумента) |
