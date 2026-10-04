# File-hosting (server)

## Назначение

`apps/file-hosting` — самостоятельный Go HTTP-server. Он рекурсивно сканирует каталог файлов, вычисляет SHA-1 для каждого обычного файла, публикует карту и отдаёт файлы для `mc-build-updater`.

Модуль намеренно не имеет сторонних Go-зависимостей.

## Точки входа и ответственность кода

| Путь | Ответственность |
| --- | --- |
| `cmd/file-hosting/main.go` | CLI-флаги, переменные окружения, запуск HTTP-server. |
| `internal/filemap/filemap.go` | Рекурсивный scan, сортировка, SHA-1, структура публичной записи. |
| `internal/httpserver/server.go` | In-memory карта, версия, HTTP-маршруты и выдача файлов. |
| `internal/httpserver/server_test.go` | Контрактные HTTP-тесты: карта, версия, загрузка по hash и имени. |
| `files/` | Данные, которые server публикует. В production монтируются в контейнер как `/data`. |
| `Dockerfile` | Multi-stage production image. |
| `compose.yaml` | Production service на порту `1447`. |

## Конфигурация

Приоритет параметров: CLI-флаг выше переменной окружения, переменная окружения выше значения по умолчанию.

| Настройка | CLI | Переменная | Dev default | Production default |
| --- | --- | --- | --- | --- |
| Адрес | `-addr` | `ADDR` | `:1447` | `:1447` |
| Каталог файлов | `-files-path` | `FILES_PATH` | `files` | `/data` |

Пример нативного запуска без Air:

```powershell
cd apps/file-hosting
go run ./cmd/file-hosting -addr 127.0.0.1:1447 -files-path files
```

## Жизненный цикл карты

1. На старте `httpserver.New()` вызывает `Refresh()`.
2. `filemap.Build()` обходит `FILES_PATH` с `filepath.WalkDir`.
3. Для каждого обычного файла вычисляется SHA-1; каталоги и специальные файлы пропускаются.
4. Элементы сортируются по абсолютному `path`.
5. Карта и её версия атомарно заменяются под `RWMutex`.
6. Повторный scan выполняется только по `GET /map/update`.

Следствие: добавление, удаление или изменение файла в `files/` **не** отражается в `/map` автоматически. Для применения изменений вызови `/map/update`.

```powershell
Invoke-WebRequest http://localhost:1447/map/update
```

Подробный внешний формат карты и маршрутов: [contracts.md](contracts.md).

## Данные в `files/`

Текущие каталоги:

| Каталог | Содержимое |
| --- | --- |
| `config/` | `remote.json` с `LastVersion` client. |
| `MM/` | JSON-карты модов веток. |
| `mods/` | JAR и другие файлы модов, выдаются как `/mods/<sha1>`. |
| `static/` | Вспомогательные и release-файлы, например `7z_x32.exe`. |

`files/` — versioned project data. Не добавляй его в `.gitignore` и не исключай из Docker bind mount.

### Добавление или обновление мода

1. Помести файл в `apps/file-hosting/files/mods/`.
2. Запусти или обнови server.
3. Вызови `GET /map/update`.
4. Получи SHA-1 нужного файла из `GET /map`.
5. Добавь или обнови запись `{ "hash": "…", "path": "…" }` в `files/MM/<branch>.json`.
6. Убедись, что `GET /mods/<sha1>` отвечает `200`.

Не помещай в карту ветки hash, отсутствующий в `/map`: client завершит синхронизацию ошибкой.

## Docker production

```powershell
make build-server
make start-server
make stop
```

`Dockerfile` собирает статический Linux-binary в `golang:1.26-alpine`, затем запускает его в отдельном Alpine image от непривилегированного пользователя `app`.

Compose:

- публикует `1447:1447`;
- задаёт `FILES_PATH=/data`;
- монтирует `./files:/data`;
- перезапускает контейнер с `restart: unless-stopped`.

Данные не копируются в image. Поэтому изменение `files/` не требует Docker rebuild, но по-прежнему требует `GET /map/update` в уже запущенном server.

## Изменения HTTP-слоя

Перед правкой `internal/httpserver`:

1. Прочитай [contracts.md](contracts.md).
2. Сохрани маршруты, lower-case JSON-имена и response headers, если изменение не согласовано явно.
3. Добавь или скорректируй тест в `internal/httpserver/server_test.go`.
4. Запусти:

   ```powershell
   make test-server
   cd apps/file-hosting; go vet ./...
   ```

## Ограничения совместимости

- SHA-1 используется как идентификатор содержимого по legacy-протоколу.
- Версия map строится из MD5 сериализованной карты и timestamp; это не security checksum.
- Поле `path` в HTTP-ответе — абсолютный путь среды server. Client не должен полагаться на платформу или вид этого пути.
- Поиск файла требует точного совпадения `dir` и hash либо имени файла. Одинаковые имена в разных каталогах допустимы.
