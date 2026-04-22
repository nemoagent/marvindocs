# Синхронизация данных документации

marvindocs не поставляется с документацией devdocs. Вспомогательный
скрипт `scripts/sync-docs.sh` скачивает её напрямую с
`downloads.devdocs.io`, используя только `curl` и `tar` — без Docker,
без Ruby, без клона devdocs.

## Требования

- `curl`
- `tar`

Всё. Ни Python, ни Docker, ни Ruby — обе утилиты есть на любом Linux,
macOS и WSL из коробки.

## Быстрый старт

```bash
make build                    # собрать бинарь marvindocs (один раз)
./scripts/sync-docs.sh        # скачать ~29 популярных доков, перестроить индекс
./bin/marvindocs serve --http :3333
```

## Использование

```
./scripts/sync-docs.sh [DOCS...] [OPTIONS]
```

| Аргумент / опция | Значение |
|---|---|
| *(ничего)* | Скачать встроенный популярный набор (~29 доков). |
| `react`, `python~3.13`, … | Только указанные slug'и. |
| `--all` | Обработать все slug'и из `data/public/docs/docs.json`. |
| `--check` | Dry-run: вывести таблицу up-to-date / outdated / missing и выйти, ничего не скачивая. |
| `--force` | Игнорировать локальный mtime — всегда перекачать запрошенные slug'и. |
| `--no-index` | Не перестраивать `marvindocs.db` после загрузки. |
| `--jobs N` | Параллельных загрузок (по умолчанию `4`). |
| `--list-remote` | Сходить в живой каталог devdocs.io и напечатать все доступные slug'и с именем и версией. Ничего не скачивает, индекс не трогает. |
| `--search-remote QUERY` | То же, что `--list-remote`, но оставляет только строки, где `slug` или `name` содержит `QUERY` (регистронезависимо). |

### Примеры

```bash
# Несколько конкретных доков
./scripts/sync-docs.sh react vue~3 typescript python~3.13

# Просто посмотреть, что устарело — без скачивания
./scripts/sync-docs.sh --check --all

# Обновить всё, что уже лежит на диске (неизменившееся пропустится через HTTP 304)
./scripts/sync-docs.sh --all

# Перекачать всё с нуля, даже если ничего не изменилось
./scripts/sync-docs.sh --all --force

# Скачать без перестройки индекса
./scripts/sync-docs.sh --all --no-index
./bin/marvindocs index build      # построить индекс вручную позже

# Посмотреть удалённый каталог перед скачиванием
./scripts/sync-docs.sh --list-remote | head
./scripts/sync-docs.sh --search-remote kubernetes
```

## Проверка обновлений

Каждый архив devdocs на сервере помечен HTTP-заголовком `Last-Modified`.
После успешного скачивания скрипт создаёт файл-метку
`data/public/docs/<slug>/.last-modified`, чей fs-mtime равен серверному
`Last-Modified`. На следующем запуске:

- `curl --time-cond <marker>` передаёт mtime метки как
  `If-Modified-Since`.
- Если архив не перезаливали — сервер возвращает **304 Not Modified**,
  скрипт печатает `SKIP <slug> (up-to-date)` — передано ноль байт
  сверх заголовков.
- Если архив свежее — качаем, распаковываем, пишем `UPDATED <slug>`
  и обновляем метку.
- `--force` отключает проверку — всегда качать заново.
- `--check` только печатает таблицу статусов и выходит, ничего не
  трогая.

Почему файл-метка, а не поле `mtime` из `meta.json`? `meta.json mtime`
— это время *сборки* на стороне devdocs, которое может отличаться от
времени заливки на S3 на несколько секунд. Если сравнивать с ним,
каждый документ окажется в вечном OUTDATED. Метка использует те же
часы, что и сервер, поэтому сравнение точное.

Это значит, что прогонять скрипт ежедневно/еженедельно дёшево: один
HEAD-подобный запрос на slug — и всё, если upstream не менялся.

## Что делает скрипт

1. Составляет список slug'ов (встроенный, `--all` из существующего
   `docs.json`, или указанные вручную).
2. Параллельно скачивает `https://downloads.devdocs.io/<slug>.tar.gz`
   через `curl` и распаковывает в `data/public/docs/<slug>/`.
   Неудачные slug'и (404/403) выводятся и пропускаются, скрипт не падает.
3. Пересобирает `data/public/docs/docs.json` из файлов `meta.json`
   каждого скачанного slug'а — чистый bash, без Python.
4. *(Если не передан `--no-index`)* Запускает `./bin/marvindocs index build`.

## Встроенный популярный набор

При запуске без аргументов скрипт скачивает:

```
angular  bash      c         cpp       css        django
docker   go        html      http      javascript  jinja
jquery   kubernetes linux    markdown  nginx       node
npm      postgresql python~3.13 react  redis      ruby
rust     sqlite    typescript  vue~3
```

Чтобы изменить набор — отредактируй `DEFAULT_SLUGS` в начале файла
`scripts/sync-docs.sh`.

## Как найти нужный slug

Каждое название на <https://devdocs.io> — это slug для скачивания.
Версионные варианты используют тильду: `python~3.12`, `angular~18`,
`react~18`.

Смотреть живой каталог можно прямо из скрипта — браузер не нужен:

```bash
# Все ~794 доков
./scripts/sync-docs.sh --list-remote

# Grep по slug или name (регистронезависимо)
./scripts/sync-docs.sh --search-remote python
./scripts/sync-docs.sh --search-remote "vue"
./scripts/sync-docs.sh --search-remote rust

# Пайп в shell
./scripts/sync-docs.sh --list-remote | awk 'NR>1 {print $1}' | sort > all-slugs.txt
```

Каталог берётся из `https://devdocs.io/docs.json` в момент запроса —
данные всегда актуальные.

Посмотреть, что уже лежит локально:

```bash
grep -o '"slug":"[^"]*"' data/public/docs/docs.json | cut -d'"' -f4
```

## Повторная синхронизация / обновление

Скрипт идемпотентен — повторный запуск перезаписывает файлы.
Чтобы обновить только несколько доков:

```bash
./scripts/sync-docs.sh react vue~3
```

## Место на диске

| Набор | Примерный размер | Время скачивания (100 Мбит) |
|---|---|---|
| Встроенный (~29 доков) | ~1–2 ГБ | ~2–5 мин |
| Все (~790 доков) | ~17 ГБ | ~30–90 мин |
