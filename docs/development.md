# Разработка, проверки и диагностика

## Требования

| Инструмент | Назначение | Проверка |
| --- | --- | --- |
| Go 1.26+ | сборка, тесты, Air | `go version` |
| GNU Make | единый интерфейс | `make --version` |
| Docker Desktop | только production server | `docker version` |

`air` отдельно устанавливать не нужно: `make dev-server` и `make dev-client` запускают его через `go run github.com/air-verse/air@latest`. Первый запуск скачает зависимость в Go module cache.

## Ежедневный workflow

### Server

```powershell
make dev-server
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
make dev-client
```

- Запускается **нативно** с флагом `--dev`.
- В dev автоматически использует `http://localhost:1447/` и не выполняет self-update.
- Working directory — `apps/mc-build-updater`; там создаются runtime-файлы и каталог `mods/`.

### Оба приложения

```powershell
make dev
```

`make` запускает две цели параллельно. Останавливай процесс через `Ctrl+C` в этом терминале.

## Тесты и статический анализ

| Область | Команда |
| --- | --- |
| Server | `make test-server` |
| Client | `make test-client` |
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

## Сборка и запуск production

### Server

```powershell
make build-server
make start-server
make stop
```

- Используется `apps/file-hosting/compose.yaml`.
- Порт хоста: `1447`.
- `apps/file-hosting/files/` монтируется в контейнер как `/data` и **не** попадает в Docker image.
- Для этих команд Docker Desktop daemon должен быть запущен.

Проверка:

```powershell
docker compose -f apps/file-hosting/compose.yaml ps
Invoke-RestMethod http://localhost:1447/map/version
```

### Client

```powershell
make build-client
make start-client
```

Создаются Windows-бинарники:

```text
apps/mc-build-updater/build/mc-build-updater.exe
apps/mc-build-updater/build/mc-bu-utils.exe
```

`make start-client` сначала пересобирает клиент, затем запускает `.exe` из каталога `apps/mc-build-updater`.

### Release клиента

Release создаётся только tag-ами вида `vX.Y.Z`. Тег надо отправить в GitHub и GitLab, чтобы обе CI-площадки опубликовали идентичные release assets:

```powershell
git tag v1.2.3
git push origin v1.2.3
git push gitlab v1.2.3
```

Обе CI собирают `windows/amd64` `mc-build-updater.exe`, генерируют `checksums.txt` с SHA-256 и публикуют их в stable release. Локальную release-сборку можно получить так:

```powershell
make build-client CLIENT_VERSION=v1.2.3
```

GitLab release-job использует защищённую CI/CD-переменную `GITLAB_RELEASE_TOKEN` с областью `api`. GitHub по умолчанию использует встроенный token, либо секрет `RELEASE_TOKEN` с `contents: write`.

Проверка `go vet` запускается внутри каждого Go-модуля (`apps/file-hosting` и `apps/mc-build-updater`): в корне workspace команда `go vet ./...` неприменима, потому что там нет корневого Go-модуля.

> Не запускай `make start` по умолчанию: он одновременно поднимает production server и запускает client.

## Типичные проблемы

### `make build-server`: не удаётся подключиться к Docker API

Запусти Docker Desktop и дождись состояния Running. Проверь:

```powershell
docker version
docker context ls
```

После этого повтори `make build-server`.

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
.\apps\mc-build-updater\build\mc-build-updater.exe --file-hosting-url http://host:1447/
```
