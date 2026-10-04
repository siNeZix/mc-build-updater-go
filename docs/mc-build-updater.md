# Mc-build-updater (client)

Windows CLI синхронизирует плоский каталог `mods/` с выбранной веткой Minecraft-сборки, обновляет свой `.exe` из GitHub/GitLab Releases и публикует моды через REST API.

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
| 1 | Dev (`--dev`): всегда `http://localhost:1447/` |
| 2 | Production: `--file-hosting-url <URL>` |
| 3 | Production default: `http://mc.sinezix.ru:1447/` |

`MC_BU_FILE_HOSTING_URL` удалён. В `--dev` `--file-hosting-url` намеренно игнорируется: клиент не может обратиться к production server. Локальные HTTP-запросы ограничены 15 секундами; self-update, очистка файлов self-update и трёхсекундная стартовая пауза отключены. Ошибка отдельной параллельной загрузки не останавливает очередь работ, поэтому sync и REST-публикация не зависают.

## Self-update

Перед синхронизацией модов production-клиент проверяет latest stable release (`vX.Y.Z`) в публичных репозиториях:

1. `https://github.com/siNeZix/mc-build-updater-go/releases`;
2. `https://gitlab.com/siNeZix/mc-build-updater-go/-/releases` — только если GitHub недоступен, вернул некорректный ответ или release без нужных файлов.

Release обязан содержать `mc-build-updater.exe` и `checksums.txt`. Перед заменой клиент проверяет SHA-256 из `checksums.txt`.

- Учитываются только стабильные SemVer-теги формата `vX.Y.Z`; draft, prerelease и некорректные теги игнорируются.
- При найденной более новой версии обновление обязательно: текущий процесс не синхронизирует моды, запускает временный процесс замены и завершается. Новая версия запускается сама и продолжает работу.
- Если обе площадки недоступны, текущая версия продолжает синхронизацию модов.
- В `--dev` self-update отключён. Сборка без release-тега имеет версию `dev` и также не проверяет обновления.
- Версия release передаётся в бинарник через `-ldflags "-X main.version=vX.Y.Z"`.

### Публикация release

В GitHub и GitLab настроены независимые CI. После push тега `vX.Y.Z` каждая площадка выполняет тесты, собирает `windows/amd64` бинарник, создаёт `checksums.txt` и публикует stable release.

Для GitLab в настройках CI/CD добавь защищённую переменную `GITLAB_RELEASE_TOKEN`: персональный access token с областью `api`. В GitHub workflow по умолчанию использует `github.token`; если репозиторий или правила организации не позволяют ему создавать release, добавь секрет `RELEASE_TOKEN` с правом `contents: write`.

Push тега нужен в оба remote:

```powershell
git push origin v1.2.3
git push gitlab v1.2.3
```

`apps/file-hosting` больше не хранит `config/remote.json`, архив обновления или `7z.exe`.

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

Для локальной публикации запусти server с тем же `FILE_HOSTING_TOKEN`, затем:

```powershell
.\build\mc-bu-utils.exe upload-mods --dev --workers 8
```

`--dev` всегда использует `http://localhost:1447/` и игнорирует `--file-hosting-url`; токен остаётся обязательным, чтобы локальный режим не ослаблял авторизацию REST API.

Команда сканирует только прямые регулярные файлы в `mods/`, получает `/map`, вычисляет SHA-1 и отправляет `PUT /api/files/mods/<name>` только для новых или изменённых файлов. Удаления на server не выполняет. Токен обязателен и передаётся как Bearer.

SFTP, SSH-ключи и переменные `MC_BU_SFTP_*` не используются.
