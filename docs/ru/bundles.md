# Оффлайн-наборы

Набор (bundle) — готовая часть документации: страницы доков вместе с
поисковым индексом и эмбеддингами. Вместо загрузки всего devdocs
(~17 ГБ), сборки индекса и эмбеддинга чанков (часы без GPU) вы ставите
только нужные наборы и сразу запускаете сервер. Гибридный поиск в
`get_docs` работает сразу, эмбеддинг не нужен.

Наборы публикуются файлами GitHub-релиза
[`bundles`](https://github.com/nemoagent/marvindocs/releases/tag/bundles)
репозитория `nemoagent/marvindocs`.

## Доступные наборы

| Набор | Доков | Архив | Состав |
|---|---|---|---|
| `core` | 10 | 199 МБ | bash, zsh, fish, git, http, man, markdown, jq, gnu_make, cmake |
| `go` | 3 | 13 МБ | go, go-std, go-dev |
| `python` | 26 | 300 МБ | python, django, flask, fastapi, numpy, pandas, pytorch, ... |
| `db` | 15 | 91 МБ | postgresql, mariadb, sqlite, redis, mongodb, duckdb, ... |
| `infra` | 23 | 336 МБ | nginx, haproxy, docker, kubernetes, terraform, ansible, ... |
| `js` | 80 | 287 МБ | javascript, typescript, node, react, vue, angular, css, html, ... |
| `php` | 13 | 230 МБ | php, composer, phpunit, laravel, symfony, wordpress, ... |
| `ruby` | 8 | 23 МБ | ruby, rails, rack, minitest, nokogiri, ... |
| `jvm` | 16 | 697 МБ | openjdk, kotlin, scala, groovy, clojure, spring_boot, ... |
| `native` | 18 | 384 МБ | c, cpp, gcc, rust, ... |
| `other` | 73 | 308 МБ | все доки, не попавшие в другие наборы |

Все 11 наборов: 285 доков, ~2,9 ГБ архивов. Точные списки — в
[`bundles.yaml`](../../bundles.yaml); размеры по сборке 2026-10-29.

### Какие версии входят

Для каждого перечисленного дока в набор попадают:

- новейшая версия (`python~3.14`, а не `python~3.13`);
- поддерживаемые LTS-релизы из списка `lts` в `bundles.yaml`
  (openjdk 8/11/17/21, node 22/24, django 5.2, haproxy 2.6-3.2,
  julia 1.10);
- варианты без версии (`haxe~cpp`, `rethinkdb~java`) и новейшие версии
  каждого варианта (`gcc~14` и `gcc~14_cpp`).

Старых версий в наборах нет; их можно получить через
[`scripts/sync-docs.sh`](sync-docs.md) и `index build`.

## Установка

Нужны только бинарник `marvindocs` и доступ в интернет. Репозиторий
публичный, поэтому GitHub CLI, вход и токен не нужны.

```bash
marvindocs bundle install core go python
marvindocs serve --http :3333
```

Что делает `install` для каждого набора:

1. скачивает `SHA256SUMS` и `NAME.tar.gz` по HTTPS из
   `https://github.com/nemoagent/marvindocs/releases/download/bundles/` во временный каталог и сверяет контрольную сумму архива
   (имя, которого нет в `SHA256SUMS`, или несовпадение прерывают установку);
2. распаковывает `docs/<slug>/` в `--docs-path` (`data/public/docs`),
   заменяя существующие каталоги тех же доков;
3. добавляет доки в `docs.json` (записи с тем же slug заменяются);
4. сливает индекс набора в `--index-path` (`marvindocs.db`): доки с тем
   же slug заменяются, остальные сохраняются; полнотекстовый индекс
   заполняется из текстов чанков, без конвертации HTML и эмбеддинга.

Файл индекса создаётся, если его нет. Повторная установка набора
обновляет его на месте. Запущенный `serve` новых доков не видит —
перезапустите его.

Оффлайн-установка из каталога со скачанными архивами:

```bash
mkdir -p /tmp/b && cd /tmp/b
curl -LO https://github.com/nemoagent/marvindocs/releases/download/bundles/go.tar.gz -LO https://github.com/nemoagent/marvindocs/releases/download/bundles/SHA256SUMS
sha256sum -c --ignore-missing SHA256SUMS
marvindocs bundle install --from /tmp/b go
```

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--repo` | `nemoagent/marvindocs` | GitHub-репозиторий с релизом; адрес загрузки `https://github.com/REPO/releases/download/TAG/`. |
| `--tag` | `bundles` | Тег релиза. |
| `--from` | (пусто) | Каталог с `NAME.tar.gz`; без загрузки. |
| `--docs-path` | `data/public/docs` | Каталог данных. |
| `--index-path` | `marvindocs.db` | Файл индекса SQLite. |

Наборы сочетаются с другими способами получения доков: после установки
инкрементальный `index build` сохраняет доки наборов и их векторы, пока
их файлы не менялись, а `sync-docs.sh` + `index build` + `index embed`
могут добавить доки, которых нет ни в одном наборе.

Для Docker ставьте наборы в смонтированные каталог данных и файл индекса
на хосте, затем перезапустите контейнер.

## Сборка и публикация (для мейнтейнеров)

Наборы собираются из полной локальной установки, где все доки наборов
скачаны, проиндексированы и имеют векторы.

```bash
make update                        # или sync-docs.sh + index build + index embed
marvindocs bundle build            # все наборы -> dist/bundles/NAME.tar.gz
marvindocs bundle build go js      # только эти
```

`build` отказывается собирать док, который не проиндексирован или
изменился после последнего `index build` (его чанки не совпали бы с
сохранёнными векторами): сначала выполните `index build` и `index embed`.
Сборка всех наборов занимает около 12 минут. Доки без векторов
упаковываются как есть; в установленном индексе поиск по ним только
лексический.

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--config` | `bundles.yaml` | Описание наборов. |
| `--out` | `dist/bundles` | Каталог результата (`dist/` в git-ignore). |
| `--docs-path` | `data/public/docs` | Каталог данных. |
| `--index-path` | `marvindocs.db` | Полный индекс SQLite. |

Публикация (заменяет существующие файлы):

```bash
cd dist/bundles
sha256sum *.tar.gz > SHA256SUMS
gh release upload bundles -R nemoagent/marvindocs SHA256SUMS *.tar.gz --clobber
```

Большая загрузка может оборваться на середине: проверьте файлы командой
`gh release view bundles -R nemoagent/marvindocs` и догрузите
недостающие по одному.

### Правка `bundles.yaml`

```yaml
lts:                 # релизы, сохраняемые помимо новейшей версии
  - node~24_lts
bundles:
  go: [go, go-std, go-dev]   # имена доков: часть slug до "~"
```

Имя дока может быть только в одном наборе; неперечисленные доки идут в
`other`. При пересборке сверяйте список `lts` с <https://endoflife.date>.

## Формат архива

`NAME.tar.gz` содержит:

| Путь | Содержимое |
|---|---|
| `bundle.db` | Индекс SQLite доков набора: `docs`, `entries`, `pages`, `chunks`, `chunk_vecs` и `chunk_text` (breadcrumb и текст каждого чанка; из него при слиянии заполняется contentless-таблица `chunks_fts`). |
| `docs.json` | Записи `docs.json` для доков набора. |
| `docs/<slug>/...` | Каталоги доков, см. [Формат данных](data-format.md). |
