# Mc-build-updater (client)

## Назначение

`apps/mc-build-updater` — Windows CLI-клиент для обновления Minecraft-сборки. Он сравнивает локальные моды с картой выбранной ветки, удаляет устаревшие файлы и скачивает недостающие. В production также поддерживает обновление собственного `.exe` и отдельную SFTP-утилиту публикации модов.

Это отдельный Go-модуль. Не переноси его зависимости в `apps/file-hosting` и не запускай команды `go` для него из корня monorepo.

## Точки входа и пакеты

| Путь | Ответственность |
| --- | --- |
| `cmd/mc-build-updater/main.go` | Основной CLI, режимы, выбор server URL, запуск sync/self-update. |
| `cmd/mc-bu-utils/main.go` | CLI SFTP-утилиты `upload-mods`. |
| `internal/config/` | Чтение или создание YAML-конфига ветки. |
| `internal/branch/` | Текстовое представление известных веток. |
| `internal/remote/` | HTTP-клиент file-hosting, JSON и атомарная запись загрузок. |
| `internal/modsync/` | SHA-1 локальных модов, сравнение карт, удаление и параллельные загрузки. |
| `internal/selfupdate/` | Двухэтапный Windows self-update через 7-Zip. |
| `internal/uploader/` | SFTP-поиск файлов по имени/размеру и параллельная выгрузка. |

## Запуск и режимы основного CLI

| Команда или флаг | Поведение |
| --- | --- |
| `make dev-client` | Air запускает `mc-build-updater --dev`; URL server = `http://localhost:1447/`, self-update отключён. |
| `make start-client` | Собирает и запускает production `.exe`. |
| `--file-hosting-url <URL>` | Явно задаёт URL file-hosting; приоритет выше env. |
| `MC_BU_FILE_HOSTING_URL` | Задаёт URL file-hosting; приоритет выше default, ниже флага. |
| `--mm` | Создаёт `mm.json` из SHA-1 локальных файлов в `mods/`; не выполняет sync. |
| `--updated` | Удаляет временные self-update артефакты перед обычным запуском. |
| `--apply-update` | Внутренний режим копии `update.exe`: распаковывает update архив и запускает свежий client. Не предназначен для ручного использования. |

Production default URL: `http://mc.sinezix.ru:1447/`.

## Рабочий каталог и runtime-файлы

Client всегда работает относительно текущего процесса. Для Make-команд это `apps/mc-build-updater/`.

| Путь | Назначение | Git |
| --- | --- | --- |
| `mc-mods-updater.config.yml` | Конфигурация выбранной ветки. | ignore |
| `mods/` | Локальная Minecraft-сборка пользователя. | ignore |
| `mc-mods-updater.log` | Лог основного CLI. | ignore |
| `mm.json` | Результат `--mm`. | ignore |
| `7z.exe` | Загруженный helper для self-update. | ignore |
| `mc-build-updater.7z`, `update.exe` | Временные артефакты self-update. | ignore |
| `build/*.exe` | Результат `make build-client`. | ignore |

### Конфиг ветки

При первом обычном запуске создаётся `mc-mods-updater.config.yml`:

```yaml
Branch: dead-inside-land
```

Чтобы сменить сборку, измени значение `Branch` на имя JSON-файла без `.json` из `apps/file-hosting/files/MM/`. Для ветки `neko-land` необходим файл `MM/neko-land.json` на server.

## Алгоритм синхронизации

1. Создаёт `mods/`, если каталога нет.
2. Считывает `Branch` из локального YAML.
3. Загружает `MM/<branch>.json` и `/map` с file-hosting.
4. Вычисляет SHA-1 всех обычных файлов **только первого уровня** `mods/`.
5. Удаляет локальные файлы, которых нет по SHA-1 в карте ветки.
6. Для отсутствующих SHA-1 ищет запись в `/map`.
7. Скачивает файлы по `/mods/<hash>` не более чем в 10 параллельных worker-ах.
8. Пишет каждый download во временный файл, затем переименовывает его в итоговый файл.

Подробный внешний формат: [contracts.md](contracts.md).

### Важные следствия

- Актуальность определяется SHA-1, не именем файла.
- Два файла с разными именами, но одинаковым SHA-1, считаются одной и той же версией содержимого.
- Мод, указанный в `MM/<branch>.json`, но отсутствующий в `/map`, — ошибка sync.
- Вложенные подкаталоги внутри локального `mods/` игнорируются при вычислении карты и не удаляются.

## Self-update

Self-update запускается только без `--dev` и только на Windows.

1. Client скачивает `/static/7z_x32.exe` как `7z.exe`, если helper отсутствует.
2. Запрашивает `/config/remote.json`.
3. Если `LastVersion` отличается от встроенной версии (`alpha.4`), скачивает `/static/mc-build-updater.7z`.
4. Копирует текущий `.exe` в `update.exe` и запускает копию с `--apply-update`.
5. `update.exe` ждёт 100 ms, распаковывает архив командой `7z.exe x <archive> -y -p123` и запускает `mc-build-updater.exe --updated`.
6. Новый process удаляет временные файлы.

Для выпуска новой client-версии нужно синхронно:

1. Изменить константу `version` в `cmd/mc-build-updater/main.go`.
2. Собрать архив `mc-build-updater.7z`, содержащий `mc-build-updater.exe`.
3. Разместить его в `apps/file-hosting/files/static/mc-build-updater.7z`.
4. Обновить `apps/file-hosting/files/config/remote.json` с новым `LastVersion`.
5. Запустить `GET /map/update` после публикации файлов.

Нельзя поднять `LastVersion` раньше публикации архива: все старые clients начнут запрос self-update.

## SFTP: `mc-bu-utils upload-mods`

Собери утилиту:

```powershell
make build-client
```

Запусти из `apps/mc-build-updater/`:

```powershell
.\build\mc-bu-utils.exe upload-mods
.\build\mc-bu-utils.exe upload-mods 12
```

Утилита получает список удалённых файлов, сравнивает его с локальным каталогом по имени и размеру, затем выгружает отсутствующие или отличающиеся файлы параллельно. Она **не** обновляет `MM/<branch>.json` и не вызывает `/map/update`: эти шаги выполняются отдельно после upload.

| Переменная | Default | Назначение |
| --- | --- | --- |
| `MC_BU_SFTP_HOST` | `mc.sinezix.ru` | SFTP host. |
| `MC_BU_SFTP_PORT` | `22` | SFTP port. |
| `MC_BU_SFTP_USER` | `root` | SFTP user. |
| `MC_BU_SFTP_KEY_PATH` | `%USERPROFILE%\.ssh\id_rsa` | Путь к private key. |
| `MC_BU_MODS_PATH` | `<working directory>\mods` | Локальный каталог модов. |
| `MC_BU_SFTP_REMOTE_MODS_PATH` | `/root/file-hosting/files/mods/` | Удалённый каталог модов. |

Пример PowerShell:

```powershell
$env:MC_BU_SFTP_HOST = 'mc.sinezix.ru'
$env:MC_BU_SFTP_KEY_PATH = "$HOME\.ssh\id_rsa"
$env:MC_BU_MODS_PATH = 'D:\Games\Minecraft\mods'
.\build\mc-bu-utils.exe upload-mods 8
```

Private key и другие секреты не коммитить, не логировать и не добавлять в конфигурацию репозитория.

> Реализация сохраняет legacy-поведение и не проверяет SFTP host key (`ssh.InsecureIgnoreHostKey`). Не расширяй это исключение на новые сетевые интеграции. Для улучшения безопасности потребуется отдельное согласование и known-hosts contract.

## Проверки при изменении client

```powershell
make fmt-client
make test-client
cd apps/mc-build-updater; go build ./...
cd apps/mc-build-updater; go vet ./...
```

При правке `remote`, `modsync` или self-update также обнови [contracts.md](contracts.md), если изменился какой-либо внешний URL, JSON или порядок взаимодействия.
