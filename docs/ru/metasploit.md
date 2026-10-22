# Документация Metasploit Framework

Парсер `metasploit` импортирует текущую официальную документацию с
[docs.metasploit.com](https://docs.metasploit.com/) под постоянным slug
`metasploit`. Источник — sitemap и готовые HTML-страницы документации.
Парсер не запускает Metasploit и не импортирует всё дерево исходников модулей.

```bash
make parsers
./scripts/run-parser.sh metasploit
./bin/marvindocs index build
./bin/marvindocs search msfconsole --doc metasploit
./bin/marvindocs get metasploit index
```

После сборки бинаря парсер участвует в `make update`. Индексация
инкрементальная, эмбеддинги других документов сохраняются. Для новой
документации их можно построить командой
`./bin/marvindocs index embed --doc metasploit` с настроенным бэкендом.

В `data/public/docs/metasploit/` создаются `meta.json`, `index.json`,
`db.zip` и `.last-modified`. Скрипт запуска добавляет `.parser`, защищающий
импортированные страницы от перезаписи синхронизацией DevDocs.

Парсер сохраняет основной текст, примеры кода, таблицы и якоря заголовков,
удаляет навигацию сайта и футеры, преобразует ссылки между импортированными
страницами в относительные локальные ссылки. Изображения и ссылки вне
набора остаются сетевыми. Ошибка загрузки или отсутствие содержимого
останавливает импорт до записи результата, сохраняя предыдущий набор.

Прямой запуск:

```bash
./bin/metasploit-parser --out data/public/docs/metasploit --force
```

`--source` заменяет корневой URL сайта: зеркало должно предоставлять
`sitemap.xml` и ту же структуру HTML. `--version` и `--release` задают
только метки в метаданных, а не выбирают историческую версию.
По умолчанию импортируется текущий сайт без номера версии.

Исходники: `parsers/metasploit/`. Тесты: `make parsers-test`.
