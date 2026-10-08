# Разработка, проверки и диагностика

## Требования

| Инструмент | Назначение | Проверка |
| --- | --- | --- |
| Go 1.26+ | сборка, тесты, Air | `go version` |
| GNU Make | единый интерфейс | `make --version` |
| Docker Desktop | только ручной запуск Compose server | `docker version` |

`air` отдельно устанавливать не нужно: `make dev-s` и `make dev-c` запускают его через `go run github.com/air-verse/air@latest`. Первый запуск скачает зависимость в Go module cache.

Цели `make` загружают переменные из `.env` в корне репозитория. При прямом запуске собранный `.exe` загружает `.env` рядом с собой. Шаблон: `.env.example`.

## Ежедневный workflow

### Server

```powershell
make dev-s
```

- Запускается **нативно**, слушает `http://localhost:1447`.
- Air наблюдает только за файлами `.go`.
- Содержимое `apps/file-hosting/files/` не запускает перекомпиляцию; после изменения файлов обнови карту запросом `GET /map/update`.

Проверка:

```powershell
Invoke-RestMethod http://localhost:1447/map
Invoke-WebRequest http://localhost:1447/map/update
```

### Client

Сначала запусти server, затем отдельным терминалом:

```powershell
make dev-c
```

- Запускается **нативно** с флагом `--dev`.
- В dev всегда использует `http://localhost:1447/`, даже если передан `--file-hosting-url`; self-update, его очистка и стартовая пауза отключены, HTTP timeout — 15 секунд.
- Working directory — `apps/mc-build-updater`; там создаются runtime-файлы и каталоги `mods/`, `resourcepacks/`, `shaderpacks/`.
- `dev-c` запускай после `dev-s`: клиент выполняет синхронизацию и завершается, а Air остаётся наблюдать за исходниками для следующего запуска после их изменения.

## Тесты и статический анализ

| Область | Команда |
| --- | --- |
| Server | `make test-s` |
| Client | `make test-c` |
| Оба модуля | `make test` |
| Форматирование | `make fmt` |
| Дополнительная проверка server | `cd apps/file-hosting; go vet ./...` |
| Дополнительная проверка client | `cd apps/mc-build-updater; go vet ./...` |

Перед передачей изменений выполни минимум:

```powershell
make fmt
make test
git diff --check
```

Для затронутого приложения также выполни:

```powershell
go build ./...
go vet ./...
```

Запускай эти две команды из директории соответствующего модуля, а не из корня.

## Сборка и запуск

### Server

```powershell
make build-s
make start-s
make start-s-dev
```

- `build-s` создаёт `build/server/file-hosting.exe`.
- `start-s` всегда сначала выполняет `build-s`, затем запускает binary с working directory `apps/file-hosting`.
- У server нет флага `--dev`, поэтому `start-s-dev` равнозначна `start-s`: нативный запуск без Air.
- Server слушает порт `1447` и использует публикуемые файлы из `apps/file-hosting/files/`.
- Docker в Makefile не используется. Для ручного Compose-развёртывания выполни `docker compose -f apps/file-hosting/compose.yaml up --detach --build`.

### Production deploy

Полный production-деплой из корня репозитория:

```powershell
Copy-Item deploy/.env.example deploy/.env
# заполни deploy/.env
make deploy
```

Настройки rclone, remote-пути и SSH находятся в `deploy/.env`. Скрипт использует фиксированный `rclone sync`, затем выполняет по SSH `docker compose down`, `docker compose build --pull --build-arg GOPROXY=...`, `docker compose up -d --force-recreate --remove-orphans` и `docker compose ps`. При сетевой недоступности `proxy.golang.org` внутри Docker задай в `deploy/.env` `DEPLOY_GOPROXY=https://goproxy.cn`. Подробнее: [`../deploy/README.md`](../deploy/README.md).

Проверка:

```powershell
Invoke-RestMethod http://localhost:1447/map/version
```

### Client

```powershell
make build-c
make start-c
make start-c-dev
```

Создаются Windows-бинарники:

```text
build/client/mc-build-updater.exe
build/client/mc-bu-utils.exe
```

`make start-c` сначала пересобирает клиент, затем запускает `.exe` с working directory `apps/mc-build-updater`: runtime-файлы, `mods/`, `resourcepacks/` и `shaderpacks/` остаются в каталоге приложения.

`make start-c-dev` запускает тот же binary с `--dev`, но без Air: client использует `http://localhost:1447/`, отключает self-update и стартовую паузу.

### Release клиента

Release создаётся только tag-ами вида `vX.Y.Z`. Тег надо отправить в GitHub и GitLab, чтобы обе CI-площадки опубликовали идентичные release assets:

```powershell
git tag v1.2.3
git push origin v1.2.3
git push gitlab v1.2.3
```

Обе CI собирают `windows/amd64` `mc-build-updater.exe`, генерируют `checksums.txt` с SHA-256 и публикуют их в stable release. Локальную release-сборку можно получить так:

```powershell
make build-c CLIENT_VERSION=v1.2.3
```

GitLab release-job использует защищённую CI/CD-переменную `GITLAB_RELEASE_TOKEN` с областью `api`. GitHub по умолчанию использует встроенный token, либо секрет `RELEASE_TOKEN` с `contents: write`.

Проверка `go vet` запускается внутри каждого Go-модуля (`apps/file-hosting` и `apps/mc-build-updater`): в корне workspace команда `go vet ./...` неприменима, потому что там нет корневого Go-модуля.

## Типичные проблемы

### Server не запускается через `make start-s`

`make start-s` не использует Docker. Убедись, что порт `1447` не занят:

```powershell
Get-NetTCPConnection -LocalPort 1447 -ErrorAction SilentlyContinue
```

Заверши процесс, занимающий порт, затем повтори `make start-s`.

### Server запускается, но новые файлы отсутствуют в `/map`

Карта строится на старте и по явному `GET /map/update`. Обнови её:

```powershell
Invoke-WebRequest http://localhost:1447/map/update
```

### Client не может скачать мод

Проверь по порядку:

1. Server работает на `:1447`.
2. В `/map` есть файл с ожидаемым SHA-1.
3. В `files/MM/<branch>.json` есть запись с тем же `hash`.
4. Имя ветки в `apps/mc-build-updater/mc-mods-updater.config.yml` соответствует существующему `MM/<branch>.json`.

### Client в production пытается подключиться не туда

Переменная среды имеет приоритет над режимом:

```powershell
./build/client/mc-build-updater.exe --file-hosting-url http://host:1447/
```
