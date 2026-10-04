# File-hosting (server)

`apps/file-hosting` публикует файлы по HTTP, хранит карту файлов в SQLite и предоставляет защищённую REST-запись. Это отдельный Go-модуль.

## Конфигурация

| Настройка | CLI/env | Default |
| --- | --- | --- |
| Адрес | `-addr` / `ADDR` | `:1447` |
| Каталог файлов | `-files-path` / `FILES_PATH` | `files` (production `/data`) |
| Токен записи | `FILE_HOSTING_TOKEN` | пусто, запись отключена |

Production Compose передаёт `FILE_HOSTING_TOKEN` из окружения host.

## SQLite-карта

База находится рядом с публикуемыми файлами: `files/.file-hosting.sqlite` (в контейнере `/data/.file-hosting.sqlite`). Она хранит SHA-1, путь, имя, каталог, размер и версию карты.

При старте server рекурсивно сканирует `FILES_PATH` и синхронизирует базу. Ручные изменения в работающем каталоге подхватываются через `GET /map/update`. SQLite, `-wal` и `-shm` не публикуются.

REST PUT/DELETE сразу обновляют карту. Идентичный PUT не меняет версию. Порядок `/map` детерминирован: абсолютный путь.

## HTTP API

Полный публичный контракт, фильтры `/map`, Range-выдача и защищённые `PUT`/`DELETE` описаны в [contracts.md](contracts.md).

## Запуск и проверка

```powershell
make dev-server
Invoke-RestMethod http://localhost:1447/map
Invoke-WebRequest http://localhost:1447/map/update
```

Для записи задай `FILE_HOSTING_TOKEN` до запуска server и передай тот же токен в `Authorization: Bearer ...`.
