# Векторный поиск

Эмбеддинги чанков хранятся в индексе и используются MCP-инструментом
`get_docs` для гибридного поиска (векторы + BM25). Функция опциональна:
без `--embedder` ничего не меняется.

## Что хранится

В индексе есть таблица `chunk_vecs(chunk_id INTEGER PRIMARY KEY, vec BLOB)`
с одной строкой на каждый чанк из `chunks`. `vec` — вектор bge-m3,
квантованный в int8: 4 байта масштаба float32 (little-endian), затем по
одному знаковому байту на измерение (1024, около 1 КБ на чанк). Вектор
считается по хлебным крошкам чанка (`doc > page > heading`) и его тексту,
обрезанному до 4000 байт (эмбеддер ONNX дополнительно режет вход до
512 токенов).
Строки удаляются вместе с докой при `index build` / sync.

## Как его использует поиск

Если `serve` запущен с `--embedder`, а у доки есть векторы, `get_docs`
ищет по этой доке гибридно:

1. Тема превращается в один вектор запроса (около 30-50 мс для ONNX int8
   на CPU).
2. Три ранжированных списка чанков объединяются через Reciprocal Rank
   Fusion (k=60): оценка чанка — сумма `вес / (60 + место)` по всем
   спискам, где он встретился.

   | Сигнал | Вес | Чанки |
   |---|---|---|
   | векторы | 1 | Топ чанков по косинусному сходству (полный перебор векторов одной доки, кэшируются в памяти при первом обращении). |
   | лексика | 0.5 | Топ чанков по BM25 (тот же лексический поиск по чанкам, что и в `search_pages`). |
   | записи | 0.25 | Страницы, у которых записи (символы из `index.json` и заголовки страниц) содержат все слова запроса, по рангу BM25; каждая страница считается своим первым чанком. |

   Векторы идут первыми, потому что справляются с перефразировками;
   лексика выручает, когда в запросе точное имя; записи поднимают
   страницу, названную по теме запроса.
3. Страницы ранжируются по лучшему объединённому чанку.

Доки без векторов, а также инструменты `search` и `search_pages`
по-прежнему используют лексический поиск BM25 без изменений.

## Бенчмарк

Бенчмарк — это Go-тест `TestBench` в `internal/mcpserver/bench_test.go`
на 96 кейсах из `internal/mcpserver/testdata/bench_cases.json`: 12 док,
36 запросов-ключевых слов (`lex`) и 60 запросов на естественном языке
(`nl`), у каждого есть ожидаемая страница. hit@1 / hit@3 — число запросов,
у которых ожидаемая страница стоит первой / среди первых трёх страниц
ответа `get_docs`.

Результаты на полностью проэмбеженном дампе devdocs, гибридный поиск с
весами выше:

| Запросы | hit@1 | hit@3 |
|---|---|---|
| все (96) | 63 | 81 |
| ключевые слова (36) | 31 | 35 |
| естественный язык (60) | 32 | 46 |

Без сигнала записей (`titleWeight` 0) результат 61/96 и 80/96.
`titleWeight` от 0.5 и выше начинает ухудшать запросы на естественном
языке; вес лексики 1 чуть хуже, чем 0.5.

Запуск:

```bash
BENCH_INDEX=marvindocs.db BENCH_DOCS=data/public/docs \
BENCH_OLLAMA=http://localhost:11434 \
go test -tags sqlite_fts5 -run 'TestBench$' -v ./internal/mcpserver/
```

- Тест пропускается, если не заданы `BENCH_INDEX` и `BENCH_DOCS`.
- `BENCH_OLLAMA` (Ollama с bge-m3) включает гибридный поиск; без него
  бенчмарк идёт только по лексике.
- `BENCH_WEIGHTS="lex:title,lex:title,..."` (например, `0.5:0,0.5:0.25`)
  прогоняет бенчмарк по одному разу для каждой пары весов.
- В индексе должны быть векторы для 12 док бенчмарка: ansible, docker,
  git, go, javascript, kubernetes, nginx, postgresql~17, python~3.13,
  react, rust, typescript.

## Построение векторов

`marvindocs index embed` считает эмбеддинги только для чанков без
вектора, поэтому его можно прервать (Ctrl-C) и запустить снова; каждый
батч коммитится отдельно.

```bash
marvindocs index build
marvindocs index embed --doc react --doc vue~3
marvindocs index embed                     # все доки индекса
```

| Флаг | По умолчанию | Примечания |
|---|---|---|
| `--doc` | все доки | Slug доки; можно повторять. |
| `--embedder` | `onnx` | `ollama` или `onnx`. |
| `--ollama-url` | `http://localhost:11434` | Базовый URL Ollama. |
| `--ollama-model` | `bge-m3` | Имя модели в Ollama. |
| `--model-dir` | `data/onnx/model` | onnx: каталог с `model_int8.onnx` и `tokenizer.json`. |
| `--onnxruntime` | `data/onnx/lib/libonnxruntime.so` | onnx: путь к `libonnxruntime.so`. |
| `--batch` | 64 | Чанков на вызов эмбеддера и на транзакцию. |
| `--docs-path`, `--index-path` | как у `index build` | |

Значения по умолчанию берутся из переменных окружения, флаги их
переопределяют: `MARVINDOCS_EMBEDDER` (`none`, `ollama`, `onnx`),
`MARVINDOCS_OLLAMA_URL`, `MARVINDOCS_OLLAMA_MODEL`, `MARVINDOCS_MODEL_DIR` и
`MARVINDOCS_ONNXRUNTIME`. `scripts/update.sh` загружает их из `.env`
(в git-игноре; шаблон - `.env.example`); см.
[Конфигурацию](configuration.md#переменные-окружения). `index embed` по
умолчанию использует `onnx` (локально на CPU, медленно: для всего дампа
часы), а `serve` по-прежнему по умолчанию `none`.

Измеренная скорость:

| Эмбеддер | Скорость | Примечания |
|---|---|---|
| Ollama, bge-m3 на GPU | ~110-130 чанков/с | Полный дамп devdocs (~5.2 млн чанков) считается ~12 ч и добавляет в индекс ~6.5 ГБ (~1.3 КБ на чанк); полный индекс в итоге занимает ~9 ГБ. |
| ONNX int8 на CPU | заметно медленнее | Один текст за раз; годится для нескольких док, но не для полного дампа. |

Поэтому массово считать векторы лучше через Ollama на машине с GPU, а для
`serve` рекомендуется ONNX: он локальный и не требует отдельного сервера.

### Ollama

```bash
ollama pull bge-m3
marvindocs index embed --doc react
```

## Запуск сервера с векторами

`serve` принимает те же флаги эмбеддера, `--embedder none|ollama|onnx`
(по умолчанию `none`).

```bash
marvindocs serve --http :3333 --embedder onnx \
    --model-dir /path/to/model --onnxruntime /path/to/libonnxruntime.so
```

С Ollama:

```bash
marvindocs serve --http :3333 --embedder ollama \
    --ollama-url http://localhost:11434 --ollama-model bge-m3
```

int8-модель ONNX занимает в процессе сервера около 1.1 ГБ ОЗУ.

## Сборка с ONNX

Для ONNX-эмбеддера нужна сборка с тегом `onnx`; библиотека токенизатора
линкуется статически через cgo. Без тега `--embedder onnx` завершится
ошибкой "built without onnx support (rebuild with -tags onnx)".

Образ Docker (`Dockerfile`, alpine) собирается без тега `onnx`, поэтому
`--embedder onnx` в нём недоступен: бинарники ONNX Runtime собраны под
glibc. В Docker используйте `--embedder ollama` или бинарь с хоста,
собранный через `make build-onnx`.

### Быстрый путь

```bash
make build-onnx    # onnx-fetch + bin/marvindocs с -tags "sqlite_fts5 onnx"
make run-onnx      # build-onnx + serve --http :3333 --embedder onnx ...
```

`make onnx-fetch` запускает `scripts/fetch-onnx.sh`, который скачивает
файлы в `data/onnx/` (каталог в git-ignore; путь можно переопределить
переменной окружения `ONNX_DIR`):

| Файл | Источник |
|---|---|
| `lib/libtokenizers.a` | daulet/tokenizers v1.27.0 |
| `lib/libonnxruntime.so` | ONNX Runtime 1.30.0 |
| `model/model_int8.onnx` | Xenova/bge-m3 (568 МБ) |
| `model/tokenizer.json` | Xenova/bge-m3 |

Скрипт идемпотентен: уже существующие файлы не перекачиваются. Он
поддерживает только Linux x86_64; на других платформах скачайте эти же
четыре файла вручную в ту же раскладку.

`make build-onnx` собирает в Docker, как и `make build`, с
`CGO_LDFLAGS=-L/src/data/onnx/lib`. `make run-onnx` запускает
`serve --http :3333 --embedder onnx --model-dir data/onnx/model
--onnxruntime data/onnx/lib/libonnxruntime.so`.

### Вручную

1. Скачайте `libtokenizers.a` для своей платформы из релизов
   https://github.com/daulet/tokenizers и положите в каталог, например `lib/`.
2. Скачайте архив релиза ONNX Runtime (v1.30 или совместимый) и возьмите
   из него `lib/libonnxruntime.so`.
3. Скачайте `onnx/model_int8.onnx` и `tokenizer.json` из
   https://huggingface.co/Xenova/bge-m3 в один каталог.
4. Соберите и запустите:

```bash
CGO_LDFLAGS="-L$PWD/lib" go build -tags "sqlite_fts5 onnx" -o marvindocs ./cmd/marvindocs
./marvindocs index embed --embedder onnx \
    --model-dir /path/to/model --onnxruntime /path/to/libonnxruntime.so
```

## Обновление векторов

`marvindocs index build` (по умолчанию инкрементальная синхронизация)
переиндексирует только новые или изменённые доки и удаляет их строки
вместе с векторами. У неизменённых док векторы сохраняются.

Изменение определяется по содержимому, а не только по mtime: индекс хранит
sha256 от `db.json` + `index.json` в `docs.content_hash`. Если mtime
изменился, а содержимое нет (типично после повторного запуска парсера),
обновляется только mtime, векторы сохраняются. Если содержимое изменилось,
дока пересобирается, и `index embed` считает её заново. В индексе от
старой версии колонка добавляется автоматически, а хеши заполняются при
первом `index build`.

`index embed` считает эмбеддинги только для чанков без вектора, поэтому
после каждого `index build` запускайте `index embed` (или `make embed`)
снова: посчитаются только изменённые доки.

`index build --full` удаляет всё, включая векторы, так что потребуется
полный пересчёт (~11 ч для полного дампа). Чтобы не сделать это случайно,
команда отказывается работать, если в индексе есть векторы, и печатает
ошибку; для подтверждения добавьте `--drop-vectors`. Вместо этого
используйте обычный `index build`.

`make update` (`scripts/update.sh`) выполняет весь цикл, никогда не
удаляя векторы: синхронизация, парсеры, инкрементальный `index build`,
`index embed`. См. [Синхронизацию данных](sync-docs.md#обновление-одной-командой).

Дока, в которой часть чанков уже с векторами, а часть ещё нет, считается в
`get_docs` докой с векторами. Доведите `index embed` до конца, прежде чем
полагаться на гибридные результаты по такой доке.

## Развёртывание на другой машине

```bash
scripts/sync-docs.sh     # скачать доки
make index               # построить индекс
make embed               # посчитать эмбеддинги; для полного дампа часы
make run-onnx            # запустить сервер с ONNX-эмбеддером
```

`make embed` выполняет `scripts/update.sh --embed-only`, то есть
`index embed` с эмбеддером из `.env`. По умолчанию это локальный ONNX; чтобы
использовать сервер Ollama с bge-m3 (на GPU заметно быстрее), добавьте в
`.env`:

```bash
MARVINDOCS_EMBEDDER=ollama
MARVINDOCS_OLLAMA_URL=http://host:11434
```

Вместо пересчёта можно просто скопировать файл индекса `marvindocs.db` с
векторами на другую машину.
