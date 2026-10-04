# Mc-build-updater (client)

Windows CLI синхронизирует плоский каталог `mods/` с выбранной веткой Minecraft-сборки, обновляет свой `.exe` и публикует моды через REST API.

Это отдельный Go-модуль. Зависимости не переносятся в `file-hosting`.

## Пакеты

| Путь | Ответственность |
| --- | --- |
| `cmd/mc-build-updater/` | Основной CLI и sync/self-update. |
| `cmd/mc-bu-utils/` | REST-команда `upload-mods`. |
| `internal/config/` | Локальный YAML-конфиг ветки. |
| `internal/remote/` | HTTP-клиент, обычные и Range-загрузки. |
| `internal/modsync/` | Локальная SHA-1 карта, удаление и загрузка модов. |
| `internal/uploader/` | Сравнение `/map` и REST-публикация модов. |

## URL file-hosting

| Приоритет | Значение |
| --- | --- |
| 1 | `--file-hosting-url <URL>` |
| 2 | Dev: `http://localhost:1447/`; production: `http://mc.sinezix.ru:1447/` |

`MC_BU_FILE_HOSTING_URL` удалён.

## Синхронизация

Основной клиент запрашивает только нужную карту:

```text
GET /map?dir=mods&branch=<Branch>
```

Локальные файлы, hash которых не входят в ответ, удаляются. Недостающие скачиваются максимум четырьмя файлами одновременно.

- файл до 2 MiB включительно — один GET;
- файл больше 2 MiB — блоки Range по 2 MiB;
- один большой файл качает до четырёх блоков одновременно, независимо от лимита в четыре файла;
- если server/proxy не поддерживает `206`, клиент откатывается к обычному GET;
- перед атомарной заменой вычисляется SHA-1 и сверяется с картой.

Вложенные каталоги `mods/` не поддерживаются.

## Публикация модов

```powershell
cd apps/mc-build-updater
$env:FILE_HOSTING_TOKEN = '...'
.\build\mc-bu-utils.exe upload-mods --file-hosting-url http://host:1447/ --workers 8
```

Команда сканирует только прямые регулярные файлы в `mods/`, получает `/map`, вычисляет SHA-1 и отправляет `PUT /api/files/mods/<name>` только для новых или изменённых файлов. Удаления на server не выполняет. Токен обязателен и передаётся как Bearer.

SFTP, SSH-ключи и переменные `MC_BU_SFTP_*` не используются.
