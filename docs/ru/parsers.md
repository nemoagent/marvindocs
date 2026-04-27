# Парсеры

В marvindocs есть отдельные бинари-парсеры, которые готовят документацию
в DevDocs-формате из произвольных источников. Они нужны, когда нужно
проиндексировать документацию, которой нет в апстрим DevDocs (Laravel,
внутренние хэндбуки, OpenAPI-спеки).

Парсеры лежат в `parsers/` и **не зависят от основного модуля
marvindocs**. Они пишут три файла в том же формате, что использует
DevDocs, поэтому остальной marvindocs подхватывает их без изменений.

## Контракт на диске

Для slug `<slug>` парсер пишет в `data/public/docs/<slug>/`:

| Файл | Назначение |
|---|---|
| `meta.json` | Плоская метадата: name, slug, type, version, release, mtime, db_size, attribution. |
| `index.json` | `{entries: [{name, path, type}], types: [{name, count, slug}]}`. |
| `db.json` | `{<path>: "<html fragment>", ...}` — фрагменты body, без `<html>`/`<head>`. |
| `.last-modified` | Пустой файл, mtime равен `meta.json.mtime`; нужен для `sync-docs.sh`. |

Эти файлы читает `internal/docstore` — тот же путь, что и для дампов
DevDocs.

## Готовые парсеры

| Парсер | Источник | Output slug | README |
|---|---|---|---|
| `laravel` | github.com/laravel/docs | `laravel~<short>` (например `laravel~12`, `laravel~5.8`) | `parsers/laravel/README.md` |

## Workflow

```bash
# 1. Собрать бинарь парсера.
make parsers                                          # все парсеры
# или:  cd parsers/laravel && go build -o ../../bin/laravel-parser .

# 2. Запустить. Выход кладётся туда, где читает marvindocs.
./bin/laravel-parser --version 12.x \
    --out data/public/docs/laravel~12 --force

# 3. Перестроить индекс. Новый док добавится к остальным.
./bin/marvindocs index build --full
```

После шага 3 док доступен в Web UI и через MCP-инструменты
(`list_docs`, `search`, `get_page`).

## Свой парсер

1. `parsers/<name>/` со своим `go.mod`. Держим изолированно.
2. Добавь `replace github.com/piligrim/marvindocs/parsers/devdocsfmt => ../devdocsfmt`.
3. Создай `*devdocsfmt.Writer`, вызывай `AddPage(path, html)` и
   `AddEntry(name, path, type)` для каждой страницы и символа, в
   конце — `Flush(out, force)`.
4. Используй стандартный `devdocsfmt.Flags`, чтобы пользователи
   видели одинаковые `--source`, `--out`, `--version`, `--release`,
   `--force`.

Парсер Laravel (`parsers/laravel/`) — самый простой end-to-end
пример.

## Почему парсеры изолированы

- Серверный бинарь остаётся компактным (никаких markdown/HTML
  скрабберов внутри).
- Зависимости парсеров (markdown-движки, GitHub-клиенты) не
  протекают в `internal/`.
- Парсер можно писать на любом языке. Python, Rust, даже shell —
  лишь бы получались те четыре файла, что описаны выше.
