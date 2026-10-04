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
2. Топ чанков по косинусному сходству (полный перебор векторов одной
   доки, кэшируются в памяти при первом обращении) и топ чанков по BM25
   (тот же лексический поиск по чанкам, что и в `search_pages`)
   объединяются через Reciprocal Rank Fusion: k=60, вес лексики 0.5, вес
   векторов 1.
3. Страницы ранжируются по лучшему объединённому чанку.

Доки без векторов, а также инструменты `search` и `search_pages`
по-прежнему используют лексический поиск BM25 без изменений.

Бенчмарк: 32 вопроса разработчиков по 5 докам (postgresql, python, go,
ansible, react):

| Метод | hit@1 | hit@3 |
|---|---|---|
| Только BM25 | 25/32 | 29/32 |
| Только векторы | 24/32 | 28/32 |
| Гибрид (RRF) | 30/32 | 31/32 |

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
| `--embedder` | `ollama` | `ollama` или `onnx`. |
| `--ollama-url` | `http://localhost:11434` | Базовый URL Ollama. |
| `--ollama-model` | `bge-m3` | Имя модели в Ollama. |
| `--model-dir` | | onnx: каталог с `model_int8.onnx` и `tokenizer.json`. |
| `--onnxruntime` | | onnx: путь к `libonnxruntime.so`. |
| `--batch` | 64 | Чанков на вызов эмбеддера и на транзакцию. |
| `--docs-path`, `--index-path` | как у `index build` | |

Измеренная скорость:

| Эмбеддер | Скорость | Примечания |
|---|---|---|
| Ollama, bge-m3 на GPU | ~110-130 чанков/с | Полный дамп devdocs (~5.2 млн чанков) считается ~11 ч и добавляет в индекс ~5 ГБ (~1 КБ на чанк). |
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
линкуется статически через cgo:

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

Без тега `onnx` `--embedder onnx` завершится ошибкой "built without onnx
support (rebuild with -tags onnx)".
