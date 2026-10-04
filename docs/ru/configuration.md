# Конфигурация

marvindocs настраивается флагами командной строки и несколькими
переменными окружения (см. [Переменные окружения](#переменные-окружения)).
Файла конфигурации нет — намеренно, чтобы было просто разворачивать в
контейнерах и разбирать в скриптах.

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
| `--http ADDR` | пусто | Слушать HTTP-клиентов на `ADDR`. MCP Streamable transport — на `/mcp`. Голый `:3333` слушает только `127.0.0.1`; для публичного доступа — `0.0.0.0:3333`. |
| `--auth-token TOKEN` | пусто | Требовать `Authorization: Bearer TOKEN` на каждый HTTP-запрос (включая веб-интерфейс и `/mcp`), кроме `/healthz`. Если флаг пуст — читается `MARVINDOCS_AUTH_TOKEN`. |
| `--insecure` | `false` | Разрешить `--http` на не-loopback адресе без токена. Также включается через `MARVINDOCS_INSECURE=1`. |
| `--embedder NAME` | `none` | `none`, `ollama` или `onnx`. Включает гибридный поиск (векторы + BM25) в `get_docs`; см. [Векторный поиск](vectors.md). |
| `--ollama-url URL` | `http://localhost:11434` | Базовый URL Ollama (`--embedder ollama`). |
| `--ollama-model NAME` | `bge-m3` | Имя модели в Ollama (`--embedder ollama`). |
| `--model-dir DIR` | пусто | `--embedder onnx`: каталог с `model_int8.onnx` и `tokenizer.json`. |
| `--onnxruntime PATH` | пусто | `--embedder onnx`: путь к `libonnxruntime.so`. |

Надо указать хотя бы один из `--stdio` / `--http`.

### Вне loopback токен обязателен

`serve` отказывается стартовать, если `--http` привязан к адресу, отличному
от `127.0.0.1`/`localhost`/`::1` (например `0.0.0.0:3333` или IP в LAN), а
токен не задан:

```
error: --http 0.0.0.0:3333 is reachable from other hosts but no auth token is set; pass --auth-token (or MARVINDOCS_AUTH_TOKEN), or --insecure to serve without auth
```

Задайте `--auth-token` / `MARVINDOCS_AUTH_TOKEN` либо передайте `--insecure`
(`MARVINDOCS_INSECURE=1`), если хосту действительно можно доверять.
Loopback-привязка токена не требует.

## Переменные окружения

| Переменная | Значения | По умолчанию | Описание |
|---|---|---|---|
| `MARVINDOCS_AUTH_TOKEN` | любая строка | пусто | Bearer-токен для HTTP; используется, если не задан `--auth-token`. |
| `MARVINDOCS_INSECURE` | `1` | не задана | То же, что `--insecure`: разрешить не-loopback `--http` без токена. |
| `MARVINDOCS_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` | Уровень логирования. |
| `MARVINDOCS_LOG_FORMAT` | `text`, `json` | `text` | Формат логов. Логи пишутся в stderr. |

## Флаги только для `index build`

| Флаг | Значение по умолчанию | Описание |
|---|---|---|
| `--full` | `false` | Принудительная полная пересборка вместо инкрементальной синхронизации по умолчанию (переиндексируются только изменившиеся документы). |

## Флаги только для `index embed`

`index embed` принимает `--doc`, `--embedder` (по умолчанию `ollama`),
`--batch` и те же `--ollama-*`, `--model-dir` и `--onnxruntime`, что и
`serve`. См. [Векторный поиск](vectors.md).

## Конфигурация времени сборки

Образ Docker и флаги Go лежат в `Makefile`:

```make
GO_IMAGE = golang:1.25
# docker run ... -e CGO_ENABLED=1 $(GO_IMAGE)
#   go build -tags sqlite_fts5 -buildvcs=false -o bin/marvindocs ./cmd/marvindocs
```

SQLite приходит из CGo-драйвера `github.com/mattn/go-sqlite3`, поэтому
для сборки нужны `CGO_ENABLED=1` и build-тег `sqlite_fts5` (без него
FTS5 отсутствует и индекс не откроется). Бинарь динамически
слинкован с системным libc.

## Цели Makefile

Все Go-цели выполняются внутри контейнера `golang:1.25`.

| Цель | Что делает |
|---|---|
| `build` | Собирает `./bin/marvindocs` (CGO, `-tags sqlite_fts5`). |
| `tidy` | `go mod tidy`. |
| `test` | `go test -short -tags sqlite_fts5 ./...`: только тесты на фикстурах, укладывается в секунды. |
| `test-full` | `go test -timeout 30m -tags sqlite_fts5 ./...`: дополнительно собирает полный индекс из `data/public/docs` (10-20 минут; пропускается, если данных нет). |
| `vet` | `go vet -tags sqlite_fts5 ./...`. |
| `lint` | `golangci-lint run ./...` (v2.13.2, конфиг в `.golangci.yml`). |
| `vuln` | `govulncheck ./...` (v1.1.4) по проекту и тулчейну из образа. |
| `check` | `vet`, `lint`, `vuln`, `test` по очереди; проверка перед коммитом или PR. |
| `fmt` | `gofmt -w cmd internal parsers`. |
| `index` | `build`, затем `./bin/marvindocs index build`. |
| `run-http` | `build`, затем `serve --http :3333`. |
| `run-stdio` | `build`, затем `serve --stdio`. |
| `parsers` | Собирает каждый модуль `parsers/*` (кроме `devdocsfmt`) в `bin/<name>-parser` без CGO. |
| `parsers-test` | Запускает `go test ./...` в каждом модуле парсера. |
| `clean` | Удаляет `bin`, `.gocache`, `.gomodcache` и `marvindocs.db*`. |

## Docker

`Dockerfile` собирает бинарь в `golang:1.25-alpine` (CGO,
`sqlite_fts5`) и кладёт его в небольшой образ `alpine`. `docker compose up -d`
собирает образ и поднимает HTTP на порту 3333; монтируются
`./data/public/docs` в `/data` (только чтение) и `./data/index` в `/index`.
Индекс нужно построить заранее, чтобы существовал
`./data/index/marvindocs.db`, например:
`./bin/marvindocs index build --index-path data/index/marvindocs.db`.
Compose-файл задаёт `MARVINDOCS_LOG_FORMAT=json` и
`MARVINDOCS_LOG_LEVEL=info`, а также пробрасывает `MARVINDOCS_AUTH_TOKEN` и
`MARVINDOCS_INSECURE` из окружения хоста (или из `.env` рядом с
`docker-compose.yml`).

`CMD` образа: `serve --http 0.0.0.0:3333 --docs-path /data
--index-path /index/marvindocs.db`. Это не-loopback привязка, а compose
публикует порт 3333 на всех интерфейсах хоста, поэтому контейнер сразу
завершается, если не задан `MARVINDOCS_AUTH_TOKEN` (или
`MARVINDOCS_INSECURE=1` для доверенного хоста). См.
[Быстрый старт](getting-started.md#7-запуск-через-docker-compose).

## Куда что пишется

| Путь | Пишет | Назначение |
|---|---|---|
| `./marvindocs.db` | `index build` | основной файл SQLite |
| `./marvindocs.db-wal`, `-shm` | runtime SQLite | WAL-журнал, shared memory |
| `./bin/marvindocs` | `make build` | скомпилированный CLI |
| `./.gocache/`, `./.gomodcache/` | Docker-сборка | кэши Go build + модулей |

Ни один из этих каталогов заранее создавать не нужно — `make` делает
всё сам.
