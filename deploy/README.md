# Production deploy

`make deploy` синхронизирует проект с production-сервером через `rclone`, затем пересобирает и перезапускает `file-hosting` через Docker Compose.

## Настройка

1. Создай локальный файл настроек:

   ```powershell
   Copy-Item deploy/.env.example deploy/.env
   ```

2. Заполни `deploy/.env`:

   - `DEPLOY_RCLONE_REMOTE` — имя настроенного rclone remote;
   - `DEPLOY_REMOTE_PATH` — каталог проекта на server, например `/opt/mc-build-updater`;
    - `DEPLOY_SSH_TARGET`, `DEPLOY_SSH_PORT`, `DEPLOY_SSH_KEY` — SSH-подключение для запуска Compose;
    - `DEPLOY_GOPROXY` — Go module proxy на время Docker-сборки (по умолчанию `https://proxy.golang.org,direct`); если production Docker не может скачать модули с основного прокси, задай `https://goproxy.cn`;
    - `FILE_HOSTING_TOKEN` — production-токен server.

rclone remote и SSH должны указывать на один server. Авторизация rclone хранится в его конфиге, секреты в Git не добавляются.

## Запуск

Из корня репозитория:

```powershell
make deploy
```

Скрипт получает файлы через `git ls-files --cached --others --exclude-standard`, поэтому `.gitignore` применяется автоматически. `deploy/.env` не синхронизируется как файл проекта: скрипт создаёт remote `deploy/.env` только с `FILE_HOSTING_TOKEN`.

Режим rclone фиксирован: `sync`. Файлы на server, которых нет в локальном проекте, удаляются. Runtime-файлы server — `deploy/.env` и `apps/file-hosting/files/.file-hosting.sqlite*` — исключены из удаления.

После синхронизации по SSH выполняются:

```text
docker compose down --remove-orphans
docker compose build --pull --build-arg GOPROXY=...
docker compose up -d --force-recreate --remove-orphans
docker compose ps
```

Контейнер server использует `root` только для записи SQLite-карты в bind mount `apps/file-hosting/files/`, который на production принадлежит `root`. Сам сервис не требует записи в другие каталоги контейнера.
