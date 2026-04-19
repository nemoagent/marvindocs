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
