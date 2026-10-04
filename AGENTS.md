# Инструкции для агентов

## Язык

- Рабочий язык проекта — **русский**.
- Пиши по-русски ответы, комментарии к задаче, документацию, тексты CLI и сообщения об ошибках, если это не ломает внешний контракт.
- Английский допустим только там, где его требует код, стандартный формат, внешний API, имя переменной или зависимость.

## Старт новой сессии

1. Прочитай этот файл полностью.
2. Прочитай [`docs/README.md`](docs/README.md) — навигатор и актуальный статус.
3. Открой документ только для затронутой области:
   - server: [`docs/file-hosting.md`](docs/file-hosting.md);
   - client: [`docs/mc-build-updater.md`](docs/mc-build-updater.md);
   - разработка, сборка, диагностика: [`docs/development.md`](docs/development.md);
   - публичные контракты между приложениями: [`docs/contracts.md`](docs/contracts.md).
4. Перед изменениями посмотри `git status --short` и не удаляй чужие изменения.
5. После изменений запусти минимальные релевантные проверки из `docs/development.md`.

## Суть проекта

- Это Go-монорепозиторий из двух **независимых Go-модулей**, связанных `go.work`.
- `apps/file-hosting` (`server`) строит карту файлов и отдаёт файлы по HTTP. Dev, build и start нативные; Compose остаётся ручным production-вариантом.
- `apps/mc-build-updater` (`client`) — Windows CLI для синхронизации Minecraft-модов, self-update и SFTP-утилита.
- `server` и `client` связаны публичным HTTP-контрактом. Не меняй маршруты, JSON-поля, SHA-1 или правила поиска файлов без явного согласования и обновления `docs/contracts.md`.

## Команды

Работай из корня репозитория:

```powershell
make dev-s            # server нативно, hot reload, :1447
make dev-c            # client нативно, hot reload, localhost:1447
make test-s
make test-c
make test
make build-c          # Windows .exe в build/client
make build-s          # server .exe в build/server
make start-s          # build-s и нативный запуск server
make start-c          # build-c и нативный запуск client
make start-s-dev      # нативный server без Air
make start-c-dev      # нативный client --dev без Air
```

Общие цели `build`, `test` и `fmt` запускают обе области. Для одного приложения используй суффикс: `-c` — client, `-s` — server.

## Границы и правила

- Не объединяй модули в один `go.mod`; зависимости server и client независимы.
- Не добавляй Docker в Makefile и dev workflow. `make dev-s` и `make dev-c` должны работать нативно через Air.
- Не коммить runtime-артефакты: `build/`, `mods/`, `7z.exe`, логи и временные файлы уже в `.gitignore`.
- Файлы, публикуемые server, лежат в `apps/file-hosting/files/` и намеренно версионируются.
- Не возвращай секреты в код. SFTP настраивается только переменными окружения из `docs/mc-build-updater.md`.
- SHA-1 и MD5 здесь — требования совместимости старого протокола, не криптографические механизмы безопасности.
- При правке публичного поведения обнови соответствующий документ в `docs/` и тесты.
