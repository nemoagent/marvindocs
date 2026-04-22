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
| `--all` | Перекачать все slug'и из `data/public/docs/docs.json`. |
| `--no-index` | Не перестраивать `marvindocs.db` после загрузки. |
| `--jobs N` | Параллельных загрузок (по умолчанию `4`). |

### Примеры

```bash
# Несколько конкретных доков
./scripts/sync-docs.sh react vue~3 typescript python~3.13

# Обновить всё из текущего docs.json
./scripts/sync-docs.sh --all

# Скачать без перестройки индекса
./scripts/sync-docs.sh --all --no-index
./bin/marvindocs index build      # построить индекс вручную позже
```

## Что делает скрипт

1. Составляет список slug'ов (встроенный, `--all` из существующего
   `docs.json`, или указанные вручную).
2. Параллельно скачивает `https://downloads.devdocs.io/<slug>.tar.gz`
   через `curl` и распаковывает в `data/public/docs/<slug>/`.
   Неудачные slug'и (404/403) выводятся и пропускаются, скрипт не падает.
3. Пересобирает `data/public/docs/docs.json` из файлов `meta.json`
   каждого скачанного slug'а с помощью Python.
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
`react~18`. Посмотреть что уже есть:

```bash
python3 -c "import json; [print(d['slug']) for d in json.load(open('data/public/docs/docs.json'))]"
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
