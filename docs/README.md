# Документация

Документация ориентирована прежде всего на агента и разработчика, которые открывают репозиторий с нуля.

## Быстрый выбор документа

| Задача | Документ |
| --- | --- |
| Понять репозиторий и запустить проект | [development.md](development.md) |
| Менять HTTP-server или Docker-образ | [file-hosting.md](file-hosting.md) |
| Менять Windows-клиент, синхронизацию, self-update или REST-публикацию | [mc-build-updater.md](mc-build-updater.md) |
| Менять взаимодействие server и client | [contracts.md](contracts.md) |

## Актуальный статус

- Миграция обоих исходных TypeScript-проектов в Go выполнена.
- `file-hosting` имеет unit-тест HTTP-контракта.
- `mc-build-updater` имеет тесты локального конфига и карты локальных модов.
- Self-update клиента получает stable release с GitHub, а при ошибке GitHub — с GitLab; оба release собираются CI по тегам `vX.Y.Z`.
- `make test`, `go build ./...` и `go vet ./...` для обоих модулей проходили успешно при создании репозитория.
- Production-сборка server требует запущенный Docker Desktop. Если Docker daemon выключен, `make build-server` и `make start-server` ожидаемо завершаются ошибкой подключения к Docker API.

## Структура

```text
.
├─ AGENTS.md                         # обязательная точка входа нового агента
├─ docs/                             # эта документация
├─ Makefile                          # единый интерфейс разработки и сборки
├─ go.work                           # связывает два Go-модуля
└─ apps/
   ├─ file-hosting/                  # server
   │  ├─ cmd/file-hosting/
   │  ├─ internal/filemap/
   │  ├─ internal/httpserver/
   │  ├─ files/                      # публикуемые сервером файлы
   │  ├─ Dockerfile
   │  └─ compose.yaml
   └─ mc-build-updater/              # client
      ├─ cmd/mc-build-updater/
      ├─ cmd/mc-bu-utils/
      └─ internal/
         ├─ branch/
         ├─ config/
         ├─ modsync/
         ├─ remote/
         ├─ selfupdate/
         └─ uploader/
```

## Инварианты проекта

1. Имена Make-целей используют роли: `server` = `file-hosting`, `client` = `mc-build-updater`.
2. Разработка запускается нативно, без Docker.
3. Docker Compose нужен только для production `file-hosting`.
4. HTTP-контракт между приложениями обратно совместим со старой TypeScript-версией.
5. Корень обслуживаемых файлов server — `apps/file-hosting/files/` в dev и `/data` внутри production-контейнера.
6. Client в dev обращается к `http://localhost:1447/`; production URL по умолчанию — `http://mc.sinezix.ru:1447/`.
