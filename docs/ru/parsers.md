# Парсеры

В marvindocs есть отдельные бинари-парсеры, которые готовят документацию
в DevDocs-формате из произвольных источников. Они нужны, когда нужно
проиндексировать документацию, которой нет в апстрим DevDocs (Laravel,
внутренние хэндбуки, OpenAPI-спеки).

Парсеры лежат в `parsers/` и **не зависят от основного модуля
marvindocs**. Они пишут те же файлы, что использует DevDocs
(`index.json`, `db.json`, `meta.json` и метку `.last-modified`) и
обновляют родительский `docs.json`, поэтому остальной marvindocs
подхватывает их без изменений.

## Контракт на диске

Для slug `<slug>` парсер пишет в `data/public/docs/<slug>/`:

| Файл | Назначение |
|---|---|
| `meta.json` | Плоская метадата: name, slug, type, version, release, mtime, db_size, attribution. |
| `index.json` | `{entries: [{name, path, type}], types: [{name, count, slug}]}`. |
| `db.json` | `{<path>: "<html fragment>", ...}` — фрагменты body, без `<html>`/`<head>`. |
| `.last-modified` | Пустой файл, mtime равен `meta.json.mtime`; нужен для `sync-docs.sh`. |

Дополнительно `scripts/run-parser.sh` после каждого успешного запуска
кладёт в выходной каталог маркер `.parser` (имя парсера). `sync-docs.sh`
помечает такие каталоги как `LOCAL` и никогда их не перезаписывает, даже с
`--force`.

Эти файлы читает `internal/docstore` — тот же путь, что и для дампов
DevDocs.

## Готовые парсеры

| Парсер | Источник | Output slug | README |
|---|---|---|---|
| `laravel` | github.com/laravel/docs | `laravel~<short>` (например `laravel~12`, `laravel~5.8`) | `parsers/laravel/README.md` |
| `go-dev` | go.dev/doc/ | `go-dev` (обзор) и `go-dev~1.X` (release notes конкретной версии) | `parsers/go-dev/README.md` |
| `go-std` | `go doc` внутри Docker-образа Go | `go-std~<version>` (например `go-std~1.25`) | `parsers/go-std/README.md` |
| `mongodb` | github.com/mongodb/docs (reST-исходники Manual) | `mongodb~<version>` (например `mongodb~8.0`) | `parsers/mongodb/README.md` |

## Workflow

```bash
# 1. Собрать бинарь парсера.
make parsers                                          # все парсеры
# или:  cd parsers/laravel && go build -o ../../bin/laravel-parser .

# 2. Запустить. Выход кладётся туда, где читает marvindocs.
./bin/laravel-parser --version 12.x \
    --out data/public/docs/laravel~12 --force

# 3. Перестроить индекс. Новый док добавится к остальным.
./bin/marvindocs index build
```

`index build` инкрементальный и сохраняет существующие эмбеддинги. Не
используйте здесь `index build --full`: на индексе с векторами он
откажется работать, а `--full --drop-vectors` удалит их все (пересчёт
занимает часы).

После шага 3 док доступен в Web UI и через MCP-инструменты
(`list_docs`, `search`, `get_page`).

`make parsers` собирает каждый парсер в `bin/<name>-parser`;
`make parsers-test` запускает `go test ./...` для каждого модуля
`parsers/*/` (включая `devdocsfmt`). Обе цели выполняются внутри
Docker-образа `golang`, локальный Go не нужен.

## Запуск зарегистрированных парсеров

`parsers/registry.json` перечисляет документы, которые можно собрать
парсером. Ключ верхнего уровня — `parsers`; каждая запись содержит имя
парсера, slug, фиксированные аргументы и описание.
`scripts/run-parser.sh` читает его и запускает соответствующий бинарь
`bin/<parser>-parser` с `--out <docs-path>/<slug>` и `--force`.

Запись с `"versions"` — шаблон: её slug содержит `{v}` (`laravel~{v}`,
`go-dev~{v}`, `go-std~{v}`, `mongodb~{v}`), а `{v}` в `args` заменяется
версией. Фиксированные записи вроде `go-dev` без `versions`. Принимается
любая версия, подходящая под шаблон, даже если её ещё нет в списке:

```bash
./scripts/run-parser.sh go-std~1.26         # работает, хотя 1.26 нет в реестре
```

После успешного запуска версии, которой нет в списке, она добавляется в
`versions` в `parsers/registry.json`.

```bash
./scripts/run-parser.sh --list              # slug | parser | description
./scripts/run-parser.sh laravel~12          # запустить один slug
./scripts/run-parser.sh --all               # запустить все slug'и из реестра (не останавливается на ошибках)
./scripts/run-parser.sh --check laravel~12  # показать итоговую команду, не запуская
```

| Опция | Значение |
|---|---|
| `--no-force` | Не передавать `--force` парсеру. |
| `--docs-path DIR` | Корень вывода (по умолчанию `data/public/docs`). |

`--all` не останавливается на первой ошибке: запускает все slug'и,
печатает `summary: N ok, M failed` и список упавших и завершается с кодом
`1`, если хоть один упал.

Требуется `jq`. Коды выхода: `0` — OK, `1` — парсер завершился с
ошибкой, `2` — ошибка использования, неизвестный slug, нет `jq`/реестра
или нет бинаря парсера (сначала `make parsers`).

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
